# Phase 2 — Realtime Features Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn the inert single-server slice into a living, interactive, crash-recoverable, observable realtime runtime: autonomous NPCs, action/state packets, zone-state snapshots, Prometheus metrics, and gRPC interceptors.

**Architecture:** `entity.Lifecycle` gains `OnSimulate`/`OnAction`; an NPC loop drives `Behavior` strategies (patrol/idle/wander) every tick. New `EntityAction(0x07)`/`EntityState(0x08)` packets flow client↔server. Every N ticks the Game serializes entities to a `zone_state` table for crash recovery. A shared `pkg/metrics.Registry` + `pkg/grpc` interceptors make every service observable.

**Tech Stack:** Go 1.25, `google.golang.org/protobuf`, `github.com/prometheus/client_golang`, `github.com/jackc/pgx/v5`, gRPC interceptors, koanf config

**Preconditions (delivered by Phase 1Finish):** 8-byte `protocol.Encode(id,payload,compress,seq)` + `Decode → (version,id,payload,compressed,seq,err)`; `pkg/config.Config` with `Game`/`Gateway` sections + `config.Load(...)`; pgx repos + `migration.Run(pool,dir)`.

**Pre-existing files (checked before writing):** `pkg/entity/entity.go` (`Lifecycle` 4 methods, `entity.New`); `pkg/game/game.go` (`tick()` order `applyCmds → drain Inbox → detectZoneBoundaries → updateVisibility → sweepGhosts`; `dispatch` handles only `PacketIDPositionUpdate`; inline `encodeSpawn`/`encodeDespawn`); `pkg/protocol/protocol.go` (`PacketIDEntityAction=0x07`, `PacketIDEntityState=0x08` already declared); `proto/spatialserver/v1/common.proto` (has `EntitySnapshot`/`EntityUpdate`, no action/state); `apps/game-server/main.go` (seeds one static NPC).

---

### Task 1: Entity lifecycle hooks

**Files:**
- Modify: `pkg/entity/entity.go`, `pkg/entity/entity_test.go` (create if absent)

- [ ] **Step 1: Add failing test**

  Append to `pkg/entity/entity_test.go` (create `package entity`):

  ```go
  func TestBaseLifecycle_NoOp(t *testing.T) {
  	var l Lifecycle = BaseLifecycle{}
  	assert.NotPanics(t, func() {
  		l.Spawn(); l.Despawn(); l.OnEnterZone("z1"); l.OnLeaveZone("z1")
  		l.OnSimulate(time.Millisecond); l.OnAction("jump", nil)
  	})
  }

  func TestEntity_LifecycleAttach(t *testing.T) {
  	rec := &recordingLifecycle{}
  	e := New("e1", "npc", types.RuntimeID("r1"))
  	e.Lifecycle = rec
  	e.Lifecycle.OnSimulate(50 * time.Millisecond)
  	e.Lifecycle.OnAction("attack", []byte("x"))
  	assert.Equal(t, 1, rec.simCount)
  	assert.Equal(t, "attack", rec.lastAction)
  }

  type recordingLifecycle struct {
  	BaseLifecycle
  	simCount   int
  	lastAction string
  }

  func (r *recordingLifecycle) OnSimulate(time.Duration) { r.simCount++ }
  func (r *recordingLifecycle) OnAction(a string, _ []byte) { r.lastAction = a }
  ```

- [ ] **Step 2: Run test to verify failure**

  Run: `go test ./pkg/entity/... -run 'TestBaseLifecycle|TestEntity_LifecycleAttach' -v`
  Expected: FAIL (`OnSimulate`/`OnAction`/`BaseLifecycle` undefined).

- [ ] **Step 3: Extend `pkg/entity/entity.go`**

  Replace the `Lifecycle` interface:
  ```go
  type Lifecycle interface {
  	Spawn()
  	Despawn()
  	OnEnterZone(zoneID types.ZoneID)
  	OnLeaveZone(zoneID types.ZoneID)
  	OnSimulate(dt time.Duration)
  	OnAction(action string, payload []byte)
  }

  type BaseLifecycle struct{}

  func (BaseLifecycle) Spawn()                      {}
  func (BaseLifecycle) Despawn()                    {}
  func (BaseLifecycle) OnEnterZone(types.ZoneID)    {}
  func (BaseLifecycle) OnLeaveZone(types.ZoneID)    {}
  func (BaseLifecycle) OnSimulate(time.Duration)    {}
  func (BaseLifecycle) OnAction(string, []byte)     {}
  ```
  Add two fields to `Entity`: `Behavior string` and `Lifecycle Lifecycle`.

- [ ] **Step 4: Run test to verify pass + commit**

  Run: `go test ./pkg/entity/... -v`
  Expected: PASS.
  ```bash
  git add pkg/entity/entity.go pkg/entity/entity_test.go
  git commit -m "feat(entity): add OnSimulate/OnAction hooks + BaseLifecycle"
  ```

---

### Task 2: NPC simulation loop

**Files:**
- Create: `pkg/game/npc.go`, `pkg/game/npc_test.go`
- Modify: `pkg/game/game.go` (`tick()` + new `simulate`/`encodeMove` helpers)
- Modify: `apps/game-server/main.go`, `pkg/config/config.go` (`GameConfig.NPCs`), `configs/game-server.yml`

- [ ] **Step 1: Add failing behavior tests**

  Create `pkg/game/npc_test.go`:

  ```go
  package game

  import (
  	"math/rand", "testing", "time"
  	"github.com/stretchr/testify/assert"
  	"github.com/thaolaptrinh/spatial-server/internal/types"
  	"github.com/thaolaptrinh/spatial-server/pkg/entity"
  )

  func TestPatrolBehavior_StepsTowardWaypoint(t *testing.T) {
  	b := PatrolBehavior{Speed: 10, Waypoints: []types.Vector3{{X: 100}}}
  	e := entity.New("n1", "npc", types.RuntimeID("r1"))
  	assert.True(t, b.Step(e, time.Second))
  	assert.Greater(t, e.Position.X, 0.0)
  }

  func TestIdleBehavior_NoHorizontalDrift(t *testing.T) {
  	b := IdleBehavior{BobAmplitude: 1, BobFreq: 1}
  	e := entity.New("n2", "npc", types.RuntimeID("r1"))
  	e.Position = types.Vector3{X: 5}
  	b.Step(e, time.Second)
  	assert.Equal(t, 5.0, e.Position.X)
  }

  func TestWanderBehavior_StaysWithinRadius(t *testing.T) {
  	b := WanderBehavior{Origin: types.Vector3{X: 50, Z: 50}, Radius: 20, Speed: 100,
  		rng: rand.New(rand.NewSource(1))}
  	e := entity.New("n3", "npc", types.RuntimeID("r1"))
  	e.Position = types.Vector3{X: 50, Z: 50}
  	for i := 0; i < 50; i++ { b.Step(e, 100*time.Millisecond) }
  	dx, dz := e.Position.X-50, e.Position.Z-50
  	assert.LessOrEqual(t, dx*dx+dz*dz, 900.0)
  }

  func TestRegistry_FallbackToIdle(t *testing.T) {
  	_, ok := newBehavior("patrol").(*PatrolBehavior); assert.True(t, ok)
  	_, ok = newBehavior("nope").(*IdleBehavior); assert.True(t, ok)
  }
  ```

- [ ] **Step 2: Run test to verify failure**

  Run: `go test ./pkg/game/... -run 'TestPatrol|TestIdle|TestWander|TestRegistry' -v`
  Expected: FAIL (types undefined).

- [ ] **Step 3: Implement `pkg/game/npc.go`**

  ```go
  package game

  import (
  	"log/slog", "math", "math/rand", "time"
  	"github.com/thaolaptrinh/spatial-server/internal/types"
  	"github.com/thaolaptrinh/spatial-server/pkg/entity"
  )

  type Behavior interface {
  	Step(e *entity.Entity, dt time.Duration) (moved bool)
  }

  type PatrolBehavior struct {
  	Speed     float64
  	Waypoints []types.Vector3
  	target    int
  }

  func (p *PatrolBehavior) Step(e *entity.Entity, dt time.Duration) bool {
  	if len(p.Waypoints) == 0 { return false }
  	wp := p.Waypoints[p.target]
  	delta := p.Speed * dt.Seconds()
  	moveAxis(&e.Position.X, wp.X, delta)
  	moveAxis(&e.Position.Z, wp.Z, delta)
  	if e.Position.X == wp.X && e.Position.Z == wp.Z {
  		p.target = (p.target + 1) % len(p.Waypoints)
  	}
  	return true
  }

  type IdleBehavior struct {
  	BobAmplitude, BobFreq, phase float64
  }

  func (i *IdleBehavior) Step(e *entity.Entity, dt time.Duration) bool {
  	i.phase += i.BobFreq * dt.Seconds()
  	e.Position.Y = i.BobAmplitude * math.Sin(i.phase)
  	return false
  }

  type WanderBehavior struct {
  	Origin types.Vector3
  	Radius, Speed float64
  	rng    *rand.Rand
  	target types.Vector3
  	pause  time.Duration
  }

  func (w *WanderBehavior) Step(e *entity.Entity, dt time.Duration) bool {
  	if w.rng == nil { w.rng = rand.New(rand.NewSource(time.Now().UnixNano())) }
  	if w.pause > 0 { w.pause -= dt; return false }
  	if w.target == (types.Vector3{}) {
  		angle := w.rng.Float64() * 2 * math.Pi
  		r := w.rng.Float64() * w.Radius
  		w.target = types.Vector3{X: w.Origin.X + r*math.Cos(angle), Z: w.Origin.Z + r*math.Sin(angle)}
  	}
  	delta := w.Speed * dt.Seconds()
  	moved := moveAxis(&e.Position.X, w.target.X, delta) | moveAxis(&e.Position.Z, w.target.Z, delta)
  	if !moved { w.target = types.Vector3{}; w.pause = 500 * time.Millisecond }
  	return true
  }

  func moveAxis(pos *float64, target, delta float64) bool {
  	if *pos == target { return false }
  	if math.Abs(target-*pos) <= delta { *pos = target; return true }
  	if target > *pos { *pos += delta } else { *pos -= delta }
  	return true
  }

  var behaviorFactories = map[string]func() Behavior{
  	"patrol": func() Behavior { return &PatrolBehavior{Speed: 10} },
  	"idle":   func() Behavior { return &IdleBehavior{BobAmplitude: 0.5, BobFreq: 2} },
  	"wander": func() Behavior { return &WanderBehavior{Radius: 20, Speed: 10} },
  }

  func newBehavior(tag string) Behavior {
  	if f, ok := behaviorFactories[tag]; ok { return f() }
  	slog.Warn("unknown NPC behavior, falling back to idle", slog.String("behavior", tag))
  	return behaviorFactories["idle"]()
  }

  type NPCLifecycle struct {
  	entity.BaseLifecycle
  	Behavior Behavior
  }
  ```

- [ ] **Step 4: Run behavior tests to verify pass**

  Run: `go test ./pkg/game/... -run 'TestPatrol|TestIdle|TestWander|TestRegistry' -v`
  Expected: PASS.

- [ ] **Step 5: Add simulate loop to `pkg/game/game.go`**

  In `tick()` replace the `default:` branch of the Inbox drain loop with:
  ```go
  		default:
  			g.simulate(g.tickRate)
  			g.detectZoneBoundaries()
  			g.updateVisibility()
  			g.sweepGhosts()
  			return
  ```
  Add the `simulate` method + a move-encoder helper:
  ```go
  func (g *Game) simulate(dt time.Duration) {
  	g.mu.Lock()
  	defer g.mu.Unlock()
  	for id, e := range g.Entities {
  		lc, ok := e.Lifecycle.(*NPCLifecycle)
  		if !ok || lc == nil || lc.Behavior == nil { continue }
  		before := e.Position
  		if lc.Behavior.Step(e, dt) {
  			g.aoi.Move(id, e.Position)
  			g.enqueueMove(id, e.Position)
  		}
  		_ = before
  	}
  }

  func (g *Game) enqueueMove(id types.EntityID, pos types.Vector3) {
  	frame := encodeMove(id, pos)
  	for _, obsID := range g.aoi.EntitiesInRange(pos, g.aoiRadius) {
  		if obsID == id { continue }
  		select {
  		case g.Outbox <- OutboundPacket{ClientID: string(obsID), Data: frame}:
  		default:
  		}
  	}
  }

  func encodeMove(id types.EntityID, pos types.Vector3) []byte {
  	b, _ := proto.Marshal(&v1.EntityUpdate{
  		EntityId: string(id), Position: &v1.Vector3{X: pos.X, Y: pos.Y, Z: pos.Z},
  	})
  	return protocol.Encode(protocol.PacketIDEntityMove, b, false, 0)
  }
  ```

- [ ] **Step 6: Config-driven NPC spawn in `apps/game-server/main.go`**

  - Add to `pkg/config/config.go` `GameConfig`: `NPCs []NPCSpec \`koanf:"npcs"\`` and define `type NPCSpec struct { Type string \`koanf:"type"\`; Behavior string \`koanf:"behavior"\`; Position types.Vector3 \`koanf:"position"\`; Waypoints []types.Vector3 \`koanf:"waypoints"\`; Radius float64 \`koanf:"radius"\` }` (import `types`). 
    Note: `koanf` flattens nested structs by field tag; `Vector3`'s JSON tags (`x`/`y`/`z`) are read during `Unmarshal`, so `position: {x: 100, y: 0, z: 100}` maps correctly.
  - Replace the static `g.AddEntity(entity.New(...))` line with a loop over `cfg.Game.NPCs`:
    ```go
    for _, spec := range cfg.Game.NPCs {
    	npc := entity.New(types.NewEntityID(), spec.Type, types.RuntimeID(""))
    	npc.Position = spec.Position
    	npc.Behavior = spec.Behavior
    	npc.Lifecycle = &game.NPCLifecycle{Behavior: newBehaviorFor(spec)}
    	g.AddEntity(npc)
    }
    ```
  - Add helper:
    ```go
    func newBehaviorFor(spec config.NPCSpec) game.Behavior {
    	switch spec.Behavior {
    	case "patrol": return &game.PatrolBehavior{Speed: 10, Waypoints: spec.Waypoints}
    	case "wander": return &game.WanderBehavior{Origin: spec.Position, Radius: spec.Radius, Speed: 10}
    	default: return &game.IdleBehavior{BobAmplitude: 0.5, BobFreq: 2}
    	}
    }
    ```
  - Replace the inline koanf block in `main()` with `cfg, err := config.Load("configs/defaults.yml", "configs/game-server.yml")`; read `tickRate := cfg.Game.TickRate`, `gRPCPort := cfg.GRPC.Port`, `host := cfg.GRPC.Host`. Import `pkg/config`.
  - Append to `configs/game-server.yml`:
    ```yaml
      npcs:
        - type: npc
          behavior: patrol
          position: {x: 100, y: 0, z: 100}
          waypoints: [{x: 100, y: 0, z: 100}, {x: 110, y: 0, z: 110}]
        - type: npc
          behavior: wander
          position: {x: 90, y: 0, z: 90}
          radius: 15
    ```

- [ ] **Step 7: Verify + commit**

  Run: `go build ./... && go test ./pkg/game/... ./pkg/entity/... -v`
  Expected: PASS.
  ```bash
  git add pkg/game/npc.go pkg/game/npc_test.go pkg/game/game.go apps/game-server/main.go pkg/config/config.go configs/game-server.yml
  git commit -m "feat(game): NPC simulation loop with patrol/idle/wander behaviors"
  ```

---

### Task 3: EntityAction (0x07) + EntityState (0x08) packets

**Files:**
- Modify: `proto/spatialserver/v1/common.proto`
- Create: `pkg/game/encode.go`
- Modify: `pkg/game/game.go` (`dispatch`)
- Modify: `tools/client/main.go`

- [ ] **Step 1: Add proto messages + regenerate**

  Append to `proto/spatialserver/v1/common.proto`:
  ```proto
  message EntityAction {
    string entity_id = 1;
    string action = 2;
    bytes payload = 3;
    int32 sequence = 4;
    int64 timestamp = 5;
  }
  message EntityState {
    string entity_id = 1;
    string animation = 2;
    int32 health = 3;
    map<string, bytes> attributes = 4;
    int64 timestamp = 5;
  }
  ```
  Run: `make proto`
  Expected: `proto/gen/spatialserver/v1/common.pb.go` gains `EntityAction`/`EntityState`.

- [ ] **Step 2: Add failing dispatch test**

  Append to `pkg/game/game_test.go` (create `package game` if needed; the test exercises owner-checked action routing):

  ```go
  func TestDispatch_EntityAction_RoutesToLifecycle(t *testing.T) {
  	g := New(types.ServerID("s1"))
  	e := entity.New(types.EntityID("p1"), "avatar", types.RuntimeID("r1"))
  	e.OwnerID = types.ServerID("p1")
  	rec := &actionRecorder{}
  	e.Lifecycle = rec
  	g.AddEntity(e)
  	b, _ := proto.Marshal(&v1.EntityAction{EntityId: "p1", Action: "jump"})
  	g.dispatch(InboundPacket{
  		ClientID: "p1",
  		Data:     protocol.Encode(protocol.PacketIDEntityAction, b, false, 0),
  	})
  	assert.Equal(t, "jump", rec.lastAction)
  }

  type actionRecorder struct {
  	entity.BaseLifecycle
  	lastAction string
  }

  func (a *actionRecorder) OnAction(s string, _ []byte) { a.lastAction = s }
  ```

- [ ] **Step 3: Run test to verify failure**

  Run: `go test ./pkg/game/... -run TestDispatch_EntityAction -v`
  Expected: FAIL (0x07 not handled).

- [ ] **Step 4: Create `pkg/game/encode.go`**

  ```go
  package game

  import (
  	"github.com/thaolaptrinh/spatial-server/internal/types"
  	"github.com/thaolaptrinh/spatial-server/pkg/entity"
  	"github.com/thaolaptrinh/spatial-server/pkg/protocol"
  	v1 "github.com/thaolaptrinh/spatial-server/proto/gen/spatialserver/v1"
  	"google.golang.org/protobuf/proto"
  )

  func encodeSpawnFrame(e *entity.Entity) []byte {
  	b, _ := proto.Marshal(&v1.EntitySnapshot{
  		EntityId: string(e.ID), Type: e.Type,
  		Position: &v1.Vector3{X: e.Position.X, Y: e.Position.Y, Z: e.Position.Z},
  	})
  	return protocol.Encode(protocol.PacketIDEntitySpawn, b, false, 0)
  }

  func encodeDespawnFrame(id types.EntityID) []byte {
  	b, _ := proto.Marshal(&v1.EntityID{Id: string(id)})
  	return protocol.Encode(protocol.PacketIDEntityDespawn, b, false, 0)
  }

  func encodeState(id types.EntityID, animation string, health int32) []byte {
  	b, _ := proto.Marshal(&v1.EntityState{EntityId: string(id), Animation: animation, Health: health})
  	return protocol.Encode(protocol.PacketIDEntityState, b, false, 0)
  }
  ```
  In `pkg/game/game.go`: delete the old inline `encodeSpawn`/`encodeDespawn`/`encodeMove` (Task 2 added one); call `encodeSpawnFrame`/`encodeDespawnFrame` from `updateVisibility`/`enqueueMove` instead. (`encodeMove` from Task 2 stays, or move it into `encode.go` and delete the duplicate.)

- [ ] **Step 5: Add the 0x07 branch to `dispatch` in `pkg/game/game.go`**

  After the existing `PacketIDPositionUpdate` block:
  ```go
  	if id == protocol.PacketIDEntityAction {
  		var act v1.EntityAction
  		if err := proto.Unmarshal(payload, &act); err != nil { return }
  		e, ok := g.Entities[types.EntityID(act.GetEntityId())]
  		if !ok { return }
  		if e.OwnerID != types.ServerID(pkt.ClientID) { return }
  		if e.Lifecycle != nil {
  			e.Lifecycle.OnAction(act.GetAction(), act.GetPayload())
  		}
  	}
  ```

- [ ] **Step 6: Run test to verify pass**

  Run: `go test ./pkg/game/... -v`
  Expected: PASS.

- [ ] **Step 7: Add `-action` flag + `EntityState` print to `tools/client/main.go`**

  Add flag: `action := flag.String("action", "", "EntityAction to send on connect (e.g. jump)")`. After the first successful WS write, if `*action != ""`, marshal `EntityAction{EntityId:*player, Action:*action, Timestamp:now.UnixMilli()}`, `protocol.Encode(PacketIDEntityAction, b, false, 1)`, and `conn.Write(MessageBinary, frame)`. In the read-loop switch add:
  ```go
  		case protocol.PacketIDEntityState:
  			var st spatialserverv1.EntityState
  			if err := proto.Unmarshal(payload, &st); err != nil { continue }
  			log.Printf("STATE %s anim=%s hp=%d", st.GetEntityId(), st.GetAnimation(), st.GetHealth())
  ```

- [ ] **Step 8: Verify + commit**

  Run: `go build ./... && go vet ./pkg/game/...`
  Expected: no errors.
  ```bash
  git add proto/spatialserver/v1/common.proto proto/gen/ pkg/game/encode.go pkg/game/game.go pkg/game/game_test.go tools/client/main.go
  git commit -m "feat(game): EntityAction(0x07) dispatch + EntityState(0x08) encode"
  ```

---

### Task 4: Zone-state persistence + crash recovery

**Files:**
- Create: `pkg/storage/migrations/002_zone_state.up.sql`, `002_zone_state.down.sql`
- Create: `pkg/storage/zone_state.go`, `pkg/storage/zone_state_test.go`
- Modify: `pkg/game/game.go`
- Modify: `apps/game-server/main.go`

- [ ] **Step 1: Add the migration**

  `pkg/storage/migrations/002_zone_state.up.sql`:
  ```sql
  CREATE TABLE IF NOT EXISTS zone_state (
      zone_id     TEXT NOT NULL,
      runtime_id  TEXT NOT NULL REFERENCES runtimes(id) ON DELETE CASCADE,
      snapshot    JSONB NOT NULL,
      tick_count  BIGINT NOT NULL,
      taken_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
      PRIMARY KEY (zone_id, taken_at)
  );
  CREATE INDEX IF NOT EXISTS idx_zone_state_runtime ON zone_state(runtime_id, taken_at DESC);
  ```
  `pkg/storage/migrations/002_zone_state.down.sql`:
  ```sql
  DROP TABLE IF EXISTS zone_state;
  ```

- [ ] **Step 2: Add failing repo test**

  Create `pkg/storage/zone_state_test.go`:
  ```go
  package storage

  import (
  	"context", "testing"
  	"github.com/stretchr/testify/assert"
  	"github.com/stretchr/testify/require"
  )

  func TestZoneStateRepository_SaveLatest(t *testing.T) {
  	pool := testDB(t)
  	repo := NewZoneStateRepository(pool)
  	ctx := context.Background()
  	_, _ = pool.Exec(ctx, `INSERT INTO runtimes (id) VALUES ('r1') ON CONFLICT DO NOTHING`)
  	require.NoError(t, repo.Save(ctx, "z1", "r1", []byte(`{"tick":1}`), 1))
  	require.NoError(t, repo.Save(ctx, "z1", "r1", []byte(`{"tick":2}`), 2))
  	snap, tick, err := repo.Latest(ctx, "z1")
  	require.NoError(t, err)
  	assert.Equal(t, int64(2), tick)
  	assert.JSONEq(t, `{"tick":2}`, string(snap))
  }
  ```

- [ ] **Step 3: Run test to verify failure**

  Run: `SPATIAL_TEST_POSTGRES_DSN=... go test ./pkg/storage/... -run TestZoneStateRepository -v`
  Expected: FAIL (`NewZoneStateRepository` undefined).

- [ ] **Step 4: Implement `pkg/storage/zone_state.go`**

  ```go
  package storage

  import (
  	"context", "errors", "fmt"
  	"github.com/jackc/pgx/v5"
  	"github.com/jackc/pgx/v5/pgxpool"
  )

  var ErrNoSnapshot = errors.New("no snapshot")

  type ZoneStateRepository struct{ pool *pgxpool.Pool }

  func NewZoneStateRepository(pool *pgxpool.Pool) *ZoneStateRepository { return &ZoneStateRepository{pool: pool} }

  func (r *ZoneStateRepository) Save(ctx context.Context, zoneID, runtimeID string, snapshot []byte, tick int64) error {
  	_, err := r.pool.Exec(ctx,
  		`INSERT INTO zone_state (zone_id, runtime_id, snapshot, tick_count) VALUES ($1,$2,$3,$4)`,
  		zoneID, runtimeID, snapshot, tick)
  	if err != nil {
  		return fmt.Errorf("save zone_state %s: %w", zoneID, err)
  	}
  	return nil
  }

  func (r *ZoneStateRepository) Latest(ctx context.Context, zoneID string) ([]byte, int64, error) {
  	var snapshot []byte
  	var tick int64
  	err := r.pool.QueryRow(ctx,
  		`SELECT snapshot, tick_count FROM zone_state WHERE zone_id=$1 ORDER BY taken_at DESC LIMIT 1`,
  		zoneID).Scan(&snapshot, &tick)
  	if err != nil {
  		if errors.Is(err, pgx.ErrNoRows) {
  			return nil, 0, fmt.Errorf("zone_state %s: %w", zoneID, ErrNoSnapshot)
  		}
  		return nil, 0, fmt.Errorf("latest zone_state %s: %w", zoneID, err)
  	}
  	return snapshot, tick, nil
  }
  ```

- [ ] **Step 5: Run test to verify pass**

  Run: `SPATIAL_TEST_POSTGRES_DSN=... go test ./pkg/storage/... -run TestZoneStateRepository -v`
  Expected: PASS.

- [ ] **Step 6: Add the snapshot ticker to `pkg/game/game.go`**

  Add fields to `Game`: `snapshotter SnapshotWriter`, `snapshotEvery int`, `tickCount int64`. Add:
  ```go
  type SnapshotWriter interface {
  	Save(zoneID types.ZoneID, snapshot []byte, tick int64)
  }

  func WithSnapshotter(w SnapshotWriter, every int) Option {
  	return func(g *Game) { g.snapshotter = w; g.snapshotEvery = every }
  }
  ```
  At the top of the `tick()` `default:` branch (before `simulate`): `g.tickCount++; if g.snapshotter != nil && g.snapshotEvery > 0 && g.tickCount%int64(g.snapshotEvery) == 0 { g.snapshotAllZones() }`. Add:
  ```go
  func (g *Game) snapshotAllZones() {
  	g.mu.Lock()
  	defer g.mu.Unlock()
  	for zid := range g.Zones {
  		var rows []map[string]any
  		for _, e := range g.Entities {
  			if e.ZoneID != zid { continue }
  			rows = append(rows, map[string]any{
  				"id": string(e.ID), "type": e.Type, "behavior": e.Behavior,
  				"x": e.Position.X, "y": e.Position.Y, "z": e.Position.Z,
  			})
  		}
  		data, err := json.Marshal(rows)
  		if err != nil { slog.Warn("marshal snapshot", slog.String("error", err.Error())); continue }
  		g.snapshotter.Save(zid, data, g.tickCount)
  	}
  }
  ```
  Add `"encoding/json"` and `"log/slog"` imports.

- [ ] **Step 7: Crash recovery in `apps/game-server/main.go`**

  - Build a pgx pool (`pgPool := storage.NewPostgresPool(...)`) using `cfg.Postgres.DSN`, run `migration.Run(pgPool, "pkg/storage/migrations")`.
  - Construct `snapRepo := storage.NewZoneStateRepository(pgPool)` and pass `game.WithSnapshotter(snapshotAdapter{repo: snapRepo, runtime: "r1"}, 100)` into `game.New`.
  - Before the config-driven NPC loop (Task 2), try recovery:
    ```go
    if snap, _, err := snapRepo.Latest(context.Background(), "r1-z1"); err == nil {
    	hydrateFromSnapshot(g, snap)
    	logger.Info("recovered entities from snapshot")
    } else {
    	// config-driven NPC seeding from Task 2 runs here
    }
    ```
  - Add the adapter + hydrate helper:
    ```go
    type snapshotAdapter struct {
    	repo    *storage.ZoneStateRepository
    	runtime string
    }
    func (s snapshotAdapter) Save(zoneID types.ZoneID, snap []byte, tick int64) {
    	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
    	defer cancel()
    	if err := s.repo.Save(ctx, string(zoneID), s.runtime, snap, tick); err != nil {
    		slog.Warn("snapshot save", slog.String("error", err.Error()))
    	}
    }
    func hydrateFromSnapshot(g *game.Game, data []byte) {
    	var rows []struct {
    		ID       string  `json:"id"`
    		Type     string  `json:"type"`
    		Behavior string  `json:"behavior"`
    		X        float64 `json:"x"`
    		Y        float64 `json:"y"`
    		Z        float64 `json:"z"`
    	}
    	if json.Unmarshal(data, &rows) != nil { return }
    	for _, r := range rows {
    		npc := entity.New(types.EntityID(r.ID), r.Type, types.RuntimeID(""))
    		npc.Position = types.Vector3{X: r.X, Y: r.Y, Z: r.Z}
    		npc.Behavior = r.Behavior
    		npc.Lifecycle = &game.NPCLifecycle{Behavior: newBehaviorFor(config.NPCSpec{Behavior: r.Behavior, Position: npc.Position})}
    		g.AddEntity(npc)
    	}
    }
    ```

- [ ] **Step 8: Verify + commit**

  Run: `go build ./... && go vet ./pkg/game/... ./pkg/storage/...`
  Expected: no errors.
  ```bash
  git add pkg/storage/migrations/002_zone_state.* pkg/storage/zone_state.go pkg/storage/zone_state_test.go pkg/game/game.go apps/game-server/main.go
  git commit -m "feat(game): periodic zone-state snapshots + crash recovery"
  ```

---

### Task 5: Prometheus metrics endpoint

**Files:**
- Create: `pkg/metrics/metrics.go`, `pkg/metrics/metrics_test.go`
- Modify: `apps/gateway/main.go`, `apps/room-service/main.go`, `apps/game-server/main.go`
- Modify: `pkg/config/config.go` (add `Metrics`), `configs/defaults.yml`, `go.mod`

- [ ] **Step 1: Add dependency**

  Run: `go get github.com/prometheus/client_golang@latest`

- [ ] **Step 2: Add failing test**

  Create `pkg/metrics/metrics_test.go`:
  ```go
  package metrics

  import (
  	"io", "net/http", "net/http/httptest", "strings", "testing"
  	"github.com/stretchr/testify/assert"
  	"github.com/stretchr/testify/require"
  )

  func TestRegistry_HandlerExposesMetrics(t *testing.T) {
  	reg := NewRegistry()
  	reg.ActiveConnections.Inc()
  	reg.PacketsPerSec.WithLabelValues("in", "0x03").Inc()
  	reg.TickDurationSeconds.Observe(0.05)
  	srv := httptest.NewServer(reg.Handler())
  	defer srv.Close()
  	resp, err := http.Get(srv.URL)
  	require.NoError(t, err)
  	defer resp.Body.Close()
  	body, _ := io.ReadAll(resp.Body)
  	s := string(body)
  	assert.True(t, strings.Contains(s, "spatial_active_connections"))
  	assert.True(t, strings.Contains(s, "spatial_packets_per_sec"))
  	assert.True(t, strings.Contains(s, "spatial_tick_duration_seconds"))
  }
  ```

- [ ] **Step 3: Run test to verify failure**

  Run: `go test ./pkg/metrics/... -v`
  Expected: FAIL (package missing).

- [ ] **Step 4: Implement `pkg/metrics/metrics.go`**

  ```go
  package metrics

  import (
  	"net/http"
  	"github.com/prometheus/client_golang/prometheus"
  	"github.com/prometheus/client_golang/prometheus/promhttp"
  )

  type Registry struct {
  	ActiveConnections   prometheus.Gauge
  	PacketsPerSec       *prometheus.CounterVec
  	EntityCount         *prometheus.GaugeVec
  	TickDurationSeconds prometheus.Histogram
  	GRPCRequestDuration *prometheus.HistogramVec
  	reg                 *prometheus.Registry
  }

  func NewRegistry() *Registry {
  	r := prometheus.NewRegistry()
  	m := &Registry{reg: r,
  		ActiveConnections: prometheus.NewGauge(prometheus.GaugeOpts{Namespace: "spatial", Name: "active_connections"}),
  		PacketsPerSec: prometheus.NewCounterVec(prometheus.CounterOpts{Namespace: "spatial", Name: "packets_per_sec_total"}, []string{"direction", "packet_id"}),
  		EntityCount: prometheus.NewGaugeVec(prometheus.GaugeOpts{Namespace: "spatial", Name: "entity_count"}, []string{"type"}),
  		TickDurationSeconds: prometheus.NewHistogram(prometheus.HistogramOpts{Namespace: "spatial", Name: "tick_duration_seconds"}),
  		GRPCRequestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{Namespace: "spatial", Name: "grpc_request_duration_seconds"}, []string{"service", "method"}),
  	}
  	r.MustRegister(m.ActiveConnections, m.PacketsPerSec, m.EntityCount, m.TickDurationSeconds, m.GRPCRequestDuration)
  	return m
  }

  func (r *Registry) Handler() http.Handler { return promhttp.HandlerFor(r.reg, promhttp.HandlerOpts{}) }
  ```

- [ ] **Step 5: Run test to verify pass**

  Run: `go test ./pkg/metrics/... -v`
  Expected: PASS.

- [ ] **Step 6: Expose `/metrics` in all three mains**

  - Add `MetricsConfig{GatewayPort int \`koanf:"gateway_port"\`; RoomServicePort int \`koanf:"room_service_port"\`; GameServerPort int \`koanf:"game_server_port"\`}` + `Metrics MetricsConfig \`koanf:"metrics"\`` to `pkg/config/config.go`.
  - Append to `configs/defaults.yml`: `metrics: {gateway_port: 9100, room_service_port: 9101, game_server_port: 9102}`.
  - In each main: `reg := metrics.NewRegistry()`; start a sidecar HTTP server on the configured port serving `reg.Handler()`:
    ```go
    go func() {
    	mux := http.NewServeMux()
    	mux.Handle("/metrics", reg.Handler())
    	_ = (&http.Server{Addr: fmt.Sprintf(":%d", port), Handler: mux}).ListenAndServe()
    }()
    ```
    (gateway uses `cfg.Metrics.GatewayPort`, etc.) The `reg` built here is reused by Task 6 for interceptors; the Game loop feeds `reg.TickDurationSeconds`/`EntityCount` (pass `*metrics.Registry` into `Game` via a new `WithMetrics(*metrics.Registry)` option and observe `time.Since(start)` at end of `tick()`).

- [ ] **Step 7: Verify + commit**

  Run: `go build ./... && go vet ./pkg/metrics/...`
  Expected: no errors.
  ```bash
  git add pkg/metrics/ apps/ configs/defaults.yml pkg/config/config.go pkg/game/game.go go.mod go.sum
  git commit -m "feat(metrics): prometheus /metrics on all services"
  ```

---

### Task 6: gRPC interceptors

**Files:**
- Create: `pkg/grpc/interceptor.go`, `pkg/grpc/interceptor_test.go`
- Modify: `apps/gateway/main.go`, `apps/room-service/main.go`, `apps/game-server/main.go`

- [ ] **Step 1: Add failing interceptor tests**

  Create `pkg/grpc/interceptor_test.go`:
  ```go
  package grpcinterceptor

  import (
  	"context", "testing"
  	"github.com/stretchr/testify/assert"
  	"google.golang.org/grpc"
  	"google.golang.org/grpc/codes"
  	"google.golang.org/grpc/status"
  	"github.com/thaolaptrinh/spatial-server/pkg/metrics"
  )

  func TestRecovery_RecoversPanic(t *testing.T) {
  	h := RecoveryInterceptor(metrics.NewRegistry())
  	info := &grpc.UnaryServerInfo{FullMethod: "/svc/Boom"}
  	_, err := h(context.Background(), nil, info, func(context.Context, any) (any, error) { panic("boom") })
  	st, _ := status.FromError(err)
  	assert.Equal(t, codes.Internal, st.Code())
  }

  func TestMetrics_RecordsLatency(t *testing.T) {
  	h := MetricsUnaryInterceptor(metrics.NewRegistry(), "svc")
  	_, err := h(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/svc/Ping"}, func(context.Context, any) (any, error) { return nil, nil })
  	assert.NoError(t, err)
  }
  ```

- [ ] **Step 2: Run test to verify failure**

  Run: `go test ./pkg/grpc/... -v`
  Expected: FAIL (package missing).

- [ ] **Step 3: Implement `pkg/grpc/interceptor.go`**

  ```go
  package grpcinterceptor

  import (
  	"context", "log/slog", "runtime/debug", "strings", "time"
  	"google.golang.org/grpc"
  	"google.golang.org/grpc/codes"
  	"google.golang.org/grpc/status"
  	"github.com/thaolaptrinh/spatial-server/pkg/metrics"
  )

  func RecoveryInterceptor(_ *metrics.Registry) grpc.UnaryServerInterceptor {
  	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
  		defer func() {
  			if r := recover(); r != nil {
  				slog.Error("grpc panic", slog.Any("panic", r), slog.String("method", info.FullMethod), slog.String("stack", string(debug.Stack())))
  				err = status.Error(codes.Internal, "internal error")
  			}
  		}()
  		return handler(ctx, req)
  	}
  }

  func RecoveryStreamInterceptor(_ *metrics.Registry) grpc.StreamServerInterceptor {
  	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) (err error) {
  		defer func() {
  			if r := recover(); r != nil {
  				slog.Error("grpc stream panic", slog.Any("panic", r), slog.String("stack", string(debug.Stack())))
  				err = status.Error(codes.Internal, "internal error")
  			}
  		}()
  		return handler(srv, ss)
  	}
  }

  func LoggingUnaryInterceptor() grpc.UnaryServerInterceptor {
  	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
  		start := time.Now()
  		resp, err := handler(ctx, req)
  		if err != nil {
  			slog.Warn("grpc request", slog.String("method", info.FullMethod), slog.String("error", err.Error()), slog.Duration("duration", time.Since(start)))
  		} else {
  			slog.Info("grpc request", slog.String("method", info.FullMethod), slog.Duration("duration", time.Since(start)))
  		}
  		return resp, err
  	}
  }

  func MetricsUnaryInterceptor(reg *metrics.Registry, service string) grpc.UnaryServerInterceptor {
  	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
  		start := time.Now()
  		resp, err := handler(ctx, req)
  		reg.GRPCRequestDuration.WithLabelValues(service, method(info.FullMethod)).Observe(time.Since(start).Seconds())
  		return resp, err
  	}
  }

  func MetricsStreamInterceptor(reg *metrics.Registry, service string) grpc.StreamServerInterceptor {
  	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
  		start := time.Now()
  		err := handler(srv, ss)
  		reg.GRPCRequestDuration.WithLabelValues(service, method(info.FullMethod)).Observe(time.Since(start).Seconds())
  		return err
  	}
  }

  func ServerOptions(service string, reg *metrics.Registry) []grpc.ServerOption {
  	return []grpc.ServerOption{
  		grpc.ChainUnaryInterceptor(RecoveryInterceptor(reg), LoggingUnaryInterceptor(), MetricsUnaryInterceptor(reg, service)),
  		grpc.ChainStreamInterceptor(RecoveryStreamInterceptor(reg), MetricsStreamInterceptor(reg, service)),
  	}
  }

  func method(full string) string {
  	if i := strings.LastIndex(full, "/"); i >= 0 { return full[i+1:] }
  	return full
  }
  ```

- [ ] **Step 4: Run tests to verify pass**

  Run: `go test ./pkg/grpc/... -v`
  Expected: PASS.

- [ ] **Step 5: Wire into all three mains**

  Replace each `srv := grpc.NewServer()` with `srv := grpc.NewServer(grpcinterceptor.ServerOptions("<service>", reg)...)` (`"gateway"` / `"room-service"` / `"game-server"`), using the `reg` from Task 5. Import `grpcinterceptor "github.com/thaolaptrinh/spatial-server/pkg/grpc"`.

- [ ] **Step 6: Verify + commit**

  Run: `go build ./... && go vet ./pkg/grpc/... ./apps/...`
  Expected: no errors.
  ```bash
  git add pkg/grpc/ apps/
  git commit -m "feat(grpc): recovery/logging/metrics interceptors on all services"
  ```

---

## Self-Review Checklist

- **Spec coverage:** entity lifecycle hooks (T1), NPC simulation patrol/idle/wander + NPCLifecycle + registry (T2), EntityAction(0x07) dispatch + EntityState(0x08) encode + client `-action` (T3), zone-state persistence migration 002 + repo + snapshot loop + crash recovery (T4), Prometheus `/metrics` on all services (T5), gRPC interceptors recovery/logging/metrics (T6). No spec section uncovered.
- **Placeholder scan:** no "TBD/TODO/implement later"; created files show complete code, modified files use precise snippet changes; no "similar to Task N".
- **Type consistency:** `game.Behavior` + the three behavior structs referenced identically in T2 & T4; `game.NPCLifecycle{Behavior: ...}` attached the same way in T2 & T4; `encodeSpawnFrame`/`encodeDespawnFrame`/`encodeState` (T3) replace the inline functions deleted from `game.go`; `game.SnapshotWriter` + `snapshotAdapter` (T4) match `WithSnapshotter`; `metrics.Registry` field names (`ActiveConnections`/`PacketsPerSec`/`GRPCRequestDuration`/`TickDurationSeconds`) used consistently in T5 & T6; 8-byte `protocol.Encode(...,seq)` signature carried from Phase 1Finish.

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/phase-2-realtime-features.md`. Two execution options:

**1. Subagent-Driven (recommended)** — dispatch a fresh subagent per task (6 tasks, sequential), review between tasks, fast iteration.

**2. Inline Execution** — execute tasks in this session with checkpoints.

Which approach?
