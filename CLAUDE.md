# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

Distributed realtime **spatial server** in Go (1.25) for 3D Showroom, Virtual Office, Digital Twin, and Event platforms. Three cooperating services over gRPC/Protobuf, with WebSocket client ingress.

`AGENTS.md` is the canonical, more detailed agent guide (commands, conventions, standards pointers). This file captures the same essentials plus the big-picture architecture; keep the two in sync when you change one.

## Commands

```bash
make build                 # go build ./...
make test                  # unit tests, -race -count=1, across internal/ pkg/ apps/
make test-fast             # same without -race
make lint                  # golangci-lint run ./internal/... ./pkg/...
make fmt                   # gofmt internal/ pkg/
make ci                    # lint + test + build

# Single test (always pass -run; the suite uses -count=1 to bypass cache)
go test ./internal/room/... -v -run TestLookupZone -count=1
go test ./internal/game/entity/... -v -run "TestEntityID/GivenValidInput_ReturnsID" -count=1

make proto                 # buf generate --path proto/spatialserver/v1  (regenerates proto/gen/)
make proto-lint            # buf lint proto/
make proto-breaking        # buf breaking proto/ --against .git#branch=main

# Integration tests spin up their own Postgres/Redis via Testcontainers (needs Docker)
make test-integration      # = go test -tags=integration -count=1 -timeout=120s ./tests/integration/...

# Chaos tests (15 scenarios; needs Docker for the process/compose injectors)
go test -tags=validation -run TestProcessChaosScenarios -count=1 -timeout=30m ./tests/validation/

make dev-up                # Postgres + Redis only (compose.yaml) — fast local `go run`
make dev-up-full           # + app services (compose.yaml + compose.app.yaml)
make dev-down              # stop the dev stack
make demo                  # bring up the stack + run a test client against gateway:8080
make scale-up              # 2 named game-server nodes (compose.scaling.yaml)
```

Build tags gate the heavier test suites: `integration` (`tests/integration/`), `validation` + `chaos` (`tests/validation/`, `tests/chaos/`). `internal/validation/` is the reusable engine those suites drive.

## Architecture

Hybrid model: a **lightweight metadata coordinator** (Room Service) + **direct gRPC P2P** between Game Servers. The coordinator decides who owns a zone; once known, game servers talk to each other directly.

```
Clients ──WSS(TLS+JWT)──▶ Gateway ──gRPC lookup──▶ Room Service ──zone→server map──▶ Game Servers ──gRPC P2P──▶
```

| Service (binary) | Role | State | Ports |
|---|---|---|---|
| **Gateway** (`apps/gateway`) | WebSocket termination, JWT auth, rate limiting, zone lookup→routing, relay | Stateless | client WSS 443 / gRPC 9000 |
| **Room Service** (`apps/room-service`) | Zone ownership registry, runtime lifecycle, load balancing, leader election, heartbeat sweeper, HA coordination | Lightweight metadata (PG+Redis) | gRPC 9000 |
| **Game Server** (`apps/game-server`) | Entity simulation, NPC AI (patrol/idle/wander), in-memory AOI queries, zone state, replication, cross-server entity migration | Zone state (in-memory + PG) | gRPC 9000 |

**The `SpatialServerAPI`** (`proto/.../spatial_server_api.proto`) is the inter-server contract: `PrepareTransfer` / `TransferZone` (zone handoff), `MigrateEntity` / `NotifyEntityEnter|Leave` / `SendEntityUpdate` (cross-server AOI + entity migration), `Register`/`Heartbeat`/`PrepareShutdown` (node lifecycle). `RoomService` (lookup, ownership watch stream, runtime CRUD) and `GatewayService` (relay, entity queries) sit alongside it. Read `docs/diagrams/` (ownership, rpc-flow, runtime-lifecycle, sequences) before touching inter-service flows.

**Layout principle:** `apps/*` are thin `main`s; all logic lives in `internal/*` (not importable outside the module). Only `pkg/protocol/` (binary packet protocol) is meant for external reuse.

### Dependency rules (cardinal)

Dependencies flow **downward only**: `apps/* → internal/* → stdlib + external`.

- **Service boundary law:** a service must NEVER import another service's implementation. Cross-service talk is gRPC only.
- **Zero-infra leaves:** `internal/game/entity/` and `internal/game/aoi/` depend only on `internal/types/` + stdlib — no storage/transport/grpc imports. Keep them pure.
- `internal/gateway/` depends on the `internal/transport/` `Connection` abstraction, **not** `coder/websocket` directly.
- `internal/storage/` is the only place `pgx`/`go-redis` may appear.
- Define interfaces in the **consumer** package, not the implementor; prefer 1–3 method interfaces.

Full rules: `docs/standards/dependency-rules.md`. Run `go vet ./...` to catch import cycles.

## Critical conventions

**Terminology — the platform is Runtime-based.** NEVER use `World`, `MMO`, `Open World`, `Global World`, or `World Streaming` (deprecated). Use `Runtime`, `Zone`, `AOI`, `Entity`, `Gateway`, `Room Service`, `Game Server`. Conceptual chain: `Business Backend → Room Service → Runtime → Zone → AOI → Entity`. See `docs/glossary.md`.

**Identity types** (`internal/types/types.go`) are deliberately distinct and non-interchangeable: `EntityID`, `ZoneID`, `RuntimeID`, `ServerID` (node identity), `PlayerID` (external participant, only carried never issued by the runtime), `OwnerID` (entity controller). Don't cast between them. `Status` enums (`ZoneStatus`, `RuntimeStatus`, `ServerStatus`) each carry a `ValidTransition()` state machine — respect it.

**Errors** (`docs/standards/error-handling.md`): sentinel errors in `internal/types` (`ErrNotFound`, `ErrConflict`, `ErrInvalidArg`, `ErrNotOwned`, `ErrNotEmpty`); wrap with `fmt.Errorf("lookup zone %s: %w", id, err)`; **never prefix with "failed to"**; log once at the service/goroutine boundary, not in business logic; map Go errors → gRPC status codes at boundaries.

**IDs:** UUIDv7 for all entity/zone IDs (`github.com/google/uuid`). **Logging:** `slog` with structured fields. **Config:** koanf, layered `configs/defaults.yml` + `configs/<service>.yml`, env overrides. **Naming/formatting:** soft 100 / hard 120 cols; see `docs/standards/naming.md` (acronyms stay PascalCase: `AOIIndex`, `HTTPHandler`).

## Generation & persistence

- **Protobuf is generated.** Edit `.proto` in `proto/spatialserver/v1/`, run `make proto`, commit `proto/gen/`. Never hand-edit generated code. Field numbers are immutable — see `docs/standards/protobuf-convention.md`.
- **Migrations** live in `internal/storage/migrations/` and are **auto-applied on service startup** via `internal/migration` (Room Service + Game Server). No standalone migrate CLI step in the dev flow; use a service binary or the `golang-migrate` CLI directly against that dir.
- **Linting** is strict (`.golangci.yml`: errcheck, staticcheck `all`, revive `exported`, goimports with local-prefix `github.com/thaolaptrinh/spatial-server`, etc.). `proto/gen/` and `docs/` are excluded.

## Where to look

- `AGENTS.md` — full conventions + command reference (keep in sync with this file).
- `docs/standards/` — coding, naming, dependency-rules, error-handling, grpc/protobuf/configuration conventions.
- `docs/adr/` — 27 numbered ADRs (increment, never reuse a number). Format: Context → Problem → Decision → Alternatives → Tradeoffs → Consequences → Future.
- `docs/diagrams/` — system-context, component, deployment, sequences, runtime-lifecycle, ownership, rpc-flow (all Mermaid).
- `docs/testing/` and `internal/validation/` — validation/chaos/benchmark harness.
- README.md has the phase roadmap (Phases 0–5 complete; Phase 6 production hardening planned: TLS/mTLS, K3s+Helm, rate limiting, session resumption, autoscaling, OTel).
