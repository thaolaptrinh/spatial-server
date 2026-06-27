# ADR 006: Game Server Lifecycle

## Status

Approved

## Context

Game Servers have a lifecycle: starting, running, draining, shutting down, or crashed. The platform must handle each state correctly.

## Problem

Game Servers transition through multiple states during their lifetime. Without a well-defined lifecycle, servers may shut down with active zones, crash without recovery, or process requests in incorrect states.

## Decision

### States

| State | Description |
|-------|-------------|
| `JOINING` | Server started, registered, no zones assigned |
| `ACTIVE` | Server has zones, processing simulation ticks |
| `DRAINING` | Server is preparing to shut down, zones being transferred away |
| `SHUTDOWN` | Server has released all zones, process will exit |

### Transitions

- JOINING → ACTIVE: Room Service assigns at least one zone.
- ACTIVE → DRAINING: Room Service initiates (scale-down or rolling update).
- DRAINING → SHUTDOWN: All zones transferred, server confirms.
- ACTIVE → (crash): Heartbeat timeout → Room Service marks zones orphan → reassign.

### Graceful Shutdown

1. Signal (SIGTERM) received.
2. Game Server sends `PrepareShutdown()` to Room Service.
3. Room Service sets status to DRAINING, stops assigning new zones.
4. Room Service transfers all zones to other ACTIVE servers.
5. Once all zones transferred, Room Service sends `ShutdownConfirmed()`.
6. Game Server exits.

### Crash Recovery

1. Heartbeat timeout (15s) → Room Service marks server as LOST.
2. Room Service marks all its zones as ORPHAN.
3. Room Service assigns orphan zones to ACTIVE servers.
4. New owners load zone state from PostgreSQL.
5. Redis metadata cache is invalidated for recovered zones.

## Alternatives

1. **Immutable Game Servers**: Always create new servers for scale-out, never drain or transfer zones. Simple but wasteful for zone churn and slow to rebalance.
2. **State machine with external store**: Store lifecycle state exclusively in PostgreSQL/Redis with transitions enforced by the store. More durable but adds latency per transition.
3. **Lifecycle managed by orchestrator (K3s)**: Rely solely on K3s pod lifecycle hooks. Less control over graceful shutdown timing and zone transfer coordination.

## Tradeoffs

- Well-defined states make recovery deterministic but add code complexity for state validation.
- Graceful shutdown ensures zero data loss but requires sufficient ACTIVE capacity to accept transferred zones.
- Crash recovery reloads from last DB snapshot, losing up to 5s of in-memory state — acceptable for MVP.

## Consequences

- Graceful shutdown requires sufficient ACTIVE servers to accept transferred zones.
- Crash recovery means zones reload from DB (may lose in-memory state since last persistence interval).
- Should persist zone state to PostgreSQL periodically (configurable, default 5s).

## Future Considerations

- Automated canary deployment with staged DRAINING.
- Preemptible instance handling for cloud spot instances.
- Lifecycle metrics and alerts for each state transition.

## Replaces

None.
