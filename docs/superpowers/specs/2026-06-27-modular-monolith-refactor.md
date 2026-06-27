# Modular Monolith Refactor — Design Spec

> **Last Updated:** 2026-06-27
> **Status:** Draft

## Purpose

The repository currently has service-private packages in `pkg/` that should be under `internal/`. This refactor enforces strict service boundaries so that extracting Gateway, Room Service, or Game Server into separate repositories requires only directory moves + import updates — no architectural redesign.

## Scope

- Move all service-private packages from `pkg/` to `internal/<service>/`
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
│   ├── gateway/
│   │   ├── handler.go + handler_test.go       (HTTP routes, health, auth check)
│   │   ├── relay.go + relay_test.go           (gRPC relay orchestration)
│   │   ├── cache.go + cache_test.go           (RouterCache)
│   │   ├── auth.go + auth_test.go             (JWT validation)
│   │   └── session.go + session_test.go       (Session, Pool)
│   ├── room/
│   │   ├── registry.go + registry_test.go     (ServerRegistry)
│   │   ├── ownership.go + ownership_test.go   (ZoneOwnership)
│   │   ├── store.go                           (ServerStore/ZoneStore interfaces)
│   │   └── api.go + api_test.go               (SpatialServerAPI)
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
│   ├── transport/
│   │   └── websocket/
│   │       ├── conn.go + conn_test.go         (Connection interface)
│   │       └── coder/
│   │           └── conn.go + conn_test.go     (coder/websocket impl)
│   ├── storage/
│   │   ├── pools.go + pools_test.go           (PG/Redis pool factories)
│   │   ├── testdb_test.go
│   │   ├── migrations/                         (SQL files)
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
│   ├── types/
│   │   └── types.go + id.go + types_test.go + id_test.go
│   └── migration/
│       └── migration.go
├── pkg/
│   └── protocol/
│       └── protocol.go + protocol_test.go     (ONLY pkg — wire format)
├── proto/
├── tools/client/
├── tests/integration/
├── build/docker/
├── deploy/
└── configs/
```

## Package Move Matrix

| Current | Target | Rationale |
|---------|--------|-----------|
| `pkg/gateway/gateway.go` | `internal/gateway/cache.go` | Gateway-only |
| `pkg/gateway/handler.go` | `internal/gateway/handler.go` + `relay.go` | Split transport |
| `pkg/auth/auth.go` | `internal/gateway/auth.go` | Gateway-only |
| `pkg/session/session.go` | `internal/gateway/session.go` | Gateway-only |
| `pkg/room/room.go` | `internal/room/registry.go` + `ownership.go` | Room-only |
| `pkg/room/store.go` | `internal/room/store.go` | Room-only |
| `pkg/api/spatial_server.go` | `internal/room/api.go` | Room-only |
| `pkg/game/game.go` | `internal/game/simulation.go` | Game-only |
| `pkg/game/encode.go` | `internal/game/codec.go` | Game-only |
| `pkg/game/npc.go` | `internal/game/npc.go` | Game-only |
| `pkg/game/peer.go` | **DELETE** | Dead code |
| `pkg/entity/entity.go` | `internal/game/entity/entity.go` | Game-only |
| `pkg/aoi/aoi.go` | `internal/game/aoi/aoi.go` | Game-only |
| `pkg/zone/zone.go` | `internal/game/zone/zone.go` | Game-only |
| `pkg/storage/storage.go` | `internal/storage/pools.go` | Shared infra |
| `pkg/storage/server_repo.go` | `internal/storage/room/server_repo.go` | Room domain |
| `pkg/storage/zone_repo.go` | `internal/storage/room/zone_repo.go` | Room domain |
| `pkg/storage/snapshot_store.go` | `internal/storage/game/snapshot_store.go` | Game domain |
| `pkg/storage/testdb_test.go` | `internal/storage/testdb_test.go` | Shared test helper |
| `pkg/storage/migrations/` | `internal/storage/migrations/` | Shared |
| `pkg/grpc/interceptor.go` | `internal/grpc/interceptor.go` | Shared infra |
| `pkg/config/config.go` | `internal/config/config.go` | Shared infra |
| `pkg/logging/logging.go` | `internal/logging/logging.go` | Shared infra |
| `pkg/metrics/metrics.go` | `internal/metrics/metrics.go` | Shared infra |
| `pkg/protocol/protocol.go` | `pkg/protocol/protocol.go` | **STAYS** — external reuse |

## Transport Isolation

Extract WebSocket dependency behind interface:

```go
// internal/transport/websocket/conn.go
package websocket

type Connection interface {
    Read(ctx context.Context) (messageType int, data []byte, err error)
    Write(ctx context.Context, messageType int, data []byte) error
    Close(code int, reason string) error
    SetReadLimit(n int64)
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
// ... Read, Write, Close, SetReadLimit
```

Gateway handler depends on `transport.Accepter` interface, not `coder/websocket` directly.

## Dependency Law (for dependency-rules.md)

```
A service may depend on:
  ✓ Shared contracts (proto/gen/)
  ✓ Shared infrastructure (internal/storage/, internal/grpc/, internal/config/)
  ✓ Shared utilities (internal/types/, internal/logging/, internal/metrics/)
  ✓ Shared transport (internal/transport/)

A service must NEVER depend on:
  ✗ Another service's implementation
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
7. Update `dependency-rules.md` + `repository-structure.md`
8. `go build ./... && go test ./... -race` — all pass

## Files Changed

| File | Action |
|------|--------|
| ~40 `.go` files | Move/rename |
| ~40 `_test.go` files | Move/rename |
| ~60 `.go` files | Update import paths |
| `internal/transport/websocket/conn.go` | Create |
| `internal/transport/websocket/coder/conn.go` | Create |
| `docs/standards/dependency-rules.md` | Modify — add boundary law |
| `docs/architecture/repository-structure.md` | Modify — new layout |
| `AGENTS.md` | Modify — update structure block |

## References

- [ADR-015 Architecture Principles](../../adr/015-architecture-principles.md)
- [dependency-rules.md](../../standards/dependency-rules.md)
- [repository-structure.md](../../architecture/repository-structure.md)
