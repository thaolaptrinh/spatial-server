# Error Handling Standards

> **Last Updated:** 2026-06-26

## Purpose

Define consistent error handling patterns across all Go code in Spatial Server, covering error creation, wrapping, propagation, logging, and gRPC error mapping.

## Error Creation

### Sentinel Errors

Use sentinel errors for expected, recoverable failures:

```go
var (
    ErrNotFound     = errors.New("not found")
    ErrZoneNotOwned = errors.New("zone not owned")
    ErrServerFull   = errors.New("server at capacity")
)
```

Sentinel errors:
- Are defined at the package level using `var`, not `const`
- Use `errors.New()` (not `fmt.Errorf`) for static messages
- Are compared with `errors.Is()` (not `==`)

### Custom Error Types

Use custom error types when the caller needs structured information:

```go
type ZoneError struct {
    ZoneID   string
    ServerID string
    Err      error
}

func (e *ZoneError) Error() string {
    return fmt.Sprintf("zone %s on server %s: %s", e.ZoneID, e.ServerID, e.Err)
}

func (e *ZoneError) Unwrap() error {
    return e.Err
}
```

## Error Wrapping

### General Rule

Wrap all errors with context before returning, except when the error is passed through unchanged at a boundary:

```go
// Good — wrap with context
if err != nil {
    return fmt.Errorf("bind entity %s to zone %s: %w", entityID, zoneID, err)
}

// Good — no context when the error is already descriptive enough at the boundary
if err != nil {
    return err  // only at the outermost service boundary
}

// Bad — no context
if err != nil {
    return err  // caller has no idea what failed
}

// Bad — redundant "failed to"
if err != nil {
    return fmt.Errorf("failed to bind entity %s: %w", entityID, err)
}
```

### Wrapping Conventions

| Situation | Pattern | Example |
|-----------|---------|---------|
| Operation + subject | `fmt.Errorf("operation %s on %s: %w", id, target, err)` | `"lookup zone abc: %w"` |
| Validation failure | `fmt.Errorf("invalid %s: %w", field, err)` | `"invalid runtime_id: %w"` |
| gRPC boundary | `status.Errorf(codes.NotFound, "zone %s: %s", zoneID, err)` | gRPC status propagation |
| Retry exhaustion | `fmt.Errorf("lookup zone %s after %d retries: %w", id, n, err)` | Include retry count |

### What NOT to Wrap

- Do not wrap errors with `"failed to"` — it is redundant
- Do not wrap with the function name — the stack trace is available
- Do not wrap with repetitive context already present in the error chain

## Error Propagation

### Within a Package

```go
func LookupZone(zoneID string) (*Zone, error) {
    zone, err := db.QueryZone(zoneID)
    if err != nil {
        return nil, fmt.Errorf("lookup zone %s: %w", zoneID, err)
    }
    return zone, nil
}
```

### Across Service Boundaries (gRPC)

```go
// Server-side: convert to gRPC status
func (s *RoomServiceServer) LookupZone(ctx context.Context, req *pb.LookupZoneRequest) (*pb.LookupZoneResponse, error) {
    zone, err := s.store.GetZone(req.ZoneId)
    if err != nil {
        if errors.Is(err, ErrNotFound) {
            return nil, status.Errorf(codes.NotFound, "zone %s not found", req.ZoneId)
        }
        return nil, status.Errorf(codes.Internal, "lookup zone %s: %v", req.ZoneId, err)
    }
    return &pb.LookupZoneResponse{...}, nil
}

// Client-side: unwrap gRPC status
func lookupZone(client pb.RoomServiceClient, zoneID string) (*Zone, error) {
    resp, err := client.LookupZone(ctx, &pb.LookupZoneRequest{ZoneId: zoneID})
    if err != nil {
        if status.Code(err) == codes.NotFound {
            return nil, fmt.Errorf("zone %s: %w", zoneID, ErrNotFound)
        }
        return nil, fmt.Errorf("lookup zone %s: %w", zoneID, err)
    }
    return resp, nil
}
```

### gRPC Error Code Mapping

| Go Error | gRPC Status Code | Log Level |
|----------|-----------------|-----------|
| `ErrNotFound` | `NotFound` | Info |
| `ErrAlreadyExists` | `AlreadyExists` | Info |
| Validation errors | `InvalidArgument` | Warn |
| State conflict | `FailedPrecondition` | Warn |
| Rate limited | `ResourceExhausted` | Warn |
| Service unavailable | `Unavailable` | Warn |
| Unexpected error | `Internal` | Error |

## Error Handling Patterns

### Log at the Boundary

Errors should be logged at the service boundary (gRPC handler, HTTP handler, goroutine entry point), not deep in business logic:

```go
// Good — log at boundary
func (s *GatewayServer) handlePacket(conn *websocket.Conn, packet []byte) {
    err := s.processPacket(conn, packet)
    if err != nil {
        s.logger.Warn("packet processing failed", "error", err)
        // Send error response to client
    }
}

// Bad — log deep inside business logic
func (p *PacketProcessor) validate(packet []byte) error {
    if len(packet) > maxSize {
        p.logger.Warn("packet too large")  // should return error, not log
        return ErrPacketTooLarge
    }
}
```

### Panic Handling

Panics are for truly unrecoverable states only:

```go
// Acceptable panic — programmer error, must fail fast
if config == nil {
    panic("config must not be nil")
}

// Unacceptable panic — recoverable error
if err != nil {
    panic(err)  // wrong — return the error
}
```

All gRPC servers have a panic recovery interceptor that converts panics to `Internal` errors.

### Retryable Errors

```go
func isRetryable(err error) bool {
    code := status.Code(err)
    return code == codes.Unavailable || code == codes.DeadlineExceeded
}
```

Only idempotent operations are retried. See [gRPC Convention](grpc-convention.md) for retry strategy.

## References

- [Coding Standards](coding.md)
- [gRPC Convention](grpc-convention.md) — gRPC-specific error handling
- [API Convention](api-convention.md) — Error code definitions
