# Production Gap Closure Report

> **Date:** 2026-06-28
> **Baseline:** v0.1.0-alpha (Architecture Frozen)
> **Prior Audit:** EVIDENCE_AUDIT.md

---

## Summary

3 **Critical** production gaps from the Evidence Audit have been closed. The repository remains at 211 passing tests, race-clean, vet-clean, build-clean.

| Gap | Severity | Status | Verification |
|-----|----------|--------|-------------|
| NF1 — Orphan zone detection never executes | Critical | **CLOSED** | `ProductionSweeper` wired in `apps/room-service/main.go` |
| NF2 — Disconnected entities permanently leak | Critical | **CLOSED** | `SweepDisconnected()` called in `tick()` |
| NF4 — Mutex serializes reads in RuntimeStore | High | **CLOSED** | `RWMutex` with `RLock()` for reads |

---

## F1: Orphan Zone Detection (NF1)

### Problem
The `Sweeper` (`internal/room/sweeper.go`) was defined and tested but never started in the Room Service binary. When a Game Server crashed, its heartbeat stopped updating PG, but no component detected this or reassigned its zones.

### Solution
Created `ProductionSweeper` (`internal/room/sweeper_production.go`) using the existing `ServerStore` and `ZoneStore` interfaces — no dependency on the in-memory `ServerRegistry`/`ZoneOwnership` maps. Added PG-backed methods:
- `ServerRepository.ListDead(ctx, since)` — query: `SELECT id FROM game_servers WHERE status='active' AND last_heartbeat < NOW() - $1::INTERVAL`
- `ServerRepository.MarkShutdown(ctx, id)` — query: `UPDATE game_servers SET status='shutdown' WHERE id=$1`
- `ZoneRepository.ListByServer(ctx, serverID)` — query: `SELECT id FROM zones WHERE server_id=$1`

### Wiring
```go
// apps/room-service/main.go:180-189
sweeper := room.NewProductionSweeper(service.servers, service.zones, service.allocator, service.fanout, room.SweeperConfig{
    Interval:      5 * time.Second,
    MissThreshold: 15 * time.Second,
})
sweeperCtx, sweeperCancel := context.WithCancel(context.Background())
defer sweeperCancel()
go sweeper.Run(sweeperCtx)
```

### Recovery Sequence
1. Sweeper runs every 5s
2. Queries PG for servers with heartbeat >15s stale
3. Marks dead servers `shutdown` in PG
4. Lists zones owned by dead server (`ListByServer`)
5. Releases each orphaned zone (`ZoneStore.Release`)
6. Selects new owner via `LeastLoadedAllocator`
7. Claims zone for new owner (`ZoneStore.Claim`)
8. Broadcasts ownership change via `WatcherFanout`

### Files Changed
- `apps/room-service/main.go:29,180-189` — wired import + sweeper start
- `internal/room/sweeper_production.go` — new file (117 lines)
- `internal/storage/room/server_repo.go:69-85` — `ListDead`, `MarkShutdown`
- `internal/storage/room/zone_repo.go:64-77` — `ListByServer`

### Verification
- `go build ./apps/room-service/` — ✅
- `TestServerRepository_ListDead_ReturnsTimeoutServers` — ✅
- `TestZoneRepository_ListByServer` — ✅

---

## F2: Disconnected Entity Cleanup (NF2)

### Problem
`SweepDisconnected()` (`internal/game/lifecycle.go:56-77`) existed but was never called from the tick loop. When a player disconnected, `MarkDisconnected` set the entity to `SessionDisconnected`, but the entity remained permanently in the entity map, AOI grid, and zone — visible to all other players, consuming memory forever.

### Solution
Added `g.SweepDisconnected()` call in `tick()` at `internal/game/simulation.go:407`, after ghost sweep and before metrics reporting.

```go
// internal/game/simulation.go:406-407 (tick function)
g.sweepGhosts()
g.SweepDisconnected()
```

### Behavior
- Each tick, disconnected entities past the `reconnectWindow` (30s) are removed from:
  - `g.Entities` (entity map)
  - `g.entityZone` (zone mapping)
  - `g.entityAOI` (visibility state)
  - `g.sessionStates` (session tracker)
  - Zone's AOI grid (`grid.Leave`)
- Other clients in AOI receive despawn events on the next `updateVisibility` cycle

### Files Changed
- `internal/game/simulation.go:407` — one-line addition

### Verification
- `go build ./...` — ✅
- `TestTick_CallsSweepDisconnected_EntityDespawnedAfterReconnectWindow` — ✅
- `TestTick_DoesNotDespawnRecentlyDisconnectedEntities` — ✅

---

## F3: RWMutex for Read Operations (NF4)

### Problem
`MemoryRuntimeStore` used `sync.Mutex` for all operations, including `Get()` and `List()` which are read-only. Under concurrent API calls from the Business Backend, all reads serialized on a single exclusive lock.

### Solution
Changed `sync.Mutex` to `sync.RWMutex`. `Get()` and `List()` now use `RLock()`/`RUnlock()`. `Create()` and `Destroy()` continue to use exclusive `Lock()`/`Unlock()`.

### Files Changed
- `internal/room/api.go:162` — type change
- `internal/room/api.go:192-193` — `Get()` uses `RLock`/`RUnlock`
- `internal/room/api.go:213-214` — `List()` uses `RLock`/`RUnlock`

### Verification
- `go test -race ./internal/room/...` — ✅
- Existing `MemoryRuntimeStore` tests pass unchanged — ✅

---

## Validation Report

### Build
```
go build ./...  →  SUCCESS
```

### Unit Tests
```
go test ./internal/... ./pkg/... ./apps/... -count=1  →  211 passed, 25 packages
```

### Race Detection
```
go test -race ./internal/... ./pkg/... ./apps/... -count=1  →  211 passed, 0 races
```

### Static Analysis
```
go vet ./...  →  No issues found
```

### New Tests Added (4)

| Test | Package | Proves |
|------|---------|--------|
| `TestTick_CallsSweepDisconnected_EntityDespawnedAfterReconnectWindow` | `internal/game` | `tick()` calls `SweepDisconnected()`, entity despawns after 30s window |
| `TestTick_DoesNotDespawnRecentlyDisconnectedEntities` | `internal/game` | Entity within reconnect window survives `tick()` |
| `TestServerRepository_ListDead_ReturnsTimeoutServers` | `internal/storage/room` | PG query detects servers with stale heartbeat |
| `TestZoneRepository_ListByServer` | `internal/storage/room` | PG query returns zones by owner, empty for non-owner |

---

## Files Changed Summary

| File | Change |
|------|--------|
| `apps/room-service/main.go` | +5 lines (import + sweeper wiring) |
| `internal/room/sweeper_production.go` | New file (117 lines) |
| `internal/room/api.go` | +3 lines (Mutex→RWMutex + RLock) |
| `internal/game/simulation.go` | +1 line (SweepDisconnected call) |
| `internal/storage/room/server_repo.go` | +16 lines (ListDead, MarkShutdown) |
| `internal/storage/room/zone_repo.go` | +13 lines (ListByServer) |
| `internal/game/lifecycle_test.go` | +27 lines (2 regression tests) |
| `internal/storage/room/server_repo_test.go` | +33 lines (1 regression test) |
| `internal/storage/room/zone_repo_test.go` | +22 lines (1 regression test) |
| `docs/adr/011-failure-recovery.md` | +14 lines (status table) |
| `docs/milestones/v0.1.0-alpha.md` | +13 lines (gap closure section) |
| `CHANGELOG.md` | +28 lines (new section) |

---

## Remaining Known Gaps (Deferred)

| Gap | Severity | Target Phase |
|-----|----------|-------------|
| Room Service HA failover (K3s Lease API) | High | Phase 3 |
| PostgreSQL crash graceful degradation | High | Phase 3 |
| Redis crash graceful degradation | Medium | Phase 3 |
| Gateway per-connection gRPC pooling | Medium | Phase 4 |
| Snapshot format versioning | Low | Phase 4 |
| pprof endpoints on container ports | Low | Phase 4 |
| Chaos engineering tests | Medium | Phase 4 |

---

## Repository State

```
BUILD:   go build ./...   ✅
TEST:    go test ./...    211 passed, 25 packages
RACE:    go test -race    211 passed, 0 races
VET:     go vet ./...     No issues
LINT:    deferred (requires golangci-lint binary)
```

---

## References

- `EVIDENCE_AUDIT.md` — Complete evidence audit of all architectural findings
- `ARCHITECTURE_REVIEW.md` — Independent Principal Engineer architecture review
- `docs/adr/011-failure-recovery.md` — ADR with implementation status
- `docs/milestones/v0.1.0-alpha.md` — Milestone report with gap closure section
- `CHANGELOG.md` — Full changelog
