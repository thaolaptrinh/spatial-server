# Serialization

> **Last Updated:** 2026-06-26

## Purpose

Define the serialization approach using Protocol Buffers for all client↔server and server↔server communication.

## Choice: Protocol Buffers

All packet payloads use Protocol Buffers (proto3) as the serialization format.

### Rationale

| Criterion | Protobuf | JSON | MessagePack |
|-----------|----------|------|-------------|
| Wire size | Compact (binary) | Large (text) | Compact (binary) |
| Schema enforcement | Strong (`.proto` files) | Weak (runtime) | None |
| Code generation | Go, C++, C#, JS, etc. | Manual | Manual |
| Backward compatibility | Built-in field rules | Manual versioning | None |
| Zero-copy deserialization | Partial | No | No |
| Tooling ecosystem | Mature (protoc, grpc) | Universal | Limited |

## Proto File Organization

```
proto/
├── spatial/
│   ├── common.proto          # Shared types (Position, Vector3, etc.)
│   ├── auth.proto            # AuthRequest, AuthResponse
│   ├── entity.proto          # EntitySpawn, EntityDespawn, EntityMove, EntityState
│   ├── player.proto          # PositionUpdate, EntityAction
│   ├── heartbeat.proto       # Heartbeat
│   ├── zone.proto            # ZoneTransfer, ZoneState
│   ├── runtime.proto         # Runtime lifecycle messages (server↔server)
│   └── admin.proto           # Admin API messages
└── generate.go               # go:generate directive
```

## Field Ordering Convention

When defining protobuf messages, fields are ordered by:

1. **Required/identifying fields** (e.g., `player_id`, `runtime_id`) — field numbers 1–15 (1 byte wire overhead).
2. **Frequently sent fields** (e.g., `position`, `rotation`) — field numbers 1–15 when possible.
3. **Optional/enum fields** — field numbers 16+ (2 bytes wire overhead).
4. **Future extension fields** — reserved field numbers in the 200+ range.

```protobuf
message PositionUpdate {
    string player_id = 1;      // required, frequent → 1 byte
    float x = 2;               // frequent → 1 byte
    float y = 3;               // frequent → 1 byte
    float z = 4;               // frequent → 1 byte
    float rotation = 5;        // frequent → 1 byte
    int64 timestamp = 16;      // less frequent → 2 bytes
    // Reserved for future use
    reserved 17 to 31;
}
```

## Schema Evolution Rules

| Change | Safe? | Notes |
|--------|-------|-------|
| Adding a new field | Yes | Old clients ignore unknown fields |
| Removing a field | Yes (with `reserved`) | Never reuse a field number |
| Renaming a field | Yes | Wire format uses field numbers, not names |
| Changing field type | No | Always add a new field instead |
| Changing `optional` to `repeated` | No | Wire format is incompatible |
| Adding a new enum value | Yes | Clients must handle unknown enum values |
| Removing an enum value | Deprecated first | Mark as `reserved` in proto |

## Message Size Guidelines

| Category | Max Size | Notes |
|----------|----------|-------|
| Player actions | 1 KB | PositionUpdate, EntityAction |
| Entity sync | 4 KB | EntitySpawn, EntityState |
| Chat messages | 2 KB | ChatMessage, ChatBroadcast |
| Zone state sync | 64 KB | Full zone snapshot |
| Admin responses | 1 MB | Runtime/zone listings |

Payloads exceeding these limits should be split into multiple messages or paginated.

## Zero-Copy Deserialization (Future)

In Phase 4, evaluate `proto.Unmarshal` performance for high-throughput paths (EntityMove, PositionUpdate). If protobuf decoding shows up in CPU profiles, consider:

- Pre-allocated message pools (`sync.Pool`).
- Manual parsing for hot paths (skip protobuf reflection).
- FlatBuffers for read-only broadcast paths.

## Code Generation

```bash
# Generate Go code from all proto files
protoc --go_out=. --go_opt=paths=source_relative \
    proto/spatial/*.proto

# Single file
protoc --go_out=. --go_opt=paths=source_relative \
    proto/spatial/common.proto
```

Generated code lives alongside the proto files in `proto/spatial/*.pb.go`.

## References

- [WebSocket Protocol](websocket.md)
- [Compression](compression.md)
- [Versioning](versioning.md)
- [ADR-010](../adr/010-packet-protocol.md) — Packet Protocol
