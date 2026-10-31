# WildBoat / WPass — Product Requirements Document

**Status:** Approved product direction / backend implementation specification  
**Service:** WildBoat  
**Identity product:** WPass  
**Client:** WPG (Godot)  
**Version:** 1.0

## 1. Product purpose

WildBoat is the backend platform for WPG Online 2.0. WPass is its account and player identity product. The service provides persistent identity, social play, matchmaking coordination, and cloud progression while delegating live gameplay simulation to Godot Headless Dedicated Server.

## 2. Confirmed scope

- Full WPass account and social platform.
- Friends, invitations, presence, Party, recently-played-with.
- Room code, public room browser, Quick Match.
- Co-op: Survival and Endless.
- Battle: FFA, Team Battle, Kill Race.
- Hybrid local/cloud progression.
- Four-player initial match limit.
- One active Dedicated Server game instance in initial deployment.
- Go backend, PostgreSQL, Docker-based server deployment.

## 3. Actors

- **Player:** registers, authenticates, manages profile, friends, Parties, rooms, and progression.
- **Party leader:** creates/invites/manages a Party and selects match preferences.
- **Party member:** accepts invitations, leaves, readies, and joins the Party's match.
- **Game client:** calls public HTTPS APIs and connects to the assigned game server.
- **Dedicated Server:** authenticates to server-only APIs, validates tickets, sends heartbeat, submits settlement.
- **Operator:** deploys, monitors, backs up, and maintains WildBoat.

## 4. Functional requirements

### 4.1 WPass identity
- Register and authenticate accounts.
- Email verification and password recovery.
- Stable `account_id` and WPG `player_id`.
- Profile management: display name, avatar reference, bio/summary.
- Access token issuance and refresh token rotation/revocation.
- Session listing/revocation.
- Account status and abuse controls.

### 4.2 Friends and social graph
- Send, accept, decline, cancel, and remove friend relationships.
- Block/unblock players.
- Enforce uniqueness and prevent self-friending.
- Paginated friend list.
- Presence: offline, online, in_game.
- Recently Played With from verified match participation.
- Invitations with expiry and single-use/defined-use semantics.

### 4.3 Party
- Create Party, invite, accept/decline, leave, disband.
- Leader transfer.
- Enforce maximum Party size of four for initial release.
- Party membership is independent of Room membership.
- Prevent conflicting active Party membership unless product policy explicitly changes.

### 4.4 Rooms
- Create room with mode, visibility, and settings.
- Generate six-character human-readable room code.
- Join by code, room ID, or invitation.
- Public room list with safe public metadata.
- Ready state and lifecycle transitions.
- Capacity enforcement and access checks.
- Expire abandoned rooms.

### 4.5 Matchmaking
- Quick Match for solo player or Party.
- Validate supported mode and player count.
- Match against compatible waiting rooms or queue.
- Allocate one active Dedicated Server instance in initial deployment.
- Return endpoint, protocol version, build metadata, and short-lived game ticket.
- Return explicit queue/full/capacity errors when no game slot is available.

### 4.6 Game tickets and server registry
- Issue short-lived, scoped tickets.
- Bind ticket to player, match, game instance, and expiry.
- Prevent replay/double admission.
- Provide authenticated server validation or a documented signed-token verification design.
- Register server instance and accept heartbeats.
- Revoke/expire tickets on cancellation or match lifecycle changes.

### 4.7 Match settlement and progression
- Accept settlement only from authenticated Dedicated Server.
- Deduplicate by settlement ID.
- Validate match, instance, roster, timestamps, mode, and allowed metrics.
- Persist match history and verified per-player results transactionally.
- Update achievements and aggregate stats idempotently.
- Expose read APIs to clients.
- Never accept client-submitted authoritative match outcomes.

## 5. Non-functional requirements

- Modular monolith initially.
- PostgreSQL as source of truth.
- Dockerized server-side runtime and dependencies.
- TLS at public ingress.
- Structured logs and request correlation IDs.
- Health and readiness endpoints.
- Database migration and backup/restore procedures.
- Rate limiting and abuse controls.
- Explicit API versioning.
- Transactional state transitions.
- Idempotency for retries and settlement.
- Metrics for request latency, errors, active sessions, room occupancy, matchmaking wait, ticket validation, and settlement failures.

## 6. Initial exclusions

- Guilds/clans.
- Public chat and voice.
- Cross-game account federation.
- Complex ranked seasons.
- Replay storage.
- Multi-region matchmaking.
- Multi-instance orchestration in the first deployment.
- Payment, wallet, or monetization systems.

## 7. Acceptance criteria

- Account registration/login/refresh/logout/revocation works.
- Player identity remains stable across sessions.
- Friend/Party/invitation/presence workflows work.
- Room code and room browser enforce access and capacity.
- Quick Match returns a valid assignment or explicit capacity response.
- Dedicated Server can validate a ticket and submit settlement.
- Duplicate settlement cannot duplicate stats or rewards.
- Cloud progression is queryable and consistent.
- Docker Compose local environment starts reproducibly.
- Production deployment, secret handling, backup, restore, and operational runbooks are documented.
