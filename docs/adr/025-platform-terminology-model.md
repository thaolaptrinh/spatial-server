# ADR 025: Platform Terminology Model

> **Last Updated:** 2026-06-27

## Status

Accepted — terminology definition only. This ADR defines the canonical vocabulary of the
Spatial Runtime Platform. **It does not mandate any code, package, file, SQL, metric, proto, or
configuration rename.** Repository names are stable implementation details and are not required
to mirror conceptual terms one-to-one. Naming stability and minimal churn are explicit goals.

## Purpose

Establish a single, unambiguous terminology model so that documentation, ADRs, specs, and
discussions use words consistently. This resolves the divergence between the glossary (which
overloads "Runtime") and the platform charter (which distinguishes "Space" from "Runtime Node").

## Context

The platform historically used two nouns — "Runtime" (a realtime session/namespace) and
"Game Server" (the process that simulates zones). These were used inconsistently across the
glossary, ADRs, and the charter, making it unclear whether a term referred to a deployable
service or a runtime concept.

## Decision

The platform is described in **four layers**. Each term belongs to exactly one layer.

### Layer 1 — Platform

| Term | Meaning |
|---|---|
| **Spatial Runtime Platform** | The entire business-agnostic, reusable realtime spatial synchronization platform. |

### Layer 2 — Deployable Services (physical deployment units / binaries)

| Term | Meaning |
|---|---|
| **Gateway** | Deployable service. Terminates client WebSocket connections, orchestrates auth validation, routes packets. Stateless, horizontally scalable. |
| **Room Service** | Deployable service. Coordinator: Space/Runtime Node registry, ownership table, discovery, lifecycle coordination. |
| **Game Server** | Deployable service. Hosts one or more Runtime Nodes and provides the process environment in which Spatial simulation runs. |

**"Game Server" is a deployable service name, not a business concept and not business-domain
leakage.** It sits alongside Gateway and Room Service. Its name is preserved in all repository
artifacts (`apps/game-server`, `configs/game-server.yml`, `game_server.proto`, SQL tables,
metrics, package paths).

### Layer 3 — Runtime Concepts (logical abstractions the platform implements)

| Term | Meaning |
|---|---|
| **Space** | A realtime synchronization namespace. The top-level isolation unit. Entities in different Spaces never interact; AOI stops at Space boundaries. |
| **Runtime Node** | The logical unit of spatial simulation. Executed (hosted) by a Game Server. A Runtime Node is responsible for executing one or more Spaces and owns Zones within them. |
| **Zone** | A grid cell within a Space. The atomic unit of ownership. |
| **Entity** | A generic simulated object within a Space: position, transform, attributes, components. Not coupled to any business type. |
| **AOI** | Area of Interest. The set of entities relevant to an observer within a radius/region. |
| **Ownership** | The binding of a Zone to exactly one Runtime Node at any time. |

### Layer 4 — Business Concepts (external; NEVER owned by the platform)

| Term | Owner |
|---|---|
| Authentication (issuing tokens) | Business Backend |
| Authorization (permissions, access control) | Business Backend |
| Scheduling | Business Backend |
| Users (accounts, profiles) | Business Backend |
| Business metadata (space purpose, names, settings) | Business Backend |

The platform receives authenticated requests carrying a Space identifier and opaque metadata.
It never creates, stores, or reasons about business concepts.

## Key Relationships

```
Spatial Runtime Platform
│
├── Deployable Services
│   ├── Gateway
│   ├── Room Service
│   └── Game Server ──hosts──▶ one or more Runtime Nodes
│
└── Runtime Concepts
    Space ──executed by──▶ one or more Runtime Nodes
    Runtime Node ──owns──▶ Zones
    Zone ──contains──▶ Entities
    AOI / Ownership ──govern──▶ visibility and Zone↔RuntimeNode binding
```

- A **Game Server** (deployable) hosts ≥1 **Runtime Nodes**.
- A **Runtime Node** executes ≥1 **Spaces** and owns **Zones** within them.
- A **Space** may be executed across multiple **Runtime Nodes** (distributed Space execution).
  This must never be assumed to be 1 Space = 1 Runtime Node.
- **Ownership** binds a Zone to exactly one Runtime Node at any instant.

## Terminology vs. Repository Naming

| Canonical concept | Repository artifact (current, stable) |
|---|---|
| Runtime Node | `apps/game-server`, `internal/game/`, `game_server.proto`, `game_server_*` metrics, `game_servers` table |

This is acceptable and intentional. The Game Server is the deployable that hosts Runtime Nodes;
in the current implementation the relationship is 1:1, so the deployable name is reused in
code. The conceptual distinction is captured in documentation, not forced into identifiers.
**No rename is authorized by this ADR.** Future renames require a separate ADR with an
architectural (non-cosmetic) justification.

## What This ADR Does NOT Change

- No package, file, binary, config, proto, SQL table, or metric is renamed.
- No behavioral change.
- The decisions of ADR-005/006/013/016 stand unchanged; this ADR only clarifies the words used
  to describe them.

## Consequences

- Documentation, new ADRs, specs, and the glossary must use the four-layer vocabulary
  consistently. In particular, "Space" replaces the namespace sense of "Runtime" in prose, and
  "Runtime Node" is the simulation concept a Game Server hosts.
- The glossary will be updated (terminology only) to reflect this model — no code impact.
- Engineers can reason about deployment (Layer 2) independently from runtime semantics (Layer 3).

## References

- Platform charter (terminology and hierarchy)
- [ADR-013: Platform Boundary](013-platform-boundary.md)
- [ADR-016: Runtime Lifecycle](016-runtime-lifecycle.md)
- [ADR-005: Game Server Registration](005-game-server-registration.md)
- [Glossary](../glossary.md)
