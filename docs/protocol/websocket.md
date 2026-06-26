# WebSocket Protocol

> **Last Updated:** 2026-06-26

## Purpose

Define the wire protocol between clients and Spatial Server Gateway.

## Transport

All client ↔ Gateway communication uses WebSocket (WSS in production) with a binary length-prefixed packet format.

## Versioning

```
Protocol version: sent during WebSocket handshake as query param
Format: wss://gateway.example.com/ws?v=1
Server rejects if major version mismatch
Minor version upgrades are backward compatible
```

## Packet Structure

```
All binary, WebSocket message:
┌─────────────────────────────────────────┐
│ Protocol Version (1 byte)               │
├─────────────────────────────────────────┤
│ Packet ID (2 bytes)                     │
├─────────────────────────────────────────┤
│ Message Type (1 byte)                   │
├─────────────────────────────────────────┤
│ Flags (1 byte)                          │
│   bit 0: compressed                     │
│   bit 1: encrypted                      │
├─────────────────────────────────────────┤
│ Sequence Number (4 bytes)               │
├─────────────────────────────────────────┤
│ Payload (variable, protobuf bytes)      │
└─────────────────────────────────────────┘
```

### Header Fields

| Field | Size | Description |
|-------|------|-------------|
| Protocol Version | 1 byte | Current version: 1 |
| Packet ID | 2 bytes | Message type identifier |
| Message Type | 1 byte | Sub-type within packet ID (reserved for future use) |
| Flags | 1 byte | Bit 0: compressed, Bit 1: encrypted |
| Sequence Number | 4 bytes | Monotonic counter for replay protection |
| Payload | Variable | Protobuf-encoded message body |

## Packet IDs

| ID | Direction | Name | Description |
|----|-----------|------|-------------|
| 0x01 | C→S | `AuthRequest` | JWT + session token |
| 0x02 | S→C | `AuthResponse` | Auth result, server info |
| 0x03 | C→S | `PositionUpdate` | Player position, rotation |
| 0x04 | S→C | `EntitySpawn` | New entity entered AOI |
| 0x05 | S→C | `EntityDespawn` | Entity left AOI |
| 0x06 | S→C | `EntityMove` | Position update of visible entity |
| 0x07 | C→S | `EntityAction` | Player action (interact, emote, etc.) |
| 0x08 | S→C | `EntityState` | Full state sync (attribute changes) |
| 0xFF | C↔S | `Heartbeat` | Keep-alive |

## Compression

- Per-packet gzip compression (configurable)
- Flag bit 0 indicates compressed payload
- Compression level: 3 (balance speed vs. ratio)
- Threshold: only compress payloads > 256 bytes

## Backward Compatibility

- New packet IDs can be added without breaking old clients
- Clients ignore unknown packet IDs
- Server communicates supported protocol range during handshake

## Security

- All packets validated against protobuf schema before processing
- Malformed packets rejected immediately
- Replay protection via sequence number (30s window)
- Rate limiting: 100 msg/s per connection, 500 msg/s per IP

## References

- [ADR-010](../adr/010-packet-protocol.md) — Packet Protocol
- [ADR-012](../adr/012-networking.md) — Networking
- [API Reference](../api/spatial-server.md)
