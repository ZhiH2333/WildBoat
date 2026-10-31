# Phase 2E.0 — Multiplayer / Profile API Contract

**Status:** freeze candidate for Agent A (WildBoat) + Agent B (WPG Multiplayer / Profile UI)  
**Date:** 2026-10-31  
**WildBoat branch / HEAD at audit:** `dev` @ `1c23962adf1942ecb5da9c25c8c874b7ba99d7e6`  
**Scope:** API inventory + contract design only (no large implementation in this phase)  
**Architecture:** Go modular monolith + PostgreSQL; single active Dedicated Server capacity (≤ 4 players)

This document does **not** replace `docs/phase-2b-api-contract.md`.  
Phase 2B assignment / ticket / DS / settlement contracts remain **frozen** unless an explicit compatibility note says otherwise.

---

## 0. Audit method (evidence, not docs-only)

Inspected artifacts:

| Area | Paths |
|---|---|
| Git | `dev` tracking `origin/dev`; working tree clean at audit start |
| Router | `internal/httpapi/router.go` |
| Auth / Me handlers | `internal/httpapi/handlers/auth.go`, `me.go` |
| Games / Server handlers | `internal/httpapi/handlers/games.go`, `server.go` |
| Identity domain | `internal/identity/{service,store,models,validate}.go` |
| Match domain | `internal/match/{service,store,models}.go` |
| Migrations | `migrations/000002_wpass_identity.up.sql`, `000003_phase2b_game_ops.up.sql` |
| Tests | `internal/httpapi/handlers/auth_http_test.go`, `games_http_test.go`, `internal/identity/service_integration_test.go`, `internal/match/service_integration_test.go` |
| Prior contracts | `docs/phase-2b-api-contract.md`, `docs/data-models-and-apis.md`, `docs/phase-2d-backend-readiness.md`, `docs/task-breakdown-and-implementation-plan.md` |

Search confirmation: **no** HTTP routes or tables for `rooms`, `room_members`, `matchmaking`, public `/stats`, or `/matches/history`.  
`matches.room_id` column exists (nullable) but has **no FK** and is unused by assignment today.

---

## 1. Existing capability inventory

Status legend:

| Label | Meaning |
|---|---|
| 已实现且可用 | Route + domain logic + migration + tests observed |
| 已实现但不完整 | Partial persistence or write path only; missing read/API or product semantics |
| 尚未实现 | No route / no table / no service use case |
| 需要客户端配合 | Backend usable, but product flow depends on client/ops (email delivery, avatar assets, DS) |
| UNKNOWN | Could not confirm from repo evidence |

| # | Capability | Status | Evidence / notes |
|---|---|---|---|
| 1 | 注册、邮箱验证、登录、刷新 Token、退出登录 | **已实现且可用** + **需要客户端配合** | Routes: `POST /v1/auth/register\|login\|refresh\|email/verify\|logout\|logout-all` (+ forgot/reset). Tests in `auth_http_test.go` / identity integration. Email delivery: production provider not wired; `EMAIL_MODE=dev` returns `dev_verification_token` (client/ops must complete verify). |
| 2 | 玩家 Profile 查询和编辑 | **已实现且可用** + **需要客户端配合** | `GET /v1/me`, `PATCH /v1/me/profile` (`display_name`, `avatar_id`, `bio`). Validation: name 2–32, avatar_id ≤64, bio ≤280. **No** avatar binary upload; `avatar_id` is an opaque client catalog key. |
| 3 | 玩家统计与比赛记录查询 | **已实现但不完整** / **尚未实现** | `player_stats` written on DS settlement (`matches_played`, `kills`, `deaths`, `score`). **No** `GET` stats route. Match rows + `match_players` exist after assign/settle, but **no** match-history API. Achievements / recently-played: **尚未实现**. |
| 4 | Quick Match | **尚未实现** | No `/v1/matchmaking/*`. `POST /v1/games/assign` is **immediate** DS allocation (join open match / create match) — **not** a lobby/queue Quick Match. Do not treat assign as product Quick Match. |
| 5 | 创建私人房间 | **尚未实现** | No `rooms` / `room_members` tables; no routes. |
| 6 | 房间码加入 | **尚未实现** | — |
| 7 | 房间成员列表 | **尚未实现** | — |
| 8 | Ready / Unready | **尚未实现** | — |
| 9 | 房主开始比赛 | **尚未实现** | — |
| 10 | Assignment 与 Game Ticket 发放 | **已实现且可用** | `POST /v1/games/assign` + DS validate/settlement per Phase 2B. Tests + Phase 2D smoke evidence. **需要客户端配合** for DS register/heartbeat and ENet join. |

### Related existing routes (not Multiplayer lobby)

| Method | Path | Auth | Role |
|---|---|---|---|
| `POST` | `/v1/games/assign` | player access token | Immediate match assignment + ticket (Phase 2B) |
| `POST` | `/v1/server/instances/register` | DS credential | Instance register |
| `POST` | `/v1/server/instances/{game_instance_id}/heartbeat` | DS credential | Liveness |
| `POST` | `/v1/server/tickets/validate` | DS credential | Consume ticket |
| `POST` | `/v1/server/matches/{match_id}/settlement` | DS credential | Trusted settlement |

Friends / Party / presence / public room browser: **尚未实现** (Phase 3 / later). Out of Phase 2E Multiplayer MVP unless explicitly expanded later.

---

## 2. Design goals for Phase 2E Multiplayer

Product entry after login:

1. **Quick Match** — join/create a public matchmaking lobby for a mode; when host starts (or auto-start policy), enter game via tickets.  
2. **Private room** — create room, share **server-generated** room code, join, ready, host start.  
3. **Assignment** — after room reaches `starting`/`in_game`, each member obtains a Phase-2B-compatible assignment (host + members).  
4. Keep **Access Token → WildBoat only**; **Game Ticket → DS only**.

Non-goals for this contract freeze:

- Real-time frame sync / combat (DS / client)  
- Changing Phase 2B DS endpoints  
- Multi-DS scheduling  
- Friends / Party as join prerequisites (Party may be attached later via nullable `party_id`)

---

## 3. Multiplayer API Contract (new)

Base: `/v1`  
Auth (unless noted): `Authorization: Bearer <access_token>`  
Error envelope (unchanged):

```json
{
  "error": {
    "code": "ROOM_FULL",
    "message": "human readable",
    "request_id": "<uuid-or-opaque>",
    "details": {}
  }
}
```

Identifiers: plain UUID strings (same as Phase 2B).  
Timestamps: UTC RFC 3339.

### 3.1 Shared types

#### Room

```json
{
  "room_id": "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
  "room_code": "K7M2QX",
  "owner_player_id": "7c9e6679-7425-40de-944b-e07fc1f90ae7",
  "mode": "coop_survival",
  "visibility": "private",
  "state": "open",
  "max_players": 4,
  "member_count": 2,
  "match_id": null,
  "created_at": "2026-10-31T12:00:00Z",
  "updated_at": "2026-10-31T12:01:00Z",
  "expires_at": "2026-10-31T12:30:00Z"
}
```

#### Room member

```json
{
  "player_id": "7c9e6679-7425-40de-944b-e07fc1f90ae7",
  "display_name": "Pilot",
  "avatar_id": "default",
  "ready": false,
  "is_owner": true,
  "joined_at": "2026-10-31T12:00:00Z"
}
```

#### Room detail response

```json
{
  "room": { "...": "Room" },
  "members": [ { "...": "Room member" } ],
  "self": {
    "player_id": "7c9e6679-7425-40de-944b-e07fc1f90ae7",
    "ready": false,
    "is_owner": true
  },
  "assignment_available": false
}
```

When `state` is `starting` or `in_game` and the caller is still a member, `assignment_available` is `true` and the client should call `POST /v1/rooms/{room_id}/assignment`.

### 3.2 Room rules (normative)

| Rule | Value |
|---|---|
| Max players | **4** (hard cap; request may set `max_players` 2–4) |
| Modes | Same as Phase 2B: `coop_survival`, `coop_endless`, `battle_ffa`, `battle_team`, `battle_kill_race` |
| Room code | **Server-generated** only; alphabet `A-Z` excluding ambiguous `I/O`, digits `2-9`; length **6**; unique among non-terminal rooms |
| Visibility | `private` (code join), `public` (Quick Match / future browser) |
| One active room per player | A player may belong to at most **one** non-terminal room (`open` \| `starting` \| `in_game`) |
| Ready requirement to start | All **non-owner** members must be `ready=true`; owner may start without being ready (owner is treated as ready for start checks) **or** product may require owner ready too — **Phase 2E chooses: all members including owner must be ready** |
| Min members to start | **≥ 1** (solo start allowed for private rooms and Quick Match when alone) |
| Client-claimed state | **Rejected**. Room membership, ready, owner, code, capacity are authoritative in PostgreSQL. |
| Concurrent join | Transaction + row lock on room; refuse at capacity with `ROOM_FULL` |
| Idempotency | Recommended header `Idempotency-Key` on create/join/start/assignment; same key + same player within 24h returns original success payload |

### 3.3 Room state machine

```text
                 create / quick-match join
                          │
                          ▼
                        open ◄── unready / member leave (if still ≥1)
                          │
            host start (all ready, DS available)
                          │
                          ▼
                      starting ──► (allocation fails) ──► open  (with error to starter)
                          │
            match + tickets issued (first successful assignment bind)
                          │
                          ▼
                       in_game
                          │
         match settlement / abort / DS re-register
                          │
                          ▼
                       closed
```

Terminal: `closed`, `expired`, `aborted` (treat as non-joinable).

### 3.4 Host leave / ownership transfer

| Situation | Behavior |
|---|---|
| Owner leaves and ≥1 member remains | Transfer ownership to earliest `joined_at` remaining member; room stays `open` (or stays `in_game` if already started — leaving mid-game does **not** abort DS match; only clears lobby membership; match roster follows Phase 2B leave-on-settle/re-register) |
| Last member leaves | Room → `closed` |
| Owner leaves while `starting` before assignment bind | Cancel start → `open` or `closed` if empty; revoke any unconsumed tickets created for that start attempt |
| Non-owner leaves in `open` | Remove membership; set their ready cleared; if below capacity, joinable again |

### 3.5 Expiry and cleanup

| Timer | Default | Effect |
|---|---|---|
| Idle room TTL | **30m** from `updated_at` while `open` with no successful start | Mark `expired`, remove members |
| Empty room grace | Immediate `closed` when member_count hits 0 | — |
| Matchmaking public empty TTL | **2m** after last member leave attempt / create with 0? (N/A — room always has creator) | Same idle TTL |
| Background sweeper | Required in implementation phase | Periodic job or on-read lazy expire; contract requires expire-before-join checks |

Clients must not assume a room lives forever after sharing a code.

### 3.6 Endpoints

#### 3.6.1 Create private room

`POST /v1/rooms`  
Auth: player access token  
Rate limit: recommended (same class as room joins)

Request:
```json
{
  "mode": "coop_survival",
  "visibility": "private",
  "max_players": 4
}
```

Notes:

- `visibility` must be `private` for this endpoint (Quick Match uses matchmaking).  
- Server generates `room_code`.  
- Creator becomes owner + sole member; `ready=false`.  
- If player already in a non-terminal room → `409 ROOM_ALREADY_IN`.

Response `201`:
```json
{
  "room": { "...": "Room", "visibility": "private", "state": "open" },
  "members": [ { "...": "Room member", "is_owner": true, "ready": false } ],
  "self": { "player_id": "...", "ready": false, "is_owner": true },
  "assignment_available": false
}
```

Errors: `401 AUTH_REQUIRED`, `400 INVALID_INPUT`, `409 ROOM_ALREADY_IN`, `429 RATE_LIMITED`.

#### 3.6.2 Join by room code

`POST /v1/rooms/join-by-code`  
Auth: player access token

Request:
```json
{
  "room_code": "K7M2QX"
}
```

Server normalizes code (trim, uppercase).  
Does **not** accept client `room_id` as authority for join.

Response `200`: room detail (same shape as create).

Errors:

| Code | HTTP | When |
|---|---|---|
| `ROOM_CODE_INVALID` | 404 | Unknown / expired / closed / wrong alphabet |
| `ROOM_NOT_JOINABLE` | 409 | State not `open` |
| `ROOM_FULL` | 409 | At capacity |
| `ROOM_ALREADY_IN` | 409 | Already in another room; or already in this room → **idempotent 200** with current detail preferred over error |
| `MODE_LOCKED` | 409 | Reserved; not used if join cannot change mode |

**Idempotent re-join:** if caller is already a member of the target open room, return `200` current detail (do not double-insert).

#### 3.6.3 Get room detail + members

`GET /v1/rooms/{room_id}`  
Auth: player access token  
Permission: must be current member (or owner of a just-closed room within 60s — **optional**; Phase 2E MVP: **members only**, else `403 FORBIDDEN` / `404 NOT_FOUND` without leaking existence preference: return `404 ROOM_NOT_FOUND` for non-members).

Response `200`: room detail shape above.

#### 3.6.4 Get current room (reconnect helper)

`GET /v1/rooms/current`  
Auth: player access token

Response `200`:
```json
{
  "room": null,
  "members": [],
  "self": null,
  "assignment_available": false
}
```
or full detail when in a non-terminal room.

#### 3.6.5 Leave room

`POST /v1/rooms/{room_id}/leave`  
Auth: player access token  
Permission: must be member

Request body: empty object `{}` or omitted.

Response `200`:
```json
{ "ok": true }
```

Idempotent: already not a member → `200 { "ok": true }`.

Applies host-transfer / close rules in §3.4.

Errors: `401`, `404 ROOM_NOT_FOUND` (unknown id), `409 ROOM_NOT_LEAVEABLE` if policy blocks leave during `starting` **Phase 2E: allow leave during `open` and `starting`; during `in_game` leave only removes lobby membership — match continues per Phase 2B**.

#### 3.6.6 Set ready

`PUT /v1/rooms/{room_id}/ready`  
Auth: player access token

Request:
```json
{ "ready": true }
```

Response `200`: room detail.

Rules:

- Only in `state=open`  
- Only members  
- Setting ready when already that value → idempotent `200`

Errors: `409 ROOM_NOT_JOINABLE` if not `open`, `403 FORBIDDEN`, `404 ROOM_NOT_FOUND`, `401`, `400 INVALID_INPUT`.

#### 3.6.7 Host start match

`POST /v1/rooms/{room_id}/start`  
Auth: player access token  
Permission: **owner only**

Request:
```json
{}
```

Optional future: `{ "idempotency` via header only }.

Preconditions (atomic transaction):

1. Caller is owner  
2. `state=open`  
3. All members `ready=true`  
4. `member_count` ≥ 1  
5. Assignable DS exists (same liveness rules as Phase 2B)  
6. No member currently has a conflicting open match membership outside this room flow  

On success:

1. Set room `state=starting`  
2. Allocate / create match bound with `matches.room_id = room_id`  
3. Insert `match_players` for all current members  
4. Issue **per-player** tickets (hashes stored)  
5. Set room `state=in_game`, store `match_id`  
6. Return **caller’s** assignment (Phase 2B field-compatible)

Response `200`:
```json
{
  "room": { "state": "in_game", "match_id": "...", "...": "..." },
  "assignment": {
    "game_instance_id": "...",
    "match_id": "...",
    "server_id": "...",
    "host": "127.0.0.1",
    "port": 24567,
    "transport": "enet_udp",
    "game_ticket": "<opaque>",
    "ticket_expires_at": "2026-10-31T12:05:00Z",
    "protocol_version": 1,
    "server_build": "wpg-ds-local",
    "mode": "coop_survival"
  }
}
```

Other members discover via poll `GET /v1/rooms/{room_id}` (`assignment_available=true`) then `POST .../assignment`.

Errors:

| Code | HTTP | When |
|---|---|---|
| `NOT_ROOM_OWNER` | 403 | Non-owner |
| `ROOM_NOT_READY` | 409 | Someone not ready |
| `ROOM_NOT_JOINABLE` | 409 | Wrong state |
| `GAME_SERVER_UNAVAILABLE` | 503 | No live DS |
| `GAME_CAPACITY_FULL` | 409 | Instance cannot fit roster |
| `ROOM_ALREADY_STARTED` | 409 | Already `starting`/`in_game` — if caller already has ticket path, prefer redirecting client to `/assignment` (implementation may return `200` with assignment for owner retry) |

**Idempotent start:** second start by owner while `in_game` with same room → return current owner assignment (new ticket, revoke old unconsumed — same as Phase 2B duplicate assign semantics).

#### 3.6.8 Claim assignment from room

`POST /v1/rooms/{room_id}/assignment`  
Auth: player access token  
Permission: member of room in `starting` or `in_game`

Request: `{}`

Response `200`: same `assignment` object as Phase 2B / start response (wrapper optional):

```json
{
  "assignment": { "...": "Phase 2B Assignment fields" }
}
```

Semantics mirror Phase 2B duplicate assign: same match, **new** ticket, revoke prior unconsumed tickets for that player/match.

Errors: `409 ASSIGNMENT_NOT_READY` if room still `open`; `403`/`404`; `503`/`409` capacity rare after start.

#### 3.6.9 Quick Match

`POST /v1/matchmaking/quick`  
Auth: player access token

Request:
```json
{
  "mode": "battle_ffa"
}
```

Behavior (Phase 2E MVP, single DS):

1. If player already in non-terminal room → `409 ROOM_ALREADY_IN` (or return that room if it is already a public matchmaking room for same mode — **prefer return existing** for idempotent UX).  
2. Find an `open` + `visibility=public` + same `mode` room with capacity → join.  
3. Else create new public room (server code still generated; may be unused by UI) with creator as owner.  
4. Do **not** auto-start. Clients show lobby; **owner** starts when ready (same as private).  
5. Optional later: auto-start when `member_count == max_players` and all ready — **not** required in MVP.

Response `200`/`201`: room detail (same as create/join).

Errors: `401`, `400 INVALID_INPUT`, `409 ROOM_ALREADY_IN`, `429 RATE_LIMITED`.  
Note: Quick Match does **not** bypass Ready/Start. It only finds/creates a public lobby.

#### 3.6.10 Cancel matchmaking / leave current

`POST /v1/matchmaking/cancel`  
Auth: player access token

Equivalent to leave current public room if any; `200 { "ok": true }` idempotent.

#### 3.6.11 Relationship to `POST /v1/games/assign`

| Path | Use |
|---|---|
| `/v1/games/assign` | **Unchanged** Phase 2B/2D path: skip lobby, immediate ticket. Remains for tooling / interim clients. |
| Room start + `/assignment` | Official Multiplayer UI path; sets `matches.room_id`. |

Both produce Phase-2B-shaped assignment JSON. DS validation/settlement **unchanged**.

Conflict rule: a player with an open match membership (`assigned`/`admitted`) cannot join another room / quick match → `409 MATCH_ALREADY_ACTIVE`. Room start must enroll the whole lobby into one match transactionally.

---

## 4. Profile API Contract

### 4.1 Already implemented (freeze as-is; document for UI)

#### 4.1.1 Get current account + profile

`GET /v1/me`  
Auth: player access token

Response `200` (existing):
```json
{
  "account_id": "3fa85f64-5717-4562-b3fc-2c963f66afa6",
  "email": "player@example.com",
  "status": "active",
  "email_verified": true,
  "player": {
    "player_id": "7c9e6679-7425-40de-944b-e07fc1f90ae7",
    "display_name": "Pilot",
    "avatar_id": "default",
    "bio": "",
    "created_at": "2026-10-31T06:00:00Z",
    "updated_at": "2026-10-31T06:00:00Z"
  }
}
```

This is the **account basics** source for Profile / Settings (email, status, player_id).

#### 4.1.2 Patch profile (nickname / avatar / bio)

`PATCH /v1/me/profile`  
Auth: player access token

Request (all fields optional; at least one required):
```json
{
  "display_name": "Renamed",
  "avatar_id": "pilot_01",
  "bio": "Fly safe."
}
```

Response `200`:
```json
{
  "player": {
    "player_id": "...",
    "display_name": "Renamed",
    "avatar_id": "pilot_01",
    "bio": "Fly safe.",
    "created_at": "...",
    "updated_at": "..."
  }
}
```

Validation (existing):

| Field | Rule |
|---|---|
| `display_name` | trim; 2–32 Unicode runes |
| `avatar_id` | trim; non-empty; ≤ 64 runes; **not** a URL upload |
| `bio` | ≤ 280 runes; may be empty |

Errors: `401 AUTH_REQUIRED`, `400 INVALID_INPUT` / `INVALID_JSON`.

**Avatar images:** WildBoat stores only `avatar_id`. Asset packs live in the Godot client. Do not invent CDN upload APIs in Phase 2E.

### 4.2 New read APIs (to implement)

Only expose data WildBoat truly owns after settlement.

#### 4.2.1 Get player aggregate stats

`GET /v1/me/stats`  
Auth: player access token

Response `200`:
```json
{
  "player_id": "7c9e6679-7425-40de-944b-e07fc1f90ae7",
  "matches_played": 12,
  "kills": 40,
  "deaths": 18,
  "score": 5600,
  "updated_at": "2026-10-31T11:00:00Z"
}
```

If no `player_stats` row yet (never settled):
```json
{
  "player_id": "...",
  "matches_played": 0,
  "kills": 0,
  "deaths": 0,
  "score": 0,
  "updated_at": null
}
```

Do **not** fabricate win-rate, rank, achievements, or mode breakdown until those columns/events exist.

#### 4.2.2 Get match history

`GET /v1/me/matches?limit=20&cursor=`  
Auth: player access token

Response `200`:
```json
{
  "items": [
    {
      "match_id": "22222222-2222-2222-2222-222222222222",
      "mode": "coop_survival",
      "result": "completed",
      "join_state": "left",
      "kills": 3,
      "deaths": 1,
      "score": 420,
      "team_id": "",
      "started_at": "2026-10-31T07:50:00Z",
      "ended_at": "2026-10-31T08:00:00Z",
      "room_id": null
    }
  ],
  "next_cursor": null
}
```

Inclusion rules:

- Only matches where the player appears in `match_players` **and** a settlement exists (`match_settlements`), **or** match `state` is `completed`/`aborted` with roster line updated.  
- Prefer settlement-backed rows so clients never see unverified mid-match scores as history.  
- `limit` default 20, max 50. Cursor opaque.

If implementation cannot yet join settlement payload cleanly, ship stats-only first and keep history endpoint returning empty `items` **only** until query is correct — **never fake rows**.

#### 4.2.3 Public player profile by id (optional, later)

`GET /v1/players/{player_id}` — **not required** for Phase 2E Profile self-view. Marked deferred (friends/social). Self Profile uses `/v1/me*`.

### 4.3 Session / account settings (existing)

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/v1/me/sessions` | List sessions |
| `DELETE` | `/v1/me/sessions/{session_id}` | Revoke one |
| `POST` | `/v1/auth/logout` | Revoke current |
| `POST` | `/v1/auth/logout-all` | Revoke all |
| `POST` | `/v1/auth/password/forgot` | Recovery start |
| `POST` | `/v1/auth/password/reset` | Recovery finish |

---

## 5. Data model (Phase 2E additions)

### 5.1 New tables (implementation phase)

#### `rooms`

| Column | Type | Notes |
|---|---|---|
| `id` | uuid PK | `room_id` |
| `room_code` | text unique among active | server-generated |
| `owner_player_id` | uuid FK players | |
| `party_id` | uuid null | reserved; unused until Party |
| `mode` | text | check same modes |
| `visibility` | text | `private` \| `public` |
| `state` | text | `open` \| `starting` \| `in_game` \| `closed` \| `expired` \| `aborted` |
| `max_players` | int | 1–4 (product default 4; min start 1) |
| `match_id` | uuid null FK matches | set on start |
| `version` | int | optimistic concurrency |
| `created_at` / `updated_at` / `expires_at` | timestamptz | |

#### `room_members`

| Column | Type | Notes |
|---|---|---|
| `room_id` | uuid | FK cascade |
| `player_id` | uuid | FK players |
| `ready` | boolean | default false |
| `joined_at` | timestamptz | ownership transfer order |
| `last_seen_at` | timestamptz | optional heartbeat later |

Constraints:

- PK `(room_id, player_id)`  
- Unique active membership: one non-terminal room per player (partial unique index)  
- Capacity enforced in transaction, not only CHECK

### 5.2 Existing tables (reuse)

| Table | Phase 2E use |
|---|---|
| `players` | Profile fields |
| `accounts` | Account basics on `/v1/me` |
| `matches.room_id` | Populate on room start (add FK when `rooms` exists) |
| `match_players` / `game_tickets` | Unchanged ticket rules |
| `player_stats` | Back `/v1/me/stats` |
| `match_settlements` | Gate trusted history |

### 5.3 Assignment vs room flow

```text
Lobby (rooms) --start--> matches (room_id set) + match_players + game_tickets
                              │
                              ▼
                     DS validate / play / settlement
                              │
                              ▼
                     player_stats (+ history projection)
```

---

## 6. Error codes and permissions

### 6.1 New / Multiplayer-focused codes

| Code | HTTP | Meaning |
|---|---|---|
| `ROOM_NOT_FOUND` | 404 | Unknown or not visible to caller |
| `ROOM_CODE_INVALID` | 404 | Bad/expired code |
| `ROOM_FULL` | 409 | Capacity reached |
| `ROOM_NOT_JOINABLE` | 409 | Wrong state for join/ready |
| `ROOM_ALREADY_IN` | 409 | Already in another active room |
| `ROOM_NOT_READY` | 409 | Start blocked: not all ready |
| `ROOM_ALREADY_STARTED` | 409 | Start conflict |
| `NOT_ROOM_OWNER` | 403 | Host-only action |
| `ASSIGNMENT_NOT_READY` | 409 | Room not started yet |
| `MATCH_ALREADY_ACTIVE` | 409 | Open match membership blocks lobby |
| `MATCHMAKING_UNAVAILABLE` | 503 | Reserved if queue backend fails (MVP may unused) |

Existing codes remain valid: `AUTH_REQUIRED`, `INVALID_INPUT`, `GAME_SERVER_UNAVAILABLE`, `GAME_CAPACITY_FULL`, ticket codes, etc.

### 6.2 Permission matrix

| Action | Owner | Member | Non-member |
|---|---|---|---|
| View room | yes | yes | no (`404`) |
| Leave | yes | yes | idempotent ok |
| Ready | yes | yes | no |
| Start | yes | no | no |
| Claim assignment | yes | yes | no |
| Change mode/settings | owner only (settings API deferred) | no | no |

WildBoat never trusts client-supplied `player_id` for authz; always from access token.

---

## 7. Compatibility with Phase 2B

| Topic | Decision |
|---|---|
| `docs/phase-2b-api-contract.md` | **Unchanged** — still source of truth for assign/ticket/DS/settlement |
| `POST /v1/games/assign` | Kept; behavior frozen |
| Assignment JSON fields | Room paths **must** return the same required fields as Phase 2B §6 |
| Ticket strategy | Opaque + online validate; no Access Token to DS |
| Settlement | DS-only; idempotent `settlement_id` |
| Capacity | Still one active DS, max 4 |
| Breaking changes | **None** intended for Phase 2B clients |

If a bug is found in Phase 2B contract text vs code, fix with an explicit erratum and Agent B notification — do not silently redefine fields in this 2E doc.

### Migration note for `matches.room_id`

Today: nullable uuid without FK.  
Implementation: add FK to `rooms(id)` once table exists; backfill remains null for assign-created matches.

---

## 8. Not implemented yet (explicit)

| Item | Notes |
|---|---|
| All `/v1/rooms*` and `/v1/matchmaking*` routes | Contract only |
| `rooms` / `room_members` migrations | — |
| Ready / host start / room code | — |
| `/v1/me/stats`, `/v1/me/matches` | Data partially exists; APIs missing |
| Public room browser `GET /v1/rooms` | Deferred |
| Friends / Party / invites / presence | Phase 3 |
| Achievements / leaderboards | No fake data |
| Avatar file upload | Out of scope |
| Auto-start when lobby full | Optional later |
| Redis / microservices | Forbidden by architecture |
| Changing DS realtime protocol | Out of scope |

---

## 9. Implementation suggestions and dependencies

Recommended order:

1. **Migration** `rooms` + `room_members` + FK `matches.room_id` → `rooms`.  
2. **Room service** create / join-by-code / get / leave / ready (no DS).  
3. **Contract tests** for capacity, host transfer, expiry, authz.  
4. **Start + assignment claim** reusing `match.Store` ticket issuance (extract shared allocate helper from `AssignPlayer` to accept explicit roster + `room_id`).  
5. **Quick Match** as public-room find-or-create on top of room service.  
6. **`GET /v1/me/stats`** (trivial read).  
7. **`GET /v1/me/matches`** (settlement-gated query).  
8. WPG Multiplayer UI integration against frozen paths.  
9. Keep `/v1/games/assign` for non-lobby flows / DS soak tests.

Dependencies:

- Live DS register + heartbeat (existing) before start can succeed.  
- Access token auth middleware (existing).  
- Rate limits for join/create/start (extend in-process limiter or ingress).  
- Party **not** required for MVP private/quick rooms.

### 9.1 Contract test design (implement later; not required this phase)

| Test | Assert |
|---|---|
| Create private room | Server `room_code` length/alphabet; creator owner; member_count=1 |
| Join by code | Second player appears; capacity 4th ok, 5th `ROOM_FULL` |
| Join invalid code | `ROOM_CODE_INVALID` |
| Ready + start gate | Start without all ready → `ROOM_NOT_READY`; non-owner start → `NOT_ROOM_OWNER` |
| Start success | `matches.room_id` set; all members can claim assignment; tickets validate on DS routes |
| Host leave | Ownership transfers; last leave closes room |
| One room per player | Second create/join → `ROOM_ALREADY_IN` |
| Quick Match | Two players same mode land in same public open room when capacity allows |
| Stats read | After settlement, counters match; before settlement zeros |
| History | No rows for in-progress matches; settled matches appear |
| Phase 2B regress | Existing `games_http_test` / match integration still pass unchanged |

---

## 10. Client integration sketch (Agent B)

### Multiplayer private

1. Login → access token  
2. `POST /v1/rooms` → show `room_code`  
3. Peer `POST /v1/rooms/join-by-code`  
4. Poll `GET /v1/rooms/{room_id}` (or `/current`) for members/ready  
5. `PUT .../ready` `{ "ready": true }`  
6. Owner `POST .../start` → connect with returned ticket  
7. Others `POST .../assignment` → connect  
8. Never send access/refresh tokens to DS  

### Quick Match

1. `POST /v1/matchmaking/quick` `{ "mode" }`  
2. Same ready/start/assignment as private  
3. `POST /v1/matchmaking/cancel` to leave  

### Profile

1. `GET /v1/me` for identity + editable fields  
2. `PATCH /v1/me/profile` for nickname/avatar_id/bio  
3. `GET /v1/me/stats` when implemented  
4. `GET /v1/me/matches` when implemented — until then UI shows empty/coming-soon **without fabricated stats**

### Interim without rooms

If UI ships before room APIs land: only `POST /v1/games/assign` works (Phase 2D path). Label it clearly as temporary, not product Quick Match.

---

## 11. Audit summary for stakeholders

| Question | Answer |
|---|---|
| What was checked? | Git `dev`@`1c23962`, router, identity/match handlers+services+stores, migrations 000002/000003, HTTP/integration tests, Phase 2B/2D docs |
| Truly usable now? | Auth lifecycle, `/v1/me` + profile patch, `/v1/games/assign` + full DS ticket/settlement path |
| Missing for Multiplayer UI? | Entire room/matchmaking lobby surface; ready; host start; room-scoped assignment claim |
| Missing for Profile UI? | Stats/history **read** APIs (write path for stats exists); achievements absent |
| Phase 2B compatibility? | Additive only; no breaking change to frozen assign/ticket/DS contract |
| This phase code changes? | Documentation only (`docs/phase-2e-multiplayer-profile-api-contract.md`) |
| Commit/push? | **Not performed** (per Phase 2E.0 scope) |

---

## Document control

| Field | Value |
|---|---|
| Contract id | `phase-2e-multiplayer-profile-api` |
| Supersedes | Nothing (additive) |
| Depends on | `docs/phase-2b-api-contract.md` |
| Next phase | Implement migrations + room/matchmaking/profile-read APIs behind this freeze |

### Implementation note (Phase 2E.1)

Frozen request/response fields and semantics in this document are unchanged.  
Agent A implementation details, smoke steps, and residual limits live in:

`docs/phase-2e-multiplayer-profile-api-implementation.md`
