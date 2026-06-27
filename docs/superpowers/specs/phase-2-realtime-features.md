# Phase 2 — Realtime Features

> **Last Updated:** 2026-06-27
> **Status:** Draft

## Purpose

Phase 1Finish leaves a durable, protocol-compliant, single-server vertical slice: a client connects, sees a static NPC, and can move around. But the slice is inert — nothing *happens* unless a client sends a packet. The platform cannot yet simulate a believable runtime, survive a crash, or tell an operator how it is performing.

Phase 2 turns the slice into a living, observable realtime experience. Four gaps are addressed:

1. **The world is static.** The single demo NPC in `apps/game-server/main.go` never moves. [ADR-023](../../adr/023-entity-model.md) describes an `entity.Lifecycle` but the interface lacks an `OnSimulate` hook, and there is no simulation loop driving non-player entities. NPCs need configurable behaviors (patrol, idle, wander) so a runtime is populated by autonomous motion.
2. **Clients cannot act.** Packets `0x07 EntityAction` and `0x08 EntityState` are defined in [ADR-010](../../adr/010-packet-protocol.md) and reserved as constants in `pkg/protocol/protocol.go`, but neither is produced or consumed. A client cannot jump, attack, or emote; the server cannot broadcast attribute changes (health, animation state).
3. **Crashes lose everything.** Entity positions live only in `pkg/game.Game.Entities`. A Game Server restart re-seeds from the static demo spawn. Periodic zone-state snapshots to PostgreSQL are required so a crashed Game Server can recover the last known entity layout.
4. **The platform is opaque.** There are no metrics, no gRPC observability, no way to answer "how many connections, what is the tick duration, what is the p95 Relay latency." Operators are blind.

**Phase 2 closes all four.** After this phase the single-server runtime is autonomous (NPCs move on tick), interactive (action packets), durable (crash-recoverable from snapshots), and observable (Prometheus + Grafana). This is the last single-server phase before distributed scaling in Phase 3.

## Scope

- NPC simulation loop with three configurable behaviors: `patrol` (waypoint loop), `idle` (stationary with optional bob), `wander` (random walk within a radius). NPCs update position on tick and emit `EntityMove` (0x06) to observers.
- `EntityAction` packet (0x07): client → server action commands (`jump`, `attack`, `interact`). Game Server dispatches to the owning entity's `OnAction` lifecycle hook.
- `EntityState` packet (0x08): server → client attribute/animation broadcasts (health, animation clip, status effects).
- Entity lifecycle hooks: extend `entity.Lifecycle` with `OnSimulate(dt)` and `OnAction(action)`. Existing `Spawn`/`Despawn`/`OnEnterZone`/`OnLeaveZone` retained.
- Periodic zone-state persistence: serialize every entity in a zone to a new `zone_state` table every N ticks (default 5s, configurable). On Game Server startup, load the latest snapshot for each assigned zone before serving traffic (crash recovery).
- `/metrics` endpoint on all three services (Prometheus text format): active connections, packets/sec, entity count, tick duration histogram, gRPC latency histograms, NPC count.
- gRPC server interceptors on all three services: recovery (panic → `codes.Internal`), logging (request + duration at `slog.Info`), metrics (Prometheus unary/streaming histograms).
- NPC spawn/despawn driven by config (`configs/game-server.yml`) and by `SpatialServerAPI` (runtime-scoped NPC templates, deferred — config-only in this phase).

**Out of scope:**
- Cross-zone AOI, ghost-entity handoff, zone migration (Phase 3)
- Session resumption, backpressure, rate limiting (Phase 4)
- Full Grafana dashboards / Loki / Alertmanager (Phase 5 — this phase ships `/metrics` + a scrape target only)
- Player-to-NPC combat resolution rules (only the action *dispatch* is wired; game logic is application-specific)
- NPC behavior scripting (Lua/embedded DSL) — behaviors are compiled Go strategies registered in a map

## Architecture

```
   Client (WSS)
      │  PositionUpdate(0x03) ──┐
      │  EntityAction(0x07)  ──┤   (jump / attack / interact)
      │                         │
      ▼                         ▼
  ┌──────────────────────────────────────────────────────┐
  │  Gateway :8080   /ws · /ready · /live · /metrics      │
  │  interceptors: recovery · logging · metrics           │
  └──────────────┬───────────────────────────────────────┘
                 │ gRPC Relay (bidi)  [interceptors: rec/log/met]
       ┌─────────▼───────────────────────────────────┐
       │  Game Server :9000                          │
       │  ┌────────────────────────────────────────┐ │
       │  │ Game.Run tick (50ms)                   │ │
       │  │  1. applyCmds                          │ │
       │  │  2. drain Inbox (0x03 move, 0x07 act)  │ │
       │  │  3. for each entity:                   │ │
       │  │       e.Lifecycle.OnSimulate(dt)       │ │
       │  │         └─ NPC behavior.step(pos)      │ │
       │  │  4. detectZoneBoundaries / updateAOI   │ │
       │  │  5. every N ticks → SnapshotZone()     │ │
       │  │       └─ pgx INSERT zone_state         │ │
       │  │  6. emit 0x06 Move / 0x08 State → Outbox│ │
       │  └────────────────────────────────────────┘ │
       │  Startup: LoadZoneSnapshot() before Serve  │
       │  /metrics: entities, NPCs, tick histogram  │
       └──────────┬──────────────────────────────────┘
                  │ gRPC (Register/Heartbeat/LookupZone/
                  │       SpatialServerAPI)
       ┌──────────▼──────────────┐   ┌────────────────────┐
       │  Room Service :9000     │   │  PostgreSQL        │
       │  + SpatialServerAPI     │   │  + zone_state      │
       │  /metrics · interceptors│   │  (migration 002)   │
       └─────────────────────────┘   └────────────────────┘
                                         ┌────────────────┐
   Prometheus ──scrape──► /metrics ◀─────│  Redis (ready) │
                                         └────────────────┘
```

## Components

### 1. Entity Lifecycle Hooks

Extend the `entity.Lifecycle` interface in `pkg/entity/entity.go` so the Game loop can drive autonomous behavior and inbound actions.

```go
type Lifecycle interface {
    Spawn()
    Despawn()
    OnEnterZone(zoneID types.ZoneID)
    OnLeaveZone(zoneID types.ZoneID)
    OnSimulate(dt time.Duration)        // new — called every tick
    OnAction(action string, payload []byte) // new — from EntityAction(0x07)
}
```

- **Modify `pkg/entity/entity.go`**: add the two methods to `Lifecycle`, add a `Behavior string` field (e.g. `"patrol"`, `"idle"`, `"wander"`) and a `Lifecycle` holder on `Entity`. Provide a no-op `BaseLifecycle{}` so player avatars (which do not simulate) embed it without reimplementing every method.
- The Game loop only invokes hooks when `Entity.Lifecycle != nil`, so existing player entities are unaffected.
- **Modify `pkg/entity/entity_test.go`**: cover the new interface methods.

### 2. NPC Simulation

- **New file `pkg/game/npc.go`**: defines `Behavior` interface (`Step(e *entity.Entity, dt time.Duration) (moved bool)`) and three implementations:
  - `PatrolBehavior{ Waypoints []types.Vector3 }` — cycles waypoints at a fixed speed; `moved=true` each tick until reaching the current target.
  - `IdleBehavior{ BobAmplitude, BobFreq float64 }` — vertical bob around a fixed origin; emits periodic state to keep clients in sync but does not change AOI cell.
  - `WanderBehavior{ Origin types.Vector3, Radius, Speed float64, rng *rand.Rand }` — picks a random target within `Radius`, walks to it, pauses, repeats.
- A registry `npcBehaviors map[string]func() Behavior` lets new behaviors register by string tag (consistent with ADR-023's string-typed entities). Unknown behavior strings fall back to `IdleBehavior` with a `slog.Warn`.
- `NPCLifecycle` embeds `entity.BaseLifecycle`, holds a `Behavior`, and implements `OnSimulate`: it calls `Step`, mutates `Entity.Position`, and signals the Game loop to update the AOI index and emit `EntityMove(0x06)`.
- **Modify `pkg/game/game.go`**: in `tick()`, after draining the Inbox and before `updateVisibility`, iterate `g.Entities` and call `OnSimulate(g.tickRate)` for any entity whose `Lifecycle != nil`. Track moved entities and feed their new positions to `g.aoi.Move` + enqueue `EntityMove` frames to observers' `Outbox` (reuse the existing visibility machinery).
- **Modify `apps/game-server/main.go`**: replace the single static demo NPC with config-driven spawning. Read `game.npcs` (a list of `{type, behavior, position, waypoints, radius}`) from `configs/game-server.yml`; on startup, for each entry, `entity.New(...)` + attach `NPCLifecycle{Behavior: npcBehaviors[behavior]()}` + `g.AddEntity`.

### 3. EntityAction (0x07) and EntityState (0x08)

- **Modify `proto/spatialserver/v1/common.proto`**: add
  ```proto
  message EntityAction  { string entity_id = 1; string action = 2; bytes payload = 3; int32 sequence = 4; int64 timestamp = 5; }
  message EntityState   { string entity_id = 1; string animation = 2; int32 health = 3; map<string, bytes> attributes = 4; int64 timestamp = 5; }
  ```
  Run `make proto` to regenerate. (Field numbers continue the existing sequence; no existing field is renumbered.)
- **Modify `pkg/game/game.go` `dispatch`**: handle `PacketIDEntityAction(0x07)` — `proto.Unmarshal` into `EntityAction`, look up the entity, validate that `pkt.ClientID == entity.OwnerID` (only the owning player may act on their avatar per ADR-023), then call `entity.Lifecycle.OnAction(action, payload)`. Unknown actions are logged and dropped (not an error).
- **New file `pkg/game/encode.go`** (extracted from `game.go`): add `encodeState(e, anim, health)` producing an `EntityState(0x08)` frame, alongside the existing `encodeSpawn`/`encodeDespawn`. The Game loop calls `encodeState` when `OnSimulate` or `OnAction` reports an attribute change (e.g. animation clip switch, health delta) and enqueues it to every observer in the entity's AOI set.
- `tools/client/main.go` gains a `-action` flag that, on Enter keypress, sends an `EntityAction(0x07)` frame and prints inbound `EntityState(0x08)` lines (`"STATE {id} anim={anim} hp={hp}"`).

### 4. Zone-State Persistence + Crash Recovery

- **New migration `pkg/storage/migrations/002_zone_state.up.sql`**:
  ```sql
  CREATE TABLE IF NOT EXISTS zone_state (
      zone_id     TEXT NOT NULL,
      runtime_id  TEXT NOT NULL REFERENCES runtimes(id) ON DELETE CASCADE,
      snapshot    JSONB NOT NULL,            -- [{id,type,x,y,z,attrs}, …]
      tick_count  BIGINT NOT NULL,
      taken_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
      PRIMARY KEY (zone_id, taken_at)
  );
  CREATE INDEX idx_zone_state_runtime ON zone_state(runtime_id, taken_at DESC);
  ```
  Plus `002_zone_state.down.sql` (`DROP TABLE zone_state;`). Run automatically by `migration.Run` on Game Server and Room Service startup.
- **New file `pkg/storage/zone_state.go`**: `ZoneStateRepository` with `Save(ctx, zoneID, runtimeID, snapshot, tick)` (`INSERT`) and `Latest(ctx, zoneID) (snapshot, tick, takenAt, err)` (`SELECT … ORDER BY taken_at DESC LIMIT 1`). Snapshot marshalling lives in `pkg/game` (it knows the entity layout); the repository accepts an opaque `[]byte`/JSON.
- **Modify `pkg/game/game.go`**: add a `persistTicker` counter; every `game.snapshot_interval` ticks (default 100 ticks = 5s at 50ms), call `SnapshotZone(zoneID)` which serializes `{id, type, x, y, z, attrs}` for every entity in the zone and writes via the repository. Snapshots are best-effort: a pgx error is logged at `slog.Warn` and the tick continues (persistence must never stall the loop).
- **Modify `apps/game-server/main.go`**: after `Register` + before `srv.Serve`, for each zone the server will own (initially: the single demo zone, later assigned via Phase 3 `AssignZone`), call `ZoneStateRepository.Latest`; if a snapshot exists, hydrate entities from it (attaching the right `Lifecycle` based on `type`/`behavior`) instead of seeding from config. Config seeding only runs when no snapshot is found (fresh runtime).

### 5. Observability: `/metrics` + gRPC Interceptors

- **New file `pkg/metrics/metrics.go`**: a `Registry` wrapping `prometheus` collectors — `ActiveConnections`, `PacketsPerSec` (counter, labeled by `direction` and `packet_id`), `EntityCount` (gauge, labeled by `type`), `TickDurationSeconds` (histogram), `GRPCRequestDurationSeconds` (histogram, labeled by `service`/`method`). Expose `Handler()` returning an `http.Handler` writing Prometheus text format (via `promhttp.HandlerFor`).
- **New file `pkg/grpc/interceptor.go`**: three server option funcs:
  - `RecoveryInterceptor` — `recover()` → `status.Error(codes.Internal, "internal error")` + `slog.Error` with stack.
  - `LoggingInterceptor` — logs method, duration, code at `slog.Info` (or `Warn` on error).
  - `MetricsInterceptor` — observes `GRPCRequestDurationSeconds`; handles both unary and streaming handlers.
  Export `ServerOptions(reg *metrics.Registry) []grpc.ServerOption` so each main wires all three with one line.
- **Modify all three mains**: `srv := grpc.NewServer(pkggrpc.ServerOptions(reg)…)`; add `mux.Handle("/metrics", metricsHandler)` (Gateway via its HTTP mux; Room Service and Game Server spin up a small `http.Server` on a sidecar port `metrics.port`, default 9100/9101/9102). The Game Server feeds `EntityCount`, `TickDurationSeconds` from its loop; the Gateway feeds `ActiveConnections`, `PacketsPerSec`.
- **Modify `configs/defaults.yml`**: add `metrics.port` per service.

## Data Flow

**NPC wander tick (new autonomous path):**
```
1. Game.tick() at 50ms
2. drain Inbox (no client packets this tick)
3. for e in Entities where e.Lifecycle != nil:
     npc := e.Lifecycle.(*NPCLifecycle)
     moved := npc.Behavior.Step(e, 50ms)   // WanderBehavior picks (x+dx, z+dz)
     if moved:
        g.aoi.Move(e.ID, e.Position)
4. updateVisibility(): observers of e see position change
5. enqueue EntityMove(0x06) frame to each observer's Outbox
6. drainOutbox → Relay → Gateway → WS → clients print "MOVE npc-3 → (…)"
```

**Client action (new interactive path):**
```
1. Client presses Space → EntityAction(0x07, action="jump", seq=42)
2. WS → Gateway (PacketsPerSec[client→server,0x07]++)
3. Relay KIND_DATA → Game Server Inbox
4. Game.dispatch: unmarshal EntityAction, lookup entity, verify OwnerID == clientID
5. e.Lifecycle.OnAction("jump", payload) → NPCLifecycle/PlayerLifecycle sets animation="jump"
6. Game encodes EntityState(0x08, anim="jump") → Outbox → all observers
7. Observer clients print "STATE p1 anim=jump hp=100"
```

**Crash recovery (new durability path):**
```
1. Game Server running, tick=4200, every 100 ticks → SnapshotZone(z1) → INSERT zone_state
2. Process crashes (OOM) at tick=4250
3. Orchestrator restarts Game Server → Register with Room Service (re-claims z1)
4. main: ZoneStateRepository.Latest(z1) → snapshot from tick=4200
5. Hydrate entities (3 NPCs + 0 players — players reconnect via Phase 4 session, not here)
6. Attach NPCLifecycle per entity behavior; serve traffic
7. Clients reconnect, see NPCs at their last-snapshotted positions (≤5s stale)
```

## Files Changed

| File | Action |
|------|--------|
| `pkg/entity/entity.go` | Modify — extend `Lifecycle` (`OnSimulate`, `OnAction`), add `Behavior` field, `BaseLifecycle` |
| `pkg/entity/entity_test.go` | Modify — cover new hooks + `BaseLifecycle` |
| `pkg/game/npc.go` | Create — `Behavior` interface + `Patrol`/`Idle`/`Wander` + registry |
| `pkg/game/npc_test.go` | Create — behavior unit tests (deterministic via seeded rng) |
| `pkg/game/encode.go` | Create — `encodeState`, extracted `encodeSpawn`/`encodeDespawn`/`encodeMove` |
| `pkg/game/game.go` | Modify — `OnSimulate` loop, 0x07 dispatch, 0x08 emit, snapshot ticker, `SnapshotZone` |
| `pkg/game/game_test.go` | Modify/ Create — simulate tick, action dispatch, snapshot trigger |
| `pkg/storage/zone_state.go` | Create — `ZoneStateRepository` (pgx) |
| `pkg/storage/zone_state_test.go` | Create — repository unit tests (Testcontainers pg) |
| `pkg/storage/migrations/002_zone_state.up.sql` | Create — `zone_state` table |
| `pkg/storage/migrations/002_zone_state.down.sql` | Create — rollback |
| `pkg/metrics/metrics.go` | Create — Prometheus registry + HTTP handler |
| `pkg/metrics/metrics_test.go` | Create — collector unit tests |
| `pkg/grpc/interceptor.go` | Create — recovery, logging, metrics interceptors |
| `pkg/grpc/interceptor_test.go` | Create — interceptor unit tests |
| `proto/spatialserver/v1/common.proto` | Modify — add `EntityAction`, `EntityState` messages |
| `proto/gen/spatialserver/v1/*.pb.go` | Regenerate — `make proto` |
| `apps/gateway/main.go` | Modify — wire metrics + interceptors, `/metrics` endpoint |
| `apps/room-service/main.go` | Modify — wire metrics + interceptors, `/metrics` sidecar |
| `apps/game-server/main.go` | Modify — NPC config spawn, `LoadZoneSnapshot` recovery, metrics + interceptors |
| `tools/client/main.go` | Modify — `-action` flag, `EntityAction(0x07)` send, `EntityState(0x08)` print |
| `configs/defaults.yml` | Modify — `metrics.port` per service, `game.snapshot_interval` |
| `configs/game-server.yml` | Modify — `game.npcs` list (type/behavior/position/waypoints/radius) |
| `go.mod` / `go.sum` | Modify — add `github.com/prometheus/client_golang` |
| `test/integration/realtime_test.go` | Modify — extend E2E test: assert NPC moves, action round-trips, snapshot recovers |
| `Makefile` | Modify — `make proto` already present; add `make bench` scaffold for tick benchmarks |

## References

- [ADR-003 AOI Strategy](../../adr/003-aoi-strategy.md)
- [ADR-010 Packet Protocol](../../adr/010-packet-protocol.md) — defines packets 0x07 / 0x08
- [ADR-021 Gateway Architecture](../../adr/021-gateway-architecture.md)
- [ADR-023 Entity Model](../../adr/023-entity-model.md) — lifecycle + string-typed entities
- [Master Phase Roadmap](./master-phase-roadmap.md)
- [Phase 1Finish spec](./phase-1finish-hardened-vertical-slice.md) — predecessor (depends on)
- [Phase 3 spec](./phase-3-distributed-scaling.md) — successor (multi-server)
- [Logging standard](../../standards/logging.md)
- [gRPC convention](../../standards/grpc-convention.md)
