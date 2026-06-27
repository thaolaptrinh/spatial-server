# ADR 004: Coordinator (Room Service)

## Status

Approved

## Context

Room Service is the central coordinator for the spatial server cluster. It manages zone ownership, Game Server registration, and routing hints. Its design determines whether it becomes a bottleneck as the cluster scales.

## Problem

A central coordinator can easily become a bottleneck and single point of failure. The design must balance coordination capability with scalability and fault tolerance.

## Decision

- Room Service is a **lightweight metadata coordinator** — not a router.
- Responsibilities:
  - Zone → Game Server ownership table
  - Load-based zone reassignment decisions
  - Game Server registration and health tracking
  - Gateway routing hints (which Game Server owns which zone)
- Room Service does NOT handle runtime data forwarding.
- Cross-zone RPCs go directly Game Server → Game Server (not through Room Service).
- HA: single instance in dev, K3s Lease API for leader election in production.

## Alternatives

1. **Full proxy/router (GoWorld-style)**: All inter-service traffic routes through the coordinator. Simple consistency model but creates a bottleneck and single point of failure.
2. **Decentralized (gossip protocol)**: Servers discover each other and share state via gossip. Highly scalable but complex convergence and debugging.
3. **Stateless coordinator + distributed store**: Coordinator handles only metadata queries; all data flows peer-to-peer. Chosen approach.

## Tradeoffs

- Lightweight coordinator is not a bottleneck but adds complexity for cross-server operations.
- No runtime data through Room Service improves scalability but requires Game Servers to communicate directly.
- K3s Lease API for leader election ties HA strategy to Kubernetes (K3s).

## Consequences

- Room Service is NOT a bottleneck — metadata operations are cheap.
- Adding more Game Servers does not pressure Room Service.
- Losing Room Service does not break existing gameplay (Gateway cache + direct P2P continue).
- Only zone lookups, heartbeats, and ownership changes are affected during Room Service downtime.

## Future Considerations

- Room Service cluster with partitioning by runtime ID.
- Caching layer for ownership lookups to reduce PostgreSQL load.
- Admin API for manual zone reassignment and cluster observability.

## Replaces

Central coordinator approach (GoWorld-style dispatcher/router).
