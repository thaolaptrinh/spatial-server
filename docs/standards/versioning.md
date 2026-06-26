# Versioning Standards

> **Last Updated:** 2026-06-26

## Purpose

Define semantic versioning rules for the Spatial Server platform, including application releases, protocol versions, and API versioning strategy.

## Application Versioning

All Spatial Server binaries (gateway, room-service, game-server) share a single unified version following [Semantic Versioning 2.0.0](https://semver.org/):

```
MAJOR.MINOR.PATCH
```

| Component | Definition | Bump When |
|-----------|------------|-----------|
| MAJOR | Breaking change | Incompatible protocol/API changes, removed RPCs, breaking wire format |
| MINOR | New feature | New RPCs, new packet types, backward-compatible additions |
| PATCH | Bug fix | Internal fixes, performance improvements, no contract changes |

### Pre-release Labels

| Label | Example | Purpose |
|-------|---------|---------|
| `-alpha.1` | `v1.2.0-alpha.1` | Internal testing, unstable |
| `-beta.1` | `v1.2.0-beta.1` | Staging validation, feature-complete |
| `-rc.1` | `v1.2.0-rc.1` | Release candidate, production-like testing |

### Git Tag Strategy

| Tag Pattern | Environment | Example |
|-------------|-------------|---------|
| `vMAJOR.MINOR.PATCH` | Production | `v1.2.0` |
| `vMAJOR.MINOR.PATCH-rc.N` | Staging | `v1.2.0-rc.1` |
| `vMAJOR.MINOR.PATCH-beta.N` | Staging | `v1.2.0-beta.1` |
| `main` (branch) | Dev | Latest commit on main |

All services are built from the same git tag. There is no per-service version divergence.

## Protocol Versioning

### Wire Protocol (Client ↔ Gateway)

The binary packet protocol version is communicated during WebSocket handshake:

```
wss://gateway.example.com/ws?v=1
```

| Component | Encoding | Location |
|-----------|----------|----------|
| Major version | 4 bits | URL query param `v` + packet header byte |
| Minor version | 4 bits | AuthResponse `protocol_version` field |

**Rules:**

- Major version mismatch → server rejects connection with `PROTOCOL_VERSION_MISMATCH`.
- Minor version upgrades are backward compatible — old clients continue to work.
- Server communicates supported protocol range (`min_version`, `max_version`) during AuthResponse.
- New packet IDs are additive — clients ignore unknown packet IDs.

### gRPC API Versioning

Internal gRPC services follow a package-based versioning strategy:

```protobuf
package spatialserver.v1;
```

| Element | Strategy |
|---------|----------|
| Package | `spatialserver.v{N}` — increment on breaking change |
| Service | `service GameServer { ... }` — evolving, not versioned separately |
| Message | Always additive (field numbers never reused, never removed) |
| Enum | New values appended, never reordered or removed |

**Breaking changes require a new package version (`v1` → `v2`):**

| Change | Breaking? |
|--------|-----------|
| Adding a new RPC | No |
| Adding a new field to message | No |
| Adding a new enum value | No |
| Removing a field | Yes |
| Renaming a field | Yes |
| Changing field type | Yes |
| Removing an RPC | Yes |
| Changing RPC request/response type | Yes |

### Service Dependencies

| Service | API Consumer | Version Source |
|---------|-------------|----------------|
| SpatialServerAPI | Business Backend | Proto package + runtime negotiation |
| Gateway (WebSocket) | Client | Wire protocol version |
| RoomService | Gateway, Game Server | Proto package |
| GameServer | Room Service, Game Server | Proto package |

## Compatibility Matrix

| Server Version | Protocol Version | API Package | Business Backend Compat |
|----------------|------------------|-------------|------------------------|
| v1.0.x | v1 | `spatialserver.v1` | v1.x |
| v1.1.x | v1 | `spatialserver.v1` | v1.x |
| v2.0.x | v2 | `spatialserver.v2` | v2.x |

## References

- [ADR-010](../adr/010-packet-protocol.md) — Packet Protocol (wire versioning)
- [ADR-009](../adr/009-rpc-contract.md) — RPC Contract (proto definitions)
- [Coding Standards](coding.md) — Git tag conventions
