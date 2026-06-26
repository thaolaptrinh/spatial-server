# Configuration Management Standards

> **Last Updated:** 2026-06-26

## Purpose

Define how Spatial Server services load, structure, and override configuration using koanf with consistent YAML schema, environment variable naming, and precedence ordering.

## Config Library

All services use [koanf](https://github.com/knadh/koanf) as the configuration library with the following providers in precedence order:

## Precedence (highest to lowest)

| Priority | Source | Provider | Example |
|----------|--------|----------|---------|
| 1 | CLI flags | koanf's `posflag` | `--log-level=debug` |
| 2 | Environment variables | koanf's `env` | `SPATIAL_LOG_LEVEL=debug` |
| 3 | Dynamic config | PostgreSQL watched keys | Zone size, tick rate, AOI radius |
| 4 | Config files | koanf's `file` | `configs/game-server.yml` |
| 5 | Defaults | Hardcoded struct defaults | `LogLevel: "info"` |

## Environment Variable Naming

All environment variables use the `SPATIAL_` prefix and follow a flat, underscored naming convention derived from the YAML key path:

| YAML Path | Env Variable |
|-----------|-------------|
| `log.level` | `SPATIAL_LOG_LEVEL` |
| `server.host` | `SPATIAL_SERVER_HOST` |
| `server.port` | `SPATIAL_SERVER_PORT` |
| `storage.postgres.dsn` | `SPATIAL_STORAGE_POSTGRES_DSN` |
| `storage.redis.addr` | `SPATIAL_STORAGE_REDIS_ADDR` |
| `auth.jwt.public_key_path` | `SPATIAL_AUTH_JWT_PUBLIC_KEY_PATH` |
| `metrics.enabled` | `SPATIAL_METRICS_ENABLED` |

**Rules:**

- All caps, underscore-separated.
- `SPATIAL_` prefix is mandatory to avoid collisions with system variables.
- Nested keys are flattened with `_` separators.
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

### Schema Conventions

```yaml
# configs/game-server.yml
service:
  name: game-server
  instance_id: "${HOSTNAME}"

server:
  host: "0.0.0.0"
  port: 9001
  grpc:
    max_recv_msg_size: 4194304    # 4 MiB (bytes)
    max_send_msg_size: 4194304    # 4 MiB (bytes)
    initial_conn_window_size: 65536
    initial_window_size: 65536

log:
  level: info                      # debug | info | warn | error
  format: json                     # json | text
  add_source: false

storage:
  postgres:
    dsn: "postgres://spatial:spatial@localhost:5432/spatial?sslmode=disable"
    max_conns: 20
    min_conns: 2
    max_conn_lifetime: 30m
    max_conn_idle_time: 5m
  redis:
    addr: "localhost:6379"
    password: ""
    db: 0
    pool_size: 10
    min_idle_conns: 2

auth:
  jwt:
    public_key_path: "/etc/spatial/jwt.pub"
    algorithm: EdDSA

metrics:
  enabled: true
  port: 9090
  path: /metrics

runtime:
  zone_size: 100.0                 # default zone size in world units
  tick_rate_hz: 20
  aoi_interest_radius: 300.0
  ghost_entity_timeout_ms: 500
```

**Rules:**

- All durations as Go duration strings (`30s`, `5m`, `1h`) or milliseconds where noted.
- All byte sizes as integers in bytes.
- Enums as lowercase strings.
- Sensitive values (passwords, keys) from env vars or Kubernetes Secrets — never hardcoded in YAML.
- Config structs defined once in `pkg/config/` and shared across services.

### Struct Tag Convention

```go
type Config struct {
    Service  ServiceConfig  `koanf:"service"`
    Server   ServerConfig   `koanf:"server"`
    Log      LogConfig      `koanf:"log"`
    Storage  StorageConfig  `koanf:"storage"`
    Auth     AuthConfig     `koanf:"auth"`
    Metrics  MetricsConfig  `koanf:"metrics"`
    Runtime  RuntimeConfig  `koanf:"runtime"`
}
```

Tags always lowercase. The `koanf` struct tag MUST match the YAML key exactly.

## Service-Specific Config

Each service loads `defaults.yml` first, then its own config file, then env/flags override:

```go
func LoadConfig(path string) (*Config, error) {
    k := koanf.New(".")
    // 1. Load defaults
    k.Load(file.Provider("configs/defaults.yml"), yaml.Parser())
    // 2. Load service-specific file
    k.Load(file.Provider(path), yaml.Parser())
    // 3. Override with env vars (SPATIAL_ prefix, case-insensitive, underscore)
    k.Load(env.Provider("SPATIAL_", ".", func(s string) string {
        return strings.Replace(strings.ToLower(
            strings.TrimPrefix(s, "SPATIAL_")), "_", ".", -1)
    }), nil)
    // 4. Override with CLI flags
    k.Load(posflag.Provider(nil, ".", k), nil)
    // 5. Unmarshal
    var cfg Config
    k.Unmarshal("", &cfg)
    return &cfg, nil
}
```

## Secret Management

| Environment | Mechanism |
|-------------|-----------|
| Development | `.env` file (gitignored) |
| Staging | Kubernetes Secrets → env vars |
| Production | Kubernetes Secrets → env vars |

Secrets are NEVER committed to the repository. Config YAMLs reference secrets via `---` markers or env var injection.

## References

- [ADR-014](../adr/014-infrastructure-platform.md) — Infrastructure Platform (K8s ConfigMap/Secret strategy)
- [ADR-008](../adr/008-deployment.md) — Deployment (ConfigMap loading)
- [Coding Standards](coding.md) — Package dependency rules for `pkg/config`
