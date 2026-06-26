# Integration Testing

> **Last Updated:** 2026-06-26

## Purpose

Define the integration testing approach for services that depend on PostgreSQL, Redis, and cross-service RPC.

## Approach

Integration tests validate real interactions between a service and its dependencies using actual infrastructure, not mocks. Each integration test stands up real PostgreSQL and/or Redis containers via Testcontainers, runs migrations, and executes service-level scenarios.

## Directory Structure

```
test/integration/
├── gateway/
│   ├── gateway_test.go
│   ├── connection_test.go
│   └── auth_test.go
├── room/
│   ├── room_test.go
│   ├── runtime_test.go
│   └── zone_test.go
├── gameserver/
│   ├── gameserver_test.go
│   ├── entity_test.go
│   └── persistence_test.go
├── rpc/
│   └── rpc_test.go
├── fixtures/
│   ├── migrations/
│   └── seed/
├── testmain.go
└── docker-compose.yaml
```

## Testcontainers Setup

Integration tests use Testcontainers for Go to manage PostgreSQL and Redis containers:

```go
func TestMain(m *testing.M) {
    ctx := context.Background()

    postgres, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
        ContainerRequest: testcontainers.ContainerRequest{
            Image: "postgres:16-alpine",
            Env: map[string]string{
                "POSTGRES_DB":       "spatial_test",
                "POSTGRES_PASSWORD": "test",
            },
            ExposedPorts: []string{"5432/tcp"},
            Cmd:          []string{"postgres", "-c", "fsync=off"},
        },
        Started: true,
    })

    redis, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
        ContainerRequest: testcontainers.ContainerRequest{
            Image:        "redis:7-alpine",
            ExposedPorts: []string{"6379/tcp"},
        },
        Started: true,
    })

    // Run migrations
    migrateUp(postgresEndpoint, redisEndpoint)

    code := m.Run()
    postgres.Terminate(ctx)
    redis.Terminate(ctx)
    os.Exit(code)
}
```

## Service-Level Integration Tests

### Gateway Integration
- WebSocket handshake with valid/invalid JWT
- Packet round-trip (encode → send → receive → decode)
- Rate limiter under load
- Concurrent connection handling

### Room Service Integration
- Runtime CRUD with PostgreSQL persistence
- Zone allocation and release
- Leader election (PostgreSQL advisory locks)
- Game Server registration and heartbeat

### Game Server Integration
- Entity simulation loop with state persistence
- AOI queries against live entity grid
- Zone transfer (serialize entity → send → load entity)
- Position update broadcast to connected clients

### RPC Integration
- Direct Game Server ↔ Game Server calls
- Timeout and retry behavior
- Streaming zone state sync

## Running Integration Tests

```bash
# Run all integration tests
go test ./test/integration/... -v -tags=integration -timeout 300s

# Single service
go test ./test/integration/gateway/... -v -tags=integration

# Skip integration tests in normal unit runs
go test ./internal/... -v  # integration tests excluded by build tag
```

Integration tests use the `integration` build tag:

```go
//go:build integration

package gateway_test
```

## CI Pipeline

- Integration tests run on every PR, not only on merge.
- Each PR gets a fresh Testcontainers environment.
- Cleanup is enforced via `t.Cleanup()` and `defer`.
- Testcontainers reuses containers across test functions within the same binary for speed.
- Timeout: 5 minutes max for the full suite.

## References

- [Unit Testing](unit-testing.md)
- [Testing Strategy](strategy.md)
- [ADR-008](../adr/008-deployment.md) — Deployment
