# Runtime Lifecycle

> **Last Updated:** 2026-06-26

## Purpose

Defines the lifecycle of a Runtime — an instantiated realtime session corresponding to a business entity (room/showroom/meeting) in the Business Backend. Covers state transitions, player join/leave flows, and API surface.

## Runtime State Machine

See [Runtime Lifecycle Diagram](../diagrams/runtime-lifecycle.md#runtime-state-machine) for the state diagram.

| State | Description | Accepts Connections? |
|-------|-------------|---------------------|
| `creating` | Zones being allocated, Game Servers being assigned | No |
| `active` | Runtime is running, realtime simulation active | Yes |
| `draining` | Destroy initiated, players being disconnected | No (existing kept) |
| `destroyed` | All resources released, metadata may remain briefly for audit | No |

## Full Lifecycle Flow

See [Runtime Lifecycle Diagram](../diagrams/runtime-lifecycle.md#runtime-lifecycle-swimlane) for the swimlane sequence diagram.

The lifecycle follows a four-state progression: `creating → active → draining → destroyed`.

**Creating:** Business Backend calls `CreateRuntime()`. Room Service allocates zone IDs, selects optimal Game Servers via load-aware assignment, persists ownership records, and returns the Gateway address to the Business Backend.

**Active:** The runtime is live. Players connect via Gateway, entity simulation runs at 20 Hz, AOI queries process, and Game Servers send heartbeats every 5 seconds. Zone transfers and rebalancing occur within this state without transitioning out.

**Draining:** Business Backend calls `DestroyRuntime()`. Room Service signals Gateway to disconnect all players with a shutdown notification. Game Servers persist any dirty state, stop simulation, and acknowledge shutdown.

**Destroyed:** All resources are released: zone ownership records deleted, runtime metadata cleaned from PostgreSQL.

## API Surface

| Method | Description | Called By |
|--------|-------------|-----------|
| `CreateRuntime` | Allocate zones and Game Servers for a new runtime | Business Backend |
| `DestroyRuntime` | Release all resources for a runtime | Business Backend |
| `GetRuntimeInfo` | Query runtime status, player count, zone count | Business Backend |
| `GetRuntimeMetrics` | Query runtime performance metrics | Business Backend |
| `ListRuntimes` | List all active runtimes | Business Backend |

See ADR-009 for full protobuf definitions.

## Ownership

- Runtime is owned by the Business Backend that created it
- Spatial Server does not enforce multi-tenant isolation (trusted Business Backend model)
- Spatial Server never stores business metadata — only runtime operational state

## References

- [ADR-016](../adr/016-runtime-lifecycle.md) — Runtime Lifecycle (source ADR)
- [ADR-013](../adr/013-platform-boundary.md) — Platform Boundary
- [ADR-009](../adr/009-rpc-contract.md) — RPC Contract (protobuf definitions)
- [Overview](overview.md) — Architecture overview
