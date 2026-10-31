# WildBoat

Go backend for WPG Online 2.0 and the WPass identity/social platform.

## Toolchain pins (Phase 0/1)

| Component | Version |
|---|---|
| Go | 1.26.x |
| PostgreSQL | 17.6 (Compose image `postgres:17.6-alpine`) |
| Alpine runtime | 3.22 |

## Status

Phase 0 (repository bootstrap) and Phase 1 (runtime foundation) are implemented:

- Modular monolith layout under `cmd/` and `internal/`
- Config validation, structured JSON logging, request IDs
- PostgreSQL pool + embedded SQL migrations
- `/healthz` and `/readyz`
- Graceful shutdown
- Dockerfile (multi-stage, non-root) + Docker Compose
- Unit tests and optional PostgreSQL integration test

Identity, social, matchmaking, and tickets are **not** implemented yet.

## Local development

```bash
cp .env.example .env   # if needed
make up                # build API + start PostgreSQL
make smoke             # hit /healthz and /readyz
make test
make down
```

Integration test against a reachable Postgres:

```bash
DATABASE_URL='postgres://wildboat:local-dev-only-change-me@localhost:5432/wildboat?sslmode=disable' make test-integration
```

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
