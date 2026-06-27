# Operations Runbook

> **Last Updated:** 2026-06-26

## Purpose

Standard operating procedures for running Spatial Server in production.

## Monitoring

| Check | Endpoint | Interval | Failure Action |
|-------|----------|----------|----------------|
| **Health** | `/health` (returns 200) | 10s | Remove from LB, emit alert |
| **Readiness** | `/ready` (returns 200 when fully operational) | 10s | Remove from LB |
| **Liveness** | `/live` (returns 200 when process is alive) | 30s | Restart container |
| **Heartbeat** | Game Server → Room Service (gRPC) | 5s | Zone reassignment after 3 missed |
| **Heartbeat** | Room Service → Game Server (ping) | 10s | Detect zone ownership staleness |

## Alerting Rules

| Alert | Condition | Severity |
|-------|-----------|----------|
| GatewayDown | `/health` non-200 | critical |
| GameServerDown | Heartbeat timeout > 15s | critical |
| TickOverrun | Tick duration > 50ms × 10 consecutive | warning |
| HighLatency | RPC p99 > 100ms | warning |
| ConnectionSaturation | Gateway > 8,000 connections | warning |

## Failure Recovery Procedures

| Failure Scenario | Detection | Recovery |
|-----------------|-----------|----------|
| **Gateway Crash** | Health check fails | LB routes to other Gateway. Clients reconnect via session recovery. |
| **Room Service Crash** | Health check fails | Follower becomes leader (Kubernetes Lease API). Gateway cache valid for up to 5s. |
| **Game Server Crash** | Heartbeat timeout (15s) | Room Service marks zones as orphan. Assigns to available Game Servers. |
| **Redis Crash** | Connection error | Degraded mode: Game Server reads from PostgreSQL directly. |
| **PostgreSQL Crash** | Connection error | Degraded mode: in-memory only. Writes queued. Replay on recovery. |
| **Network Partition** | Heartbeat timeout on both sides | PostgreSQL acts as tiebreaker (advisory lock). |

## Scaling Guide

### Gateway — Horizontal, Stateless

```
Scale: add/remove instances behind load balancer
No session affinity needed
Health: /health (TCP or HTTP)
```

### Room Service — Leader Election, HA

```
Active/Passive pair
Production: K3s (Kubernetes) Lease API (coordination.k8s.io)
Leader handles: zone assignment, load balancing, service discovery
Follower: warm standby
Failover: <5 seconds
```

### Game Server — Horizontal, Coordinator-Managed

```
Room Service decides when to add/remove Game Servers
New Game Server joins → Room Service assigns orphan zones
Game Server drains → Room Service reassigns zones
Game Server crash → Room Service detects via heartbeat timeout
```

## Key Metrics

| Category | Metric | Labels |
|----------|--------|--------|
| Latency | `rpc_duration_ms` | service, rpc_name, status |
| RPC | `rpc_requests_total` | service, rpc_name, status |
| RPC | `rpc_errors_total` | service, rpc_name, error_code |
| AOI | `aoi_query_duration_ms` | zone_id |
| AOI | `entities_per_zone` | zone_id |
| Player | `connected_players` | gateway_id |
| Player | `players_per_zone` | zone_id |
| Connection | `websocket_connections` | gateway_id |

## Logging

All logs are structured JSON with consistent fields:

```json
{
  "timestamp": "2026-06-26T10:00:00Z",
  "level": "info",
  "service": "game-server",
  "instance": "gs-1",
  "trace_id": "abc123",
  "request_id": "req-456",
  "session_id": "sess-789",
  "zone_id": "zone-42",
  "entity_id": "ent-123",
  "message": "entity_moved",
  "latency_ms": 12
}
```

## References

- [Deployment Guide](deployment.md)
- [Backup](backup.md)
- [Restore](restore.md)
- [Disaster Recovery](disaster-recovery.md)
- [Incident Response](incident-response.md)
- [Scaling Guide](scaling-guide.md)
- [ADR-011](../adr/011-failure-recovery.md) — Failure Recovery
- [ADR-019](../adr/019-observability.md) — Observability
