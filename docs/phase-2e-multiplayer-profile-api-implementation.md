# Phase 2E.1 — Multiplayer / Profile API Implementation

**Status:** implemented on `dev` (uncommitted until owner review)  
**Contract:** `docs/phase-2e-multiplayer-profile-api-contract.md` (frozen fields unchanged)  
**Date:** 2026-10-31

---

## 1. Endpoints implemented

| Method | Path | Status |
|---|---|---|
| `POST` | `/v1/rooms` | implemented |
| `POST` | `/v1/rooms/join-by-code` | implemented |
| `GET` | `/v1/rooms/{room_id}` | implemented |
| `GET` | `/v1/rooms/current` | implemented |
| `POST` | `/v1/rooms/{room_id}/leave` | implemented |
| `PUT` | `/v1/rooms/{room_id}/ready` | implemented |
| `POST` | `/v1/rooms/{room_id}/start` | implemented |
| `POST` | `/v1/rooms/{room_id}/assignment` | implemented |
| `POST` | `/v1/matchmaking/quick` | implemented |
| `POST` | `/v1/matchmaking/cancel` | implemented |
| `GET` | `/v1/me/stats` | implemented |
| `GET` | `/v1/me/matches` | implemented |
| `POST` | `/v1/games/assign` | **unchanged** (Phase 2B) |

All player routes require `Authorization: Bearer <access_token>`. Identity is taken from the access token only.

### Create private room — `201`

```http
POST /v1/rooms
Authorization: Bearer <access_token>
Content-Type: application/json

{"mode":"coop_survival","visibility":"private","max_players":4}
```

```json
{
  "room": {
    "room_id": "...",
    "room_code": "K7M2QX",
    "owner_player_id": "...",
    "mode": "coop_survival",
    "visibility": "private",
    "state": "open",
    "max_players": 4,
    "member_count": 1,
    "match_id": null,
    "created_at": "...",
    "updated_at": "...",
    "expires_at": "..."
  },
  "members": [{"player_id":"...","display_name":"...","avatar_id":"...","ready":false,"is_owner":true,"joined_at":"..."}],
  "self": {"player_id":"...","ready":false,"is_owner":true},
  "assignment_available": false
}
```

### Start — `200` (owner assignment)

```json
{
  "room": {"state":"in_game","match_id":"...","...":"..."},
  "assignment": {
    "game_instance_id": "...",
    "match_id": "...",
    "server_id": "...",
    "host": "127.0.0.1",
    "port": 24567,
    "transport": "enet_udp",
    "game_ticket": "<opaque>",
    "ticket_expires_at": "...",
    "protocol_version": 1,
    "server_build": "...",
    "mode": "coop_survival"
  }
}
```

Other members poll `GET /v1/rooms/{room_id}` until `assignment_available=true`, then:

```http
POST /v1/rooms/{room_id}/assignment
```

```json
{"assignment": { "...same Phase 2B fields..." }}
```

### Stats / history

```http
GET /v1/me/stats
GET /v1/me/matches?limit=20&cursor=
```

Never-settled players get zero counters and `updated_at: null`. History only includes rows with a `match_settlements` record.

---

## 2. Migration and data model

Migration: `migrations/000004_phase2e_rooms.up.sql` / `.down.sql`

| Table | Notes |
|---|---|
| `rooms` | Server-generated `room_code`; states `open\|starting\|in_game\|closed\|expired\|aborted`; visibility `private\|public` |
| `room_members` | PK `(room_id, player_id)`; unique `player_id` (active membership only; rows deleted on leave/close) |
| `matches.room_id` | FK → `rooms(id)` added; existing NULL rows retained |

Indexes:

- Unique active room codes: partial unique on `room_code` where state ∈ (`open`,`starting`,`in_game`)
- One room per player: unique `room_members(player_id)`
- Public open rooms by mode for Quick Match

Capacity is enforced in a transaction with `FOR UPDATE` on the room row (not only CHECK).

---

## 3. Error codes

| Code | HTTP | When |
|---|---|---|
| `ROOM_NOT_FOUND` | 404 | Unknown / non-member |
| `ROOM_CODE_INVALID` | 404 | Bad alphabet, unknown, expired, closed |
| `ROOM_FULL` | 409 | At capacity |
| `ROOM_NOT_JOINABLE` | 409 | Wrong state for join/ready |
| `ROOM_ALREADY_IN` | 409 | Already in another active room |
| `ROOM_NOT_READY` | 409 | Start blocked: not all ready |
| `ROOM_ALREADY_STARTED` | 409 | Start conflict (non-idempotent path) |
| `NOT_ROOM_OWNER` | 403 | Host-only action |
| `ASSIGNMENT_NOT_READY` | 409 | Room still `open` |
| `MATCH_ALREADY_ACTIVE` | 409 | Open match membership blocks lobby |
| `GAME_SERVER_UNAVAILABLE` | 503 | No live DS |
| `GAME_CAPACITY_FULL` | 409 | Instance cannot fit roster |

Envelope unchanged from Phase 2B.

---

## 4. Idempotent behaviors

| Action | Behavior |
|---|---|
| Join by code while already a member of that open room | `200` current detail |
| Leave while not a member of an existing room | `200 {"ok":true}` |
| Ready set to current value | `200` detail |
| Owner start while already `in_game` | Reissue owner ticket; return assignment |
| Claim assignment | New ticket; revoke prior unconsumed for player/match |
| Quick Match while already in same-mode public room | Return that room |
| Matchmaking cancel with no room | `200 {"ok":true}` |

`Idempotency-Key` header persistence is **not** implemented in MVP (semantic idempotency above is).

---

## 5. DS allocation and ticket lifecycle

Room start reuses Phase 2B ticket machinery via `match.Store.AssignRosterTx` / `ReissueTicketTx`:

1. Lock room; require owner + `open` + all members `ready=true`.
2. In the **same transaction**, allocate a ready DS instance with `occupied_slots = 0` and capacity ≥ roster size.
3. Insert one `matches` row with `room_id` set; insert all `match_players`; insert per-player `game_tickets` (hash only).
4. Set room `state=in_game` and `match_id`.
5. Commit. On any failure the transaction rolls back — room stays `open`, no half-started `in_game`.

Tickets remain opaque, one-time, expiry-checked, replay-rejected via existing `POST /v1/server/tickets/validate`.

Settlement / DS re-register closes or aborts linked rooms and clears members.

`POST /v1/games/assign` is untouched.

---

## 6. Quick Match semantics

`POST /v1/matchmaking/quick` is **public lobby find-or-create**:

1. Prefer return existing same-mode public active room for the player.
2. Else join an open public room of the same mode with free capacity.
3. Else create a new public room (still gets a server `room_code`).
4. Does **not** auto-ready or auto-start.
5. Does **not** call `/v1/games/assign`.

`POST /v1/matchmaking/cancel` leaves the current room (idempotent).

---

## 7. Stats / History sources

| API | Source |
|---|---|
| `GET /v1/me/stats` | `player_stats` (written only by DS settlement) |
| `GET /v1/me/matches` | `match_players` ⨝ `matches` ⨝ `match_settlements` only |

Cursor is opaque base64(JSON `{ended_at, match_id}`) — clients cannot expand the query window beyond keyset pagination. Invalid cursor → `400 INVALID_INPUT`.

No fabricated win-rate, rank, achievements, or mode breakdown.

---

## 8. Expiry

- Idle open rooms expire **30m** after `expires_at` (set from create/`updated_at` window via idle TTL on create).
- Lazy expire on join/get before membership checks.
- Background sweeper every **1m** in `cmd/api` (`SweepExpired`).

---

## 9. How to start / smoke

```bash
# From WildBoat repo root
cp -n .env.example .env   # fill secrets if needed
docker compose up --build -d
curl -s http://127.0.0.1:8080/healthz

# Integration tests (host Go against Compose Postgres)
export DATABASE_URL='postgres://wildboat:<password>@127.0.0.1:5432/wildboat?sslmode=disable'
go test ./... -count=1
```

Manual two-player room smoke (API must be rebuilt with this code; DS registered + heartbeat ready):

1. Register/verify/login two accounts → access tokens.
2. Host: `POST /v1/rooms` → note `room_code` / `room_id`.
3. Guest: `POST /v1/rooms/join-by-code`.
4. Both: `PUT /v1/rooms/{id}/ready` `{"ready":true}`.
5. Host: `POST /v1/rooms/{id}/start` → connect DS with returned ticket.
6. Guest: `POST /v1/rooms/{id}/assignment` → validate ticket on DS.
7. Profile: `GET /v1/me/stats` / `GET /v1/me/matches` after settlement.

Phase 2B smoke (`./scripts/smoke.sh`) remains valid for `/v1/games/assign`.

---

## 10. Not implemented / limited

| Item | Notes |
|---|---|
| `Idempotency-Key` header store | Deferred; semantic idempotency only |
| Public room browser `GET /v1/rooms` | Deferred |
| Friends / Party / presence | Out of scope |
| Auto-start when lobby full | Not implemented |
| Avatar upload | Out of scope |
| Redis / microservices | Forbidden |
| Changing DS realtime protocol | Out of scope |
| Room start while another open match occupies the only DS | Returns `GAME_CAPACITY_FULL` (requires free instance with `occupied_slots=0`) |

---

## 11. Package layout

| Path | Role |
|---|---|
| `internal/room/` | Lobby domain (create/join/ready/leave/quick/start/claim) |
| `internal/match/store_room.go` | Shared roster assign + ticket reissue + room close on settle |
| `internal/match/profile.go` | Stats + settlement-gated history |
| `internal/httpapi/handlers/rooms.go` | Room HTTP |
| `internal/httpapi/handlers/matchmaking.go` | Quick Match HTTP |
| `internal/httpapi/handlers/profile.go` | Stats/history HTTP |
