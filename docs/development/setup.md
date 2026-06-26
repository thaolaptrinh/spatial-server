# Development Setup

> **Last Updated:** 2026-06-26

## Purpose

Guide developers through setting up a local development environment for Spatial Server.

## Prerequisites

- Go 1.22+
- Docker + Docker Compose
- protoc (Protocol Buffers compiler)
- golangci-lint (optional, for CI linting)

## Quick Start

```bash
# Clone the repository
git clone https://github.com/yourorg/spatial-server.git
cd spatial-server

# Start infrastructure (PostgreSQL, Redis)
docker compose -f deploy/docker-compose/docker-compose.yml up -d

# Run database migrations
SPATIAL_POSTGRES_DSN="postgres://spatial:spatial@localhost:5432/spatial?sslmode=disable" \
  go run ./pkg/storage/migrations/migrate.go -dsn "$SPATIAL_POSTGRES_DSN" -direction up

# Run tests
go test ./pkg/... -v -race -cover
```

Or use the dev script:

```bash
./scripts/dev-up.sh
```

## Build

```bash
make build
```

## Common Commands

| Command | Description |
|---------|-------------|
| `make build` | Build all service binaries |
| `make test` | Run all unit tests |
| `make lint` | Run golangci-lint |
| `make proto` | Regenerate protobuf code |
| `make dev-up` | Start Docker Compose environment |
| `make dev-down` | Stop Docker Compose environment |
| `make migrate-up` | Run database migrations |
| `make migrate-down` | Rollback database migrations |

## Protobuf Code Generation

```bash
# Install protoc and Go plugins
sudo apt-get install -y protobuf-compiler
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Generate
mkdir -p gen
protoc --go_out=gen --go_opt=paths=source_relative \
       --go-grpc_out=gen --go-grpc_opt=paths=source_relative \
       -I proto proto/*.proto
```

## Repository Structure

```
spatial-server/
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
└── docs/               # Documentation
```

## References

- [Contribution Guide](contributing.md)
- [Testing Strategy](../testing/strategy.md)
