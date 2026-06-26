# Heartbeat Protocol

> **Last Updated:** 2026-06-26

## Purpose

Define the heartbeat mechanism for maintaining connection health between clients and the Gateway, and between Game Servers and Room Service.

## Client↔Gateway Heartbeat

### Mechanism

Both the client and Gateway send `Heartbeat` packets (ID: `0xFF`) at regular intervals. The heartbeat serves as a bidirectional keep-alive and latency probe.

### Intervals

| Direction | Interval | Purpose |
|-----------|----------|---------|
| Client → Gateway | 5 seconds | Client liveness signal |
| Gateway → Client | 5 seconds | Server liveness signal |

### Timeout Handling

| Condition | Action |
|-----------|--------|
| Gateway misses 3 consecutive heartbeats from client (15s) | Gateway sends `AuthResponse(failure)` and closes the connection |
| Client misses 3 consecutive heartbeats from Gateway (15s) | Client initiates reconnection |

### Heartbeat Packet

```protobuf
message Heartbeat {
    int64 client_timestamp_ms = 1;  // set by sender
    int64 server_timestamp_ms = 2;  // set by Gateway on response
}
```

The Gateway echoes `client_timestamp_ms` in its response, allowing the client to compute round-trip time:

```
rtt = now() - echo(heartbeat.client_timestamp_ms)
```

### Heartbeat Acceleration

If no other packets have been sent in the interval, the heartbeat is sent as a standalone packet. If other packets have been sent, the heartbeat can be skipped (piggybacking on existing traffic).

```go
func shouldSendHeartbeat(lastSend time.Time, interval time.Duration) bool {
    return time.Since(lastSend) >= interval
}
```

## Game Server↔Room Service Heartbeat

### Mechanism

Game Servers send periodic heartbeats to Room Service to indicate they are alive and accepting zones.

### Intervals

| Direction | Interval | Purpose |
|-----------|----------|---------|
| Game Server → Room Service | 5 seconds | Liveness and capacity reporting |
| Room Service → Game Server | — | No response needed (fire-and-forget) |

### Heartbeat Payload

```protobuf
message GameServerHeartbeat {
    string game_server_id = 1;
    int32 zone_count = 2;
    int32 max_zones = 3;
    float cpu_percent = 4;
    float memory_mb = 5;
    int64 timestamp_ms = 6;
}
```

### Timeout Handling

| Condition | Action |
|-----------|--------|
| Room Service misses 3 heartbeats from a Game Server (15s) | Room Service marks Game Server as `dead`, reassigns its zones |
| Game Server misses 3 heartbeat acknowledgments (implicit) | Game Server continues serving; zones are not automatically released |

## Monitoring

Heartbeat metrics are exported to Prometheus:

- `heartbeat_missed_total{service="gateway"}` — count of missed client heartbeats.
- `heartbeat_missed_total{service="gameserver"}` — count of missed Game Server heartbeats.
- `heartbeat_latency_ms` — RTT measured from client heartbeat echo (p50, p95, p99).
- `connections_active` — current number of active, heartbeating connections.

## References

- [WebSocket Protocol](websocket.md)
- [Reconnection Protocol](reconnect.md)
- [ADR-012](../adr/012-networking.md) — Networking
- [ADR-006](../adr/006-game-server-lifecycle.md) — Game Server Lifecycle
