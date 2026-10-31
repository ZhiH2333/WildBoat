# Phase 2D — Backend Readiness Report

**Role:** Agent A (WildBoat backend)  
**Audience:** Agent B (WPG client + Dedicated Server + in-game sync)  
**Date:** 2026-10-31  
**WildBoat branch / commit:** `dev` @ `f7b24ee`  
**WPG known baseline (reported):** `65fd75d`  
**API Contract:** `docs/phase-2b-api-contract.md` — **unchanged**  
**Verdict:** **PASS**

> 本报告只证明 WildBoat HTTP 后端足以支撑多人局生命周期。  
> **不**证明完整游戏内实时同步；那是 Agent B / Dedicated Server 的验收范围。

---

## 1. Executive verdict

| Question | Answer |
|---|---|
| WildBoat 是否已具备 Phase 2D 所需后端能力？ | **是** |
| 是否存在必须修复的阻塞问题？ | **否** — 本阶段无需后端代码修改 |
| 真实 PostgreSQL 测试结果 | **PASS**（单元/集成 + live smoke） |
| API Contract 是否有变化？ | **无** |
| Agent B 是否可直接开始实现游戏状态同步？ | **可以** |
| 结论标签 | **PASS** |

Phase 2B 已实现的 Assignment / Ticket / DS Register+Heartbeat / Settlement 足够支撑 Phase 2D。  
实时移动、战斗、帧同步 **不属于** WildBoat；由 WPG Dedicated Server 负责。

---

## 2. Repository state (Agent A)

```text
branch:  dev (tracks origin/dev)
HEAD:    f7b24ee133e738d276eea6ee7503d1cce9e28898
message: :sparkles: implement Phase 2B game assignment, tickets, and settlement
tree:    clean at inspection start
action:  no commit / push / merge / deploy performed
```

Protect existing work: working tree was clean; readiness doc is additive documentation only.

---

## 3. Capability checklist (Phase 2D goals)

| # | Requirement | Status | Evidence |
|---|---|---|---|
| 1 | ≥2 players allocated to same match | **OK** | Assign joins open match on same instance+mode; live smoke `SAME_MATCH_OK` |
| 2 | Player identity + `game_ticket` binding | **OK** | Ticket bound to `player_id` + `match_id` + `game_instance_id`; validated online |
| 3 | `game_instance` ↔ `match` lifecycle clear | **OK** | See §6 |
| 4 | DS register / heartbeat / status | **OK** | Contract §8.2–8.3 implemented |
| 5 | Trusted settlement after match | **OK** | DS-only `/v1/server/matches/{match_id}/settlement` |
| 6 | Settlement idempotent (no double-count) | **OK** | Same `settlement_id` → `already_processed: true` |
| 7 | DS crash does not leave permanently unrecoverable match | **OK** | Re-register aborts open matches + frees roster + revokes tickets |

---

## 4. Interfaces Agent B can use as-is

Base: `http(s)://<wildboat>/v1`  
Error envelope: `{ "error": { "code", "message", "request_id" } }`

### 4.1 Auth reminder

| Caller | Header |
|---|---|
| Player (Godot client) | `Authorization: Bearer <access_token>` |
| Dedicated Server | `Authorization: Bearer <DEDICATED_SERVER_CREDENTIAL>` |

Never send player access/refresh tokens to DS.  
Never send DS credential to game clients.

### 4.2 `POST /v1/games/assign` (player)

Request:
```json
{ "mode": "coop_survival" }
```

Allowed modes: `coop_survival`, `coop_endless`, `battle_ffa`, `battle_team`, `battle_kill_race`.

Response `200` (required fields):
```json
{
  "game_instance_id": "...",
  "match_id": "...",
  "server_id": "...",
  "host": "127.0.0.1",
  "port": 24567,
  "transport": "enet_udp",
  "game_ticket": "<opaque>",
  "ticket_expires_at": "2026-10-31T09:31:13Z",
  "protocol_version": 1,
  "server_build": "wpg-ds-local",
  "mode": "coop_survival"
}
```

Errors: `401 AUTH_REQUIRED`, `400 INVALID_INPUT`, `503 GAME_SERVER_UNAVAILABLE`, `409 GAME_CAPACITY_FULL`, `429 RATE_LIMITED`.

**Multiplayer join rule (important for Phase 2D):**
- First assignable player on a ready instance creates match (`allocating` → `active`).
- Later players with the **same mode** join that open match until capacity (default 4).
- Different mode while an open match exists → `409 GAME_CAPACITY_FULL`.
- Same player re-assign while still in open match → **same** `match_id` + **new** ticket (old unconsumed tickets revoked).

WildBoat does **not** return seat index. Seating / spawn slots are owned by the Dedicated Server.

### 4.3 `POST /v1/server/instances/register` (DS)

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
  "server_id": "...",
  "game_instance_id": "...",
  "status": "starting",
  "capacity": 4,
  "heartbeat_ttl_seconds": 30
}
```

Re-register effects (crash/restart recovery):
1. Prior instances for this server → `offline`
2. Open matches (`allocating`/`active`) → `aborted`
3. Open roster (`assigned`/`admitted`) → `left`
4. Unconsumed tickets for that server → `revoked_at` set
5. New `game_instance_id` issued

### 4.4 `POST /v1/server/instances/{game_instance_id}/heartbeat` (DS)

Request:
```json
{
  "status": "ready",
  "occupied_slots": 2,
  "active_match_id": "<uuid-or-omit>"
}
```

Notes:
- Recommended interval: ≤ `heartbeat_ttl_seconds / 2` (default TTL 30s → heartbeat every ~10–15s).
- Assignable only if `status=ready` **and** `last_heartbeat_at` within TTL.
- `occupied_slots` / `active_match_id` are **hints**; WildBoat remains authoritative for assignment capacity via DB roster.
- `draining` / `offline` revokes unconsumed tickets for that instance.

Response `200`:
```json
{ "ok": true, "status": "ready", "server_time": "..." }
```

Errors: `401 SERVER_AUTH_INVALID`, `403 FORBIDDEN`, `404 NOT_FOUND`, `400 INVALID_INPUT`.

### 4.5 `POST /v1/server/tickets/validate` (DS)

Request:
```json
{
  "game_ticket": "<opaque>",
  "player_id": "...",
  "match_id": "...",
  "game_instance_id": "..."
}
```

Response `200`:
```json
{
  "valid": true,
  "player_id": "...",
  "match_id": "...",
  "game_instance_id": "...",
  "server_id": "...",
  "consumed_at": "..."
}
```

Errors:
| Code | HTTP | Meaning |
|---|---|---|
| `SERVER_AUTH_INVALID` | 401 | Bad DS credential |
| `TICKET_INVALID` | 400 | Unknown / wrong binding / wrong server |
| `TICKET_EXPIRED` | 409 | Past `expires_at` |
| `TICKET_REPLAYED` | 409 | Already consumed |
| `TICKET_REVOKED` | 409 | Revoked (re-register / drain / settle) |

Rules for Agent B:
- Fail closed if WildBoat unreachable/timeout/5xx (recommend 3s client timeout).
- One-time consume; do not cache “valid” across reconnects.
- On peer reconnect needing a new ticket, client must call `/v1/games/assign` again.

### 4.6 `POST /v1/server/matches/{match_id}/settlement` (DS)

Request:
```json
{
  "settlement_id": "<ds-chosen-uuid>",
  "result": "completed",
  "started_at": "2026-10-31T08:00:00Z",
  "ended_at": "2026-10-31T08:10:00Z",
  "players": [
    { "player_id": "...", "team_id": "", "kills": 1, "deaths": 0, "score": 10 }
  ],
  "mode_stats": { "duration_seconds": 600 }
}
```

`result`: `completed` | `aborted`.

Response `200`:
```json
{
  "ok": true,
  "settlement_id": "...",
  "match_id": "...",
  "already_processed": false
}
```

Retry with **same** `settlement_id` → `already_processed: true` (no double stats).  
Errors: `401 SERVER_AUTH_INVALID`, `403 FORBIDDEN`, `404 NOT_FOUND`, `409 MATCH_NOT_SETTLEABLE`, `400 INVALID_INPUT`.

Only DS may settle. Client-reported scores must never be treated as authoritative by WildBoat player APIs (there is no client settlement route).

---

## 5. Identifier relationships

```text
dedicated_servers.server_id
        │
        ▼
game_instances.game_instance_id  (endpoint/slot; capacity ≤ 4)
        │
        ▼
matches.match_id  (one open match per instance in Phase 2B/2D)
        │
        ├── match_players (player_id, join_state: assigned|admitted|left)
        ├── game_tickets  (hash only; bound to player+match+instance)
        └── match_settlements (settlement_id unique; one settlement per match)
```

- `account_id` stays inside WPass; DS sees `player_id` only.
- Ticket never contains access/refresh tokens.

---

## 6. State machines

### 6.1 Game instance

```text
register(status=starting)
        │
        ▼
   starting ──heartbeat(ready)──► ready ◄──settlement (if was full)
                  │                 │
                  │                 ├──capacity filled──► full
                  │                 ├──heartbeat(draining)──► draining
                  │                 └──heartbeat(offline)/re-register──► offline
```

Assignable iff: `ready` + fresh heartbeat + `occupied_slots < capacity`.

### 6.2 Match

```text
assign (create) → allocating → (first assign activates) → active
                                                      │
                         settlement(result=completed) ┼──► completed
                         settlement(result=aborted)   ┼──► aborted
                         DS re-register               ┘──► aborted
```

### 6.3 Player join_state

```text
assign → assigned → ticket validate → admitted → settlement/re-register → left
```

At most one open membership (`assigned`/`admitted`) per player.

### 6.4 Ticket

```text
issue → (valid) → consume once
              ↘ expire / revoke / replay → rejected
```

---

## 7. Recommended DS runtime sequence (Phase 2D)

1. Boot with env `DEDICATED_SERVER_CREDENTIAL` (same value as WildBoat).
2. `POST /v1/server/instances/register` (`status=starting`).
3. When accepting peers: heartbeat `status=ready` on an interval within TTL.
4. On client connect: online `tickets/validate` with ticket + ids from client assignment; fail closed.
5. Run authoritative gameplay sync **locally in DS** (not via WildBoat polling).
6. On natural end: `settlement` with `result=completed` and unique `settlement_id`.
7. On forced end while DS still alive: `settlement` with `result=aborted`.
8. On process restart: **re-register** (do not reuse old `game_instance_id`); clients must re-`assign`.
9. On graceful shutdown: heartbeat `draining` then `offline` (or stop; TTL ages out assignability).

### DS crash / hang guidance

| Situation | WildBoat behavior | Agent B action |
|---|---|---|
| Heartbeat stops | Instance stops being assignable after TTL | Existing local match may continue until DS recovers or dies |
| DS process restart | Re-register aborts old open matches; tickets revoked | Clients re-assign; do not reuse old tickets |
| DS dies forever without re-register | Open matches remain until that server credential re-registers | Ops restart DS (single-instance architecture); no permanent lock once DS returns |
| Settlement network flake | Idempotent on `settlement_id` | Retry same payload / same id |

There is **no** background sweeper that auto-aborts matches solely because heartbeat went stale. Recovery path is intentional: **re-register** or **settlement**. This matches Phase 2B contract §9 and is sufficient for Phase 2D local/single-DS operation.

---

## 8. What Phase 2D does **not** need from WildBoat

Do **not** wait on / request these for Phase 2D gameplay sync:

- Friends / Party / invites / presence
- Room codes / public browser / Quick Match queues
- Seat assignment API
- Real-time movement / combat / frame sync over HTTP
- Client-submitted authoritative match results
- Achievements / leaderboards / full match history APIs
- Multiple concurrent Dedicated Servers
- Redis or new infra

---

## 9. Test evidence (PostgreSQL)

Environment: Docker Compose `postgres:17.6-alpine` on `127.0.0.1:5432`, DB `wildboat`.

### 9.1 Go tests (`DATABASE_URL` set)

```text
ok  github.com/ZhiH2333/WildBoat/internal/match
ok  github.com/ZhiH2333/WildBoat/internal/httpapi/handlers
ok  github.com/ZhiH2333/WildBoat/internal/persistence
ok  github.com/ZhiH2333/WildBoat/internal/identity
```

Covered behaviors include: assign auth, ticket consume/replay/binding, capacity/full, concurrent assign without overfill, settlement auth + idempotency, ticket expiry, revoke-on-reregister, migrations.

### 9.2 Live HTTP smoke (running api+db)

Executed against `http://127.0.0.1:8080`:

1. DS register + heartbeat `ready`
2. Two distinct players `POST /v1/games/assign` same mode
3. Assert **identical** `match_id` and `game_instance_id`, distinct tickets → `SAME_MATCH_OK`
4. Both tickets validate successfully (one-time)
5. Settlement then retry same `settlement_id` → `already_processed: true` → `SETTLEMENT_IDEMPOTENT_OK`

Result: `PHASE2D_LIVE_SMOKE_PASS`

---

## 10. Backend change decision

| Option | Decision |
|---|---|
| Code changes | **None** |
| Contract changes | **None** (`phase-2b-api-contract.md` remains frozen source of truth) |
| Breaking field/API changes | **None** — Agent B existing Phase 2B/2C integration remains valid |

No blockers were found that require a minimal compatible patch before Agent B proceeds.

### Known non-blocking limitations (document, do not expand scope)

1. No auto-abort sweeper on heartbeat TTL alone (recovery = re-register / settlement).
2. No player “leave match” API; membership clears on settle or DS re-register.
3. Mode mismatch against an open match surfaces as `GAME_CAPACITY_FULL` (not a separate code).
4. Seat / team spawn layout is DS-owned, not returned by assign.
5. Single active DS capacity remains the architectural limit.

---

## 11. Acceptance answers (required)

1. **Phase 2D backend ready?** Yes.  
2. **Must-fix blockers?** None.  
3. **Real PostgreSQL tests?** PASS (package tests + live smoke).  
4. **API Contract changed?** No.  
5. **Agent B can start in-game sync?** Yes — use existing assign/ticket/heartbeat/settlement; sync traffic stays on DS.  
6. **Out of scope / not implemented here:** friends, party, rooms, leaderboards, achievements, realtime HTTP sync, multi-DS.  
7. **Git:** branch `dev`, HEAD `f7b24ee`, no Agent A commit/push for this phase; readiness doc may be uncommitted locally until explicitly requested.

### Final label

**PASS** — WildBoat backend needs no Phase 2D changes; Agent B may implement Dedicated Server gameplay state sync against the frozen Phase 2B contract.
