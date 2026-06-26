# ADR 019: Observability

## Status

Accepted

## Context

A distributed realtime platform requires observability across all services to detect issues, diagnose failures, and measure performance. Without a standardized observability stack, each service would use different tools and formats, making cross-service debugging impossible.

## Decision

### Stack

| Domain | Technology | Purpose |
|--------|------------|---------|
| Metrics | Prometheus | Time-series collection and alerting |
| Dashboards | Grafana | Visualization and monitoring panels |
| Log collection | Promtail | DaemonSet on each K3s node, tailing container logs |
| Log storage | Loki | Log aggregation with Prometheus-compatible labels |
| Tracing | OpenTelemetry | Distributed trace collection |
| Alerting | Prometheus Alertmanager | Route alerts to Slack/PagerDuty |

### Logging Requirements

- All logs must be structured JSON.
- Every log entry includes: `service`, `instance`, `level`, `timestamp`, `message`.
- Every request context includes: `trace_id`, `request_id`, `session_id`.
- Log levels: `debug`, `info`, `warn`, `error`. `panic` reserved for unrecoverable states.
- Production: JSON output to stdout. Promtail collects and forwards to Loki.
- Development: text format for readability.

### Metrics (Prometheus)

Every service exposes `/metrics` endpoint on a dedicated port.

**Gateway metrics:**
- `gateway_connections_total` — cumulative connection count
- `gateway_connections_active` — current active connections
- `gateway_messages_received_total` — per packet type
- `gateway_messages_sent_total` — per packet type
- `gateway_rate_limited_total` — rate limit violations
- `gateway_auth_failures_total` — JWT validation failures

**Room Service metrics:**
- `room_service_runtimes_active` — active runtime count
- `room_service_servers_registered` — registered Game Servers
- `room_service_zones_total` — total zone count
- `room_service_zone_transfers_total` — zone transfer operations
- `room_service_rpc_duration_ms` — per RPC method

**Game Server metrics:**
- `game_server_entities_total` — entity count per zone
- `game_server_tick_duration_ms` — game loop tick duration
- `game_server_aoi_queries_total` — AOI query count
- `game_server_aoi_query_duration_ms` — AOI query latency
- `game_server_broadcast_messages_total` — messages sent to clients
- `game_server_rpc_duration_ms` — per RPC method
- `game_server_cpu_usage` — process CPU
- `game_server_memory_usage` — process memory

**System metrics (per node):**
- `node_cpu_seconds_total`, `node_memory_MemTotal_bytes`, `node_network_receive_bytes_total`

### Tracing (OpenTelemetry)

- Every gRPC call propagates trace context via gRPC metadata.
- Spans: client request (full), Gateway processing, Game Server tick, AOI query, RPC call.
- Trace sampling: 1% for production (configurable). 100% for staging.
- Exporter: OTLP gRPC to OpenTelemetry Collector (or directly to Jaeger/Grafana Tempo).

### Dashboards (Grafana)

Pre-built dashboard panels:
- **Service Overview**: CPU, memory, connections, RPS per service.
- **Game Server Detail**: tick duration, entity count, AOI latency per zone.
- **Runtime Overview**: active runtimes, players per runtime, zone distribution.
- **RPC Latency**: p50/p95/p99 for all RPC methods.
- **Cluster Health**: node status, resource usage, pod status.

### Alerting Rules

| Alert | Condition | Severity |
|-------|-----------|----------|
| GatewayDown | `/health` returns non-200 | critical |
| GameServerDown | Heartbeat timeout > 15s | critical |
| TickOverrun | Tick duration > 50ms for 10 consecutive ticks | warning |
| HighLatency | RPC p99 > 100ms | warning |
| LowDisk | Disk usage > 85% | warning |
| ConnectionSaturation | Gateway connections > 8,000 | warning |

## Consequences

- Prometheus + Grafana + Loki + OpenTelemetry is the standard — all services must integrate.
- Promtail on every node collects logs without application changes.
- Tracing adds minimal overhead (1% sampling for production).
- Dashboards are pre-built, not created ad-hoc.
- Observability is mandatory — no service can skip metrics or structured logging.

## Replaces

- None. Observability was implicit in the initial design.
