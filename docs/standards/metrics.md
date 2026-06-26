# Metrics Standards

> **Last Updated:** 2026-06-26

## Purpose

Define Prometheus metric naming conventions, label usage, and histogram/ counter/gauge patterns that all Spatial Server services must follow.

## Naming Convention

Metrics follow the [Prometheus naming best practices](https://prometheus.io/docs/practices/naming/):

```
<service>_<component>_<unit>[_<qualifier>]
```

| Component | Rule | Example |
|-----------|------|---------|
| Service prefix | Lowercase, singular | `gateway_`, `room_service_`, `game_server_` |
| Context | What the metric measures | `connections`, `messages`, `tick`, `aoi_query` |
| Unit | Plural, omitted for counts | `duration_ms`, `bytes`, `total` for counters |
| Qualifier | Additional disambiguation | `active`, `in_flight`, `per_zone` |

### Metric Types

| Prometheus Type | When to Use | Example |
|-----------------|-------------|---------|
| Counter | Cumulative count of events | `gateway_messages_received_total` |
| Gauge | Snapshot values that go up/down | `gateway_connections_active` |
| Histogram | Latency / size distributions | `game_server_tick_duration_ms` |
| Summary | Quantile-based latency (rare) | `game_server_aoi_query_duration_ms` (only when histograms don't fit) |

## Standard Labels

Every metric MUST include these labels:

| Label | Value | Example |
|-------|-------|---------|
| `service` | Service name | `gateway`, `room_service`, `game_server` |
| `instance` | Instance identifier | `gs-1`, `gateway-abc123` |

### Contextual Labels

| Category | Labels | Used By |
|----------|--------|---------|
| RPC | `rpc_name`, `status`, `error_code` | All RPC metrics |
| Zone | `zone_id`, `runtime_id` | Game Server metrics |
| Player | `gateway_id` | Gateway connection metrics |
| Packet | `packet_id`, `direction` | Gateway message metrics |

**Rules:**

- Label cardinality must be bounded — never put unbounded values (entity_id, session_id) as labels.
- `zone_id` is acceptable because zones are pre-defined per runtime.
- `error_code` uses a finite set of known error codes.
- Label values are snake_case.

## Per-Service Metrics

### Gateway

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `gateway_connections_total` | Counter | `service`, `instance` | Cumulative connections accepted |
| `gateway_connections_active` | Gauge | `service`, `instance` | Current active connections |
| `gateway_messages_received_total` | Counter | `service`, `instance`, `packet_id` | Received client messages |
| `gateway_messages_sent_total` | Counter | `service`, `instance`, `packet_id` | Sent client messages |
| `gateway_messages_dropped_total` | Counter | `service`, `instance`, `reason` | Messages dropped due to backpressure |
| `gateway_rate_limited_total` | Counter | `service`, `instance` | Rate limit violations |
| `gateway_auth_failures_total` | Counter | `service`, `instance`, `reason` | JWT validation failures |
| `gateway_rpc_duration_ms` | Histogram | `service`, `instance`, `rpc_name`, `status` | gRPC call latency |
| `gateway_connection_duration_seconds` | Histogram | `service`, `instance` | Connection lifetime |

### Room Service

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `room_service_runtimes_active` | Gauge | `service`, `instance` | Active runtime count |
| `room_service_servers_registered` | Gauge | `service`, `instance` | Registered Game Servers |
| `room_service_zones_total` | Gauge | `service`, `instance` | Total zone count across all runtimes |
| `room_service_zone_transfers_total` | Counter | `service`, `instance`, `status` | Zone transfer operations |
| `room_service_rpc_duration_ms` | Histogram | `service`, `instance`, `rpc_name`, `status` | gRPC call latency |
| `room_service_heartbeat_missed_total` | Counter | `service`, `instance`, `server_id` | Missed Game Server heartbeats |
| `room_service_leader_active` | Gauge | `service`, `instance` | 1 if this instance is leader, 0 otherwise |

### Game Server

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `game_server_entities_total` | Gauge | `service`, `instance`, `zone_id` | Entity count per zone |
| `game_server_tick_duration_ms` | Histogram | `service`, `instance` | Game loop tick duration |
| `game_server_tick_overrun_total` | Counter | `service`, `instance` | Ticks exceeding deadline |
| `game_server_aoi_queries_total` | Counter | `service`, `instance` | AOI query count |
| `game_server_aoi_query_duration_ms` | Histogram | `service`, `instance`, `zone_id` | AOI query latency |
| `game_server_broadcast_messages_total` | Counter | `service`, `instance`, `packet_id` | Messages broadcast to clients |
| `game_server_rpc_duration_ms` | Histogram | `service`, `instance`, `rpc_name`, `status` | gRPC call latency |
| `game_server_zones_owned` | Gauge | `service`, `instance` | Number of zones owned |
| `game_server_entities_synced_total` | Counter | `service`, `instance` | Cross-server entity syncs |
| `game_server_memory_used_bytes` | Gauge | `service`, `instance` | Process RSS memory |

## Histogram Buckets

| Metric | Buckets (ms) |
|--------|-------------|
| `rpc_duration_ms` | 1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000 |
| `tick_duration_ms` | 1, 5, 10, 25, 50, 100, 250 |
| `aoi_query_duration_ms` | 0.1, 0.5, 1, 2, 5, 10, 25, 50, 100 |
| `connection_duration_seconds` | 60, 300, 600, 1800, 3600, 7200 |

## Metric Registration

All metrics are registered in a central `pkg/metrics` package:

```go
package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
    TickDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
        Name:    "game_server_tick_duration_ms",
        Help:    "Game loop tick duration in milliseconds",
        Buckets: []float64{1, 5, 10, 25, 50, 100, 250},
    }, []string{"service", "instance"})

    EntitiesTotal = prometheus.NewGaugeVec(prometheus.GaugeOpts{
        Name: "game_server_entities_total",
        Help: "Entity count per zone",
    }, []string{"service", "instance", "zone_id"})
)
```

Services register all metrics at startup and expose via `/metrics` endpoint.

## References

- [ADR-019](../adr/019-observability.md) — Observability (Prometheus stack)
- [ADR-017](../adr/017-capacity-planning.md) — Capacity Planning (scaling thresholds)
- [Coding Standards](coding.md) — Package dependency rules for `pkg/metrics`
