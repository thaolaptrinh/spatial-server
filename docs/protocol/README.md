# Protocol Documentation

> **Last Updated:** 2026-06-26

## Purpose

Define the client↔server wire protocol for Spatial Server — WebSocket transport, packet format, versioning, heartbeat, reconnection, serialization, and compression.

## App Client Platform

The reference client is a **Unity application** targeting three platforms:

| Platform | WebSocket | Protobuf | Binary Protocol | Notes |
|----------|-----------|----------|-----------------|-------|
| Desktop | `System.Net.WebSockets` | `Google.Protobuf` | Native `byte[]` | Full performance |
| Mobile | `System.Net.WebSockets` | `Google.Protobuf` | Native `byte[]` | Lower update rate |
| WebGL | Browser native WebSocket API | Pre-compiled proto | `ArrayBuffer` | ~200MB heap limit |

The packet protocol is platform-agnostic by design. No architectural changes are required for Unity support.

## Contents

| File | Description |
|------|-------------|
| [websocket.md](websocket.md) | WebSocket transport and binary packet structure |
| [versioning.md](versioning.md) | Protocol versioning strategy (major.minor) and handshake negotiation |
| [heartbeat.md](heartbeat.md) | Heartbeat mechanism — interval, timeout, and connection health |
| [reconnect.md](reconnect.md) | Reconnection strategy — backoff, session recovery, and state sync |
| [serialization.md](serialization.md) | Protobuf serialization — message structure and type definitions |
| [compression.md](compression.md) | Per-packet compression — gzip configuration and thresholds |

## Reading Order

1. Start with [websocket.md](websocket.md) for the wire format.
2. Read [versioning.md](versioning.md) for compatibility guarantees.
3. Review [heartbeat.md](heartbeat.md) and [reconnect.md](reconnect.md) for connection management.
4. Consult [serialization.md](serialization.md) and [compression.md](compression.md) for data encoding.

## Related Documents

- [ADR-010](../adr/010-packet-protocol.md) — Packet Protocol
- [ADR-012](../adr/012-networking.md) — Networking
- [API](../api/spatial-server.md) — Business Backend API
