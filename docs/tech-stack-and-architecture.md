# WildBoat — Tech Stack & Architecture

## 1. Stack

| Component | Technology | Decision |
|---|---|---|
| Language | Go | Modular monolith |
| HTTP API | `net/http` or a small established router | Keep framework surface small |
| Database | PostgreSQL | Authoritative business store |
| SQL access | `database/sql` with a chosen driver or `pgx` | Pick one and standardize |
| Migrations | Versioned SQL migrations | Required |
| API contract | JSON over HTTPS, `/v1` | Required |
| Container | Docker | Required for server-side deployment |
| Local orchestration | Docker Compose | Required |
| Reverse proxy/TLS | Deployment ingress (e.g. Caddy/Nginx/managed ingress) | Choose per host |
| Observability | Structured logs + Prometheus-compatible metrics (or equivalent) | Add incrementally |
| Cache/queue | None required initially | Add only when measured need exists |

Pin exact Go, PostgreSQL, and dependency versions during repository bootstrap. Do not use floating production image tags.

## 2. Modular monolith layout

Suggested initial layout:
```text
wildboat/
  cmd/
    api/
      main.go
  internal/
    config/
    httpapi/
      middleware/
      handlers/
      response/
    identity/
    auth/
    profile/
    friends/
    presence/
    party/
    rooms/
    matchmaking/
    tickets/
    gameinstances/
    settlement/
    progression/
    persistence/
    security/
  migrations/
  deploy/
    docker/
  docs/
  scripts/
  Dockerfile
  compose.yaml
  .env.example
  .dockerignore
  go.mod
  CLAUDE.md
  README.md
```
This is a proposed starting structure, not an assertion that files already exist.

## 3. Service boundaries

### Identity/Auth
Owns account credentials, email verification state, password reset, sessions, token rotation, and account status.

### Player Profile
Owns WPG player identity and public profile fields.

### Friends/Social
Owns friend requests, relationships, blocks, and recently-played-with projection.

### Presence
Owns ephemeral online/in-game status and heartbeat/expiry policy. Initial implementation may use PostgreSQL with bounded update frequency; introduce Redis only if scale/latency requires it.

### Party
Owns Party membership, leader, invitations, and membership transitions.

### Rooms
Owns room metadata, room code uniqueness, visibility, occupancy, ready state, and room lifecycle.

### Matchmaking
Owns compatibility rules, queue/room selection, and allocation policy.

### Tickets
Issues and validates short-lived match-scoped admission tickets.

### Game Instance Registry
Tracks configured/registered Dedicated Server endpoint, health, build, capacity, and current assignment.

### Settlement/Progression
Accepts trusted game results, persists them idempotently, and updates achievements/statistics.

Keep modules in one deployable API initially. Avoid premature microservices.

## 4. Docker deployment

All WildBoat server-side components must be deployable with Docker:
- WildBoat API container.
- PostgreSQL container for local development/small single-host deployment.
- Optional reverse proxy container if not using managed ingress.
- Optional future worker container for email/outbox/background jobs.
- Godot Dedicated Server container is built/deployed from the WPG repository, not from WildBoat.

Production may use managed PostgreSQL rather than a database container if operationally preferable; the application still connects over a private network. If the requirement is a fully self-hosted server, Compose can run PostgreSQL, API, and reverse proxy on the host.

### Docker security
- Multi-stage build.
- Non-root API user.
- Minimal runtime image.
- No secrets baked into image.
- Healthcheck/readiness.
- Resource limits.
- Persistent PostgreSQL volume and tested backups.
- Pin image versions/digests for production.
- Expose only required ports.
- Use private Docker network for database.
- Use TLS at ingress.

## 5. Initial single game instance scheduling

Model game instances even though only one is deployed. This avoids hardcoding an unscalable global endpoint.

- `game_instances` tracks instance ID, endpoint, protocol, build, status, capacity, and heartbeat.
- `match_allocations` reserves the active slot transactionally.
- Initial policy permits one active match allocation.
- Use database transaction/locking to prevent two simultaneous requests from claiming the same slot.
- When unavailable, return a clear `GAME_CAPACITY_FULL` or queue response.
- Later scale-out adds more registered instances without changing the client contract.

## 6. Ticket strategy

Choose one approach and document it before implementation:

**Option A: online validation**
- Ticket is an opaque random secret stored hashed in DB.
- Dedicated Server calls a server-only validation endpoint.
- Supports immediate revocation but requires server-to-backend reachability.

**Option B: signed self-contained ticket**
- Backend signs short-lived claims; Dedicated Server verifies signature with public key.
- Requires key rotation, audience/issuer checks, expiry, nonce/replay control, and a revocation strategy.
- Avoid embedding symmetric signing secrets in client builds.

The implementation must bind claims to `player_id`, `match_id`, `game_instance_id`, expiry, and protocol/build constraints. Do not casually invent a JWT design without threat review.

## 7. Data consistency

- Use database transactions for Party membership, room occupancy, and match allocation.
- Use unique constraints for normalized email, player ID, room code, and settlement ID.
- Use optimistic version columns where concurrent updates are expected.
- Use idempotency keys for externally retried mutations.
- Store timestamps in UTC.
- Keep ephemeral presence separate from durable account/profile records.
- Use an outbox pattern if reliable email or background event delivery becomes necessary.

## 8. Security architecture

- Password hashing with Argon2id or another vetted modern password hash implementation.
- Short-lived access token; rotating refresh token stored hashed.
- Rate limits on authentication and abuse-prone endpoints.
- Email verification and password reset secrets are single-use and expire.
- Authorization checks are resource- and membership-based.
- Server-only endpoints use workload credentials/mTLS or equivalent.
- Secrets supplied through environment/secret manager, never committed.
- Audit account security and administrative actions.
- Avoid leaking whether an email exists in recovery responses.
- CORS allowlist only where browser clients require it.

## 9. Operational endpoints

- `GET /healthz`: process health.
- `GET /readyz`: dependencies ready (including DB).
- `GET /metrics`: restricted metrics endpoint, if enabled.
- Server heartbeat endpoint is authenticated and separate from public health endpoints.

## 10. Future scale-out

Later additions may include:
- Multiple Dedicated Server containers and allocation scheduler.
- Redis for presence/pub-sub/rate limiting if justified.
- Background worker for email/outbox.
- Separate presence gateway.
- Multi-region server registry.
Do not split services before operational evidence requires it.
