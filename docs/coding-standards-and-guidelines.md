# WildBoat — Coding Standards & Guidelines

## 1. General principles

- Modular monolith first; no premature microservices.
- Keep domain logic independent from HTTP and SQL.
- Prefer explicit state transitions and narrow interfaces.
- Make authorization checks visible at service boundaries.
- Optimize for correctness, security, operability, and testability.

## 2. Go conventions

- Use `gofmt` and standard Go project layout.
- Use idiomatic Go names; avoid stutter in package APIs.
- Keep packages cohesive and avoid giant `common`/`utils` packages.
- Pass `context.Context` as the first parameter for request-scoped operations.
- Do not store request context in long-lived structs.
- Wrap errors with context; expose stable API error codes separately from internal errors.
- Avoid panics for expected operational failures.
- Validate configuration at startup and fail fast on invalid required settings.
- Pin dependencies and review upgrades.

## 3. HTTP/API layer

- Handlers parse/validate transport DTOs and call application services.
- Do not put business logic or SQL directly in handlers.
- Set request body size limits.
- Apply request deadlines and graceful shutdown.
- Use consistent JSON envelopes and status codes.
- Add request/correlation ID middleware.
- Redact credentials and secrets from logs.
- Enforce authentication and resource authorization on every protected route.
- CORS must be restrictive and only enabled where needed.

## 4. Persistence

- Use versioned SQL migrations.
- Use transactions for multi-row state changes.
- Add foreign keys, unique constraints, check constraints, and indexes.
- Avoid N+1 query patterns.
- Use parameterized SQL only.
- Store UTC timestamps.
- Do not rely on application-level checks alone for uniqueness/capacity races.
- Add integration tests against PostgreSQL for critical transaction logic.

## 5. Security

- Password hash with Argon2id or another vetted modern algorithm; use a maintained implementation.
- Never store raw passwords, raw refresh tokens, or raw game tickets.
- Store refresh-token/ticket hashes when using opaque token storage.
- Use crypto-secure random values for tokens and room codes.
- Compare secrets safely.
- Rate-limit login, registration, password reset, friend requests, invites, and room joins.
- Use single-use expiring email action tokens.
- Prevent IDOR by checking resource ownership/membership.
- Separate player auth from server-to-server auth.
- Use least privilege for database users and deployment credentials.
- Keep secrets out of source, Docker build args, image layers, logs, and checked-in `.env`.
- Add security audit events for sensitive account actions.

## 6. Concurrency and consistency

- Model Party, Room, Match, and Game Instance as explicit state machines.
- Protect room capacity and single-instance allocation with database transactions/locks/constraints.
- Use optimistic version checks where appropriate.
- Idempotency is mandatory for settlement.
- Use an outbox if a database change must reliably trigger external delivery.
- Define expiry/cleanup jobs for sessions, invitations, rooms, tickets, and presence.

## 7. Docker and operations

- Multi-stage Docker build.
- Non-root runtime.
- Minimal runtime image.
- Pin base image versions.
- Health/readiness checks.
- Graceful SIGTERM handling.
- No secrets baked into images.
- PostgreSQL data stored in persistent volumes.
- Document backup and restore commands.
- Restrict database port exposure in production.
- Compose is the local/single-host baseline; production ingress and secrets management must be explicit.

## 8. Testing

Required:
- Unit tests for domain state machines and validation.
- HTTP handler tests.
- PostgreSQL integration tests for transaction/concurrency behavior.
- Auth/token expiry/revocation tests.
- Room code collision/capacity tests.
- Party membership race tests.
- Game instance allocation concurrency tests.
- Ticket replay/expiry tests.
- Settlement idempotency tests.
- Docker build and Compose smoke tests.
- Migration up/down or forward-only policy tests.

Never claim tests passed unless actually run.

## 9. Documentation/change control

- Keep OpenAPI or API contract documentation synchronized with implementation.
- Document config variables in `.env.example` without real secrets.
- Add migration notes for schema changes.
- Update task status only when complete.
- Do not deploy or create external infrastructure without explicit approval.
