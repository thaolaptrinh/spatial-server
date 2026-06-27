# Security Overview

> **Last Updated:** 2026-06-27

## Purpose

Document the security model and mechanisms for Spatial Server. Spatial Server is a realtime communication platform that proxies WebSocket connections from game clients to Game Servers. It does not manage users, issue tokens, or store business data. Security is enforced at the Gateway edge, with trust boundaries clearly defined between external clients, internal services, and the Business Backend.

> **Note on phasing:** Many security controls below are not yet implemented. Each section states
> its **current implementation status** explicitly; items marked **Planned (Phase X)** describe the
> target architecture and have no corresponding code yet.

## Trust Boundaries

| Boundary | Trusted | Untrusted |
|----------|---------|-----------|
| **External → Spatial Server** | Nothing at the wire level | All client connections, all packets, all tokens until validated |
| **Internal services** | Services within private network after authentication | Any service outside private CIDR |
| **Business Backend** | JWT signing secret (HMAC, shared with Gateway for signature validation), lifecycle API calls | Business Backend network itself (separate trust domain; API calls validated via gRPC mTLS when configured) |

### External → Spatial Server Boundary

- The Gateway is the only public-facing service.
- No packet from a client is trusted until it passes JWT validation and protobuf schema validation.
- The Gateway does not trust any client claim — JWT claims are verified cryptographically (HMAC-SHA256).

> **Planned:** The validation pipeline will grow to include TLS termination (Phase 5), rate limiting
> (Phase 4), and sequence-number replay protection (Phase 1 finish). See the relevant sections below.

### Internal Service Trust Model

- Services within the private network share a trust domain.
- Inter-service gRPC calls are authenticated by network origin (private CIDR).
- No internal service exposes a port to the public internet except: Gateway WebSocket (:443 target / :8080 current), Gateway management (:8080), Room Service management (:8080).
- In future phases, mTLS will provide per-call authentication between services.

### Business Backend Trust Model

- The Business Backend is a separate trust domain managed by a different team/service.
- Spatial Server trusts the Business Backend's JWT signing secret (HMAC) and its API calls to `CreateRuntime` / `DestroyRuntime`.
- Spatial Server never calls the Business Backend — it only accepts incoming API calls and validates client JWTs.

## Authentication

### JWT Authentication

**Current implementation:** HMAC-SHA256 with shared secret.

- Gateway validates JWT tokens signed with HS256 using `gateway.jwt_secret` (config key).
- Token passed via WebSocket query parameter: `ws://host:8080/ws?token=<jwt>`.
- Claims: `player_id`, `runtime_id`, `zone_id`, `exp`.
- The Gateway currently reads `player_id`, `runtime_id`, and `zone_id` from the token. `exp` is **not** enforced yet (planned).
- No JWKS, no public key rotation — single shared HMAC secret.
- Tokens are validated only at connection establishment. There is no mid-session revalidation.

> **Planned (Phase 4+):** Migrate to asymmetric keys (EdDSA/Ed25519) with JWKS endpoint for key
> rotation. See ADR-018.

### Rate Limiting for Auth Attempts

> **Planned (Phase 4):** Failed JWT validation attempts will be rate-limited per IP (target: 10
> failures/minute, with the IP blocked for 60 seconds beyond the limit). Not yet implemented —
> currently every connection attempt is processed.

## Transport Security

> **Planned (Phase 5):** TLS 1.3 termination at Gateway. Currently plain HTTP — no TLS is configured
> anywhere in the codebase.

| Layer | Mechanism | Status |
|-------|-----------|--------|
| **External** | TLS 1.3 (WSS) | **Planned (Phase 5).** Currently plain HTTP/WS. |
| **Internal** | gRPC over mTLS | **Planned (future).** Not enabled for MVP. |

### External TLS 1.3

> **Planned (Phase 5):** TLS 1.3 termination at Gateway. Currently plain HTTP.

**Target design:**

- All client traffic will use TLS 1.3 with a minimum set of cipher suites: TLS_AES_128_GCM_SHA256, TLS_AES_256_GCM_SHA384, TLS_CHACHA20_POLY1305_SHA256.
- Certificates managed by cert-manager with Let's Encrypt issuer.
- Auto-renewal: cert-manager handles renewal 30 days before expiry.

> **Planned (Phase 5):** HSTS header `Strict-Transport-Security: max-age=31536000; includeSubDomains`
> on all HTTP responses. Not currently set.

### Internal mTLS

- Not enabled for MVP to reduce operational complexity.
- When enabled: each service gets a certificate from the internal CA (cert-manager with self-signed issuer).
- Service-to-service connections verify peer certificates against the CA root.
- gRPC interceptors enforce mTLS at the transport level.

## Authorization

**Spatial Server does not implement its own authorization.** Authorization is entirely delegated to the Business Backend via JWT claims.

- The JWT contains `runtime_id` which determines which runtime (room/showroom/meeting) the player can access.
- Zone-level access control: the player can only connect to zones assigned to their runtime. The Gateway validates the `zone_id` claim by looking up the zone assignment (Room Service `LookupZone`) before proxying.
- There is no concept of roles, permissions, or ACLs within Spatial Server.
- The Business Backend is responsible for ensuring players only receive tokens for runtimes they are authorized to access.

## Replay Protection

> **Planned (Phase 1 finish):** Sequence number validation in the packet header. The current packet
> header has no sequence field — it is 3 bytes (1 byte flags + 2-byte packet ID). Replay protection
> is therefore not yet implemented.

**Target design:**

- Each client packet will include a monotonic sequence number (uint32) in the packet header.
- Gateway tracks the highest sequence number per connection in memory.
- A packet is rejected if its sequence number is ≤ the last processed sequence number.
- **30-second window enforcement**: if a packet's sequence number is more than 30 seconds old (based on server arrival time), it is rejected even if it's higher than the last processed number.
- **Uint32 wrap handling**: on wrap (sequence number resets to 0 after 2^32-1), the Gateway resets the per-connection sequence tracker. Wrap is expected to be rare given the 100 msg/s rate (wrap occurs after ~497 days of continuous connection).
- Rejected packets are silently dropped — no error response sent to avoid oracle attacks.

## Packet Validation

- All client messages are validated against protobuf schema definitions before any processing occurs (the `protocol.Decode` step rejects malformed frames).
- **Malformed packet handling:** packets that fail protobuf deserialization are silently dropped. No error response is sent to the client to prevent oracle attacks that could be used to probe the schema.
- **Unknown field handling:** protobuf fields unknown to the schema are silently ignored (standard protobuf behavior).
- Validation occurs at the Gateway before forwarding to the Game Server.

> **Planned (Phase 1 finish):** Packet size enforcement (target: 64 KB maximum, configurable).
> Currently only a 3-byte minimum-length check exists (`packet too short`); there is no upper bound.

## Rate Limiting

> **Planned (Phase 4):** Per-connection (100 msg/s) and per-IP (500 msg/s) token buckets. Not yet
> implemented. The values below describe the target design.

| Limit | Value | Scope | Enforcement |
|-------|-------|-------|-------------|
| Per-connection throughput | 100 msg/s | Individual WebSocket connection | Token bucket at Gateway |
| Per-IP throughput | 500 msg/s | All connections from same source IP | Shared token bucket at Gateway |
| Auth attempts | 10 failures/min | Per source IP | Separate token bucket |
| Admin API | 100 req/s | Per source IP | HTTP middleware |

### Rate Limit Enforcement Flow

> **Planned (Phase 4):** The graduated enforcement flow below is not yet implemented.

1. **First violation**: packet silently dropped, Gateway logs the event internally.
2. **Second violation**: packet dropped, warning message sent to client (`rate_limit_warning`).
3. **Third violation (within 60s window)**: connection terminated with `rate_limit_exceeded` reason.

All limits are intended to be configurable via dynamic config and updateable without restarting the Gateway.

## Secrets Management

- Secrets are **never** stored in source code or committed to version control.
- **Production**: Kubernetes Secrets, encrypted at rest via etcd encryption.

> **Note:** Production uses K3s (lightweight Kubernetes). Kubernetes Secrets work identically on K3s.

- **Development**: environment variables loaded from `.env` files (`.env` in `.gitignore`).

### Secrets Inventory

| Secret | Storage | Rotation |
|--------|---------|----------|
| JWT signing secret (HMAC) | K3s Secret / env var (`gateway.jwt_secret`) | Manual — update Secret, restart Gateway pods (invalidates all existing tokens) |
| Database credentials (PostgreSQL) | K3s Secret / env var | Manual rotation via `ALTER USER ... WITH PASSWORD` |
| Redis password | K3s Secret / env var | Manual rotation |
| TLS private keys (Let's Encrypt) | cert-manager managed | **Planned (Phase 5):** auto-renewal every 60 days |
| Admin API key | K3s Secret / env var | Manual rotation |

### Rotation Procedures

- **JWT signing secret**: manual — update `gateway.jwt_secret` in the Secret and restart Gateway pods. All existing tokens are invalidated on rotation.
- **DB credentials**: update the Secret, restart affected pods. Use connection draining to avoid drops.
- **TLS certificates**: **Planned (Phase 5)** — fully automated via cert-manager.

> **Planned (Phase 4+):** JWKS endpoint and key rotation. Currently a single shared HMAC secret (see
> JWT Authentication).

## Threat Model

> Mitigations annotated with **(Planned Phase X)** are target design and not yet implemented.

| Threat | Vector | Impact | Mitigation | Severity |
|--------|--------|--------|------------|----------|
| Unauthorized WebSocket connection | Attacker presents forged/stolen JWT | Adversary joins runtime, intercepts realtime data | JWT signature validation (HS256, shared secret); token binds `runtime_id` + `zone_id` (Gateway verifies zone via Room Service `LookupZone`) | Critical |
| JWT theft (interception) | Attacker captures token from network traffic | Adversary impersonates user for token lifetime | **(Planned Phase 5)** TLS 1.3 encrypts all traffic — currently plaintext. Short-lived tokens limit exposure window once TLS lands. | High |
| Replay attack | Attacker captures and re-sends a valid packet | Duplicate state mutations or desynchronized game state | **(Planned Phase 1 finish)** Monotonic sequence number per connection; Gateway rejects sequence ≤ last processed; 30s time window. Not yet implemented (header has no sequence field). | High |
| DoS via connection flood | Attacker opens thousands of WebSocket connections | Resource exhaustion on Gateway, denial of service for legitimate users | **(Planned Phase 4)** Per-IP rate limiting (500 msg/s); connection limits per Gateway instance (10,000); connection rate limiting. Not yet implemented. | High |
| DoS via large packets | Attacker sends oversized packets | Memory exhaustion on Gateway | **(Planned Phase 1 finish)** 64 KB maximum packet size enforced before deserialization. Currently no upper-bound check (only 3-byte minimum). | Medium |
| DoS via malformed packets | Attacker sends invalid protobuf data | CPU wasted on deserialization | Malformed packets silently dropped (no error response). **(Planned Phase 4)** rate limiting caps throughput. | Medium |
| Internal eavesdropping | Attacker with private network access intercepts gRPC traffic | Exposure of realtime game state | Private network segmentation; **(Planned future)** optional mTLS. | Medium |
| Compromised Game Server | Attacker gains control of a Game Server pod | Unauthorized access to zone state, potential manipulation of game logic | Zone isolation (each server owns specific zones); stateless Gateway (no credentials stored); database read-only access for Game Servers | High |
| JWT replay across zones | Attacker uses a valid JWT for zone A to access zone B | Unauthorized cross-zone access | Gateway validates `zone_id` via Room Service `LookupZone` before proxying | Critical |
| HMAC secret compromise | Attacker obtains the shared JWT signing secret | Forge valid tokens for any player/runtime | Secret stored in K3s Secret (not in source); rotation invalidates all tokens; **(Planned Phase 4+)** asymmetric keys (EdDSA) remove shared-secret risk | Critical |
| Brute-force JWT secret | Attacker attempts to guess the JWT signing key | Forge valid tokens | HMAC secret is high-entropy; **(Planned Phase 4)** rate limiting on auth failures (10 failures/min/IP) | Low |
| Side-channel via error messages | Attacker probes schema by observing error responses | Information leakage about packet structure | Malformed packets silently dropped (no error response); auth failures return generic error | Low |
| Connection hijacking via IP spoofing | Attacker spoofs source IP to bypass per-IP rate limits | Bypass rate limiting, amplify DoS | **(Planned Phase 4)** per-connection token bucket still applies; TCP handshake makes IP spoofing impractical for sustained attacks | Low |

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
