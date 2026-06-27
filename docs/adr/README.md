# Architecture Decision Records

> **Last Updated:** 2026-06-26

This directory contains Architecture Decision Records (ADRs) for the Spatial Server platform.

## Index

| # | Title | Status |
|---|-------|--------|
| 001 | [Zone Ownership](001-zone-ownership.md) | Approved |
| 002 | [Zone Migration](002-zone-migration.md) | Approved |
| 003 | [AOI Strategy](003-aoi-strategy.md) | Approved |
| 004 | [Coordinator Pattern](004-coordinator.md) | Approved |
| 005 | [Game Server Registration](005-game-server-registration.md) | Approved |
| 006 | [Game Server Lifecycle](006-game-server-lifecycle.md) | Approved |
| 007 | [Autoscaling](007-autoscaling.md) | Approved |
| 008 | [Deployment](008-deployment.md) | Approved |
| 009 | [RPC Contract](009-rpc-contract.md) | Approved |
| 010 | [Packet Protocol](010-packet-protocol.md) | Approved |
| 011 | [Failure Recovery](011-failure-recovery.md) | Approved |
| 012 | [Networking](012-networking.md) | Approved |
| 013 | [Platform Boundary](013-platform-boundary.md) | Approved |
| 014 | [Infrastructure Platform](014-infrastructure-platform.md) | Accepted |
| 015 | [Architecture Principles](015-architecture-principles.md) | Accepted |
| 016 | [Runtime Lifecycle](016-runtime-lifecycle.md) | Accepted |
| 017 | [Capacity Planning](017-capacity-planning.md) | Accepted |
| 018 | [Security](018-security.md) | Accepted |
| 019 | [Observability](019-observability.md) | Accepted |
| 020 | [Benchmark Strategy](020-benchmark-strategy.md) | Accepted |
| 021 | [Gateway Architecture](021-gateway-architecture.md) | Accepted |
| 022 | [Session Management](022-session-management.md) | Accepted |
| 023 | [Entity Model](023-entity-model.md) | Accepted |

## Reading Order

ADRs are numbered sequentially. Start with ADR-001 for foundational decisions and read in order for the full decision history. Key ADRs for new readers:

1. **ADR-013** [Platform Boundary](013-platform-boundary.md) — Spatial Server vs Business Backend boundary
2. **ADR-015** [Architecture Principles](015-architecture-principles.md) — Guiding design principles
3. **ADR-004** [Coordinator Pattern](004-coordinator.md) — Room Service role
4. **ADR-001** [Zone Ownership](001-zone-ownership.md) — Core ownership model
5. **ADR-009** [RPC Contract](009-rpc-contract.md) — Inter-service communication
6. **ADR-016** [Runtime Lifecycle](016-runtime-lifecycle.md) — Runtime state machine
7. **ADR-008** [Deployment](008-deployment.md) — Infrastructure decisions

## Related Documents

- [Architecture](../architecture/README.md) — Architecture documentation
- [Standards](../standards/README.md) — Coding and design standards
- [Glossary](../glossary.md) — Terminology reference

## ADR Format

Each ADR includes:

- **Context** — Why this decision was needed
- **Problem** — What problem it solves
- **Decision** — The chosen approach
- **Alternatives** — Options considered
- **Tradeoffs** — Advantages and disadvantages
- **Consequences** — Impact on the system
- **Future Considerations** — What might change this decision
