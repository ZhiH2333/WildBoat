# WildBoat — Task Breakdown / Implementation Plan

## Phase 0 — Repository bootstrap
- [x] Create the separate `WildBoat` Git repository.
- [x] Add this documentation package.
- [x] Choose/pin Go and PostgreSQL versions.
- [x] Initialize Go module and standard layout.
- [x] Add formatting, test, vet, and lint workflow.
- [x] Add `.gitignore`, `.dockerignore`, `.env.example`.
- [x] Add Dockerfile and Compose skeleton.
**Exit:** clean bootstrap builds and starts a minimal API + PostgreSQL locally.

## Phase 1 — Runtime foundation
- [x] Configuration loader with validation.
- [x] Structured logging and request ID.
- [x] `/healthz` and `/readyz`.
- [x] PostgreSQL connection pool and migrations.
- [x] Graceful shutdown.
- [x] Docker health checks and non-root API runtime.
- [x] Integration test harness using PostgreSQL.
**Exit:** reproducible Docker Compose environment and migration pipeline.

## Phase 2 — WPass identity
- [ ] Account/player schema.
- [ ] Registration and email verification token flow.
- [ ] Password hashing.
- [ ] Login and short-lived access token.
- [ ] Refresh token rotation/revocation.
- [ ] Logout/session listing/revocation.
- [ ] Password recovery.
- [ ] Profile APIs.
- [ ] Rate limits and security audit events.
**Exit:** secure account lifecycle works end-to-end.

## Phase 3 — Social and Party
- [ ] Friend request lifecycle.
- [ ] Friendship and block rules.
- [ ] Presence with expiry.
- [ ] Party lifecycle and membership constraints.
- [ ] Invitations and expiry.
- [ ] Leader transfer/disband.
- [ ] Recently Played With projection.
**Exit:** complete social/Party API with concurrency tests.

## Phase 4 — Rooms and discovery
- [ ] Room schema and state machine.
- [ ] Six-character room code generation.
- [ ] Room visibility/access policy.
- [ ] Atomic join/capacity enforcement.
- [ ] Ready state.
- [ ] Public room browser.
- [ ] Room invitations.
- [ ] Expiry/cleanup.
**Exit:** room creation/join/listing is safe under concurrent requests.

## Phase 5 — Matchmaking and game tickets
- [ ] Game instance registry.
- [ ] Authenticated heartbeat.
- [ ] Single-active-instance capacity policy.
- [ ] Quick Match request lifecycle.
- [ ] Atomic match allocation.
- [ ] Ticket strategy decision and implementation.
- [ ] Ticket validation endpoint for Dedicated Server.
- [ ] Rejoin policy and endpoint.
**Exit:** WPG client receives an assignment and Dedicated Server can validate admission.

## Phase 6 — Settlement and cloud progression
- [ ] Server-authenticated settlement endpoint.
- [ ] Match roster and metric validation.
- [ ] Settlement idempotency.
- [ ] Match history.
- [ ] Player aggregate stats.
- [ ] Achievements.
- [ ] Recently-played-with from verified matches.
- [ ] Cloud progression read APIs.
**Exit:** trusted game results persist exactly once.

## Phase 7 — Production hardening
- [ ] TLS ingress and deployment config.
- [ ] Secret management.
- [ ] Backup and restore runbook.
- [ ] Metrics and alerting.
- [ ] Rate limiting and abuse review.
- [ ] Load/concurrency tests.
- [ ] Security review.
- [ ] Docker image pinning and vulnerability scanning.
- [ ] Operational incident and recovery procedures.
**Exit:** release candidate passes reliability/security checklist.

## Phase 8 — Multi-instance expansion
- [ ] Multiple game instance registration.
- [ ] Allocation scheduler and capacity leases.
- [ ] Instance draining and health-based removal.
- [ ] Queue fairness and timeout policy.
- [ ] Optional Redis/presence gateway only if justified.
**Exit:** multiple independent matches can run concurrently across instances.

## Cross-repository integration sequence

1. Agree API contract changes in both repositories.
2. Implement backend endpoint and migration.
3. Add contract/integration tests.
4. Update WPG DTO/client integration.
5. Test against a pinned WildBoat build.
6. Release compatible backend/client versions.

## Definition of Done

- Code and migrations are implemented.
- Tests cover success, failure, authorization, concurrency, and retry paths.
- Docker build and Compose smoke test pass.
- Logs contain request IDs but no secrets.
- Docs and API contract match implementation.
- Deployment/rollback/backup implications are documented.
- No production deployment occurs without explicit approval.
