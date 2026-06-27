# Dependency Rules

> **Last Updated:** 2026-06-27

## Purpose

Define the strict dependency rules that govern package imports across the Spatial Server codebase. These rules prevent circular dependencies, enforce layering, and maintain architectural boundaries.

## Layer Architecture

```
apps/*  →  internal/*  →  standard library
               ↓
        external dependencies (gRPC, protobuf, slog, pgx, jwt, etc.)
```

**Cardinal rule:** Dependencies flow downward only. No package may depend on a package in a higher layer.

## Service Boundary Law

The repository is a **Modular Monolith** — three services (Gateway, Room Service, Game Server) in one module. Strict service boundaries enable future microservice extraction.

```
A service may depend on:
  ✓ Shared contracts (proto/gen/)
  ✓ Shared infrastructure (internal/auth/, internal/session/, internal/storage/,
    internal/grpc/, internal/config/, internal/logging/, internal/metrics/)
  ✓ Shared utilities (internal/types/, internal/migration/)
  ✓ Shared transport (internal/transport/)

A service must NEVER depend on:
  ✗ Another service's implementation (internal/gateway/, internal/room/, internal/game/)
  ✗ Another service's domain types

Cross-service communication occurs ONLY through gRPC + Protocol Buffers.
```

## Layer Definitions

### apps/ (Entry Points)

Thin `main.go` binaries that wire dependencies and start services. May import any `internal/` or `pkg/` package.

**Allowed dependencies:** All `internal/*`, `pkg/*`, Google gRPC, slog, koanf.

### internal/ (Service Implementation + Shared Infrastructure)

Service implementation lives under `internal/<service>/`. Shared infrastructure lives under `internal/<concern>/`.

**Allowed dependencies:** Standard library, external libraries (pgx, jwt, websocket, etc.), proto generated code.

**Forbidden cross-service imports:**
- `internal/gateway/` MUST NOT import `internal/room/` or `internal/game/`
- `internal/room/` MUST NOT import `internal/gateway/` or `internal/game/`
- `internal/game/` MUST NOT import `internal/gateway/` or `internal/room/`

### pkg/ (Public Libraries)

Only `pkg/protocol/` — wire-format definitions reusable by external clients.

**Allowed dependencies:** Standard library, `google.golang.org/protobuf/proto`.

## Per-Package Dependency Table

| Package | Allowed Dependencies | Notes |
|---------|---------------------|-------|
| `apps/*` | all `internal/*`, `pkg/*`, google gRPC, koanf, slog | Entry points — wire everything |
| `internal/game/entity/` | `internal/types/`, standard library only | Pure model — no infrastructure |
| `internal/game/aoi/` | `internal/types/`, standard library only | In-memory spatial index |
| `internal/game/zone/` | `internal/types/`, standard library only | Zone management |
| `internal/game/` | `internal/types/`, `internal/game/entity`, `internal/game/aoi`, `internal/game/zone`, `pkg/protocol`, `google.golang.org/protobuf/proto`, proto gen | Game loop |
| `internal/gateway/` | `internal/transport/websocket/`, `internal/auth`, `internal/session`, `internal/types/`, proto gen | WebSocket termination, routing |
| `internal/room/` | `internal/types/`, proto gen | Room Service logic |
| `internal/auth/` | `github.com/golang-jwt/jwt/v5`, standard library (crypto) | JWT validation — cross-cutting |
| `internal/session/` | standard library | Session management — cross-cutting |
| `internal/storage/` | `github.com/jackc/pgx/v5`, `github.com/redis/go-redis/v9`, standard library | Connection pools |
| `internal/storage/room/` | `github.com/jackc/pgx/v5`, `internal/types/`, `internal/room/`, `internal/storage/` | Room domain repos |
| `internal/storage/game/` | `github.com/jackc/pgx/v5`, standard library | Game domain persistence |
| `internal/transport/websocket/` | standard library | Transport abstraction (interface) |
| `internal/transport/websocket/coder/` | `github.com/coder/websocket`, `internal/transport/websocket/` | coder/websocket implementation |
| `internal/grpc/` | `google.golang.org/grpc`, standard library | gRPC interceptors |
| `internal/config/` | `github.com/knadh/koanf/v2` (+ providers/parsers), standard library | Configuration loading |
| `internal/logging/` | `log/slog`, standard library | Logging setup |
| `internal/metrics/` | `github.com/prometheus/client_golang/prometheus`, standard library | Prometheus metrics |
| `internal/types/` | standard library only | Shared IDs, Vector3, statuses, errors |
| `internal/migration/` | `github.com/golang-migrate/migrate/v4`, `pgx`, standard library | Migration runner |
| `pkg/protocol/` | `google.golang.org/protobuf/proto`, standard library | Binary packet protocol — external reuse |

## Enforcement

- **CI**: `go vet` and `golangci-lint` run on every commit and PR.
- **Import cycle detection**: `go vet` catches circular imports.
- **Layer violation detection**: Custom linter enforces service boundary rules.
- **Review**: Dependency changes require explicit attention in code review.

## Prohibited Patterns

| Pattern | Why Prohibited | Alternative |
|---------|---------------|-------------|
| `internal/<service>/` importing another `internal/<service>/` | Violates service boundary | Communicate via gRPC + protobuf |
| `internal/game/entity/` importing `internal/storage/` | Entity model must not depend on storage | Use interfaces, inject at `apps/` level |
| Shared infrastructure importing service code | Cross-cutting code must not depend on service impl | Keep infra logic independent |
| Direct HTTP framework in business logic | Couples to transport | Use interfaces, inject at `apps/` level |
| `pkg/protocol/` importing `internal/` | `pkg/` is for external reuse — no `internal/` deps | Move dependency to `internal/` consumer |

## References

- [Repository Structure](../architecture/repository-structure.md)
- [Coding Standards](coding.md)
- [Architecture Principles](../architecture/principles.md)
