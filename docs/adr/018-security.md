# ADR 018: Security

## Status

Accepted

## Context

Spatial Server handles realtime communication between clients. Security must protect against unauthorized access, data tampering, replay attacks, and resource exhaustion — without adding unnecessary latency to the realtime path.

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

## Consequences

- Security model is simple: validate at Gateway edge, trust internal network.
- No session management in Spatial Server (JWT stateless validation).
- Rate limiting adds minimal overhead (token bucket is O(1)).
- Replay protection adds 4 bytes per packet (sequence number).
- mTLS for internal communication is deferred to maintain deployment simplicity.

## Replaces

- Initial design had Spatial Server issuing JWT tokens (now delegated to Business Backend).
