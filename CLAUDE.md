# CLAUDE.md — WildBoat

## Project identity

WildBoat is a new, separate Go backend repository for WPG Online 2.0. It does not currently exist as an implemented repository. This folder is the bootstrap specification only.

## Product baseline

- Identity platform name: WPass.
- Social scope: Full (friends, Party, invitations, presence, recently played with).
- Co-op modes: `coop_survival`, `coop_endless`.
- Battle modes: `battle_ffa`, `battle_team`, `battle_kill_race`.
- Match discovery: room code, public room browser, Quick Match.
- Progression: local + cloud hybrid; cloud owns verified online records.
- Initial match capacity: 4 players.
- Initial active game capacity: one Dedicated Server instance.
- Database: PostgreSQL.
- Deployment: server-side components run in Docker.

## Service responsibilities

WildBoat owns:
- WPass registration, authentication, account recovery, sessions, identity.
- Player profiles.
- Friend requests, friend/block relationships, presence.
- Party and invitation lifecycle.
- Room metadata, room codes, visibility, ready state.
- Quick Match and allocation policy.
- Short-lived game tickets.
- Cloud progression, achievements, match history, statistics.
- Dedicated Server registration/heartbeat and authenticated settlement ingestion.

WildBoat does not own:
- Godot rendering or UI.
- Real-time combat simulation.
- Enemy AI, collision, damage, player movement simulation, or authoritative live score.
- ENet/UDP packet relay.

## Architecture constraints

- Start as a modular monolith, not microservices.
- Go HTTP API over HTTPS.
- PostgreSQL is the system of record.
- Use migrations; never mutate production schema ad hoc.
- Use transactions for multi-entity state transitions.
- Use idempotency keys for settlement and retryable mutations.
- Separate public client routes from server-to-server routes.
- Never expose server credentials to game clients.
- Keep secrets out of Git, images, logs, and example configs.
- Dockerize API and supporting server-side processes.
- Do not put the Godot client in Docker.
- The initial game server is one active instance; allocation must enforce this capacity.

## Security requirements

- Passwords use a vetted password-hashing algorithm and parameters.
- Access tokens are short-lived; refresh tokens are rotatable/revocable.
- Store refresh token hashes, not raw refresh tokens.
- Rate-limit login, registration, recovery, friend requests, room joins, and invite operations.
- Validate ownership and membership for every resource mutation.
- Use constant-time comparison for secret/token checks where applicable.
- Server-to-server authentication for ticket validation, heartbeat, and settlement.
- Audit security-sensitive account/session actions.
- Never log passwords, access tokens, refresh tokens, game tickets, or recovery secrets.

## Coding standards

Follow `docs/coding-standards-and-guidelines.md`. Keep domain logic independent of HTTP handlers and SQL. Use context deadlines, structured logging, explicit error codes, and tests for state transitions.

## Required workflow

1. Initialize repository and record Go/toolchain versions.
2. Create a minimal build/test pipeline.
3. Add Docker Compose for local development.
4. Implement migrations and health endpoints.
5. Implement one vertical slice at a time.
6. Run tests, static analysis, migration checks, and container smoke tests.
7. Report actual evidence; do not mark unimplemented features complete.

Do not create cloud resources, deploy publicly, rotate production credentials, or modify external repository settings unless explicitly instructed.
