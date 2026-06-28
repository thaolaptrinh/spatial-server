# ADR 026: Runtime Extension Model

> **Last Updated:** 2026-06-27

## Status

Accepted — descriptive. This ADR documents the extension model that already
exists in the codebase after the runtime-boundary refactor. It introduces no
new framework.

## Purpose

Define how new behavior and new capabilities are added to the Spatial Runtime
without modifying the simulation core, and without leaking business logic into
it. This records the contracts the core exposes so future modules (autonomous
agents, voice, recording, analytics) integrate against stable interfaces.

## Context

Earlier audit findings showed two forms of coupling between the simulation
core and concrete concerns:

1. The simulation loop type-switched on a concrete `*NPCLifecycle`, so adding
   any new autonomous lifecycle required editing the core (Open/Closed
   violation), and NPC AI lived in the runtime core.
2. The simulation encoded wire packets directly, coupling the core to the
   binary protocol and to business-specific state fields (`animation`,
   `health`).

The platform boundary ([ADR-013](013-platform-boundary.md)) requires the
runtime to remain business-agnostic and reusable.

## Decision

The runtime core exposes **three interface-based extension points** and
nothing else. No plugin framework, registry discovery, or dynamic loading is
used — modules are plain Go types wired at process start.

### 1. Entity Lifecycle (per-entity behavior)

Defined in `internal/game/entity`:

```go
type Lifecycle interface {
    Spawn()
    Despawn()
    OnEnterZone(zoneID types.ZoneID)
    OnLeaveZone(zoneID types.ZoneID)
    OnSimulate(e *Entity, dt time.Duration)
    OnAction(action string, payload []byte)
}
```

`BaseLifecycle` provides no-op defaults. The simulation core calls only this
interface — it never type-switches on a concrete lifecycle. `OnSimulate` is the
hook for autonomous behavior: an entity's lifecycle mutates the entity's
position/state, and the core detects movement and propagates it (AOI update +
move event) generically.

The concrete NPC behaviors (`PatrolBehavior`, `WanderBehavior`, `IdleBehavior`)
in `internal/game` are an **example consumer** of this hook, mounted via
`NPCLifecycle`. They are not part of the runtime contract; a future product may
replace them or add new lifecycles without touching the core.

### 2. Runtime Events (outbound observation)

Defined in `internal/game`:

```go
type Event struct {
    Kind     EventKind   // Spawn | Despawn | Move | State
    Space    types.RuntimeID
    Observer types.EntityID
    EntityID types.EntityID
    Type     string
    Position types.Vector3
    Payload  []byte       // opaque state blob; the core never interprets it
}
```

The simulation publishes `Event` values to a buffered channel consumed by a
single downstream adapter (in `apps/game-server`) that translates them into the
client wire format. The core therefore depends only on its own event model and
never on `pkg/protocol` or protobuf wire types. New consumers (e.g., a future
recording or analytics module) subscribe to the same events without the core
changing.

### 3. Opaque Entity Attributes (data extensibility)

Entities carry `Attrs map[string][]byte` and a free-form string `Type`. The
core never interprets attribute contents or entity types — it stores, migrates,
and forwards them verbatim. New entity types and attributes require no protobuf
recompilation and no core changes.

## What Is Not an Extension Point

- The simulation loop, AOI index, zone ownership, and Space isolation are core
  responsibilities and are not extensible by modules.
- There is intentionally no plugin/registry abstraction. Capabilities are added
  by implementing the interfaces above and wiring them at process start in the
  service binary. This keeps the core simple and the dependency graph explicit.

## Consequences

- The runtime core contains no business logic and no concrete gameplay.
- New autonomous entity behavior = implement `Lifecycle` (notably `OnSimulate`).
- New outbound consumers = subscribe to `Event`s.
- New entity data = opaque `Attrs`; new entity types = string `Type`.
- The core's public surface is stable; future capabilities are additive.

## References

- [ADR-013: Platform Boundary](013-platform-boundary.md)
- [ADR-023: Entity Model Design](023-entity-model.md)
- [ADR-025: Platform Terminology Model](025-platform-terminology-model.md)
- `internal/game/event.go`, `internal/game/entity/entity.go`,
  `internal/game/simulation.go`
