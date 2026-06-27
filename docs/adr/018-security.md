# ADR 018: Security

## Status

Accepted

## Context

Spatial Server handles realtime communication between clients. Security must protect against unauthorized access, data tampering, replay attacks, and resource exhaustion — without adding unnecessary latency to the realtime path.

## Problem

A realtime platform exposed directly to clients must defend against unauthorized access, data tampering, replay attacks, and resource exhaustion. Poorly chosen security primitives (e.g., per-packet signatures, token issuance inside the infrastructure) add latency to the hot path or couple the platform to business logic.

## Decision

### Authentication

- JWT tokens are **issued by Business Backend**, never by Spatial Server.
- Gateway validates JWT signature using Business Backend's public key (configurable).
- Token contents: `player_id`, `runtime_id`, `exp` (expiration timestamp).
- Tokens are short-lived (15 minutes access, 24 hours refresh — managed by Business Backend).
- Gateway rejects tokens with invalid signature, expired timestamp, or missing runtime_id.

### Transport Layer Security (TLS)

- External (Client → Gateway): TLS 1.3 required. WebSocket over WSS.
- Certificate management: Let's Encrypt via cert-manager (K3s) or LB-level termination.
- Internal (Service ↔ Service): TLS optional for MVP. mTLS considered for future phases.

### Rate Limiting

- Per connection: 100 messages/second (token bucket).
- Per IP: 500 messages/second (shared token bucket across connections from same IP).
- Rate limit violation: packet dropped, warning sent, connection terminated after 3 violations.
- All limits configurable via dynamic config.

### Replay Protection

- Each client packet includes a monotonic sequence number (uint32).
- Gateway tracks the highest sequence number per connection.
- Packets with sequence ≤ last processed are rejected.
- Sequence number wraps at max uint32 (acceptable for per-session use).

### Packet Validation

- All client messages validated against protobuf schema before processing.
- Malformed packets: silently dropped. No error response to avoid oracle attacks.
- Maximum packet size: 64 KB (configurable). Larger packets rejected.

### Internal Security

- Private network: all inter-service gRPC traffic stays within private network.
- No internal service exposed to public internet (except Gateway WebSocket and Room Service management port).
- Game Servers bind to private IPs only.
- Room Service gRPC port: private network only.
- PostgreSQL and Redis: bind to database network only (not accessible from public or private networks).

### Secrets Management

- Secrets never stored in source code.
- Kubernetes Secrets for production.
- Environment variables for development.
- Secrets include: JWT public key, DB credentials, Redis password, TLS private keys.

### Compliance (Future)

- No credit card or PII data flows through Spatial Server (those are in Business Backend).
- Audit logging: all Runtime lifecycle operations logged with player_id and runtime_id.

## Alternatives

1. **Spatial Server issues JWTs**: Have the infrastructure mint and manage auth tokens. Rejected because it couples the platform to business identity and forces Spatial Server to manage users (per [ADR-013](013-platform-boundary.md)).
2. **Per-message MAC/signature**: Cryptographically sign every packet. Rejected because the per-packet verification cost is too high for a 20Hz realtime path; sequence-number replay protection plus TLS is sufficient.
3. **mTLS on all internal traffic from day one**: Enforce mutual TLS between services immediately. Rejected for MVP because the deployment and certificate-management complexity outweighs the benefit on a private network; deferred to a future phase.

## Tradeoffs

- Stateless JWT validation is fast and horizontally scalable, but a compromised or revoked token remains valid until its `exp` (session revocation is addressed by [ADR-022](022-session-management.md)).
- Edge rate limiting and replay protection add O(1) overhead per packet and 4 bytes for the sequence number, an acceptable cost for protection of downstream Game Servers.

## Consequences

- Security model is simple: validate at Gateway edge, trust internal network.
- Session management is deferred — [ADR-022](022-session-management.md) defines Redis-backed session tokens with 30s reconnection. Until [ADR-022](022-session-management.md) is implemented, JWT validation is stateless with no session resumption.
- Rate limiting adds minimal overhead (token bucket is O(1)).
- Replay protection adds 4 bytes per packet (sequence number).
- mTLS for internal communication is deferred to maintain deployment simplicity.

## Future Considerations

- mTLS for all internal (service ↔ service) gRPC traffic.
- Token revocation and shorter session windows via [ADR-022](022-session-management.md) session tokens.
- Per-player resource budgets beyond the shared rate limits.

## Replaces

- Initial design had Spatial Server issuing JWT tokens (now delegated to Business Backend).
