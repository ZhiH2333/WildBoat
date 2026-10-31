# WildBoat

Go backend for WPG Online 2.0 and the WPass identity/social platform.

## Toolchain pins

| Component | Version |
|---|---|
| Go | 1.26.x |
| PostgreSQL | 17.6 (Compose image `postgres:17.6-alpine`) |
| Alpine runtime | 3.22 |

## Status

Phase 0/1 runtime foundation and **Phase 2 WPass identity** are implemented:

- Modular monolith layout under `cmd/` and `internal/`
- Config validation, structured JSON logging, request IDs
- PostgreSQL pool + embedded SQL migrations
- `/healthz` and `/readyz`
- WPass registration, email verification tokens, login, JWT access tokens, rotating refresh sessions
- Profile + session management APIs under `/v1`
- Graceful shutdown, Dockerfile, Docker Compose
- Unit tests and optional PostgreSQL integration tests

Friends, Party, rooms, matchmaking, tickets, and progression are **not** implemented yet.

## Local development

```bash
cp .env.example .env   # set TOKEN_SIGNING_KEY and POSTGRES_PASSWORD
make up                # build API + start PostgreSQL
make smoke             # hit /healthz and /readyz
make test
DATABASE_URL='postgres://wildboat:local-dev-only-change-me@localhost:5432/wildboat?sslmode=disable' go test ./...
make down
```

With `EMAIL_MODE=dev` (development/test only), registration returns `dev_verification_token` so you can call `POST /v1/auth/email/verify` without a mail provider. This is intentionally not production verification.

## Auth token lifecycle

| Token | Lifetime (default) | Storage | Notes |
|---|---|---|---|
| Access JWT | 15m | Not stored | HS256, bearer auth |
| Refresh opaque | 720h | SHA-256 hash in `sessions` | Rotated on each refresh; revocable |
| Email action | verify 24h / reset 1h | SHA-256 hash | Single-use |

Logout / logout-all / delete-session set `revoked_at`. Refresh reuse outside a short grace window revokes all account sessions.

## Read order

1. `CLAUDE.md`
2. `docs/prd.md`
3. `docs/tech-stack-and-architecture.md`
4. `docs/data-models-and-apis.md`
5. `docs/coding-standards-and-guidelines.md`
6. `docs/task-breakdown-and-implementation-plan.md`

## Ownership

WildBoat owns accounts, identity, social graph, Party, room metadata, matchmaking allocation, tickets, cloud progression, match history, and persistence.

WildBoat does **not** simulate combat. The Godot Headless Dedicated Server remains authoritative for live game state.
