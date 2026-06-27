# Modular Monolith Refactor — Design Spec

> **Last Updated:** 2026-06-27
> **Status:** Draft (revised per ownership review)

## Purpose

The repository currently has service-private packages in `pkg/` that should be under `internal/`. This refactor enforces strict service boundaries so that extracting Gateway, Room Service, or Game Server into separate repositories requires only directory moves + import updates — no architectural redesign.

## Ownership Principles

1. **A service owns only its implementation.** Gateway owns WebSocket transport + connection lifecycle + packet routing. Room Service owns the runtime registry + heartbeats + zone metadata. Game Server owns the simulation loop + entity lifecycle + AOI + zone state.
2. **Shared infrastructure is not owned by any service.** Auth, session, config, logging, metrics, gRPC interceptors, storage — these are cross-cutting concerns that services *depend on* but do not *own*.
3. **Contracts live in proto.** Proto-generated code is the canonical API contract. Service packages implement those contracts; they do not own them.
4. **`pkg/` contains only truly public libraries.** `pkg/protocol/` stays because wire-format definitions are reusable by external clients.

## Scope

- Move all service-private packages from `pkg/` to `internal/<service>/`
- Move cross-cutting concerns to `internal/<concern>/` (NOT into a service)
- Split `pkg/game/` mini-app into focused packages
- Split `pkg/storage/` into per-domain repos
- Extract WebSocket transport into `internal/transport/`
- Split `pkg/gateway/handler.go` (HTTP + WS + gRPC → separate concerns)
- Delete dead code (`pkg/game/peer.go`)
- Keep only `pkg/protocol/` in `pkg/` (genuinely reusable)
- Update ALL import paths across codebase + tests
- Update `dependency-rules.md` with immutable boundary law
- Update `repository-structure.md` to match new layout

**Out of scope:**
- No ADR changes
- No protocol/proto changes
- No functionality changes (pure move + rename)
- No config struct splitting (keep unified Config for now)
- No new abstractions beyond transport interface

## Target Structure

```
spatial-server/
├── apps/
│   ├── gateway/main.go
│   ├── room-service/main.go
│   └── game-server/main.go
├── internal/
│   │
│   │   # ── Service Implementation ──
│   ├── gateway/
│   │   ├── handler.go + handler_test.go       (HTTP routes, health, /ws entry)
│   │   ├── relay.go + relay_test.go           (gRPC relay orchestration, pump goroutines)
│   │   └── cache.go + cache_test.go           (RouterCache)
│   ├── room/
│   │   ├── registry.go + registry_test.go     (ServerRegistry)
│   │   ├── ownership.go + ownership_test.go   (ZoneOwnership)
│   │   ├── store.go                           (ServerStore/ZoneStore interfaces)
│   │   └── api.go + api_test.go               (SpatialServerAPI gRPC impl)
│   ├── game/
│   │   ├── simulation.go + simulation_test.go (Game struct, tick, visibility)
│   │   ├── codec.go + codec_test.go           (wire-frame encoders)
│   │   ├── npc.go + npc_test.go               (NPC behaviors)
│   │   ├── entity/
│   │   │   └── entity.go + entity_test.go
│   │   ├── aoi/
│   │   │   └── aoi.go + aoi_test.go
│   │   └── zone/
│   │       └── zone.go + zone_test.go
│   │
│   │   # ── Shared Infrastructure ──
│   ├── auth/
│   │   └── auth.go + auth_test.go             (JWT validation — cross-cutting)
│   ├── session/
│   │   └── session.go + session_test.go       (Session, Pool — cross-cutting)
│   ├── transport/
│   │   └── websocket/
│   │       ├── conn.go + conn_test.go         (Connection interface)
│   │       └── coder/
│   │           └── conn.go + conn_test.go     (coder/websocket impl)
│   ├── storage/
│   │   ├── pools.go + pools_test.go           (PG/Redis pool factories)
│   │   ├── testdb_test.go
│   │   ├── migrations/
│   │   ├── room/
│   │   │   ├── server_repo.go + server_repo_test.go
│   │   │   └── zone_repo.go + zone_repo_test.go
│   │   └── game/
│   │       └── snapshot_store.go + snapshot_store_test.go
│   ├── grpc/
│   │   └── interceptor.go + interceptor_test.go
│   ├── config/
│   │   └── config.go + config_test.go
│   ├── logging/
│   │   └── logging.go + logging_test.go
│   ├── metrics/
│   │   └── metrics.go + metrics_test.go
│   │
│   │   # ── Shared Utilities ──
│   ├── types/
│   │   └── types.go + id.go + types_test.go + id_test.go
│   └── migration/
│       └── migration.go
│
├── pkg/
│   └── protocol/
│       └── protocol.go + protocol_test.go     (ONLY pkg — wire format)
│
├── proto/
├── tools/client/
├── tests/integration/
├── build/docker/
├── deploy/
└── configs/
```

## Package Move Matrix

### Service Implementation

| Current | Target | Rationale |
|---------|--------|-----------|
| `pkg/gateway/gateway.go` | `internal/gateway/cache.go` | Gateway-only (RouterCache) |
| `pkg/gateway/handler.go` | `internal/gateway/handler.go` + `relay.go` | Split transport fusion |
| `pkg/room/room.go` | `internal/room/registry.go` + `ownership.go` | Room Service implementation |
| `pkg/room/store.go` | `internal/room/store.go` | Room Service interfaces |
| `pkg/api/spatial_server.go` | `internal/room/api.go` | gRPC server impl (see justification below) |
| `pkg/game/game.go` | `internal/game/simulation.go` | Game Server implementation |
| `pkg/game/encode.go` | `internal/game/codec.go` | Game-only wire encoders |
| `pkg/game/npc.go` | `internal/game/npc.go` | Game-only NPC AI |
| `pkg/game/peer.go` | **DELETE** | Dead code |
| `pkg/entity/entity.go` | `internal/game/entity/entity.go` | Game domain type |
| `pkg/aoi/aoi.go` | `internal/game/aoi/aoi.go` | Game domain type |
| `pkg/zone/zone.go` | `internal/game/zone/zone.go` | Game domain type |

### Shared Infrastructure (NOT service-owned)

| Current | Target | Rationale |
|---------|--------|-----------|
| `pkg/auth/auth.go` | `internal/auth/auth.go` | Cross-cutting: Gateway, Room, Game, CLI, tests may validate JWT |
| `pkg/session/session.go` | `internal/session/session.go` | Cross-cutting: runtime concept shared by multiple services |
| `pkg/storage/storage.go` | `internal/storage/pools.go` | Shared PG/Redis pool factories |
| `pkg/storage/server_repo.go` | `internal/storage/room/server_repo.go` | Room domain persistence |
| `pkg/storage/zone_repo.go` | `internal/storage/room/zone_repo.go` | Room domain persistence |
| `pkg/storage/snapshot_store.go` | `internal/storage/game/snapshot_store.go` | Game domain persistence |
| `pkg/storage/testdb_test.go` | `internal/storage/testdb_test.go` | Shared test helper |
| `pkg/storage/migrations/` | `internal/storage/migrations/` | Shared SQL files |
| `pkg/grpc/interceptor.go` | `internal/grpc/interceptor.go` | Shared gRPC interceptors |
| `pkg/config/config.go` | `internal/config/config.go` | Shared config loader |
| `pkg/logging/logging.go` | `internal/logging/logging.go` | Shared slog setup |
| `pkg/metrics/metrics.go` | `internal/metrics/metrics.go` | Shared Prometheus setup |

### Unchanged

| Current | Target | Rationale |
|---------|--------|-----------|
| `pkg/protocol/protocol.go` | `pkg/protocol/protocol.go` | **STAYS** — external reuse |
| `internal/types/` | `internal/types/` | Already correct |
| `internal/migration/` | `internal/migration/` | Already correct |

### `pkg/api` Placement Justification

`pkg/api/spatial_server.go` contains:
- `SpatialServerAPI` — implements `spatialserverv1.SpatialServerAPIServer` (gRPC service)
- `RuntimeStore` interface + `MemoryRuntimeStore` — runtime CRUD persistence

This is **implementation**, not contract. The proto-generated stubs (`proto/gen/`) are the canonical contract. `SpatialServerAPI` is the Room Service's gRPC handler that manages runtime lifecycle. It belongs in `internal/room/` alongside the registry and ownership logic it complements.

## Transport Isolation

Extract WebSocket dependency behind interface:

```go
// internal/transport/websocket/conn.go
package websocket

type Connection interface {
    Read(ctx context.Context) ([]byte, error)
    Write(ctx context.Context, data []byte) error
    Close(statusCode uint16, reason string) error
    SetReadLimit(n int64)
    CloseNow() error
}

type Accepter interface {
    Accept(w http.ResponseWriter, r *http.Request) (Connection, error)
}
```

```go
// internal/transport/websocket/coder/conn.go
package coder

// Implements websocket.Connection using github.com/coder/websocket
type Conn struct { c *ws.Conn }
type Accepter struct{}
```

Gateway handler depends on `transport.Accepter` interface, not `coder/websocket` directly.

## Dependency Law (for dependency-rules.md)

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

## Migration Order

1. Create `internal/` subdirectories
2. Move + rename packages (git mv to preserve history)
3. Update ALL import paths in `.go` files
4. Extract transport layer (WebSocket interface)
5. Split gateway handler.go → handler.go + relay.go
6. Delete peer.go
7. Update `dependency-rules.md` + `repository-structure.md` + `AGENTS.md`
8. `go build ./... && go test ./... -race` — all pass

## References

- [ADR-015 Architecture Principles](../../adr/015-architecture-principles.md)
- [dependency-rules.md](../../standards/dependency-rules.md)
- [repository-structure.md](../../architecture/repository-structure.md)
