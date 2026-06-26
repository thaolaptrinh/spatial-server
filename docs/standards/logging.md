# Logging Standards

> **Last Updated:** 2026-06-26

## Purpose

Define consistent logging practices across all Spatial Server services using Go's `log/slog` package.

## Log Library

All services use `log/slog` (Go standard library, structured logging) with:

| Environment | Handler | Format |
|-------------|---------|--------|
| Production | `slog.NewJSONHandler` | Structured JSON |
| Development | `slog.NewTextHandler` | Human-readable text |
| Testing | `slog.NewTextHandler(os.Discard)` | Silent (discard) |

## Log Levels

| Level | When to Use | Example |
|-------|-------------|---------|
| `Debug` | Detailed diagnostic information, not useful in normal operation | AOI query results, packet contents, internal state dumps |
| `Info` | Normal operational messages | Connection established, zone created, heartbeat received |
| `Warn` | Unexpected but recoverable conditions | Rate limit approaching, slow queries, retry attempts |
| `Error` | Failures requiring investigation | Database connection lost, authentication failure, tick overrun |
| `Debug` (panic) | Unrecoverable state (use `slog.LogAttrs` with `LevelDebug` + `Panic` separately) | Do not use `slog` for panics — use Go's `panic()` |

## Structured Logging

### Good — Structured

```go
logger.Info("entity_moved",
    "entity_id", entityID,
    "zone_id", zoneID,
    "x", pos.X,
    "y", pos.Y,
    "z", pos.Z,
)
```

### Bad — Unstructured

```go
logger.Infof("Entity %s moved to %f %f %f in zone %s", entityID, pos.X, pos.Y, pos.Z, zoneID)
```

## Standard Log Fields

Every log entry MUST include these fields:

| Field | Source | Example | Required? |
|-------|--------|---------|-----------|
| `service` | Config | `game-server` | Yes |
| `instance` | Config / hostname | `gs-1` | Yes |
| `level` | slog built-in | `info` | Yes (auto) |
| `time` | slog built-in | `2026-06-26T10:00:00Z` | Yes (auto) |
| `trace_id` | gRPC metadata / JWT | `trace-abc123` | If available |
| `request_id` | gRPC metadata (generated per call) | `req-456` | If available |
| `session_id` | Gateway (from auth) | `sess-789` | If available |
| `zone_id` | Runtime context | `zone-42` | If relevant |
| `entity_id` | Runtime context | `ent-123` | If relevant |

### Production Log Format

```json
{
    "time": "2026-06-26T10:00:00Z",
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

## Logging Conventions by Operation Type

### gRPC Handlers

```go
func loggingInterceptor(ctx context.Context, req interface{},
    info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
    start := time.Now()
    resp, err := handler(ctx, req)
    logger.InfoContext(ctx, "rpc_completed",
        "method", info.FullMethod,
        "duration_ms", time.Since(start).Milliseconds(),
        "status", status.Code(err).String(),
    )
    return resp, err
}
```

### Background Goroutines

```go
go func() {
    defer func() {
        if r := recover(); r != nil {
            logger.Error("goroutine panicked", "recover", r)
        }
    }()
    // ... work ...
}()
```

### Connection Events

```go
logger.Info("client_connected",
    "player_id", playerID,
    "runtime_id", runtimeID,
    "remote_addr", conn.RemoteAddr(),
)
```

## What NOT to Log

| Data | Reason |
|------|--------|
| JWT contents (beyond validation result) | Token may contain sensitive claims |
| Client IP addresses in production logs | PII — use anonymized identifiers |
| Full packet payloads in production | Volume too high; use Debug level if needed |
| Database credentials | Security risk |
| Stack traces to Info/Warn level | Use Error level only |

## Log Aggregation

| Environment | Collector | Storage |
|-------------|-----------|---------|
| Development | stdout | Terminal |
| Staging | Promtail → Loki | Loki (7 day retention) |
| Production | Promtail → Loki | Loki (30 day hot, 12 month cold in S3) |

## References

- [Coding Standards](coding.md)
- [gRPC Convention](grpc-convention.md) — Metadata propagation
- [ADR-019](../adr/019-observability.md) — Observability
- [Configuration Standards](configuration.md) — Log level configuration
