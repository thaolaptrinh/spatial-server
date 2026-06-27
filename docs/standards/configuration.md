# Configuration Management Standards

> **Last Updated:** 2026-06-27

## Purpose

Define how Spatial Server services load, structure, and override configuration using koanf with a flat key schema, a consistent environment variable naming convention, and a fixed precedence order.

## Config Library

All services use [koanf](https://github.com/knadh/koanf) v2 as the configuration library, initialized with the `.` (dot) delimiter:

```go
k := koanf.New(".")
```

Keys are referenced by their flat dot path (e.g. `k.String("postgres.dsn")`, `k.Int("grpc.port")`). There is no nested-struct unmarshal in the application entry points — services read keys directly off the koanf instance.

## Precedence (highest to lowest)

| Priority | Source | Provider | Notes |
|----------|--------|----------|-------|
| 1 | Environment variables | koanf's `env` | `SPATIAL_` prefix, `__` nesting separator |
| 2 | Service-specific config file | koanf's `file` | e.g. `configs/game-server.yml` |
| 3 | Shared defaults file | koanf's `file` | `configs/defaults.yml` |

**Load order is fixed:** every service loads `configs/defaults.yml` first, then its own service file, then environment variables override on top. Later loads overwrite earlier keys at the same path.

> **Note:** There are no CLI flags (koanf `posflag`) and no dynamic config (PostgreSQL-watched keys) in the current implementation. If added later, they would slot above environment variables.

## Environment Variable Naming

All environment variables use the `SPATIAL_` prefix and use **double underscore (`__`)** as the nesting separator that maps to the `.` delimiter in the koanf key path.

**Transform (applied verbatim in every `apps/*/main.go`):**

```go
k.Load(env.Provider("SPATIAL_", ".", func(s string) string {
    return strings.Replace(
        strings.ToLower(strings.TrimPrefix(s, "SPATIAL_")),
        "__", ".", -1)
}), nil)
```

| YAML Path | Env Variable |
|-----------|-------------|
| `logging.level` | `SPATIAL_LOGGING__LEVEL` |
| `grpc.host` | `SPATIAL_GRPC__HOST` |
| `grpc.port` | `SPATIAL_GRPC__PORT` |
| `postgres.dsn` | `SPATIAL_POSTGRES__DSN` |
| `redis.addr` | `SPATIAL_REDIS__ADDR` |
| `gateway.ws_port` | `SPATIAL_GATEWAY__WS_PORT` |
| `gateway.jwt_secret` | `SPATIAL_GATEWAY__JWT_SECRET` |
| `room_service.addr` | `SPATIAL_ROOM_SERVICE__ADDR` |
| `game.tick_rate` | `SPATIAL_GAME__TICK_RATE` |
| `room.heartbeat_interval` | `SPATIAL_ROOM__HEARTBEAT_INTERVAL` |

**Rules:**

- All caps, with `__` separating each nesting level and `_` separating words within a single key segment.
- `SPATIAL_` prefix is mandatory to avoid collisions with system variables.
- Single underscore `_` is a literal part of a key name (`ws_port`, `jwt_secret`); only `__` is interpreted as a nesting boundary.
- The env provider lowercases the remainder after stripping the prefix, so env var casing is not significant beyond the prefix.
- Array indices are not exposed as env vars — use YAML for array configs.

## YAML Configuration Structure

### File Location

Config files live in `configs/` at the repository root:

```
configs/
├── defaults.yml          # Shared defaults for all services
├── gateway.yml           # Gateway-specific overrides
├── room-service.yml      # Room Service-specific overrides
└── game-server.yml       # Game Server-specific overrides
```

### Shared Keys (defaults.yml)

```yaml
service:
  name: spatial-server
  instance: "1"
  debug: false

logging:
  level: info    # info | debug | warn | error
  json: true     # true → JSON output, false → text

grpc:
  host: "0.0.0.0"
  port: 9000

postgres:
  dsn: "postgres://spatial:spatial@localhost:5432/spatial?sslmode=disable"

redis:
  addr: "localhost:6379"

gateway:
  jwt_secret: "dev-secret-key-change-in-production"   # HMAC secret
```

### Service-Specific Keys

**Gateway** (`configs/gateway.yml`):

```yaml
gateway:
  ws_port: 8080          # WebSocket listener port
  jwt_secret: "..."      # HMAC secret for JWT validation

room_service:
  addr: "room-service:9000"   # host:port for Room Service gRPC dial
```

**Room Service** (`configs/room-service.yml`):

```yaml
room:
  heartbeat_timeout: 30s    # Go duration string
  heartbeat_interval: 5s    # Go duration string
```

**Game Server** (`configs/game-server.yml`):

```yaml
game:
  tick_rate: 50ms       # Go duration string — simulation tick interval
  max_entities: 10000   # int — entity capacity guard
  zone_cell_size: 100   # float — zone cell edge length in world units
  aoi_radius: 300       # float — area-of-interest radius in world units
```

### Complete Key Reference

| Key | Type | Used by | Description |
|-----|------|---------|-------------|
| `service.name` | string | all | Service identifier for logging |
| `service.instance` | string | all | Instance discriminator |
| `service.debug` | bool | all | Enable debug behavior |
| `logging.level` | string | all | `info` / `debug` / `warn` / `error` |
| `logging.json` | bool | all | JSON vs text log output |
| `grpc.host` | string | all | gRPC listen host |
| `grpc.port` | int | all | gRPC listen port |
| `postgres.dsn` | string | room-service | PostgreSQL connection string |
| `redis.addr` | string | gateway, room-service | Redis `host:port` |
| `gateway.ws_port` | int | gateway | WebSocket listener port |
| `gateway.jwt_secret` | string | gateway | HMAC secret for JWT validation |
| `room_service.addr` | string | gateway | Room Service gRPC dial target |
| `game.tick_rate` | duration | game-server | Simulation tick interval |
| `game.max_entities` | int | game-server | Entity capacity guard |
| `game.zone_cell_size` | float | game-server | Zone cell edge length |
| `game.aoi_radius` | float | game-server | Area-of-interest radius |
| `room.heartbeat_timeout` | duration | room-service | Server heartbeat expiry |
| `room.heartbeat_interval` | duration | room-service | Expected heartbeat cadence |

**Rules:**

- All durations as Go duration strings (`50ms`, `5s`, `30m`, `1h`).
- Sensitive values (DSN, JWT secret) from env vars or Kubernetes Secrets in non-dev environments — never hardcoded in production YAML.
- Default values in `defaults.yml` are dev-friendly; production overrides come via env vars or K3s ConfigMaps/Secrets.

### Struct Tag Convention

A `Config` struct is defined in `pkg/config/config.go` but is **not used by the service entry points**. Applications read keys directly from the koanf instance (`k.String(...)`, `k.Int(...)`, `k.Duration(...)`) instead of unmarshalling into the struct:

```go
// pkg/config/config.go — exists, but apps bypass it
type Config struct {
    Service  ServiceConfig  `koanf:"service"`
    Logging  LoggingConfig  `koanf:"logging"`
    GRPC     GRPCConfig     `koanf:"grpc"`
    Postgres PostgresConfig `koanf:"postgres"`
    Redis    RedisConfig    `koanf:"redis"`
}
```

koanf struct tags are lowercase and must match the YAML key exactly. When a key is read directly via `k.String("grpc.port")`, the dot path must match the YAML nesting.

## Service-Specific Config Loading

Each service loads `defaults.yml` first, then its own config file, then env vars override. The canonical pattern, identical across all three `apps/*/main.go` files:

```go
k := koanf.New(".")
// 1. Load shared defaults
k.Load(file.Provider("configs/defaults.yml"), yaml.Parser())
// 2. Load service-specific file
k.Load(file.Provider("configs/<service>.yml"), yaml.Parser())
// 3. Override with env vars (SPATIAL_ prefix, __ → . separator)
k.Load(env.Provider("SPATIAL_", ".", func(s string) string {
    return strings.Replace(
        strings.ToLower(strings.TrimPrefix(s, "SPATIAL_")),
        "__", ".", -1)
}), nil)

// Read keys by flat dot path:
wsPort := k.Int("gateway.ws_port")
dsn    := k.String("postgres.dsn")
```

> The error handling for each `Load` call (`fmt.Fprintf(os.Stderr, ...) ; os.Exit(1)`) is omitted here for brevity — see any `apps/*/main.go` for the full pattern.

## Secret Management

| Environment | Mechanism |
|-------------|-----------|
| Development | Values in `configs/*.yml` (dev secrets only) or local env vars |
| Staging | Kubernetes Secrets → env vars (`SPATIAL_*`) |
| Production | Kubernetes Secrets → env vars (`SPATIAL_*`) |

Secrets are NEVER committed to the repository beyond dev-friendly placeholders in `configs/*.yml`. Production overrides are injected via env vars sourced from K3s Secrets.

## References

- [ADR-014](../adr/014-infrastructure-platform.md) — Infrastructure Platform (K3s ConfigMap/Secret strategy)
- [ADR-008](../adr/008-deployment.md) — Deployment (ConfigMap loading)
- [Coding Standards](coding.md) — Package dependency rules for `pkg/config`
