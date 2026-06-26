# Protocol Versioning

> **Last Updated:** 2026-06-26

## Purpose

Define the protocol versioning strategy for client↔server communication, ensuring backward compatibility and smooth upgrades.

## Version Scheme

The protocol uses a **major.minor** version scheme. The WebSocket URL communicates only the major version; the minor version is exchanged in the AuthResponse handshake.

```
URL:  ?v=<major>         (e.g., ?v=1)
Auth: server_version = <major>
      min_supported_version = <major>
```

| Component | Range | Description |
|-----------|-------|-------------|
| Major | 1+ | Breaking changes (incompatible wire format) |
| Minor | 0+ | Backward-compatible additions (new packet IDs, optional fields) |

The current protocol version is **v1** (major=1, minor=0).

## Major Version

A major version increment means:
- Packet header format changed (field sizes, ordering, or presence).
- Existing packet IDs were removed or reinterpreted.
- Compression or encryption algorithm changed incompatibly.
- Sequence number semantics changed.

**Server behavior:** Reject connections whose major version does not match.

```go
if clientMajor != serverMajor {
    return ErrVersionMismatch
}
```

## Minor Version

A minor version increment means:
- New packet IDs were added (clients must ignore unknown IDs).
- New optional fields in existing protobuf messages.
- New compression levels or algorithms (server still supports old ones).
- Bug fixes in protocol handling that do not change wire format.

**Server behavior:** Accept all minor versions within the same major version. Clients that only know an older minor version continue to work.

## Handshake Negotiation

Version negotiation happens during the WebSocket handshake and authentication:

1. **Handshake:** Client sends `?v=1` as a query parameter on the WebSocket URL.

```
wss://gateway.example.com/ws?v=1
```

2. **Server validation:** Gateway checks that the major version matches. If not, the WebSocket upgrade is rejected with HTTP 400.

3. **AuthResponse:** Server communicates its supported version range in the `AuthResponse` packet:

```protobuf
message AuthResponse {
    bool success = 1;
    string error_message = 2;
    string server_version = 3;       // "1"
    string min_supported_version = 4; // "1"
    // ...
}
```

4. **Client adaptation:** The client reads `min_supported_version` and `server_version` and adjusts its behavior to the intersection of supported features.

## Backward Compatibility Guarantees

| Change Type | Guarantee |
|-------------|-----------|
| Adding a new packet ID | No breaking change; old clients ignore unknown IDs |
| Adding an optional protobuf field | No breaking change; old clients preserve unknown fields |
| Removing a packet ID | Breaking change → major version bump |
| Changing field type in protobuf | Breaking change → major version bump |
| Changing packet header format | Breaking change → major version bump |
| Bug fix in existing behavior | Minor version bump; no guarantee of identical behavior |
| Compression algorithm change | Major version bump if incompatible; minor if both old and new are supported |

## Deprecation Process

1. Mark packet ID as deprecated in documentation and protobuf comments.
2. Server continues to support it for **2 major versions** (e.g., v1.x and v2.x).
3. Log warnings when deprecated packet IDs are received.
4. Remove in the next major version after the deprecation window.

## Version Lifecycle

| Version | Status | Released |
|---------|--------|----------|
| v1 | Current | 2026-06-26 |

## References

- [WebSocket Protocol](websocket.md)
- [Serialization](serialization.md)
- [ADR-010](../adr/010-packet-protocol.md) — Packet Protocol
