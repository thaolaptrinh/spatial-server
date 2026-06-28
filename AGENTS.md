# AGENTS.md — Spatial Server

## Project Structure

```
apps/              Service binaries (gateway, room-service, game-server)
internal/           Service implementation + shared infrastructure (not importable outside module)
  gateway/          Gateway WebSocket handler, relay, router cache
  room/             Room Service registry, ownership, SpatialServerAPI
  game/             Game Server simulation, NPC, entity, AOI, zone
  auth/             JWT validation (cross-cutting)
  session/          Session pool (cross-cutting)
  transport/        WebSocket abstraction (Connection interface)
  storage/          PG/Redis pools, migrations, room/game domain repos
  grpc/             gRPC interceptors (recovery, logging, metrics)
  config/           Configuration loading (koanf)
  logging/          Structured logging (slog)
  metrics/          Prometheus metrics
  types/            Shared types (IDs, Vector3, statuses, errors)
  migration/        Migration runner
pkg/
  protocol/         Binary packet protocol (ONLY pkg/ — external reuse)
proto/             gRPC protobuf definitions (.proto + gen/ for generated code)
configs/           YAML config files per service
build/docker/      Dockerfiles
deploy/            Docker Compose
docs/              All documentation (architecture, ADRs, standards, ops, testing)
scripts/           dev-up.sh, dev-down.sh
tests/             Integration tests
tools/             CLI tools (client)
```

## Build / Lint / Test Commands

```bash
# Build all service binaries
make build

# Run all unit tests
go test ./internal/... -v -race -cover

# Run a single package's tests
go test ./internal/gateway/... -v -race -cover

# Run a single test function
go test ./internal/room/... -v -run TestLookupZone

# Run a specific test with sub-test
go test ./internal/game/entity/... -v -run "TestEntityID/GivenValidInput_ReturnsID"

# Integration tests (requires Docker for PostgreSQL/Redis)
go test ./tests/integration/... -v -race

# Lint
golangci-lint run ./...

# Check for circular imports
go vet ./...

# Generate protobuf code
make proto

# Run all tests + lint (CI pipeline)
make ci

# Documentation checks (run from repo root)
lychee --config .lychee.toml docs/**/*.md README.md    # link check
npx @mermaid-js/mermaid-cli mmdc -i diagram.mmd -o /dev/null  # mermaid validate
```

## Code Style Guidelines

### Imports
Standard library first, then third-party, then internal. Use `goimports` to enforce.

```go
import (
    "context"
    "fmt"

    "github.com/jackc/pgx/v5"
    "google.golang.org/grpc"

    "spatial-server/internal/types"
    "spatial-server/internal/game/entity"
)
```

### Formatting
- `gofmt` / `goimports` enforced in CI
- Line length: soft 100, hard 120
- No `_test.go` files checked by linters (only `go vet`)

### Naming
Full details: `docs/standards/naming.md`. Quick reference:
- Packages: lowercase, single word (`entity`, `zone`)
- Files: snake_case matching package (`entity.go`)
- Exported types: PascalCase (`EntityID`, `ZoneManager`)
- Unexported: camelCase (`zoneOwnership`)
- Interfaces: PascalCase, -er suffix (`Storage`, `Writer`)
- Constants: PascalCase (`MaxPlayersPerZone`)
- Acronyms: `AOIIndex`, `HTTPHandler`, `JWTToken` (PascalCase); `aoiIndex`, `jwtToken` (camelCase)

### Types
- Define interfaces in the **consumer** package, not the implementor
- Prefer small interfaces (1-3 methods)
- Accept interfaces, return concrete types
- Use `UUIDv7` for all entity/zone IDs (`github.com/google/uuid` or custom)

### Error Handling
Full details: `docs/standards/error-handling.md`. Quick reference:
- Sentinel errors with `errors.New()` for expected failures
- Wrap all errors with context: `fmt.Errorf("lookup zone %s: %w", id, err)`
- DO NOT use `"failed to"` in error messages (redundant)
- Log at the service boundary (gRPC handler, goroutine entry), not in business logic
- Panics only for truly unrecoverable states
- Map Go errors to gRPC status codes at service boundaries

### Dependency Rules
Full details: `docs/standards/dependency-rules.md`. Cardinal rule:
- `apps/* → internal/* → standard library + external deps` — dependencies flow DOWNWARD only
- Service boundary law: services must NEVER import another service's implementation
- `internal/game/entity/` and `internal/game/aoi/` depend only on `internal/types/` and stdlib (zero infrastructure deps)
- `internal/gateway/` depends on `internal/transport/` abstraction, not `coder/websocket` directly
- `internal/storage/` may use `pgx` and `go-redis`
- `pkg/protocol/` may use protobuf + stdlib (external reuse)

## Documentation Conventions

All docs live in `docs/`. Every doc must have:
- `> **Last Updated:** YYYY-MM-DD` header
- `## Purpose` section
- `## References` section with related docs/ADRs
- Cross-references use relative paths: `[ADR-009](../adr/009-rpc-contract.md)`

### ADRs
- 26 numbered ADRs in `docs/adr/` — never create a new one without incrementing
- Format: Context → Problem → Decision → Alternatives → Tradeoffs → Consequences → Future Considerations

### Diagrams
- All diagrams as Mermaid code blocks in `.md` files
- Diagrams directory: `docs/diagrams/` — 7 files total (system-context, component, deployment, sequences, runtime-lifecycle, ownership, rpc-flow)
- 3 diagram types used: `graph TB` (architecture), `sequenceDiagram` (flows), `stateDiagram-v2` (state machines)

### Terminology (CRITICAL)
The platform is **Runtime-based**. NEVER use:
- `World`, `MMO`, `Open World`, `Global World`, `World Streaming` (deprecated concepts)
- Use: `Runtime`, `Zone`, `AOI`, `Entity`, `Gateway`, `Room Service`, `Game Server`

## Architecture Quick Reference

```
Business Backend → Room Service → Runtime Instance → Zone → AOI → Entity
```

| Service | Role | Port |
|---------|------|------|
| Gateway (gRPC) | WebSocket termination, auth, routing | 9000 |
| Room Service (gRPC) | Zone ownership, coordinator | 9000 |
| Game Server (gRPC) | Entity simulation, AOI | 9000 |
| Gateway (client) | WSS | 443 |
| PostgreSQL | Persistence | 5432 |
| Redis | Cache, pub/sub | 6379 |

## Testing

Full details: `docs/testing/`. Quick reference:
- Unit tests: alongside source, table-driven preferred
- Integration tests: `tests/integration/` with Testcontainers
- Naming: `TestXxx` or `TestXxx_GivenY_WhenZ`
- Mocks: GoMock or hand-written interfaces
- CI gate: light load on every PR, full benchmark pre-release

## CI

Workflows removed — no CI needed at this stage.

## Key Standards Documents

Reference these before coding:
- `docs/standards/coding.md` — Package layout, interface rules
- `docs/standards/naming.md` — Naming conventions (Go, proto, config, infra, DB)
- `docs/standards/dependency-rules.md` — Layer architecture, prohibited patterns
- `docs/standards/error-handling.md` — Error creation, wrapping, gRPC mapping
- `docs/standards/logging.md` — slog conventions, standard fields
- `docs/standards/protobuf-convention.md` — Proto file layout, field numbering
- `docs/standards/grpc-convention.md` — gRPC timeouts, retries, streaming
- `docs/standards/configuration.md` — koanf conventions, env vars
- `docs/standards/versioning.md` — Semver, protocol compatibility
- `docs/architecture/repository-structure.md` — Directory structure, dependency rules
