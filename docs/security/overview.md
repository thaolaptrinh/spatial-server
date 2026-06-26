# Security Overview

> **Last Updated:** 2026-06-26

## Purpose

Document the security model and mechanisms for Spatial Server. Spatial Server is a realtime communication platform that proxies WebSocket connections from game clients to Game Servers. It does not manage users, issue tokens, or store business data. Security is enforced at the Gateway edge, with trust boundaries clearly defined between external clients, internal services, and the Business Backend.

## Trust Boundaries

| Boundary | Trusted | Untrusted |
|----------|---------|-----------|
| **External → Spatial Server** | Nothing at the wire level | All client connections, all packets, all tokens until validated |
| **Internal services** | Services within private network after authentication | Any service outside private CIDR |
| **Business Backend** | JWT public key (for signature validation), lifecycle API calls | Business Backend network itself (separate trust domain; API calls validated via gRPC mTLS when configured) |

### External → Spatial Server Boundary

- The Gateway is the only public-facing service.
- No packet from a client is trusted until it passes: TLS termination → JWT validation → rate limiting → sequence number check → protobuf schema validation.
- The Gateway does not trust any client claim — all claims are verified cryptographically.

### Internal Service Trust Model

- Services within the private network share a trust domain.
- Inter-service gRPC calls are authenticated by network origin (private CIDR).
- No internal service exposes a port to the public internet except: Gateway WebSocket (:443), Gateway management (:8080), Room Service management (:8080).
- In future phases, mTLS will provide per-call authentication between services.

### Business Backend Trust Model

- The Business Backend is a separate trust domain managed by a different team/service.
- Spatial Server trusts the Business Backend's JWT public key and its API calls to `CreateRuntime` / `DestroyRuntime`.
- Spatial Server never calls the Business Backend — it only accepts incoming API calls and validates client JWTs.

## Authentication

### JWT Validation Flow

1. Client authenticates with Business Backend (outside Spatial Server scope).
2. Business Backend issues a JWT containing `player_id`, `runtime_id`, `exp`.
3. Client presents JWT as a query parameter when establishing WebSocket connection: `wss://gateway/v1/ws?token=<jwt>`.
4. Gateway extracts and validates the JWT before accepting the connection:
   - Decode token header to identify the key ID (`kid`).
   - Fetch the corresponding public key from the local cache (populated from Business Backend's JWKS endpoint).
   - Verify the RSA/ECDSA signature.
   - Validate `exp` is in the future (with 30s leeway for clock skew).
   - Validate `runtime_id` is present and non-empty.
   - Validate `player_id` is present and non-empty.
5. On validation failure: connection rejected with a generic error message (no details leaked).
6. On success: connection upgraded to WebSocket, JWT claims cached for the session lifetime.

### Public Key Distribution and Rotation

- Business Backend exposes a JWKS endpoint.
- Gateway polls the JWKS endpoint every 5 minutes (configurable).
- Keys are cached in memory with the `kid` as the lookup key.
- Business Backend may advertise multiple keys simultaneously (overlap period for rotation).
- On validation failure with an unknown `kid`, Gateway re-fetches JWKS immediately (cache miss fallback).

### Token Refresh

- JWT access tokens are short-lived (15 minutes by default, configured by Business Backend).
- Spatial Server does not implement token refresh — the client must obtain a new JWT from the Business Backend.
- If a JWT expires mid-session, the Gateway terminates the connection gracefully.

### Rate Limiting for Auth Attempts

- Failed JWT validation attempts are rate-limited per IP: 10 failures/minute.
- Beyond the limit, the IP is blocked for 60 seconds.
- This prevents brute-force attacks against the WebSocket endpoint.

## Transport Security

| Layer | Mechanism | Detail |
|-------|-----------|--------|
| **External** | TLS 1.3 — WebSocket over WSS | Certificate from Let's Encrypt via cert-manager. Only TLS 1.3 accepted. TLS 1.2 disabled. |
| **Internal** | TLS (optional) — gRPC over mTLS | Not required for MVP. When enabled: mutual TLS with service certificates issued by internal CA. |

### External TLS 1.3

- All client traffic uses TLS 1.3 with a minimum set of cipher suites: TLS_AES_128_GCM_SHA256, TLS_AES_256_GCM_SHA384, TLS_CHACHA20_POLY1305_SHA256.
- Certificates managed by cert-manager with Let's Encrypt issuer.
- Auto-renewal: cert-manager handles renewal 30 days before expiry.
- HSTS header: `Strict-Transport-Security: max-age=31536000; includeSubDomains` on all HTTP responses.

### Internal mTLS

- Not enabled for MVP to reduce operational complexity.
- When enabled: each service gets a certificate from the internal CA (cert-manager with self-signed issuer).
- Service-to-service connections verify peer certificates against the CA root.
- gRPC interceptors enforce mTLS at the transport level.

## Authorization

**Spatial Server does not implement its own authorization.** Authorization is entirely delegated to the Business Backend via JWT claims.

- The JWT contains `runtime_id` which determines which runtime (room/showroom/meeting) the player can access.
- Zone-level access control: the player can only connect to zones assigned to their runtime. The Gateway validates this by looking up the zone assignment before proxying.
- There is no concept of roles, permissions, or ACLs within Spatial Server.
- The Business Backend is responsible for ensuring players only receive tokens for runtimes they are authorized to access.

## Replay Protection

- Each client packet includes a monotonic sequence number (uint32) in the packet header.
- Gateway tracks the highest sequence number per connection in memory.
- A packet is rejected if its sequence number is ≤ the last processed sequence number.
- **30-second window enforcement**: if a packet's sequence number is more than 30 seconds old (based on server arrival time), it is rejected even if it's higher than the last processed number.
- **Uint32 wrap handling**: on wrap (sequence number resets to 0 after 2^32-1), the Gateway resets the per-connection sequence tracker. Wrap is expected to be rare given the 100 msg/s rate (wrap occurs after ~497 days of continuous connection).
- Rejected packets are silently dropped — no error response sent to avoid oracle attacks.

## Packet Validation

- All client messages are validated against protobuf schema definitions before any processing occurs.
- **Maximum packet size enforcement**: packets exceeding 64 KB (configurable) are rejected at the transport level before deserialization.
- **Malformed packet handling**: packets that fail protobuf deserialization are silently dropped. No error response is sent to the client to prevent oracle attacks that could be used to probe the schema.
- **Unknown field handling**: protobuf fields unknown to the schema are silently ignored (standard protobuf behavior).
- Validation occurs at the Gateway before forwarding to the Game Server.

## Rate Limiting

| Limit | Value | Scope | Enforcement |
|-------|-------|-------|-------------|
| Per-connection throughput | 100 msg/s | Individual WebSocket connection | Token bucket at Gateway |
| Per-IP throughput | 500 msg/s | All connections from same source IP | Shared token bucket at Gateway |
| Auth attempts | 10 failures/min | Per source IP | Separate token bucket |
| Admin API | 100 req/s | Per source IP | HTTP middleware |

### Rate Limit Enforcement Flow

1. **First violation**: packet silently dropped, Gateway logs the event internally.
2. **Second violation**: packet dropped, warning message sent to client (`rate_limit_warning`).
3. **Third violation (within 60s window)**: connection terminated with `rate_limit_exceeded` reason.

All limits are configurable via dynamic config and can be updated without restarting the Gateway.

## Secrets Management

- Secrets are **never** stored in source code or committed to version control.
- **Production**: Kubernetes Secrets, encrypted at rest via etcd encryption.
- **Development**: environment variables loaded from `.env` files (`.env` in `.gitignore`).

### Secrets Inventory

| Secret | Storage | Rotation |
|--------|---------|----------|
| JWT public key (Business Backend) | JWKS cache + config | Via Business Backend JWKS rotation |
| Database credentials (PostgreSQL) | K8s Secret / env var | Manual rotation via `ALTER USER ... WITH PASSWORD` |
| Redis password | K8s Secret / env var | Manual rotation |
| TLS private keys (Let's Encrypt) | cert-manager managed | Auto-renewal every 60 days |
| Admin API key | K8s Secret / env var | Manual rotation |

### Rotation Procedures

- **JWT public key**: Business Backend rotates by advertising a new key in JWKS with a new `kid`. Old key remains valid during overlap period. Gateway picks up new keys via periodic polling.
- **DB credentials**: update the Secret, restart affected pods. Use connection draining to avoid drops.
- **TLS certificates**: fully automated via cert-manager.

## Threat Model

| Threat | Vector | Impact | Mitigation | Severity |
|--------|--------|--------|------------|----------|
| Unauthorized WebSocket connection | Attacker presents forged/stolen JWT | Adversary joins runtime, intercepts realtime data | JWT signature validation using Business Backend public key; token contains `runtime_id` binding | Critical |
| JWT theft (interception) | Attacker captures token from network traffic | Adversary impersonates user for token lifetime | TLS 1.3 encrypts all traffic; short-lived tokens (15 min) limit exposure window | High |
| Replay attack | Attacker captures and re-sends a valid packet | Duplicate state mutations or desynchronized game state | Monotonic sequence number per connection; Gateway rejects sequence ≤ last processed; 30s time window | High |
| DoS via connection flood | Attacker opens thousands of WebSocket connections | Resource exhaustion on Gateway, denial of service for legitimate users | Per-IP rate limiting (500 msg/s); connection limits per Gateway instance (10,000); failsafe: connection rate limiting | High |
| DoS via large packets | Attacker sends oversized packets | Memory exhaustion on Gateway | 64 KB maximum packet size enforced before deserialization | Medium |
| DoS via malformed packets | Attacker sends invalid protobuf data | CPU wasted on deserialization | Silent drop on validation failure; rate limiting limits throughput | Medium |
| Internal eavesdropping | Attacker with private network access intercepts gRPC traffic | Exposure of realtime game state | Private network segmentation; optional mTLS for future phases | Medium |
| Compromised Game Server | Attacker gains control of a Game Server pod | Unauthorized access to zone state, potential manipulation of game logic | Zone isolation (each server owns specific zones); stateless Gateway (no credentials stored); database read-only access for Game Servers | High |
| JWT replay across runtimes | Attacker uses a valid JWT from runtime A to access runtime B | Unauthorized cross-runtime access | Gateway validates `runtime_id` in JWT matches the runtime the client is trying to access | Critical |
| Public key substitution | Attacker tricks Gateway into using attacker-controlled public key | Gateway accepts forged JWTs | JWKS endpoint URL is configured, not discovered; TLS protects JWKS fetch; key pinning optional | Critical |
| Brute-force JWT secret | Attacker attempts to guess the JWT signing key | Forge valid tokens | Rate limiting on auth failures (10 failures/min/IP); RSA/ECDSA key strength ≥ 2048 bits | Low |
| Side-channel via error messages | Attacker probes schema by observing error responses | Information leakage about packet structure | Malformed packets silently dropped (no error response); auth failures return generic error | Low |
| Connection hijacking via IP spoofing | Attacker spoofs source IP to bypass per-IP rate limits | Bypass rate limiting, amplify DoS | Token bucket per connection still applies; TCP handshake makes IP spoofing impractical for sustained attacks | Low |

## Operational Security

### SSH Access Policy

- SSH access to production servers is for debugging only.
- Key-based authentication only (password auth disabled).
- All SSH sessions are logged and audited.
- Access is revoked after the debugging session completes.
- Bastion/jump host required for access to private network servers.

### Principle of Least Privilege

- **Service accounts**: each service has its own database credentials with minimal permissions (e.g., Game Server has read-only access to zone data, no DDL permissions).
- **Network access**: services can only bind to ports they need, on network planes they need.
- **Kubernetes RBAC**: service accounts have minimal permissions (no cluster-level access unless required).

### Database Access

- Game Servers connect via read-only replicas where possible.
- Connection pooling (PgBouncer) limits the number of concurrent database connections.
- Database credentials are scoped per service.

### Audit Logging

- All Runtime lifecycle operations (CreateRuntime, DestroyRuntime) are logged with `player_id` and `runtime_id`.
- Gateway connection events (connect, disconnect, rate limit termination) are logged with source IP and `player_id`.
- No game state or packet payloads are included in audit logs.

## Compliance Considerations

- **No PII/PCI data flows through Spatial Server.** All personally identifiable information and payment data are handled by the Business Backend.
- Spatial Server handles only: player IDs (opaque identifiers), runtime IDs, and realtime game state (position, attributes).
- Audit trail for all Runtime lifecycle operations is retained for a minimum of 90 days.
- Log access is restricted to operations team (read-only) and security team (full access).
- Log retention: 30 days in Loki (hot), 12 months in S3/GCS (cold archive).

## References

- [ADR-018](../adr/018-security.md) — Security Architecture
- [ADR-012](../adr/012-networking.md) — Networking
- [ADR-016](../adr/016-runtime-lifecycle.md) — Runtime Lifecycle
