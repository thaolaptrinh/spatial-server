# gRPC Convention Standards

> **Last Updated:** 2026-06-26

## Purpose

Define gRPC implementation conventions for all Spatial Server internal services, covering timeouts, retries, streaming patterns, error handling, metadata propagation, and client/server configuration.

## Client Configuration

### Default Dial Options

```go
import (
    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials/insecure"
    "google.golang.org/grpc/keepalive"
)

var defaultDialOptions = []grpc.DialOption{
    grpc.WithTransportCredentials(insecure.NewCredentials()),
    grpc.WithDefaultCallOptions(
        grpc.MaxCallRecvMsgSize(4 * 1024 * 1024),   // 4 MiB
        grpc.MaxCallSendMsgSize(4 * 1024 * 1024),   // 4 MiB
    ),
    grpc.WithKeepaliveParams(keepalive.ClientParameters{
        Time:                30 * time.Second,
        Timeout:             10 * time.Second,
        PermitWithoutStream: true,
    }),
    grpc.WithConnectParams(grpc.ConnectParams{
        MinConnectTimeout: 5 * time.Second,
    }),
}
```

### Server Configuration

```go
import (
    "google.golang.org/grpc"
    "google.golang.org/grpc/keepalive"
)

var defaultServerOptions = []grpc.ServerOption{
    grpc.MaxRecvMsgSize(4 * 1024 * 1024),
    grpc.MaxSendMsgSize(4 * 1024 * 1024),
    grpc.KeepaliveParams(keepalive.ServerParameters{
        MaxConnectionIdle:     5 * time.Minute,
        MaxConnectionAge:      30 * time.Minute,
        MaxConnectionAgeGrace: 5 * time.Second,
        Time:                  30 * time.Second,
        Timeout:               10 * time.Second,
    }),
    grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
        MinTime:             10 * time.Second,
        PermitWithoutStream: true,
    }),
    grpc.ChainUnaryInterceptor(
        metadataInterceptor,
        loggingInterceptor,
        recoveryInterceptor,
    ),
}
```

## Timeouts

Every RPC call MUST have a context timeout. The default timeout per RPC type:

| RPC Category | Default Timeout | Rationale |
|-------------|-----------------|-----------|
| Unary (read) | 2s | Simple lookup, fast DB query |
| Unary (write) | 5s | Zone allocation, entity migration |
| Streaming (zone transfer) | 30s | Large payload, multi-message transfer |
| Heartbeat | 2s | Frequent, must be fast |
| Business Backend API | 10s | May involve multiple service calls |

```go
ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
defer cancel()

resp, err := client.LookupZone(ctx, &pb.ZoneID{ZoneId: id})
```

## Retry Strategy

### Client-Side Retry

| RPC Category | Max Retries | Backoff | Idempotent |
|-------------|-------------|---------|------------|
| Unary read (Lookup, Get) | 3x | 100ms base, 2x jitter | Yes |
| Unary write (Register, Create) | 2x | 100ms base, 2x jitter | Yes |
| Streaming (Transfer, Sync) | 1x | 500ms base | Yes |
| Heartbeat | 0 | N/A | Yes (latest wins) |

```go
func retryWithBackoff(ctx context.Context, maxRetries int, fn func(context.Context) error) error {
    var err error
    for i := 1; i <= maxRetries; i++ {
        if err = fn(ctx); err == nil {
            return nil
        }
        if !isRetryable(err) {
            return err
        }
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-time.Afer(backoffDuration(i)):
        }
    }
    return err
}

func isRetryable(err error) bool {
    return status.Code(err) == codes.Unavailable ||
        status.Code(err) == codes.DeadlineExceeded
}
```

**Rules:**

- Only retry idempotent RPCs (reads, idempotent writes).
- Non-idempotent RPCs never retry on error.
- Heartbeat errors are not retried — next heartbeat sends latest state.
- Retry budget: max 3 retries per call (including across service boundary).

## Streaming Patterns

### Server-Streaming (Zone Transfer)

```protobuf
rpc TransferZone(stream ZoneSnapshot) returns (TransferZoneResponse);
```

- Source Game Server streams zone snapshot in chunks.
- Each chunk contains a subset of entities + AOI state.
- Receiver acks after full stream (no per-chunk ack).
- If stream breaks, retry with exponential backoff (max 1 retry).
- Compression: gzip enabled for streaming RPCs.

### Client-Streaming and Bidirectional

Avoid client-streaming and bidirectional streaming in Phase 1/2. Prefer:

| Instead of | Use |
|-----------|-----|
| Bidirectional streaming | Pair of unary RPCs |
| Client-streaming acknowledgments | Server-streaming with final ack |

Exceptions allowed for:
- Zone state transfer (server-streaming already approved)
- Entity sync streams (approved, Phase 3+)

## Error Handling

### Error Propagation

```
Client → Gateway → Room Service → Game Server
                       ↓
                  Wrap with context + preserve gRPC status
```

```go
// Server-side: wrap with context
return nil, status.Errorf(codes.NotFound, "zone %s not found in runtime %s", zoneID, runtimeID)

// Client-side: unwrap and handle
if status.Code(err) == codes.NotFound {
    // Handle not found gracefully
}
```

### Error Code Mapping

| Go gRPC Status | Meaning | Log Level |
|----------------|---------|-----------|
| `OK` | Success | Debug |
| `Canceled` | Context canceled by client | Debug |
| `DeadlineExceeded` | Timeout | Warn |
| `NotFound` | Resource not found | Info |
| `AlreadyExists` | Duplicate resource | Info |
| `InvalidArgument` | Bad request params | Warn |
| `FailedPrecondition` | System state prevents operation | Warn |
| `Unavailable` | Service not ready / draining | Warn |
| `ResourceExhausted` | Rate limited / no capacity | Warn |
| `Internal` | Unexpected server error | Error |
| `Unimplemented` | RPC not implemented | Error |

## Metadata Propagation

Every gRPC call propagates the following metadata:

| Metadata Key | Source | Example | Propagated |
|-------------|--------|---------|------------|
| `trace_id` | Gateway (from JWT or generated) | `trace-abc123` | All services |
| `request_id` | Caller generates | `req-456` | All services |
| `session_id` | Gateway (from auth) | `sess-789` | Game Server |
| `runtime_id` | Gateway (from auth) | `runtime-42` | Game Server |

```go
// Client-side: attach metadata
md := metadata.Pairs(
    "trace_id", traceID,
    "request_id", requestID,
)
ctx := metadata.NewOutgoingContext(ctx, md)

// Server-side: extract metadata
if md, ok := metadata.FromIncomingContext(ctx); ok {
    traceID := md.Get("trace_id")
}
```

## Interceptor Stack

Every service applies these interceptors:

| Interceptor | Order | Purpose |
|-------------|-------|---------|
| Recovery | 1st (outermost) | Panic recovery — convert panics to Internal errors |
| Metadata | 2nd | Inject/extract trace_id, request_id |
| Logging | 3rd | Log RPC name, duration, status code |
| Metrics | 4th | Record RPC duration histogram, counter |
| Validation | 5th (innermost) | Validate request fields |

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

## References

- [ADR-009](../adr/009-rpc-contract.md) — RPC Contract (service definitions, error codes)
- [ADR-011](../adr/011-failure-recovery.md) — Failure Recovery (timeout/retry in context of crash recovery)
- [API Convention](api-convention.md) — RPC naming and pattern conventions
- [Protobuf Convention](protobuf-convention.md) — .proto file conventions
