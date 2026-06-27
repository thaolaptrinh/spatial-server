# Naming Conventions

> **Last Updated:** 2026-06-26

## Purpose

Define consistent naming conventions across all Go code, protobuf definitions, configuration, and infrastructure in the Spatial Server platform.

## Go Code Naming

| Convention | Rule | Good | Bad |
|------------|------|------|-----|
| Packages | lowercase, single word | `entity`, `aoi`, `zone` | `entity_manager`, `Entity` |
| Files | snake_case, matching package | `entity.go`, `entity_test.go` | `entity-manager.go`, `Entity.go` |
| Exported types | PascalCase | `EntityID`, `ZoneManager` | `entityID`, `zone_manager` |
| Unexported | camelCase | `zoneOwnership` | `ZoneOwnership` |
| Interfaces | PascalCase, -er suffix where natural | `Storage`, `AOIIndex`, `Writer` | `IStorage`, `StorageInterface` |
| Constants | PascalCase | `MaxPlayersPerZone` | `MAX_PLAYERS_PER_ZONE` |
| Variables | camelCase | `playerCount`, `zoneID` | `player_count`, `ZoneID` |
| Tests | `TestXxx` or `TestXxx_GivenY_WhenZ` | `TestLookupZone`, `TestLookupZone_WhenNotFound_ReturnsError` | `test_lookup_zone` |
| Benchmarks | `BenchmarkXxx` | `BenchmarkAOIQuery` | `benchmark_aoi` |

### Acronym Handling

| Rule | Good | Bad |
|------|------|-----|
| All caps in PascalCase | `AOIIndex`, `HTTPHandler`, `JWTToken` | `AoiIndex`, `HttpHandler` |
| All lowercase in camelCase | `aoiIndex`, `jwtToken`, `httpHandler` | `AOIIndex`, `JWTToken` |

## Protobuf Naming

| Element | Convention | Good | Bad |
|---------|------------|------|-----|
| Package | `spatialserver.v1` | `spatialserver.v1` | `SpatialServerV1` |
| Message | PascalCase | `EntitySnapshot` | `entity_snapshot` |
| Field | snake_case | `zone_id` | `zoneID`, `ZoneId` |
| Enum type | PascalCase | `RuntimeStatus` | `runtime_status` |
| Enum value | UPPER_SNAKE_CASE | `RUNTIME_ACTIVE` | `RuntimeActive` |
| Service | PascalCase | `RoomService` | `room_service` |
| RPC | PascalCase, verb + noun | `CreateRuntime` | `create_runtime` |
| File | snake_case | `room_service.proto` | `roomService.proto` |

### gRPC Service Naming

| Pattern | Example | When |
|---------|---------|------|
| `{Verb}{Noun}` | `CreateRuntime`, `LookupZone` | Standard CRUD |
| `Prepare{Noun}` | `PrepareShutdown`, `PrepareTransfer` | Multi-step operations |
| `Notify{Noun}` | `NotifyEntityEnter`, `NotifyEntityLeave` | Informational events |
| `Report{Noun}` | `ReportMetrics` | Fire-and-forget data push |

## Configuration Naming

| Element | Convention | Good | Bad |
|---------|------------|------|-----|
| YAML keys | lowercase, dot-separated | `postgres.dsn`, `gateway.ws_port` | `postgres.DSN`, `Gateway.WsPort` |
| Env vars | `SPATIAL_` prefix, `__` nesting separator | `SPATIAL_POSTGRES__DSN`, `SPATIAL_GRPC__PORT` | `POSTGRES_DSN`, `spatial_grpc_port` |
| koanf struct tags | lowercase | `koanf:"grpc.port"` | `koanf:"GrpcPort"` |
| Config file names | kebab-case | `game-server.yml` | `game_server.yml`, `GameServer.yml` |

## Infrastructure Naming

| Element | Convention | Good | Bad |
|---------|------------|------|-----|
| K3s namespace | kebab-case | `spatial-server` | `spatialServer`, `spatial_server` |
| K3s resource names | kebab-case | `game-server`, `room-service` | `gameServer` |
| Helm release | kebab-case | `spatial-server` | `spatial_server` |
| Terraform resource | snake_case | `resource "helm_release" "game_server"` | `resource "helm_release" "gameServer"` |
| Docker images | lowercase | `spatial/game-server` | `spatial/GameServer` |
| Git branch | kebab-case | `feature/zone-transfer` | `feature/zone_transfer` |
| Git tag | semver | `v1.2.0` | `v1.2.0_beta` |

## Directory Naming

```
apps/            — lowercase, no separators
pkg/<domain>/    — single word, lowercase
internal/        — lowercase
proto/           — lowercase
configs/         — lowercase
deploy/          — lowercase
infra/           — lowercase
scripts/         — lowercase
docs/            — lowercase
```

## Database Naming

| Element | Convention | Good | Bad |
|---------|------------|------|-----|
| Table | snake_case, plural | `zone_ownership`, `game_servers` | `ZoneOwnership`, `game_server` |
| Column | snake_case | `zone_id`, `heartbeat_expires_at` | `zoneID`, `heartbeatExpiresAt` |
| Index | `idx_{table}_{column}` | `idx_zone_ownership_server_id` | `idx1`, `zone_ownership_index` |
| Constraint | `{table}_{column}_key` | `zone_ownership_pkey` | `pk1` |

## References

- [Coding Standards](coding.md) — Go code conventions
- [Protobuf Convention](protobuf-convention.md) — .proto file style
- [API Convention](api-convention.md) — RPC naming patterns
- [Configuration Standards](configuration.md) — Config key naming
- [Metrics Standards](metrics.md) — Metric name conventions
