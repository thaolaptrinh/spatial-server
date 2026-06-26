# Architecture Documentation

> **Last Updated:** 2026-06-26

## Purpose

Define the Spatial Server architecture — principles, system context, service boundaries, runtime model, data model, communication patterns, and deployment topology.

## Contents

| File | Description |
|------|-------------|
| [overview.md](overview.md) | High-level architecture overview with component diagram |
| [principles.md](principles.md) | 8 architecture principles guiding all design decisions |
| [system-context.md](system-context.md) | C4 system context diagram — external actors and boundaries |
| [service-boundaries.md](service-boundaries.md) | Platform boundary and internal service responsibilities |
| [runtime-model.md](runtime-model.md) | Runtime definition, zones, and zone-to-server mapping |
| [runtime-lifecycle.md](runtime-lifecycle.md) | Four-state runtime lifecycle (creating → active → draining → destroyed) |
| [data-model.md](data-model.md) | Core data entities: Runtime, Zone, Entity, Player, Game Server |
| [component-responsibilities.md](component-responsibilities.md) | Gateway, Room Service, Game Server responsibilities |
| [communication.md](communication.md) | Communication patterns — WebSocket, gRPC, Redis |
| [communication-matrix.md](communication-matrix.md) | Complete communication matrix — all paths, transports, and latency requirements |
| [scaling.md](scaling.md) | Per-service scaling strategies and capacity planning |
| [performance-budget.md](performance-budget.md) | Quantitative performance targets (latency, throughput, resource usage) |
| [aoi-architecture.md](aoi-architecture.md) | Grid-based AOI (Area of Interest) design |
| [deployment-topology.md](deployment-topology.md) | Local dev, staging, and production deployment topologies |
| [repository-structure.md](repository-structure.md) | Directory structure and code organization conventions |

## Reading Order

1. Begin with [overview.md](overview.md) and [principles.md](principles.md) for the big picture.
2. Read [system-context.md](system-context.md) and [service-boundaries.md](service-boundaries.md) to understand boundaries.
3. Study [runtime-model.md](runtime-model.md) and [runtime-lifecycle.md](runtime-lifecycle.md) for the core runtime concepts.
4. Review [data-model.md](data-model.md), [communication.md](communication.md), and [communication-matrix.md](communication-matrix.md) for detailed design.
5. Read [component-responsibilities.md](component-responsibilities.md) for service-level details.
6. Finish with [scaling.md](scaling.md), [performance-budget.md](performance-budget.md), and [deployment-topology.md](deployment-topology.md).

## Related Documents

- [ADRs](../adr/README.md) — All 23 Architecture Decision Records
- [Diagrams](../diagrams/README.md) — Visual diagrams of architecture and flows
- [Infrastructure](../infrastructure/README.md) — Deployment infrastructure
- [Glossary](../glossary.md) — Terminology reference
