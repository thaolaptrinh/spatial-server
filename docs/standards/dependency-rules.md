# Dependency Rules

> **Last Updated:** 2026-06-26

## Purpose

Define the strict dependency rules that govern package imports across the Spatial Server codebase. These rules prevent circular dependencies, enforce layering, and maintain architectural boundaries.

## Layer Architecture

```
apps/*  →  pkg/*  →  internal/*  →  standard library
                ↓
         external dependencies (gRPC, protobuf, slog)
```

**Cardinal rule:** Dependencies flow downward only. No package may depend on a package in a higher layer.

## Layer Definitions

### apps/ (Entry Points)

Thin `main.go` binaries that wire dependencies and start services. May import any `pkg/` package.

**Allowed dependencies:** All `pkg/*`, Google gRPC, slog, koanf.

### pkg/ (Shared Libraries)

Reusable packages shared across services. May import `internal/` packages and external libraries.

**Allowed dependencies:** Standard library, Google gRPC, protobuf, slog.

**Forbidden dependencies:** `apps/*`, HTTP frameworks (except in dedicated adapter packages).

### internal/ (Private)

Types and utilities not importable outside the module. May only import standard library.

**Allowed dependencies:** Standard library only.

## Per-Package Dependency Table

| Package | Allowed Dependencies | Notes |
|---------|---------------------|-------|
| `apps/*` | all `pkg/*`, google gRPC, slog, koanf | Entry points — wire everything |
| `pkg/entity/` | `internal/types/`, standard library only | Pure model — no infrastructure |
| `pkg/aoi/` | `internal/types/`, standard library only | In-memory spatial index |
| `pkg/game/` | google gRPC, pkg/entity, pkg/space, pkg/aoi, pkg/protocol | Game loop — depends on multiple pkg/ |
| `pkg/gateway/` | nhooyr.io/websocket, pkg/protocol, pkg/auth | WebSocket — external dependency |
| `pkg/room/` | google gRPC, pkg/cluster, pgx | Room Service logic |
| `pkg/storage/` | pgx, go-redis, standard library | Database abstractions |
| `pkg/rpc/` | google gRPC, standard library | gRPC helpers |
| `pkg/config/` | koanf, standard library | Configuration loading |
| `pkg/logging/` | slog, standard library | Logging setup |
| `pkg/metrics/` | prometheus/client_golang, standard library | Metrics registration |
| `pkg/session/` | standard library, pkg/auth | Session management |
| `pkg/cluster/` | google gRPC, standard library | Cluster membership |
| `pkg/space/` | internal/types/, standard library | Space/room model |
| `pkg/zone/` | internal/types/, standard library | Zone management |
| `pkg/protocol/` | protobuf, standard library | Binary packet protocol |
| `pkg/auth/` | standard library (crypto, JWT) | JWT generation/validation |

## Enforcement

- **CI**: `go vet` and `golangci-lint` run on every commit and PR.
- **Import cycle detection**: `go vet` catches circular imports.
- **Layer violation detection**: Custom linter (Phase 1+) enforces that `pkg/*` does not import `apps/*`.
- **Review**: Dependency changes require explicit attention in code review.

## Prohibited Patterns

| Pattern | Why Prohibited | Alternative |
|---------|---------------|-------------|
| `pkg/*` importing `apps/*` | Creates circular dependency chain | Move shared code to `pkg/` or `internal/` |
| `internal/` importing `pkg/` | Violates layer direction | Move needed types down to `internal/` |
| `pkg/entity/` importing `pkg/storage/` | Entity model must not depend on storage | Use interfaces, inject at `apps/` level |
| Direct HTTP framework in non-gateway `pkg/` | Couples to transport | Use interfaces, inject at `apps/` level |
| Importing specific DB driver in business logic | Couples to storage technology | Abstract via `pkg/storage/` |

## References

- [Repository Structure](../architecture/repository-structure.md)
- [Coding Standards](coding.md)
- [Architecture Principles](../architecture/principles.md)
