# Runtime Invariants

> **Last Updated:** 2026-06-28  |  **v0.1.0-alpha**

## Purpose

Document every invariant the spatial runtime guarantees. These are part of the
platform contract: any change (refactor, feature, module) must preserve every
invariant listed here. Future versions may add invariants but never weaken them.

## Entity invariants

- I-01 **Uniqueness** — Every `EntityID` is globally unique (UUIDv7). No two
  entities anywhere in the cluster have the same ID.
- I-02 **Single authority** — At any instant, an entity exists on exactly one
  Runtime Node. The node that owns the entity's current zone is authoritative
  for its position, state, and lifecycle.
- I-03 **Space membership** — An entity is bound to exactly one Space. Its
  `RuntimeID` (Space identifier) is derived from its zone; entities from
  different Spaces share no state and never interact.
- I-04 **No lost entities** — An entity added to the system is either present
  on a node or was explicitly removed. No operation silently drops entity
  creation.

## Ownership invariants

- O-01 **Single zone owner** — Every zone is owned by at most one Runtime Node
  at any instant. The ownership table is the tiebreaker (PostgreSQL unique
  constraint on `zone_id`).
- O-02 **Authoritative control** — Only the owning node may mutate an entity's
  state, publish events for it, or transfer it. Client position updates for an
  unknown entity are silently discarded.
- O-03 **Ownership transfer atomic** — An entity moving across a zone boundary
  is removed from the source node and added to the target node. During the
  transfer window, it must not exist on both nodes (ghost entries are read-only
  replicas, not authoritative copies).

## AOI / ghost invariants

- G-01 **Ghost read-only** — A ghost entry is never authoritative; it is a
  cached position used for AOI fan-out. Ghosts are refreshed via the neighbor
  querier (`ReconcileNeighborGhosts`) and expire after a bounded TTL (6×
  configured ghost TTL).
- G-02 **Ghost bounded** — At steady state, the total ghost count across all
  nodes is bounded by the product of cross-zone entities and the number of
  adjacent zones. Ghosts never grow without limit.
- G-03 **Ghost expiry** — When a source entity is removed, all remote ghosts
  of it expire after their TTL and are swept. No entity removal leaves a
  permanent ghost.

## Space isolation invariants

- S-01 **No cross-Space visibility** — An entity in Space A never appears in
  the AOI query of an entity in Space B, regardless of coordinate overlap.
- S-02 **No cross-Space state sharing** — Queues, indexes, and publish streams
  are scoped implicitly by zone, and a zone belongs to exactly one Space, so
  cross-Space data sharing is structurally impossible.

## Scheduler invariants

- T-01 **Bounded tick** — Each tick drains at most `maxPacketsPerTick` (1024)
  inbound packets. A packet burst cannot cause a cascade overload or
  starvation.
- T-02 **Measured delta** — `simulate()` receives the wall-clock elapsed time
  between ticks (capped at `maxTickDelta` = 250ms). Movement is never
  framerate-dependent.
- T-03 **Non-blocking publish** — `publish(Event)` is a non-blocking channel
  send; a full channel is a drop (counted and exported). The tick loop never
  blocks on event delivery.
- T-04 **Control command protection** — Entity add/remove commands block
  briefly (100ms) for queue space before being dropped loudly (logged +
  counted). Control commands are never silently lost.

## Consistency model

The runtime provides **eventual consistency with bounded staleness** for
observer-visible state:

- An entity move published by node A is visible to AOI observers on node B
  after at most one ghost-reconcile cycle + network RTT.
- Zone ownership changes propagate via the room-service ownership table;
  a stale routing cache entry on a Gateway expires after the configured TTL
  (5s default).

The runtime does not provide linearizability or causal ordering guarantees.
Observers on the same node see state in publication order; observers on
different nodes may see transient gaps during ghost reconciliation.

## References

- [ADR-001: Zone Ownership](../adr/001-zone-ownership.md)
- [ADR-002: Zone Migration](../adr/002-zone-migration.md)
- [ADR-003: AOI Strategy](../adr/003-aoi-strategy.md)
- [ADR-013: Platform Boundary](../adr/013-platform-boundary.md)
- [Distributed correctness tests](../../internal/game/distributed_test.go)
