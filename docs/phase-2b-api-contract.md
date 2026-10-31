# Phase 2B — API Contract Freeze

**Status:** frozen for Phase 2B Agent A/B integration  
**Scope:** WildBoat backend game assignment, tickets, Dedicated Server (DS) auth, minimal settlement  
**Architecture:** modular monolith + PostgreSQL; single active Dedicated Server capacity

This document is the source of truth for Agent B (WPG client + Dedicated Server).  
If anything here changes, WildBoat must update this file and notify Agent B explicitly.

---

## 1. Identifier model

| Field | Type | Meaning |
|---|---|---|
| `account_id` | UUID string | WPass account (credentials). Not sent to DS. |
| `player_id` | UUID string | Public player identity. Bound into tickets and settlement. |
| `session_id` | UUID string | Refresh-session family id. Used only by WPass auth. |
| `server_id` | UUID string | Dedicated Server workload identity. |
| `game_instance_id` | UUID string | Allocatable DS endpoint/slot registered with WildBoat. |
| `match_id` | UUID string | One match/session on a game instance. |
| `game_ticket` | opaque string | Short-lived admission secret (base64url). **Never logged.** |
| `settlement_id` | UUID string | Client-chosen (DS-chosen) idempotency key for settlement. |

Conventions:
- All IDs are UUID strings **without** prefixes (`acc_`, `plr_`, etc.).
- Timestamps are UTC RFC 3339.
- WildBoat never trusts a client-supplied `player_id` for authorization; it comes from the access token.

---

## 2. Access Token vs Game Ticket

| | Access Token | Game Ticket |
|---|---|---|
| Issuer | WildBoat (WPass) | WildBoat |
| Audience | WildBoat HTTP API | Dedicated Server admission |
| Lifetime | ~15m (configurable) | ~60s (configurable, short) |
| Format | HS256 JWT | Opaque random secret |
| Storage | Not stored (JWT) | SHA-256 hash only in DB |
| Auth header | `Authorization: Bearer <access_token>` on `/v1/*` player routes | Sent to DS over game protocol; DS validates with WildBoat |
| Contains | `account_id`, `player_id`, `session_id` | Bound to `player_id` + `match_id` + `game_instance_id` |
| Must not contain | N/A | Passwords, refresh tokens, access tokens |

A valid access token does **not** admit a player onto a DS.  
A valid game ticket does **not** authorize WildBoat player APIs.

---

## 3. Ticket strategy decision: **Option A — online validation**

### Choice
WildBoat issues an **opaque** ticket. The Dedicated Server calls WildBoat `POST /v1/server/tickets/validate` to admit a player.

### Why (current architecture)
- Single DS already needs WildBoat reachability for heartbeat + settlement.
- Immediate revoke on DS restart / drain / offline without key rotation.
- Matches existing opaque hashed-token patterns (refresh, email actions).
- Avoids embedding signing secrets in the DS build or inventing a second JWT scheme.

### Trade-offs
| Pros | Cons |
|---|---|
| Instant revoke / single-use consume | DS depends on WildBoat availability at join time |
| Simple key ops (no ticket signing keys) | Extra RTT on player join |
| Replay handled in DB with row lock | Must define timeout + fail-closed policy |

### Online validation rules
- DS request timeout: **3s** recommended client-side; WildBoat handler uses request context.
- Failure policy: **fail closed** — if WildBoat unreachable/timeout/5xx, DS **must reject** admission.
- Consume: atomic `UPDATE … SET consumed_at = now() WHERE consumed_at IS NULL` — concurrent double-consume returns `TICKET_REPLAYED`.
- Cache: **none** on WildBoat; DS must not cache “valid” across reconnects. DS may cache a successful admission for that TCP/ENet peer only.
- Replay: after consume, same ticket always returns `TICKET_REPLAYED` (HTTP 409).
- Expiry: `expires_at <= now()` → `TICKET_EXPIRED`.
- Binding: ticket must match requested `player_id`, `match_id`, `game_instance_id`.
- DS restart: on re-register or transition to `offline`/`starting`, WildBoat **revokes** all unconsumed tickets for that instance (`revoked_at` set). Old tickets fail with `TICKET_REVOKED`.

---

## 4. Dedicated Server authentication

- DS uses a **server credential**, never a player access token.
- Header: `Authorization: Bearer <dedicated_server_credential>`
- Credential is provisioned via env `DEDICATED_SERVER_CREDENTIAL` (min 32 chars). WildBoat stores only SHA-256 hash.
- All `/v1/server/*` routes require this credential.
- WildBoat resolves `server_id` from the credential; DS cannot claim another server’s resources.
- Player routes under `/v1/games/*` require player access tokens and reject server credentials.

---

## 5. Game instance status

| Status | Assignable? | Meaning |
|---|---|---|
| `starting` | No | Registered; not ready for players |
| `ready` | Yes | Heartbeat fresh; accepting assignments |
| `full` | No | Capacity reached |
| `draining` | No | Finishing current match; no new joins |
| `offline` | No | Explicit offline or replaced |

**Liveness:** an instance is assignable only if:
1. `status = ready`
2. `last_heartbeat_at >= now() - HEARTBEAT_TTL` (default **30s**)
3. `occupied_slots < capacity`

A row existing in the database is **not** enough to treat a DS as online.

---

## 6. Assignment lifecycle

```text
Player (access token)
  → POST /v1/games/assign { mode }
  → WildBoat locks ready instance (FOR UPDATE)
  → create or join open match (state=allocating|active, same mode, capacity left)
  → insert match_players + issue ticket
  → return assignment

Client connects to DS with game_ticket
  → DS POST /v1/server/tickets/validate
  → on success, admit player; ticket consumed

Match ends
  → DS POST /v1/server/matches/{match_id}/settlement
  → WildBoat marks match completed; frees capacity
```

### Assignment response fields (required)
- `game_instance_id`
- `match_id`
- `server_id`
- `host`
- `port`
- `transport` (always `enet_udp` in Phase 2B)
- `game_ticket`
- `ticket_expires_at`
- `protocol_version`
- `server_build`
- `mode`

### Edge cases
| Case | Behavior |
|---|---|
| Unauthenticated | `401 AUTH_REQUIRED` |
| No ready/live DS | `503 GAME_SERVER_UNAVAILABLE` |
| Instance full | `409 GAME_CAPACITY_FULL` |
| Instance draining/offline/stale heartbeat | treated as unavailable / capacity full |
| Concurrent assigns | row lock + capacity check; never exceed `capacity` |
| Duplicate assign (same player, open match) | return **same** match + **new** ticket (old unconsumed tickets for that player/match revoked) |
| Allocation timeout | handler/request deadline; client should retry |

---

## 7. Settlement rules (minimal)

- Only authenticated DS may submit.
- `match_id` must belong to the authenticated `server_id` / `game_instance_id`.
- Match state must be `allocating` or `active` (not already `completed`/`aborted`).
- `settlement_id` is unique globally; retries with the same id return the original result (`200` + `already_processed: true`) without double-updating stats.
- Clients cannot submit settlements.
- Phase 2B updates only minimal `player_stats` counters (matches_played, kills, deaths, score). No achievements / leaderboards.

---

## 8. Endpoints

Base: `/v1`  
Error envelope: unchanged from Phase 2 (`error.code`, `error.message`, `error.request_id`).

### 8.1 Player — Create assignment

`POST /v1/games/assign`  
Auth: player access token

Request:
```json
{
  "mode": "coop_survival"
}
```

Allowed `mode` values: `coop_survival`, `coop_endless`, `battle_ffa`, `battle_team`, `battle_kill_race`.

Response `200`:
```json
{
  "game_instance_id": "11111111-1111-1111-1111-111111111111",
  "match_id": "22222222-2222-2222-2222-222222222222",
  "server_id": "33333333-3333-3333-3333-333333333333",
  "host": "127.0.0.1",
  "port": 24567,
  "transport": "enet_udp",
  "game_ticket": "<opaque>",
  "ticket_expires_at": "2026-10-31T08:01:00Z",
  "protocol_version": 1,
  "server_build": "wpg-ds-local",
  "mode": "coop_survival"
}
```

Errors: `401 AUTH_REQUIRED`, `400 INVALID_INPUT`, `503 GAME_SERVER_UNAVAILABLE`, `409 GAME_CAPACITY_FULL`, `429 RATE_LIMITED`.

### 8.2 DS — Register / update instance

`POST /v1/server/instances/register`  
Auth: dedicated server credential

Request:
```json
{
  "host": "127.0.0.1",
  "port": 24567,
  "transport": "enet_udp",
  "protocol_version": 1,
  "server_build": "wpg-ds-local",
  "capacity": 4,
  "status": "starting"
}
```

Response `200`:
```json
{
  "server_id": "33333333-3333-3333-3333-333333333333",
  "game_instance_id": "11111111-1111-1111-1111-111111111111",
  "status": "starting",
  "capacity": 4,
  "heartbeat_ttl_seconds": 30
}
```

Notes:
- Re-register from the same server credential replaces/updates the active instance endpoint.
- Previous unconsumed tickets for the prior instance are revoked.
- Initial status should be `starting`; DS moves to `ready` via heartbeat when accepting players.

### 8.3 DS — Heartbeat

`POST /v1/server/instances/{game_instance_id}/heartbeat`  
Auth: dedicated server credential

Request:
```json
{
  "status": "ready",
  "occupied_slots": 1,
  "active_match_id": "22222222-2222-2222-2222-222222222222"
}
```

`occupied_slots` and `active_match_id` are optional hints; WildBoat remains authoritative for assignment capacity based on DB match membership unless status is `full`/`draining`/`offline`.

Response `200`:
```json
{
  "ok": true,
  "status": "ready",
  "server_time": "2026-10-31T08:00:00Z"
}
```

Errors: `401 SERVER_AUTH_INVALID`, `403 FORBIDDEN`, `404 NOT_FOUND`.

### 8.4 DS — Validate ticket

`POST /v1/server/tickets/validate`  
Auth: dedicated server credential

Request:
```json
{
  "game_ticket": "<opaque>",
  "player_id": "7c9e6679-7425-40de-944b-e07fc1f90ae7",
  "match_id": "22222222-2222-2222-2222-222222222222",
  "game_instance_id": "11111111-1111-1111-1111-111111111111"
}
```

Response `200`:
```json
{
  "valid": true,
  "player_id": "7c9e6679-7425-40de-944b-e07fc1f90ae7",
  "match_id": "22222222-2222-2222-2222-222222222222",
  "game_instance_id": "11111111-1111-1111-1111-111111111111",
  "server_id": "33333333-3333-3333-3333-333333333333",
  "consumed_at": "2026-10-31T08:00:05Z"
}
```

Errors:
- `401 SERVER_AUTH_INVALID`
- `400 TICKET_INVALID` (unknown / wrong binding / wrong instance ownership)
- `409 TICKET_EXPIRED`
- `409 TICKET_REPLAYED`
- `409 TICKET_REVOKED`

Ticket is **one-time use**. Successful validation sets `consumed_at`.

### 8.5 DS — Settlement

`POST /v1/server/matches/{match_id}/settlement`  
Auth: dedicated server credential

Request:
```json
{
  "settlement_id": "44444444-4444-4444-4444-444444444444",
  "result": "completed",
  "started_at": "2026-10-31T07:50:00Z",
  "ended_at": "2026-10-31T08:00:00Z",
  "players": [
    {
      "player_id": "7c9e6679-7425-40de-944b-e07fc1f90ae7",
      "team_id": "",
      "kills": 3,
      "deaths": 1,
      "score": 420
    }
  ],
  "mode_stats": {
    "duration_seconds": 600
  }
}
```

Response `200`:
```json
{
  "ok": true,
  "settlement_id": "44444444-4444-4444-4444-444444444444",
  "match_id": "22222222-2222-2222-2222-222222222222",
  "already_processed": false
}
```

Errors: `401 SERVER_AUTH_INVALID`, `403 FORBIDDEN`, `404 NOT_FOUND`, `409 MATCH_NOT_SETTLEABLE`, `400 INVALID_INPUT`.

---

## 9. Communication failure policy (WildBoat ↔ DS)

| Scenario | Policy |
|---|---|
| DS cannot reach WildBoat for ticket validate | Reject player join (fail closed) |
| DS cannot heartbeat | After `HEARTBEAT_TTL`, WildBoat stops assigning; existing match may continue locally until settlement or DS recovery |
| DS cannot submit settlement | Retry with **same** `settlement_id`; WildBoat is idempotent |
| WildBoat restarts | DB is source of truth; DS must re-register / resume heartbeat |
| DS process restart | Re-register; old tickets revoked; clients must re-assign |

---

## 10. Local integration (Agent B)

### Start WildBoat
```bash
cp .env.example .env
# set POSTGRES_PASSWORD, TOKEN_SIGNING_KEY, DEDICATED_SERVER_CREDENTIAL
make up
make smoke
```

### Required env (new for Phase 2B)
| Variable | Required | Default | Notes |
|---|---|---|---|
| `DEDICATED_SERVER_CREDENTIAL` | yes | — | Shared DS secret, min 32 chars; never ship to game clients |
| `GAME_TICKET_TTL` | no | `60s` | Ticket lifetime |
| `DS_HEARTBEAT_TTL` | no | `30s` | Stale heartbeat → not assignable |
| `GAME_INSTANCE_CAPACITY` | no | `4` | Max players per instance/match (capped at 4) |

Existing vars (`DATABASE_URL`, `TOKEN_SIGNING_KEY`, etc.) remain required as in Phase 2.

### DS bootstrap sequence
1. Read `DEDICATED_SERVER_CREDENTIAL` from DS env (same value as WildBoat).
2. `POST /v1/server/instances/register` with host/port/build, status `starting`.
3. When ready to accept players: heartbeat with status `ready`.
4. On player connect: validate ticket online.
5. On match end: submit settlement once; retry same `settlement_id` on failure.
6. On shutdown: heartbeat `draining` then `offline`, or simply stop (heartbeat TTL will age out).

### Player bootstrap sequence
1. Register / verify / login (Phase 2).
2. `POST /v1/games/assign` with Bearer access token.
3. Connect to returned `host:port` with `game_ticket`.
4. Do not send access/refresh tokens to the DS.

---

## 11. Out of scope for Phase 2B

- Friends / Party / Rooms / Quick Match queues
- Multiple concurrent Dedicated Servers
- Offline signed tickets
- Achievements / leaderboards / full match history APIs
- Redis or other new infrastructure
