# ADR 003: AOI Strategy

## Status

Approved

## Context

Players in a spatial runtime need to know which other entities are within visual range. The Area of Interest (AOI) system tracks entity positions and determines which entities should receive updates about each other.

## Problem

AOI queries must be answered with minimal latency for smooth gameplay. Using external storage for realtime spatial queries introduces unnecessary network hops and latency, degrading the player experience.

## Decision

- AOI index is **in-memory** on the Game Server — not in Redis.
- Realtime state never passes through Redis. Redis only stores metadata.
- Grid-based: zone size = 100x100 units (configurable).
- Interest radius = 300 units (configurable, typically 3x3 grid cells).
- AOI query calculates entities within interest radius from in-memory spatial index.
- Cross-zone AOI: Game Server subscribes to adjacent zones' owners for entity enter/leave events.
- Ghost entity timeout = 500ms for smooth zone boundary transitions.

## Alternatives

1. **Redis Sorted Sets + Geohash**: Store entity positions in Redis geospatial index. Simple to implement but adds network latency to every AOI query.
2. **PostGIS**: Use PostgreSQL for spatial queries. Too slow for realtime game loops.
3. **Quadtree/R-tree in-memory**: Alternative spatial indexing approach. More complex than grid-based but handles uneven entity distribution better.

## Tradeoffs

- In-memory grid provides lowest possible latency but must be fully serialized during zone transfer.
- Memory cost scales with entity count per zone — dense zones consume significant RAM.
- Cross-zone AOI requires inter-server subscriptions, adding complexity over a single-server approach.

## Consequences

- Lowest possible latency for AOI queries (no network hop).
- Zone transfer must serialize and transfer the full in-memory AOI state.
- Redis is not a bottleneck since AOI bypasses it entirely.
- Memory usage scales with entity count per zone.

## Future Considerations

- Hybrid approach: in-memory for hot entities, database for cold/out-of-range.
- Dynamic grid cell size based on entity density.
- LOD-based AOI where distant entities receive less frequent updates.

## Replaces

Initial design used Redis Sorted Sets for AOI index.
