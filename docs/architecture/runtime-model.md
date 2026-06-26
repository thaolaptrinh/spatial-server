# Runtime Model

> **Last Updated:** 2026-06-26

## Purpose

Define the Runtime model — the conceptual and operational representation of an instantiated realtime session. Runtime is the top-level unit of isolation in Spatial Server, corresponding to a business room/showroom/meeting.

## Definition

A **Runtime** is an instantiated realtime session. It is the unit of:

- **Isolation** — Entities in different runtimes never interact. AOI boundaries stop at runtime edges.
- **Lifecycle** — Created and destroyed by Business Backend API calls.
- **Ownership** — Owned by the Business Backend that created it.
- **Resource allocation** — Zones and Game Servers are allocated per runtime.

## Runtime Structure

```
Runtime
├── runtime_id (UUIDv7)
├── status (creating | active | draining | destroyed)
├── zones[] — list of Zone objects
├── player_count (current)
└── metadata (opaque JSONB, Business Backend defined)
```

### Zone Composition

A runtime is composed of N zones (grid cells) arranged in a rectangular grid:

```
Runtime with 6 zones (2×3 grid):
┌──────┬──────┬──────┐
│ 0,0  │ 1,0  │ 2,0  │
├──────┼──────┼──────┤
│ 0,1  │ 1,1  │ 2,1  │
└──────┴──────┴──────┘

Each zone: 100×100 world units (configurable)
Total world size: 300×200 units for this runtime
```

### Entity Ownership

Each entity exists in exactly one zone within a runtime. Entities do not cross runtime boundaries.

## Runtime vs Server Mapping

Runtimes map to Game Servers through their zones:

```
Runtime A ──┬── Zone A1 ──→ Game Server 1
             ├── Zone A2 ──→ Game Server 1
             └── Zone A3 ──→ Game Server 2

Runtime B ──┬── Zone B1 ──→ Game Server 2
             └── Zone B2 ──→ Game Server 3
```

A single Game Server may host zones from multiple runtimes. A single runtime may span multiple Game Servers.

## Constraints

| Property | Constraint | Rationale |
|----------|------------|-----------|
| Zone count per runtime | Configurable at creation | Determines world size |
| Players per runtime | No hard limit | Limited by zone count × 100 players/zone |
| Entities per runtime | No hard limit | Limited by zone count × 100 entities/zone |
| Runtimes per Game Server | No hard limit | Limited by entity count (5,000 per server) |
| Runtimes per cluster | 1,000 (initial target) | Room Service metadata capacity |

## Lifetime

Runtime lifetime is managed exclusively by the Business Backend:

1. **CreateRuntime** — Business Backend decides when a runtime starts
2. **Runtime active** — Players join/leave freely, Spatial Server manages state
3. **DestroyRuntime** — Business Backend decides when a runtime ends
4. **Forced cleanup** — Room Service may clean orphaned runtimes after TTL expiry (safety net)

Spatial Server never creates or destroys runtimes autonomously (except for safety cleanup of runtimes whose owner is unreachable).

## Multi-Tenancy

Runtimes provide **soft isolation**:

- No entity in Runtime A can observe entities in Runtime B
- Zone IDs are unique across all runtimes (UUIDv7)
- Game Servers may host zones from multiple runtimes simultaneously
- No hard resource isolation between runtimes on the same Game Server
- Business Backend is responsible for its own multi-tenant isolation

## References

- [ADR-016](../adr/016-runtime-lifecycle.md) — Runtime Lifecycle
- [Runtime Lifecycle](runtime-lifecycle.md) — State machine and flows
- [Service Boundaries](service-boundaries.md)
- [Data Model](data-model.md) — Runtime table schema
- [Overview](overview.md)
