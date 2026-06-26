# Reconnection Protocol

> **Last Updated:** 2026-06-26

## Purpose

Define the reconnection mechanism for clients that lose connection to the Gateway, enabling seamless recovery without data loss.

## Overview

When a client's WebSocket connection drops, the client can reconnect within a configurable window and resume its session. The server retains the player's state (position, zone assignment, AOI subscriptions) during this window.

## Reconnection Flow

```
Client                     Gateway                  Game Server
  │                          │                          │
  │── WebSocket disconnect ──│                          │
  │                          │── Notify disconnect ────→│
  │                          │                          │
  │  ┌─ Reconnect Window ──┐ │                          │
  │  │ (default: 30s)      │ │                          │
  │  └─────────────────────┘ │                          │
  │                          │                          │
  │── WebSocket reconnect ──→│                          │
  │── Auth(session_token) ───│                          │
  │                          │── Verify session ───────→│
  │                          │── Rebind to zone ───────→│
  │                          │── Send buffered states ─→│
  │                          │                          │
  │── AuthResponse(success) ─│                          │
  │                            ← Resend missed updates ─│
```

## Session Tokens

On initial authentication, the Gateway issues a session token in the `AuthResponse`:

```protobuf
message AuthResponse {
    bool success = 1;
    string error_message = 2;
    string server_version = 3;
    string min_supported_version = 4;
    string session_token = 5;  // opaque token for reconnection
    int32 reconnect_window_sec = 6;  // how long the session is retained
}
```

The session token is an opaque, cryptographically random string (32 bytes, hex-encoded). The Gateway stores the mapping `session_token → { player_id, runtime_id, zone_id, sequence_number }` in Redis with a TTL equal to the reconnect window.

## Reconnect Window

| Environment | Window | Rationale |
|-------------|--------|-----------|
| Development | 60s | Generous for debugging |
| Staging | 30s | Matches production |
| Production | 30s | Balances recovery vs. resource usage |

During the reconnect window, the Game Server retains:
- Player entity state (position, attributes)
- AOI subscriptions
- Outgoing packet buffer (last N sequence numbers)

After the window expires, the Game Server cleans up the player entity and notifies other clients via `EntityDespawn`.

## Reconnection Handshake

1. Client opens a new WebSocket connection to the Gateway.
2. Client sends `AuthRequest` with `session_token` field set (instead of a fresh JWT):

```protobuf
message AuthRequest {
    string jwt = 1;            // empty on reconnection
    string session_token = 2;  // set on reconnection
}
```

3. Gateway looks up `session_token` in Redis:
   - **Found:** Gateway re-binds the connection to the original zone and Game Server.
   - **Expired:** Gateway returns `AuthResponse(success=false, error="session expired")` — client must re-authenticate with a fresh JWT.

4. Game Server receives the rebind and resumes sending position updates to the client.

## Sequence Number Continuity

To prevent duplicate or missed packets after reconnection:

1. The client remembers the last sequence number it received before disconnect.
2. On reconnect, the client includes `last_received_seq` in the `AuthRequest`.
3. The Game Server replays any buffered packets with sequence numbers greater than `last_received_seq`.
4. The client ignores packets with sequence numbers it has already processed.

```protobuf
message AuthRequest {
    string jwt = 1;
    string session_token = 2;
    uint32 last_received_seq = 3;  // last processed sequence number
}
```

### Replay Buffer

The Game Server maintains a circular buffer of the last N sent packets per client:

| Parameter | Default | Max |
|-----------|---------|-----|
| Buffer size | 64 packets | 256 packets |
| Retention time | 30s | 60s |

Packets older than the buffer or retention time cannot be replayed. The client must accept any gap and re-sync via full state sync (`EntityState`).

## State Recovery

If the replay buffer is insufficient (client was disconnected too long or too many packets were sent), the Game Server sends a full state snapshot:

1. Game Server detects gap: `client_last_seq < buffer_head - buffer_size`.
2. Game Server sends `EntityState` packet with full player attributes and position.
3. Game Server resumes normal incremental updates.

## Corner Cases

| Scenario | Behavior |
|----------|----------|
| Client reconnects to a different Gateway | Gateway looks up session in Redis (shared Redis cluster) and proxies to the correct Game Server |
| Game Server crashed during disconnect | Gateway detects Game Server is gone, returns `AuthResponse(failure)` — client re-authenticates |
| Zone transferred during disconnect | Gateway finds updated zone assignment, proxies to new Game Server |
| Client reconnects after window expired | Session token TTL expired — client must re-authenticate |
| Duplicate reconnection (two connections, same session) | First connection is replaced; old connection receives `AuthResponse(failure)` |

## References

- [WebSocket Protocol](websocket.md)
- [Heartbeat Protocol](heartbeat.md)
- [ADR-010](../adr/010-packet-protocol.md) — Packet Protocol
- [ADR-011](../adr/011-failure-recovery.md) — Failure Recovery
