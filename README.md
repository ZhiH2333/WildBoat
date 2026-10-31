# WildBoat — Backend Agent Handoff

**WildBoat** is the planned Go backend repository for WPG Online 2.0 and the WPass identity/social platform.

- Repository status: **not yet created** (this package is a bootstrap specification).
- Product client repository: https://github.com/ZhiH2333/WPG
- Suggested language/runtime: Go (select a currently supported stable release when bootstrapping).
- Database: PostgreSQL.
- Server-side deployment: Docker / Docker Compose for initial deployment.

## Read order

1. `CLAUDE.md`
2. `docs/prd.md`
3. `docs/tech-stack-and-architecture.md`
4. `docs/data-models-and-apis.md`
5. `docs/coding-standards-and-guidelines.md`
6. `docs/task-breakdown-and-implementation-plan.md`

## Bootstrap warning

This is a design specification, not an existing service. Do not claim endpoints, migrations, containers, or deployment already exist. Initialize the repository deliberately, choose and pin dependency versions, and record those decisions.

## Core ownership

WildBoat owns accounts, identity, social graph, Party, room metadata, matchmaking allocation, tickets, cloud progression, match history, and persistence.

WildBoat does **not** simulate combat. The Godot Headless Dedicated Server is authoritative for live game state and submits authenticated match settlements.

## Initial capacity constraint

The first deployment has one active Dedicated Server game instance. WildBoat must prevent concurrent allocation beyond that capacity or implement a queue. Multi-instance orchestration is a later milestone.
