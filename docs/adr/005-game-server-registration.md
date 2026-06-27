# ADR 005: Game Server Registration

## Status

Approved

## Context

Game Servers must announce themselves to the cluster so Room Service can assign zones and Gateway can route clients.

## Problem

Game Servers must discover and register with the cluster reliably. Without a robust registration mechanism, the cluster cannot route clients or assign zones correctly, and dead servers may remain in the pool.

## Decision

- Game Server resolves Room Service via DNS (static or K3s Service) on startup.
- Game Server sends `Register(serverID, address, metadata)` gRPC to Room Service.
- Room Service stores registration in memory + PostgreSQL (`game_servers` table).
- Registration fields: server_id, host:port, capacity (max zones), load metrics, joined_at.
- Heartbeat every 5s to keep registration alive.
- Room Service removes unregistered servers after 3 missed heartbeats (15s timeout).

## Alternatives

1. **etcd/Consul service discovery**: Use an external service registry. More dependencies, higher operational complexity, and another system to maintain.
2. **K3s-native (Endpoints API)**: Rely on Kubernetes endpoints for discovery. Ties the design to K3s, not portable to Docker Compose or local dev.
3. **mDNS/DNS-SD**: Zero-configuration discovery. Unreliable in container orchestration environments.

## Tradeoffs

- DNS bootstrap + gRPC registration is simple with no external dependencies but requires Room Service to be reachable via DNS at a known address.
- Heartbeat-based liveness introduces a 15s failure detection window.
- Room Service restart forces all Game Servers to re-register — acceptable for dev but requires HA in production.

## Consequences

- No external service discovery dependency (no etcd/Consul).
- Simple: DNS bootstrap + gRPC registration.
- Room Service restarts require all Game Servers to re-register (acceptable for dev; HA mitigates in production).

## Future Considerations

- Room Service cluster with shared registration state in PostgreSQL.
- Backoff and jitter for re-registration storms after Room Service restart.
- Readiness probes for K3s to prevent routing to unregistered servers.

## Replaces

None.
