# ADR 021: Gateway Architecture

## Status

Accepted

## Context

The Gateway is the entry point for all client connections to Spatial Server. Every WebSocket connection, every packet, and every authentication attempt flows through it. Its architecture directly affects connection handling capacity, auth security, rate limiting effectiveness, and routing correctness. The design must be stateless for horizontal scalability, must never block on backpressure, and must handle up to 10,000 concurrent connections per instance.

Existing ADRs define what the Gateway must do (validate JWTs per ADR-018, enforce rate limits per ADR-018, proxy to Game Servers per ADR-016) but do not specify the Gateway's internal architecture — how it terminates WebSocket connections, how it validates auth before processing packets, how it routes packets to the correct Game Server, and how it remains stateless.

## Problem

Without a defined Gateway architecture, each implementation may make different choices about WebSocket library, auth enforcement timing, routing strategy, and scaling model. This leads to inconsistent behavior, security gaps (e.g., processing packets before validating auth), and operational complexity (e.g., requiring session affinity for routing).

## Decision

### WebSocket Termination with nhooyr.io/websocket

- Use [nhooyr.io/websocket](https://nhooyr.io/websocket) for WebSocket termination.
- Rationale: idiomatic Go API, context-based cancellation, proper ping/pong handling, zero allocations in the hot path, and no global state.
- Read/write timeouts: 30s for control messages, configurable per connection.
- Ping interval: 30s. Connection dropped if pong not received within 10s of ping.

### JWT Validation Before Packet Processing

- JWT validation is the first operation on the connection after TLS termination.
- The connection is **not** upgraded to WebSocket until JWT validation succeeds.
- Validation order: (1) extract `token` query parameter, (2) decode and verify JWT signature, (3) validate claims (`exp`, `runtime_id`, `player_id`), (4) look up zone assignment, (5) upgrade to WebSocket.
- If validation fails at any step, the HTTP upgrade response is 401 Unauthorized with a generic message.
- No packet processing occurs before validation — not even rate limiting token deduction (rate limiting applies to authenticated sessions only; auth failures have their own rate limiter).

### Rate Limiting at the Gateway Edge

- Rate limiting is enforced at the Gateway before packets are forwarded downstream.
- Two token buckets per connection: one for auth attempts (10 failures/min), one for message throughput (100 msg/s).
- One shared token bucket per source IP (500 msg/s).
- Token bucket parameters are read from dynamic config on every refill — no restart required to change limits.
- Rate limit violation handling follows ADR-018: drop → warn → disconnect after 3 violations.

### Packet Forwarding to Game Server via gRPC

- Gateway is a transparent proxy for game data — it does not inspect, modify, or interpret packet payloads.
- After JWT validation and rate limiting, each packet is forwarded to the Game Server via a bidirectional gRPC stream.
- Packet forwarding uses a single gRPC stream per connection (not per-packet RPC) to reduce overhead.
- Gateway reads from the WebSocket and writes to the gRPC stream, and vice versa, in a read-loop / write-loop pattern.
- Backpressure: if the gRPC stream's send blocks, the Gateway applies a 5s write deadline. If exceeded, the connection is terminated (the Game Server is presumed slow or dead).

### Zone → GameServer Routing Cache

- Gateway caches the mapping from zone ID to Game Server address with a 5-second TTL.
- On cache miss: Gateway queries Room Service via gRPC.
- On cache hit: Gateway routes the packet directly to the cached Game Server.
- **Push invalidation**: Room Service sends a gRPC stream notification to all Gateways when zone ownership changes (e.g., zone migration, Game Server failure). This invalidates the relevant cache entries immediately, bypassing the TTL.
- Cache is per-Gateway instance (in-memory), not shared. This is acceptable because zone ownership changes are infrequent and push invalidation handles consistency.

### Health Check Endpoints

| Endpoint | Purpose | Expected Response |
|----------|---------|-------------------|
| `/health` | Overall health (dependencies reachable) | 200 OK if Redis, Room Service gRPC are reachable |
| `/ready` | Readiness (accepting traffic) | 200 OK if health passes and connection count < 10,000 |
| `/live` | Liveness (process is running) | 200 OK always (kubelet probe) |

- Health check interval: 15s for `/ready`, 30s for `/live`.
- `/ready` fails when connection count exceeds 9,000 (soft limit) to allow draining before hitting the hard limit.

### Connection Limits per Gateway Instance

- Hard limit: 10,000 concurrent connections per Gateway instance.
- Soft limit: 9,000 concurrent connections — at this point `/ready` returns 503, signaling the load balancer to stop sending new connections.
- Connection count is tracked in-memory via an atomic counter.
- When the hard limit is reached, new WebSocket upgrade requests receive 503 Service Unavailable.

### Fully Stateless (No Session Affinity)

- Gateway stores no session state between packets — JWT claims are cached per-connection in memory but are lost on Gateway restart (clients reconnect).
- No session affinity required at the load balancer level — any Gateway can handle any client.
- This enables horizontal scaling: add or remove Gateway instances behind a round-robin or least-connections load balancer without coordination.

## Alternatives

1. **HAProxy/Nginx for WebSocket termination**: Proven, high-performance WebSocket termination. However, custom auth logic (JWT validation, rate limiting, zone routing) would require Lua scripting (HAProxy) or embedded modules (Nginx), which are harder to test, maintain, and debug compared to Go code. Also adds a hop (LB → Gateway) with no clear benefit since Gateway already terminates TLS and WebSocket.

2. **Custom TCP protocol instead of WebSocket**: Eliminates WebSocket framing overhead and enables a binary-optimized protocol. Rejected because WebSocket is universally supported by browsers, game engines (Unity, Unreal), and mobile SDKs. A custom protocol would require client SDK development for every platform.

3. **gorilla/websocket**: Previously the standard Go WebSocket library, but it is no longer maintained (archived). nhooyr.io/websocket is the community successor with a more idiomatic Go API and better performance characteristics.

4. **Shared routing cache (Redis)**: Using Redis for the zone → GameServer cache instead of per-instance in-memory cache. Rejected because cache invalidation on zone change would require a pub/sub channel anyway, and the per-instance cache with push invalidation achieves the same consistency with lower latency and no Redis dependency on the hot path.

## Tradeoffs

- Per-instance routing cache with push invalidation is simple and fast but means a brief inconsistency window (up to 5s TTL) if the push notification is lost. Acceptable because zone ownership changes are rare and clients reconnect on routing errors.
- nhooyr.io/websocket has a smaller community than gorilla/websocket but is actively maintained and used in production by several projects.
- Stateless design means clients must reconnect on Gateway failure. This is acceptable because reconnection is handled by the client SDK (see ADR-022 for session resumption).
- gRPC stream per connection adds memory overhead (~10 KB per stream) but avoids per-packet RPC overhead.

## Consequences

- Gateway can scale horizontally to handle arbitrary connection loads.
- Security is enforced at the single entry point — no downstream service needs to validate auth.
- Rate limiting at the edge protects downstream Game Servers from overload.
- Zone routing is fast (in-memory cache for common case) and consistent (push invalidation for changes).
- No session affinity means load balancer configuration is trivial.
- Gateway is a transparent proxy: Game Servers receive packets exactly as the client sent them (minus the sequence number header stripped by Gateway).

## Future Considerations

- WebSocket compression (permessage-deflate) for bandwidth reduction on slow connections.
- gRPC WebSocket proxy optimization: direct byte forwarding without deserialization/re-serialization.
- Connection migration: if a Gateway instance is being drained (e.g., rolling update), existing connections could be migrated to another Gateway without client reconnection.
- Layer 7 load balancer integration: offload TLS termination and WebSocket protocol handling to a reverse proxy (Envoy, HAProxy) if Gateway CPU becomes a bottleneck.

## Replaces

- Initial design had the Gateway embedded in each Game Server (client → Game Server directly). Centralizing the Gateway provides consistent auth, rate limiting, and routing across all Game Servers.
