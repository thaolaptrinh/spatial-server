# ADR 010: Packet Protocol

## Status

Approved

## Context

Client ↔ Gateway communication requires a binary packet protocol over WebSocket. The protocol must support authentication, position updates, entity state sync, heartbeats, and future extension.

## Problem

Client-server communication over WebSocket requires an efficient, extensible binary protocol. Text-based protocols like JSON add bandwidth overhead, and ad-hoc binary formats lack structure and tooling support.

## Decision

### Packet Structure

```
All binary, length-prefixed (big-endian):
┌─────────────────────────────────────────┐
│ Protocol Version (1 byte)               │
├─────────────────────────────────────────┤
│ Packet ID (2 bytes)                     │
├─────────────────────────────────────────┤
│ Message Type (1 byte)                   │
├─────────────────────────────────────────┤
│ Flags (1 byte)                          │
│   bit 0: compressed (future: LZ4/Snappy)│
│   bit 1: encrypted (future: mTLS)       │
├─────────────────────────────────────────┤
│ Sequence Number (4 bytes)               │
├─────────────────────────────────────────┤
│ Payload (protobuf bytes)                │
└─────────────────────────────────────────┘
```

### Versioning

- Protocol version sent during WebSocket handshake as query param: `wss://gateway/v1/ws`
- Major version mismatch → server rejects connection.
- Minor version upgrades are backward compatible.
- Server communicates supported protocol range during AuthResponse.

### Packet IDs

| ID | Direction | Name | Description |
|----|-----------|------|-------------|
| 0x01 | C→S | AuthRequest | JWT + session token |
| 0x02 | S→C | AuthResponse | Auth result, server info, protocol range |
| 0x03 | C→S | PositionUpdate | Player position, rotation, sequence |
| 0x04 | S→C | EntitySpawn | New entity entered AOI |
| 0x05 | S→C | EntityDespawn | Entity left AOI |
| 0x06 | S→C | EntityMove | Position update of visible entity |
| 0x07 | C→S | EntityAction | Player action (interact, emote, etc.) |
| 0x08 | S→C | EntityState | Full state sync (attribute changes) |
| 0xFF | C↔S | Heartbeat | Keep-alive |

New packet IDs can be added without breaking old clients. Clients ignore unknown packet IDs.

### Compression (Future)

- LZ4 or Snappy (not gzip — gzip is too slow for realtime).
- MVP: no compression (simplicity).
- Threshold: only compress payloads > 256 bytes.
- Compression level tuned for speed over ratio.

## Alternatives

1. **JSON over WebSocket**: Human-readable and easy to debug. Higher bandwidth (2-5x larger payloads), slower to parse in JavaScript/TypeScript clients.
2. **MessagePack over WebSocket**: Binary JSON alternative with less overhead. Lacks schema enforcement, making versioning harder.
3. **Protobuf directly over WebSocket**: Full schema enforcement and efficient encoding. Requires client-side protobuf compilation in all supported languages.

## Tradeoffs

- Custom binary protocol is compact and fast but requires both client and server to implement and maintain the parser.
- Protocol versioning via query param is simple but limits dynamic capability negotiation during a session.
- Compression deferred to later phase means higher bandwidth usage in the short term.

## Consequences

- Binary protocol is compact and fast.
- Protocol versioning enables backward-compatible rolling updates.
- Compression is deferred to Phase 4.
- New packet IDs are additive — no breaking changes for existing clients.

## Future Considerations

- mTLS-based encryption for production deployments.
- Protocol negotiation during handshake for feature flags.
- Delta compression for position updates to reduce bandwidth.

## Replaces

None.
