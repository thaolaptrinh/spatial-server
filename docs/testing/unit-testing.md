# Unit Testing

> **Last Updated:** 2026-06-26

## Purpose

Define conventions and standards for unit testing across all Spatial Server services.

## Table-Driven Tests

All non-trivial functions MUST use table-driven tests. Each test case specifies inputs, expected outputs, and a descriptive name.

```go
func TestAOIQuery(t *testing.T) {
    tests := []struct {
        name   string
        pos    Position
        radius float64
        want   int
    }{
        {name: "empty grid returns zero", pos: Position{X: 0, Y: 0}, radius: 10, want: 0},
        {name: "single entity in range", pos: Position{X: 5, Y: 5}, radius: 10, want: 1},
        {name: "entity outside radius", pos: Position{X: 100, Y: 100}, radius: 10, want: 0},
    }
    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            got := aoi.Query(tc.pos, tc.radius)
            if got != tc.want {
                t.Errorf("AOIQuery() = %d, want %d", got, tc.want)
        })
    }
}
```

## Mocking

- Use GoMock (`github.com/golang/mock`) for interface mocking.
- Generate mocks with `//go:generate mockgen -source=<file>.go -destination=mock_<file>_test.go -package=<pkg>`.
- Prefer interface-based dependencies over concrete types to enable mocking.
- Mock files live alongside the interface they mock, suffixed `_test.go`.

### Mock Placement

| Interface | Mock Location |
|-----------|---------------|
| `RoomStore` | `internal/room/mock_room_test.go` |
| `GameServerClient` | `internal/gameserver/mock_client_test.go` |
| `ZoneAllocator` | `internal/zone/mock_allocator_test.go` |
| `AuthValidator` | `internal/gateway/mock_auth_test.go` |

## Test File Placement

- Unit tests live **alongside the package they test**: `internal/foo/foo_test.go`.
- Test helper packages go in `internal/testutil/`.
- Shared test fixtures (protobuf messages, sample packets) in `internal/testutil/fixtures.go`.

## Coverage Targets

| Package | Coverage Target |
|---------|-----------------|
| `internal/gateway` | 80% |
| `internal/room` | 75% |
| `internal/gameserver` | 80% |
| `internal/aoi` | 90% |
| `internal/protocol` | 85% |
| `internal/rpc` | 80% |
| `pkg/packet` | 90% |

Coverage is measured per-package with `go test -cover`. The CI gate enforces these minimums.

## Running Tests

```bash
# Run all unit tests
go test ./internal/... ./pkg/... -cover

# Single package with verbose output
go test ./internal/aoi/... -v -cover

# With race detector
go test -race ./internal/...
```

## Conventions

- Test functions follow `Test<FunctionName>` naming.
- Use `t.Parallel()` for independent tests.
- Use `github.com/stretchr/testify/require` for assertions.
- Prefer `require.NoError(t, err)` over `if err != nil { t.Fatal(err) }`.
- Avoid `context.Background()` in tests; use `t.Context()` (Go 1.24+) or `context.WithTimeout`.
- Clean up goroutines via `t.Cleanup(func() { ... })`.

## References

- [Testing Strategy](strategy.md)
- [Integration Testing](integration-testing.md)
- [ADR-015](../adr/015-architecture-principles.md) — Architecture Principles
