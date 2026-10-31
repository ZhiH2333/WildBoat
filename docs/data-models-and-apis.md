# WildBoat — Data Models & APIs

This is the proposed initial API and relational model contract shared with WPG. It must be reviewed and versioned before implementation.

## 1. Identifier and timestamp conventions

- IDs: opaque UUIDs or UUIDv7-style identifiers; choose one convention and apply consistently.
- External player/account IDs must not expose sequential database keys.
- Timestamps: UTC RFC 3339 in APIs; `timestamptz` in PostgreSQL.
- Soft deletion only where product/audit requirements justify it.
- Normalize email for uniqueness; do not use display name as identity.
- Use integer counters for counts; use explicit units for durations.

## 2. Core relational entities

### `accounts`
- `id` (PK)
- `email_normalized` (unique)
- `password_hash`
- `status` (`pending_verification`, `active`, `locked`, `closed`)
- `email_verified_at` nullable
- `created_at`, `updated_at`

### `players`
- `id` (PK; public `player_id`)
- `account_id` (unique FK accounts)
- `display_name`
- `avatar_id`
- `bio`
- `profile_version`
- `created_at`, `updated_at`

### `sessions`
- `id` (PK)
- `account_id` (FK)
- `refresh_token_hash`
- `created_at`, `expires_at`, `revoked_at`
- optional device/platform metadata with privacy minimization

### `email_action_tokens`
- `id`, `account_id`, `token_hash`
- `purpose` (`verify_email`, `password_reset`)
- `expires_at`, `consumed_at`, `created_at`

### `friend_requests`
- `id`, `sender_player_id`, `receiver_player_id`
- `status`, `created_at`, `updated_at`, `expires_at`
- unique/partial constraints to prevent duplicate pending requests

### `friendships`
- `player_low_id`, `player_high_id` (canonical sorted pair)
- `created_at`
- composite primary key

### `blocks`
- `blocker_player_id`, `blocked_player_id`, `created_at`
- composite primary key

### `presence`
- `player_id` (PK)
- `status` (`offline`, `online`, `in_game`)
- `last_seen_at`, `expires_at`
- optional current mode/match reference, avoid storing unnecessary location/device details

### `parties`
- `id`, `leader_player_id`, `status`, `version`
- `created_at`, `updated_at`, `expires_at`

### `party_members`
- `party_id`, `player_id`, `role`, `joined_at`
- unique active Party membership constraint per player (policy to confirm)

### `invitations`
- `id`, `kind` (`party`, `room`)
- `target_player_id`, `inviter_player_id`
- `party_id` nullable, `room_id` nullable
- `status`, `expires_at`, `created_at`, `consumed_at`

### `rooms`
- `id`, `room_code` (unique)
- `owner_player_id`, `party_id` nullable
- `mode`, `visibility`, `state`
- `max_players` (<= 4 initially)
- `settings_json` (validated allowlisted schema)
- `version`, `created_at`, `updated_at`, `expires_at`

### `room_members`
- `room_id`, `player_id`, `ready`, `joined_at`, `last_seen_at`
- unique membership constraints and capacity-safe transaction rules

### `game_instances`
- `id`, `host`, `port`, `transport`
- `status`, `protocol_version`, `server_build`
- `capacity`, `active_match_id` nullable
- `last_heartbeat_at`, `created_at`, `updated_at`

### `matches`
- `id`, `room_id`, `game_instance_id`
- `mode`, `state`, `started_at`, `ended_at`
- `settlement_id` unique nullable
- `created_at`, `updated_at`

### `match_players`
- `match_id`, `player_id`, `team_id` nullable
- `join_state`, `result_json` or normalized result fields
- composite primary key

### `game_tickets`
For opaque ticket strategy:
- `id`, `token_hash`, `player_id`, `match_id`, `game_instance_id`
- `expires_at`, `consumed_at`, `revoked_at`, `created_at`

### `match_settlements`
- `settlement_id` (unique)
- `match_id`, `game_instance_id`
- `payload_json` (validated canonical payload)
- `received_at`, `processed_at`, `status`

### `player_stats`
- `player_id` (PK)
- aggregate counters and `updated_at`
- update only from validated idempotent settlement processing

### `achievements` / `player_achievements`
- achievement definitions and versioned unlock records
- unlock source references a verified settlement/event

### `recent_players`
- `owner_player_id`, `other_player_id`, `last_played_at`, `matches_together`
- derived from verified match participation

Schema refinements must add foreign keys, check constraints, indexes, and deletion/retention policy.

## 3. API conventions

- Base path `/v1`.
- JSON request/response.
- HTTPS only in production.
- `Authorization: Bearer <access_token>` for player routes.
- Server routes use separate server authentication.
- `Idempotency-Key` required for settlement and recommended for retryable create/mutation endpoints.
- Pagination uses opaque cursor (`?cursor=...&limit=...`).
- Stable machine-readable error codes.
- Include `request_id` in responses/logs.

## 4. Public API

### Auth and profile
All paths are under `/v1` (for example `POST /v1/auth/register`).

- `POST /auth/register`
- `POST /auth/login`
- `POST /auth/refresh`
- `POST /auth/logout`
- `POST /auth/logout-all`
- `POST /auth/email/verify`
- `POST /auth/password/forgot`
- `POST /auth/password/reset`
- `GET /me`
- `PATCH /me/profile`
- `GET /me/sessions`
- `DELETE /me/sessions/{session_id}`

#### Phase 2 identity contract notes (implemented)

- Public IDs are UUID strings (no `acc_` / `plr_` prefix). Reason: opaque UUIDs already satisfy non-sequential identity without inventing a second encoding layer.
- Table names follow this document (`accounts`, `players`, `sessions`, `email_action_tokens`), not alternate names such as `player_profiles` / `refresh_sessions`.
- Registration does **not** issue session tokens. Login requires `status=active` after email verification (`ACCOUNT_NOT_VERIFIED` otherwise).
- Access tokens are short-lived HS256 JWTs (`Authorization: Bearer …`). Refresh tokens are opaque secrets stored only as SHA-256 hashes, rotated on each refresh, and revocable.
- Default TTLs: access `15m`, refresh `720h` (configurable).
- Refresh reuse outside a short grace window revokes all sessions for the account.
- Email provider is not configured for production yet. Responses include `email_delivery`:
  - `not_configured` when `EMAIL_MODE=none` (tokens stored, nothing faked as sent)
  - `dev_token_returned` when `EMAIL_MODE=dev` (non-production only; returns `dev_verification_token` / `dev_reset_token`)
- Auth endpoints apply an in-process per-IP rate limit; production should also enforce ingress/WAF limits.

### Friends/social
- `GET /friends`
- `POST /friend-requests`
- `POST /friend-requests/{id}/accept`
- `POST /friend-requests/{id}/decline`
- `DELETE /friends/{player_id}`
- `POST /blocks/{player_id}`
- `DELETE /blocks/{player_id}`
- `GET /players/{player_id}`
- `GET /presence/friends`
- `GET /recent-players`

### Party
- `POST /parties`
- `GET /parties/{party_id}`
- `POST /parties/{party_id}/invites`
- `POST /party-invites/{invite_id}/accept`
- `POST /party-invites/{invite_id}/decline`
- `POST /parties/{party_id}/leave`
- `POST /parties/{party_id}/disband`
- `POST /parties/{party_id}/transfer-leader`

### Room/matchmaking
- `POST /rooms`
- `GET /rooms`
- `GET /rooms/{room_id}`
- `POST /rooms/join-by-code`
- `POST /rooms/{room_id}/join`
- `POST /rooms/{room_id}/leave`
- `POST /rooms/{room_id}/ready`
- `PATCH /rooms/{room_id}/settings`
- `POST /matchmaking/quick`
- `GET /matchmaking/requests/{request_id}`
- `POST /matches/{match_id}/rejoin`

### Progression
- `GET /progression`
- `GET /progression/achievements`
- `GET /matches/history`
- `GET /stats`

## 5. Server-to-server API

These routes must not accept player access tokens as authorization:
- `POST /server/instances/register`
- `POST /server/instances/{instance_id}/heartbeat`
- `POST /server/tickets/validate`
- `POST /server/matches/{match_id}/settlement`
- `POST /server/matches/{match_id}/abort` (policy-controlled)

## 6. Example contracts

### Register
Request:
```json
{
  "email": "player@example.com",
  "password": "<user-supplied-secret>",
  "display_name": "Pilot"
}
```
Response:
```json
{
  "account_id": "3fa85f64-5717-4562-b3fc-2c963f66afa6",
  "player": {
    "player_id": "7c9e6679-7425-40de-944b-e07fc1f90ae7",
    "display_name": "Pilot",
    "avatar_id": "default",
    "bio": "",
    "created_at": "2026-10-31T06:00:00Z",
    "updated_at": "2026-10-31T06:00:00Z"
  },
  "verification_required": true,
  "email_delivery": "dev_token_returned",
  "dev_verification_token": "<only when EMAIL_MODE=dev>"
}
```

### Login
Request:
```json
{
  "email": "player@example.com",
  "password": "<user-supplied-secret>"
}
```
Response:
```json
{
  "tokens": {
    "access_token": "<jwt>",
    "refresh_token": "<opaque>",
    "token_type": "Bearer",
    "access_expires_at": "2026-10-31T06:15:00Z",
    "refresh_expires_at": "2026-11-30T06:00:00Z",
    "session_id": "9b1deb4d-3b7d-4bad-9bdd-2b0d7b3dcb6d"
  },
  "me": {
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
}
```

### Create room
```json
{
  "mode": "coop_survival",
  "visibility": "public",
  "max_players": 4,
  "settings": {
    "map_id": "default"
  }
}
```

### Quick Match
```json
{
  "mode": "battle_team",
  "party_id": "pty_opaque",
  "region": "auto"
}
```

### Assignment
```json
{
  "match_id": "mat_opaque",
  "game_instance_id": "gsi_opaque",
  "endpoint": {
    "host": "game.example.net",
    "port": 24567,
    "transport": "enet_udp"
  },
  "game_ticket": "<short-lived-secret>",
  "ticket_expires_at": "2026-10-31T04:00:00Z",
  "protocol_version": 1,
  "server_build": "wpg-server-build"
}
```

### Settlement
```json
{
  "settlement_id": "set_opaque_unique",
  "match_id": "mat_opaque",
  "mode": "coop_endless",
  "started_at": "2026-10-31T03:40:00Z",
  "ended_at": "2026-10-31T03:55:00Z",
  "result": "completed",
  "players": [
    {
      "player_id": "plr_opaque",
      "team_id": "team_alpha",
      "kills": 15,
      "deaths": 3,
      "score": 1400
    }
  ],
  "mode_stats": {
    "highest_wave": 9,
    "duration_seconds": 900
  }
}
```
The server submission must be authenticated. WildBoat validates the game instance, match roster, allowed metrics, timestamp bounds, and idempotency before updating progression.

## 7. Error format

```json
{
  "error": {
    "code": "GAME_CAPACITY_FULL",
    "message": "No game slot is currently available.",
    "request_id": "req_opaque",
    "details": {}
  }
}
```

Suggested codes include:
- `AUTH_INVALID_CREDENTIALS`
- `AUTH_SESSION_EXPIRED`
- `ACCOUNT_NOT_VERIFIED`
- `PLAYER_BLOCKED`
- `PARTY_FULL`
- `NOT_PARTY_LEADER`
- `ROOM_NOT_FOUND`
- `ROOM_FULL`
- `ROOM_CODE_INVALID`
- `ROOM_NOT_JOINABLE`
- `MATCHMAKING_UNAVAILABLE`
- `GAME_CAPACITY_FULL`
- `TICKET_EXPIRED`
- `TICKET_REPLAYED`
- `SETTLEMENT_ALREADY_PROCESSED`

## 8. Transaction and idempotency rules

- Party membership changes must be transactional.
- Room join must lock/check capacity and insert membership atomically.
- Match allocation must reserve the game instance atomically.
- Settlement processing must be idempotent by `settlement_id`.
- Stats/achievement updates must happen in the same transaction or through a durable outbox.
- Retryable endpoints must define whether the same idempotency key returns the original result.
- Presence updates may be coalesced and should expire automatically.

## 9. Open contract decisions

Resolve before production:
- UUID format and public ID encoding.
- Email provider and verification delivery.
- Opaque ticket vs signed ticket.
- Presence delivery mechanism (polling initially; push later if needed).
- Room code alphabet and expiry/collision retry.
- Exact per-mode settlement metrics.
- Rejoin window and slot reservation behavior.
- Whether Party may persist across match completion.
