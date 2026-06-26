# Admin API

> **Last Updated:** 2026-06-26

## Purpose

Define the Admin API for operators to monitor and manage Spatial Server at runtime.

## Endpoints

All Admin API endpoints are served on a dedicated admin port (default: `:9090`) separate from the client-facing Gateway port. Access is restricted to internal networks or VPN.

### Health Checks

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/admin/health` | GET | Basic health (service is running) |
| `/admin/health/ready` | GET | Readiness (service can accept traffic) |
| `/admin/health/live` | GET | Liveness (service is not deadlocked) |

**Response (200):**
```json
{
    "status": "ok",
    "service": "gateway",
    "version": "1.2.0",
    "uptime_seconds": 86400,
    "timestamp": "2026-06-26T12:00:00Z"
}
```

### Metrics

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/admin/metrics` | GET | Prometheus-formatted metrics |
| `/admin/metrics/json` | GET | JSON snapshot of key metrics |

**JSON Metrics Response (200):**
```json
{
    "connections_active": 3742,
    "connections_total": 89500,
    "connections_rate_per_sec": 42,
    "messages_in_per_sec": 18500,
    "messages_out_per_sec": 52200,
    "errors_per_sec": 3,
    "goroutines": 1250,
    "memory_mb": 256,
    "cpu_percent": 34.2
}
```

### Runtime Management

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/admin/runtimes` | GET | List all runtimes with status |
| `/admin/runtimes/{id}` | GET | Runtime detail |
| `/admin/runtimes/{id}/drain` | POST | Start graceful shutdown of a runtime |
| `/admin/runtimes/{id}/force-stop` | POST | Immediately terminate a runtime |

**Runtime List Response (200):**
```json
{
    "runtimes": [
        {
            "id": "runtime-abc123",
            "state": "active",
            "zones": 4,
            "game_servers": 2,
            "players": 87,
            "created_at": "2026-06-26T10:00:00Z"
        }
    ],
    "total": 1
}
```

### Game Server Management

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/admin/game-servers` | GET | List all registered Game Servers |
| `/admin/game-servers/{id}` | GET | Game Server detail |
| `/admin/game-servers/{id}/drain` | POST | Drain zones from a Game Server |
| `/admin/game-servers/{id}/kill` | POST | Force-kill a Game Server process |

### Zone Management

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/admin/zones` | GET | List all zones with Game Server assignment |
| `/admin/zones/{id}` | GET | Zone detail (entity count, load metrics) |
| `/admin/zones/{id}/transfer` | POST | Trigger manual zone transfer to another Game Server |

### Operator Commands

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/admin/config` | GET | Dump current runtime configuration |
| `/admin/config` | PUT | Update runtime configuration (hot-reload) |
| `/admin/log-level` | PUT | Change log level at runtime (`debug`, `info`, `warn`, `error`) |
| `/admin/pprof` | GET | Go pprof endpoint (heap, goroutine, cpu, trace) |

**Config Update Request (PUT /admin/config):**
```json
{
    "log_level": "debug",
    "rate_limit_per_conn": 200,
    "aoi_radius": 50.0
}
```

## Authentication

Admin API endpoints require authentication via:
- **mTLS** (production): Client certificate from internal PKI.
- **API Key** (staging): `X-Admin-Key` header, validated against `ADMIN_API_KEY` environment variable.

## Rate Limiting

- Admin API has its own rate limiter: 100 req/s per IP.
- Health check endpoints are exempt from rate limiting.

## References

- [Spatial Server API](spatial-server.md)
- [ADR-019](../adr/019-observability.md) — Observability
- [ADR-018](../adr/018-security.md) — Security
