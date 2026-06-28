# Spatial Server

Reusable distributed realtime spatial server platform for 3D Showroom, Virtual Office, Digital Twin, and Event Platform.

## Architecture

```
                    ┌─────────────┐
                    │   Clients   │
                    └──────┬──────┘
                           │ WebSocket (TLS + JWT)
                    ┌──────▼──────┐
                    │   Gateway   │  stateless, horizontally scalable
                    └──────┬──────┘
                           │ gRPC (lookup)
                    ┌──────▼──────┐
                    │Room Service │  HA coordinator
                    │metadata only│  zone → GameServer mapping
                    └──────┬──────┘
                     ┌─────┼─────┐
                     │     │     │
               ┌─────▼┐ ┌─▼──┐ ┌▼─────┐
               │Game 1│ │Game│ │Game N│
               │zone A│ │zone│ │zone  │
               │zone B│ │ C  │ │ D..F │
               └──┬───┘ └─┬──┘ └──┬───┘
                  │        │       │
                  └───gRPC(direct)─┘
                    direct P2P RPCs
```

**Hybrid architecture:** Lightweight coordinator (Room Service) + direct gRPC P2P between Game Servers.

## Tech Stack

| Layer | Technology |
|---|---|
| Language | Go |
| Client Protocol | WebSocket (coder/websocket) |
| Internal RPC | gRPC / Protobuf |
| Database | PostgreSQL (pgx) |
| Cache | Redis (go-redis) |
| Config | koanf |
| Logging | slog |
| Migrations | golang-migrate |
| IDs | UUIDv7 |
| Orchestration | Docker Compose (dev), Helm (production) |
| CI/CD | GitHub Actions |

## Services

| Service | Role | State |
|---|---|---|
| **Gateway** | WebSocket termination, client auth, rate limiting, connection routing | Stateless |
| **Room Service** | Zone ownership table, load balancing, service discovery, HA coordination | Lightweight metadata |
| **Game Server** | Entity simulation, AOI queries (in-memory), state persistence, client state replication | Zone state |

## Project Structure

```
├── apps/               # Service binaries (thin mains)
│   ├── gateway/        # WebSocket termination + auth orchestration + routing
│   ├── room-service/   # Zone ownership registry + runtime lifecycle + coordination
│   └── game-server/    # Spatial simulation + entity lifecycle + AOI
├── internal/           # Service implementations + shared infrastructure (not importable outside module)
│   ├── gateway/        # WebSocket handler, relay, router cache
│   ├── room/           # Registry, ownership, SpatialServerAPI
│   ├── game/           # Simulation, NPC, entity, AOI, zone
│   ├── auth/           # JWT validation (cross-cutting)
│   ├── session/        # Session pool (cross-cutting)
│   ├── transport/      # WebSocket abstraction (Connection interface)
│   ├── storage/        # PG/Redis pools, migrations, domain repos
│   ├── grpc/           # gRPC interceptors (recovery, logging, metrics)
│   ├── mtls/           # Mutual TLS helpers
│   ├── observability/  # Tracing setup
│   ├── config/         # Configuration loading (koanf)
│   ├── logging/        # Structured logging (slog)
│   ├── metrics/        # Prometheus metrics
│   ├── migration/      # Migration runner
│   └── types/          # Shared types (IDs, Vector3, statuses, errors)
├── pkg/                # Exportable libraries (external reuse)
│   └── protocol/       # Binary packet protocol
├── proto/              # gRPC protobuf definitions + generated code
│   ├── spatialserver/  # .proto sources
│   └── gen/            # generated Go code
├── configs/            # YAML config files per service
├── deploy/             # Docker Compose (dev)
├── infra/              # Helm charts + Terraform
├── scripts/            # dev-up.sh, dev-down.sh
├── tests/              # Integration, validation, chaos, fuzz, security
│   ├── integration/    # Integration tests (Testcontainers)
│   ├── validation/     # Chaos scenario definitions (15 scenarios, build tag: validation)
│   ├── chaos/          # Chaos tests (build tag: chaos)
│   ├── fuzz/           # Fuzz tests
│   └── security/       # Security tests
├── benchmarks/         # Load / simulation framework
├── tools/              # CLI tools (client)
├── artifacts/          # Build output (gitignored)
└── docs/               # Architecture, ADRs, standards, ops, testing
```

> Service implementations live under `internal/` (not importable outside the module). Only `pkg/protocol/` is intended for external reuse. See [docs/architecture/repository-structure.md](docs/architecture/repository-structure.md) and [AGENTS.md](AGENTS.md) for the dependency rules.

## Getting Started

### Prerequisites

- Go 1.25+
- Docker + Docker Compose

### Local Development

```bash
# Start infrastructure (PostgreSQL, Redis)
make dev-up
# (equivalently: docker compose -f deploy/docker-compose/docker-compose.yml up -d)

# Run unit tests (most packages need no DB)
go test ./internal/... ./pkg/... -v -race -cover

# Integration tests spin up their own Postgres/Redis via Testcontainers
go test -tags=integration -count=1 -timeout=120s ./tests/integration/...
```

Migrations live in `internal/storage/migrations/` and are applied automatically by the
Room Service and Game Server binaries on startup (via `internal/migration`). There is no
standalone migrate CLI; to apply migrations manually, run a service binary or use the
`golang-migrate` CLI directly against that directory.

### Build & Test

```bash
make build

# Unit tests
go test ./internal/... -v -race -cover

# Integration tests (Testcontainers)
go test -tags=integration -count=1 -timeout=120s ./tests/integration/...

# Chaos tests (requires Docker)
go test -tags=validation -run TestProcessChaosScenarios -count=1 -timeout=30m ./tests/validation/

# Lint
golangci-lint run ./...
```

## Development Phases

| Phase | Focus | Status |
|---|---|---|
| 0 | Architecture & ADRs | ✅ Complete |
| 1 | Core scaffolding — service binaries, types, packet protocol, WebSocket relay, game loop, AOI, `make demo` | ✅ Complete |
| 2 | Production readiness — PostgreSQL persistence, SpatialServerAPI, integration tests, config standardization | ✅ Complete |
| 3 | Realtime features — NPC simulation (patrol/idle/wander), EntityAction/EntityState, zone snapshots, Prometheus metrics, gRPC interceptors | ✅ Complete |
| 4 | Distributed scaling — zone transfer (PrepareTransfer), entity migration (MigrateEntityIn/Out), cross-server AOI, heartbeat sweeper | ✅ Complete |
| 5 | Validation framework — chaos/integration/benchmark engine, process & compose injectors, 15 chaos scenarios, 5 validators | ✅ Complete |
| 6 | Production hardening — TLS/mTLS, K3s + Helm, rate limiting, session resumption, autoscaling, OpenTelemetry | 📋 Planned |

## License

Proprietary — All rights reserved.
