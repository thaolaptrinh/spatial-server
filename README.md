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
| Client Protocol | WebSocket (nhooyr.io) |
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
├── apps/               # Service binaries
│   ├── gateway/        # WebSocket + auth + rate limiting
│   ├── room-service/   # Zone ownership + runtime lifecycle
│   └── game-server/    # Simulation loop + entity management
├── pkg/                # Shared libraries
│   ├── auth/           # JWT generation/validation
│   ├── config/         # YAML + env configuration
│   ├── entity/         # Entity model
│   ├── game/           # Game loop
│   ├── gateway/        # Gateway server + handler
│   ├── idgen/          # UUIDv7 generation
│   ├── logging/        # Structured logging (slog)
│   ├── protocol/       # Binary packet protocol
│   ├── room/           # Room service core
│   └── storage/        # PostgreSQL + Redis connections
├── proto/              # gRPC protobuf definitions
├── configs/            # YAML config files
├── deploy/             # Docker Compose + Dockerfiles
├── infra/              # Helm charts + Terraform
├── scripts/            # Dev scripts
└── docs/               # ADRs + specs
```

## Getting Started

### Prerequisites

- Go 1.22+
- Docker + Docker Compose

### Local Development

```bash
# Start infrastructure (PostgreSQL, Redis)
docker compose -f deploy/docker-compose/docker-compose.yml up -d

# Run migrations
SPATIAL_POSTGRES_DSN="postgres://spatial:spatial@localhost:5432/spatial?sslmode=disable" \
  go run ./pkg/storage/migrations/migrate.go -dsn "$SPATIAL_POSTGRES_DSN" -direction up

# Run tests
go test ./pkg/... -v -race -cover
```

Or use the dev script:

```bash
./scripts/dev-up.sh
```

### Build

```bash
make build
```

## Development Phases

| Phase | Focus | Status |
|---|---|---|
| 0 | Architecture & ADRs | ✅ Complete |
| 1 | Core infrastructure (scaffold, DB, config, logging, protobuf, Gateway, Room Service, Game Server, Docker Compose, CI) | 🔧 In Progress |
| 2 | Realtime features (AOI, position sync, entity spawn/despawn, zone crossing, chat) | 📋 Planned |
| 3 | Distributed scaling (multi-Game Server, zone transfer, heartbeat, leader election, rebalancing, metrics) | 📋 Planned |
| 4 | Production hardening (K3s, HPA, load testing, chaos testing, TLS, monitoring alerts) | 📋 Planned |

## License

Proprietary — All rights reserved.
