# Modular Monolith Refactor — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development

**Goal:** Move all service-private packages from `pkg/` to `internal/`, establish strict service boundaries, and extract transport abstraction.

**Architecture:** Three services (Gateway, Room, Game) own their implementation under `internal/<service>/`. Shared infrastructure (auth, session, storage, config, logging, metrics, grpc) lives under `internal/<concern>/`. Only `pkg/protocol/` stays public.

**Tech Stack:** Go 1.25, git mv

**Spec:** [2026-06-27-modular-monolith-refactor.md](../specs/2026-06-27-modular-monolith-refactor.md)

---

### Task 1: Create internal/ directory structure

**Files:** Create directories only

- [ ] **Step 1:** Create all target directories:
```bash
mkdir -p \
  internal/gateway \
  internal/room \
  internal/game/entity \
  internal/game/aoi \
  internal/game/zone \
  internal/auth \
  internal/session \
  internal/transport/websocket/coder \
  internal/storage/room \
  internal/storage/game \
  internal/grpc \
  internal/config \
  internal/logging \
  internal/metrics
```

- [ ] **Step 2:** Verify directories exist:
```bash
ls -d internal/gateway internal/room internal/game/entity internal/game/aoi \
  internal/game/zone internal/auth internal/session internal/transport/websocket/coder \
  internal/storage/room internal/storage/game internal/grpc internal/config \
  internal/logging internal/metrics
```

---

### Task 2: Move gateway packages

**Files:**
- Move: `pkg/gateway/*.go` → `internal/gateway/`
- Move: `pkg/auth/*.go` → `internal/auth/`
- Move: `pkg/session/*.go` → `internal/session/`

- [ ] **Step 1:** Move gateway files:
```bash
git mv pkg/gateway/gateway.go internal/gateway/cache.go
git mv pkg/gateway/gateway_test.go internal/gateway/cache_test.go
git mv pkg/gateway/handler.go internal/gateway/handler.go
git mv pkg/gateway/handler_test.go internal/gateway/handler_test.go
```

- [ ] **Step 2:** Move auth files (keep package name `auth`):
```bash
git mv pkg/auth/auth.go internal/auth/auth.go
git mv pkg/auth/auth_test.go internal/auth/auth_test.go
```

- [ ] **Step 3:** Move session files (keep package name `session`):
```bash
git mv pkg/session/session.go internal/session/session.go
git mv pkg/session/session_test.go internal/session/session_test.go
```

- [ ] **Step 4:** Remove empty directories:
```bash
rmdir pkg/gateway pkg/auth pkg/session
```

---

### Task 3: Move room packages

**Files:**
- Move: `pkg/room/*.go` → `internal/room/`
- Move: `pkg/api/*.go` → `internal/room/`

- [ ] **Step 1:** Move room files:
```bash
git mv pkg/room/room.go internal/room/registry.go
git mv pkg/room/room_test.go internal/room/registry_test.go
git mv pkg/room/store.go internal/room/store.go
```

- [ ] **Step 2:** Move API files (change package from `api` to `room`):
```bash
git mv pkg/api/spatial_server.go internal/room/api.go
git mv pkg/api/spatial_server_test.go internal/room/api_test.go
```

- [ ] **Step 3:** Fix package declaration in moved API file:
```bash
# Change "package api" to "package room" in internal/room/api.go
sed -i 's/^package api$/package room/' internal/room/api.go
# Change "package api" to "package room" in internal/room/api_test.go
sed -i 's/^package api$/package room/' internal/room/api_test.go
```

- [ ] **Step 4:** Remove empty directories:
```bash
rmdir pkg/room pkg/api
```

---

### Task 4: Move game packages

**Files:**
- Move: `pkg/game/*.go` → `internal/game/`
- Move: `pkg/entity/` → `internal/game/entity/`
- Move: `pkg/aoi/` → `internal/game/aoi/`
- Move: `pkg/zone/` → `internal/game/zone/`
- Delete: `pkg/game/peer.go`

- [ ] **Step 1:** Move game files:
```bash
git mv pkg/game/game.go internal/game/simulation.go
git mv pkg/game/game_test.go internal/game/simulation_test.go
git mv pkg/game/encode.go internal/game/codec.go
git mv pkg/game/encode_test.go internal/game/codec_test.go
git mv pkg/game/npc.go internal/game/npc.go
git mv pkg/game/npc_test.go internal/game/npc_test.go
```

- [ ] **Step 2:** Delete dead code:
```bash
git rm pkg/game/peer.go
```

- [ ] **Step 3:** Move entity, aoi, zone (whole directories):
```bash
git mv pkg/entity/entity.go internal/game/entity/entity.go
git mv pkg/entity/entity_test.go internal/game/entity/entity_test.go
git mv pkg/entity/id.go internal/game/entity/id.go
git mv pkg/entity/id_test.go internal/game/entity/id_test.go

git mv pkg/aoi/aoi.go internal/game/aoi/aoi.go
git mv pkg/aoi/aoi_test.go internal/game/aoi/aoi_test.go

git mv pkg/zone/zone.go internal/game/zone/zone.go
git mv pkg/zone/zone_test.go internal/game/zone/zone_test.go
```

- [ ] **Step 4:** Remove empty directories:
```bash
rmdir pkg/game pkg/entity pkg/aoi pkg/zone
```

---

### Task 5: Move shared infrastructure packages

**Files:**
- Move: `pkg/storage/` → `internal/storage/` (split into room/ and game/ subpackages)
- Move: `pkg/grpc/` → `internal/grpc/`
- Move: `pkg/config/` → `internal/config/`
- Move: `pkg/logging/` → `internal/logging/`
- Move: `pkg/metrics/` → `internal/metrics/`

- [ ] **Step 1:** Move storage files (split by domain):
```bash
git mv pkg/storage/storage.go internal/storage/pools.go
git mv pkg/storage/testdb_test.go internal/storage/testdb_test.go
git mv pkg/storage/migrations internal/storage/migrations

# Room domain repos
git mv pkg/storage/server_repo.go internal/storage/room/server_repo.go
git mv pkg/storage/server_repo_test.go internal/storage/room/server_repo_test.go
git mv pkg/storage/zone_repo.go internal/storage/room/zone_repo.go
git mv pkg/storage/zone_repo_test.go internal/storage/room/zone_repo_test.go

# Game domain repos
git mv pkg/storage/snapshot_store.go internal/storage/game/snapshot_store.go
git mv pkg/storage/snapshot_store_test.go internal/storage/game/snapshot_store_test.go
```

- [ ] **Step 2:** Fix package declarations in storage subpackages:
```bash
# Room repos: change "package storage" to "package room"
sed -i 's/^package storage$/package room/' internal/storage/room/server_repo.go
sed -i 's/^package storage$/package room/' internal/storage/room/server_repo_test.go
sed -i 's/^package storage$/package room/' internal/storage/room/zone_repo.go
sed -i 's/^package storage$/package room/' internal/storage/room/zone_repo_test.go

# Game repos: change "package storage" to "package game"
sed -i 's/^package storage$/package game/' internal/storage/game/snapshot_store.go
sed -i 's/^package storage$/package game/' internal/storage/game/snapshot_store_test.go
```

- [ ] **Step 3:** Move remaining shared packages (whole directories):
```bash
git mv pkg/grpc/interceptor.go internal/grpc/interceptor.go
git mv pkg/grpc/interceptor_test.go internal/grpc/interceptor_test.go

git mv pkg/config/config.go internal/config/config.go
git mv pkg/config/config_test.go internal/config/config_test.go

git mv pkg/logging/logging.go internal/logging/logging.go
git mv pkg/logging/logging_test.go internal/logging/logging_test.go

git mv pkg/metrics/metrics.go internal/metrics/metrics.go
git mv pkg/metrics/metrics_test.go internal/metrics/metrics_test.go
```

- [ ] **Step 4:** Remove empty directories:
```bash
rmdir pkg/storage pkg/grpc pkg/config pkg/logging pkg/metrics
```

- [ ] **Step 5:** Verify only `protocol/` remains in `pkg/`:
```bash
ls pkg/
# Expected output: protocol
```

---

### Task 6: Update ALL import paths

**Files:** Every `.go` file in the repository (~60 files)

The module path is `github.com/thaolaptrinh/spatial-server`.

**Import path mapping (apply longest match first to avoid partial replacements):**

| Old | New |
|-----|-----|
| `pkg/entity` | `internal/game/entity` |
| `pkg/aoi` | `internal/game/aoi` |
| `pkg/zone` | `internal/game/zone` |
| `pkg/gateway` | `internal/gateway` |
| `pkg/session` | `internal/session` |
| `pkg/auth` | `internal/auth` |
| `pkg/room` | `internal/room` |
| `pkg/api` | `internal/room` |
| `pkg/game` | `internal/game` |
| `pkg/storage` | `internal/storage` |
| `pkg/grpc` | `internal/grpc` |
| `pkg/config` | `internal/config` |
| `pkg/logging` | `internal/logging` |
| `pkg/metrics` | `internal/metrics` |

**Unchanged:** `pkg/protocol`, `internal/types`, `internal/migration`

- [ ] **Step 1:** Bulk-update all Go files using sed (ordered by specificity):
```bash
find . -name '*.go' -not -path './vendor/*' | xargs sed -i \
  -e 's|/pkg/entity|/internal/game/entity|g' \
  -e 's|/pkg/aoi|/internal/game/aoi|g' \
  -e 's|/pkg/zone|/internal/game/zone|g' \
  -e 's|/pkg/gateway|/internal/gateway|g' \
  -e 's|/pkg/session|/internal/session|g' \
  -e 's|/pkg/auth|/internal/auth|g' \
  -e 's|/pkg/room|/internal/room|g' \
  -e 's|/pkg/api|/internal/room|g' \
  -e 's|/pkg/game|/internal/game|g' \
  -e 's|/pkg/storage|/internal/storage|g' \
  -e 's|/pkg/grpc|/internal/grpc|g' \
  -e 's|/pkg/config|/internal/config|g' \
  -e 's|/pkg/logging|/internal/logging|g' \
  -e 's|/pkg/metrics|/internal/metrics|g'
```

- [ ] **Step 2:** Also update bare references in Go files (comments, string literals):
```bash
find . -name '*.go' -not -path './vendor/*' | xargs sed -i \
  -e 's|pkg/entity|internal/game/entity|g' \
  -e 's|pkg/aoi|internal/game/aoi|g' \
  -e 's|pkg/zone|internal/game/zone|g' \
  -e 's|pkg/gateway|internal/gateway|g' \
  -e 's|pkg/session|internal/session|g' \
  -e 's|pkg/auth|internal/auth|g' \
  -e 's|pkg/room|internal/room|g' \
  -e 's|pkg/api|internal/room|g' \
  -e 's|pkg/game|internal/game|g' \
  -e 's|pkg/storage|internal/storage|g' \
  -e 's|pkg/grpc|internal/grpc|g' \
  -e 's|pkg/config|internal/config|g' \
  -e 's|pkg/logging|internal/logging|g' \
  -e 's|pkg/metrics|internal/metrics|g'
```

Note: Step 1 and Step 2 can be combined since `/pkg/entity` contains `pkg/entity`, and the first sed will have already replaced it. However, running both ensures bare references without a leading `/` are also caught. Since Step 1 replaces `/pkg/entity` which includes the `pkg/entity` substring, Step 2 is redundant after Step 1 — EXCEPT for cases where `pkg/entity` appears without a leading `/` (e.g., at the start of a line in comments). **Use only Step 2** (bare `pkg/entity` pattern) as it catches both `/pkg/entity` and standalone `pkg/entity`.

**CORRECTED — use this single command:**
```bash
find . -name '*.go' -not -path './vendor/*' | xargs sed -i \
  -e 's|pkg/entity|internal/game/entity|g' \
  -e 's|pkg/aoi|internal/game/aoi|g' \
  -e 's|pkg/zone|internal/game/zone|g' \
  -e 's|pkg/gateway|internal/gateway|g' \
  -e 's|pkg/session|internal/session|g' \
  -e 's|pkg/auth|internal/auth|g' \
  -e 's|pkg/room|internal/room|g' \
  -e 's|pkg/api|internal/room|g' \
  -e 's|pkg/game|internal/game|g' \
  -e 's|pkg/storage|internal/storage|g' \
  -e 's|pkg/grpc|internal/grpc|g' \
  -e 's|pkg/config|internal/config|g' \
  -e 's|pkg/logging|internal/logging|g' \
  -e 's|pkg/metrics|internal/metrics|g'
```

- [ ] **Step 3:** Verify no stale references remain:
```bash
# Should find ZERO matches (pkg/protocol is the only allowed pkg/ reference)
grep -rn 'pkg/\(gateway\|auth\|session\|room\|api\|game\|entity\|aoi\|zone\|storage\|grpc\|config\|logging\|metrics\)' --include='*.go' .
```

- [ ] **Step 4:** Attempt build to catch remaining issues:
```bash
go build ./... 2>&1 | head -40
```

---

### Task 7: Fix package-qualified references in code

After moving packages and updating import paths, some code references the old package names.

**Key changes needed:**

- [ ] **Step 1:** In files that imported `pkg/api` as package `api`, update references from `api.NewSpatialServerAPI` → `room.NewSpatialServerAPI`. Search for all such references:
```bash
grep -rn '\bapi\.' --include='*.go' apps/ internal/ tests/ | grep -v '_test.go' | grep -v 'google\|proto\|grpc\|http\|context'
```

- [ ] **Step 2:** In files that imported `pkg/entity` as package `entity`, the import path changed but the package name stays `entity`. No code changes needed — just verify the import path is correct.

- [ ] **Step 3:** Fix any remaining import alias issues. The `api` → `room` package rename is the main one. Check `apps/room-service/main.go` and any test files that reference the API package.

- [ ] **Step 4:** Build again:
```bash
go build ./... 2>&1 | head -40
```

- [ ] **Step 5:** Fix errors iteratively until build passes.

---

### Task 8: Create transport abstraction layer

**Files:**
- Create: `internal/transport/websocket/conn.go`
- Create: `internal/transport/websocket/coder/conn.go`

- [ ] **Step 1:** Create the Connection interface:
```go
// internal/transport/websocket/conn.go
package websocket

import (
    "context"
    "net/http"
)

// Connection abstracts a WebSocket connection for testability and transport independence.
type Connection interface {
    Read(ctx context.Context) ([]byte, error)
    Write(ctx context.Context, data []byte) error
    Close(statusCode uint16, reason string) error
    CloseNow() error
    SetReadLimit(n int64)
}

// Accepter creates new Connections from HTTP upgrades.
type Accepter interface {
    Accept(w http.ResponseWriter, r *http.Request) (Connection, error)
}
```

- [ ] **Step 2:** Create the coder/websocket implementation:
```go
// internal/transport/websocket/coder/conn.go
package coder

import (
    "context"
    "fmt"
    "net/http"

    ws "github.com/coder/websocket"

    "github.com/thaolaptrinh/spatial-server/internal/transport/websocket"
)

// Conn wraps github.com/coder/websocket.Conn to implement websocket.Connection.
type Conn struct {
    c *ws.Conn
}

// Accepter creates Connections using github.com/coder/websocket.
type Accepter struct {
    Options *ws.AcceptOptions
}

// Accept implements websocket.Accepter.
func (a Accepter) Accept(w http.ResponseWriter, r *http.Request) (websocket.Connection, error) {
    opts := a.Options
    if opts == nil {
        opts = &ws.AcceptOptions{}
    }
    c, err := ws.Accept(w, r, opts)
    if err != nil {
        return nil, fmt.Errorf("websocket accept: %w", err)
    }
    return Conn{c: c}, nil
}

// Read implements websocket.Connection.
func (c Conn) Read(ctx context.Context) ([]byte, error) {
    _, data, err := c.c.Read(ctx)
    if err != nil {
        return nil, fmt.Errorf("websocket read: %w", err)
    }
    return data, nil
}

// Write implements websocket.Connection.
func (c Conn) Write(ctx context.Context, data []byte) error {
    err := c.c.Write(ctx, ws.MessageBinary, data)
    if err != nil {
        return fmt.Errorf("websocket write: %w", err)
    }
    return nil
}

// Close implements websocket.Connection.
func (c Conn) Close(statusCode uint16, reason string) error {
    return c.c.Close(ws.StatusCode(statusCode), reason)
}

// CloseNow implements websocket.Connection.
func (c Conn) CloseNow() error {
    return c.c.CloseNow()
}

// SetReadLimit implements websocket.Connection.
func (c Conn) SetReadLimit(n int64) {
    c.c.SetReadLimit(n)
}
```

- [ ] **Step 3:** Verify build:
```bash
go build ./internal/transport/...
```

---

### Task 9: Split gateway handler.go

**Files:**
- Modify: `internal/gateway/handler.go` (remove relay logic)
- Create: `internal/gateway/relay.go` (gRPC relay orchestration)

- [ ] **Step 1:** Read current `internal/gateway/handler.go` to identify the relay function(s).

- [ ] **Step 2:** Extract relay logic into `relay.go`. The relay function(s) handle:
  - gRPC dial to game-server
  - Bidirectional pump goroutines (client→server, server→client)
  - Connection lifecycle management

- [ ] **Step 3:** `handler.go` should only contain:
  - HTTP route registration
  - Health endpoints (`/health`, `/live`, `/ready`)
  - WebSocket upgrade entry point (delegates to relay)
  - Drain/ready state management

- [ ] **Step 4:** If the handler currently imports `coder/websocket` directly, switch to the `transport.Accepter` interface. Inject the Accepter via the handler struct.

- [ ] **Step 5:** Build and test:
```bash
go build ./internal/gateway/...
go test ./internal/gateway/... -race -count=1
```

---

### Task 10: Full verification + commit

- [ ] **Step 1:** Build everything:
```bash
go build ./...
```

- [ ] **Step 2:** Run all tests with race detector:
```bash
go test ./... -race -count=1
```

- [ ] **Step 3:** Run go vet:
```bash
go vet ./...
```

- [ ] **Step 4:** Verify pkg/ only has protocol:
```bash
ls pkg/
# Must output: protocol
```

- [ ] **Step 5:** Commit the refactoring:
```bash
git add -A
git status
git commit -m "refactor: modular monolith — strict service boundaries

- Move service-private packages from pkg/ to internal/<service>/
- auth, session -> internal/ (cross-cutting, not service-owned)
- Split pkg/game mini-app into simulation/codec/npc + entity/aoi/zone subpackages
- Split pkg/storage into internal/storage/{room,game} domain repos
- Extract WebSocket transport abstraction (internal/transport/websocket/)
- Split gateway handler.go into handler.go + relay.go
- Delete dead code (pkg/game/peer.go)
- Only pkg/protocol/ remains in pkg/ (genuinely reusable wire format)
- Update all import paths across codebase
- 153 tests pass with -race"
```

---

### Task 11: Update documentation

**Files:**
- Modify: `docs/standards/dependency-rules.md`
- Modify: `docs/architecture/repository-structure.md`
- Modify: `AGENTS.md`

- [ ] **Step 1:** Update `dependency-rules.md` with the new boundary law:
  - Shared infrastructure is NOT service-owned
  - Services depend on shared infra, never on each other
  - Updated dependency graph

- [ ] **Step 2:** Update `repository-structure.md` with the new directory layout (from spec).

- [ ] **Step 3:** Update `AGENTS.md` structure block to match new layout.

- [ ] **Step 4:** Commit:
```bash
git add docs/standards/dependency-rules.md docs/architecture/repository-structure.md AGENTS.md
git commit -m "docs: update dependency rules + repo structure for modular monolith"
```
