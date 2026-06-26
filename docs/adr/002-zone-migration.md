# ADR 002: Zone Migration

## Status

Approved

## Context

When load balancing requires moving a zone from one Game Server to another, or when a Game Server crashes, the zone's entity state must be transferred to the new owner.

## Problem

Zone migration is a critical operation that must be atomic, consistent, and fast. Poorly handled migrations can cause entity state loss, duplicate entities, or prolonged gameplay interruptions.

## Decision

- Zone migration is initiated by Room Service (not Game Servers).
- Migration uses direct gRPC (source → target), not routed through Room Service.
- Migration sequence:
  1. Room Service sets zone status to `TRANSFERRING` (reject new entity writes).
  2. Room Service notifies source Game Server: "transfer zone Z to target T".
  3. Source serializes zone state (entities, positions, attributes, in-memory AOI index).
  4. Source sends serialized state to target via direct gRPC (`ZoneStateSync` streaming RPC).
  5. Target loads state and begins simulation.
  6. Target confirms ownership → Room Service updates: `status = ACTIVE, server_id = target`.
  7. Room Service pushes routing update to Gateways.
  8. Source releases zone resources.

## Alternatives

1. **Room Service relayed**: Route all zone state through Room Service. Simpler topology but adds a network hop and bottlenecks Room Service.
2. **Shared storage (Redis/PostgreSQL)**: Source writes state to shared storage, target reads it. Avoids direct P2P but requires serialization to DB-optimized format.
3. **Gradual entity migration**: Migrate entities one by one instead of full zone transfer. Smoother but slower and more complex to coordinate.

## Tradeoffs

- Direct P2P transfer is fastest (single network hop) but requires mutual reachability between all Game Servers.
- Streaming transfer supports arbitrarily large zones but adds streaming complexity.
- Zone pause during migration creates a brief gameplay interruption proportional to serialization + RTT.

## Consequences

- Direct transfer is fast (single network hop).
- Zone is briefly paused during transfer (serialization + RTT).
- Large zones (many entities) may require streaming transfer.
- Zone state includes in-memory AOI index (must be serializable).

## Future Considerations

- Incremental sync (pre-copy + delta) to reduce pause time.
- Parallel zone migrations across multiple servers.
- Migration priority queue based on zone size or player count.

## Replaces

None.
