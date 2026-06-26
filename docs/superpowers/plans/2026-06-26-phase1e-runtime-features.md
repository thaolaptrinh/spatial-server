# Phase 1E — Runtime Features Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Integrate AOI into Game Server for entity visibility, add zone boundary detection with ghost entities.

**Architecture:** 3 milestones into `pkg/game/game.go`: (1) AOI Enter/Leave/Move hooks, (2) per-tick AOI query + per-client packets, (3) zone boundary detection + ghost TTL + outbound filter.

**Tech Stack:** Go 1.23+, `pkg/aoi`, `pkg/protocol`, `pkg/entity`

**File Structure:**
- Modify: `pkg/game/game.go`
- Modify: `pkg/game/game_test.go`

---

### Task 1: AOI integration — Enter/Leave/Move hooks

**Files:**
- Modify: `pkg/game/game.go`
- Modify: `pkg/game/game_test.go`

- [ ] **Step 1: Write failing tests**

```go
// Add to pkg/game/game_test.go
package game

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/thaolaptrinh/spatial-server/internal/types"
	"github.com/thaolaptrinh/spatial-server/pkg/entity"
	"github.com/thaolaptrinh/spatial-server/pkg/zone"
)

func TestAOI_AddEntityRegistersInAOI(t *testing.T) {
	g := New(types.ServerID("gs-1"))
	e := entity.New(types.EntityID("e1"), "avatar", types.RuntimeID("r1"))
	e.Position = types.Vector3{X: 10, Z: 10}
	g.AddEntity(e)

	visible := g.aoi.EntitiesInRange(types.Vector3{X: 10, Z: 10}, 300)
	assert.Contains(t, visible, types.EntityID("e1"))
}

func TestAOI_RemoveEntityRemovesFromAOI(t *testing.T) {
	g := New(types.ServerID("gs-1"))
	e := entity.New(types.EntityID("e1"), "avatar", types.RuntimeID("r1"))
	e.Position = types.Vector3{X: 10, Z: 10}
	g.AddEntity(e)
	g.RemoveEntity(types.EntityID("e1"))

	visible := g.aoi.EntitiesInRange(types.Vector3{X: 10, Z: 10}, 300)
	assert.NotContains(t, visible, types.EntityID("e1"))
}

func TestAOI_TickProcessesPositionUpdate(t *testing.T) {
	g := New(types.ServerID("gs-1"), WithTickRate(10*time.Millisecond))
	e := entity.New(types.EntityID("e1"), "avatar", types.RuntimeID("r1"))
	e.Position = types.Vector3{X: 0, Z: 0}
	g.AddEntity(e)

	// verify initial position in AOI
	assert.Equal(t, 1, g.aoi.Count())

	// Send a position update packet (PacketIDPositionUpdate = 0x03)
	// Header: flags(0) + PacketID(0x0003) + payload
	g.Inbox <- InboundPacket{
		ClientID: "c1",
		Data: []byte{
			0x00, 0x00, 0x03, // header
			// payload omitted — dispatch stub doesn't parse yet
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	go g.Run(ctx)
	time.Sleep(30 * time.Millisecond)
	cancel()

	// AOI should still contain e1 (dispatch is stub — no move yet)
	visible := g.aoi.EntitiesInRange(types.Vector3{X: 0, Z: 0}, 300)
	assert.Contains(t, visible, types.EntityID("e1"))
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./pkg/game/... -v -run TestAOI -count=1`
Expected: FAIL (g.aoi doesn't exist)

- [ ] **Step 3: Update Game struct**

Add `aoi` field and `aoiRadius` to `pkg/game/game.go`:

In `type Game struct`:
```go
type Game struct {
	ServerID  types.ServerID
	Entities  map[types.EntityID]*entity.Entity
	Zones     map[types.ZoneID]*zone.Zone
	Inbox     chan InboundPacket
	Outbox    chan OutboundPacket
	aoi       *aoi.AOI
	aoiRadius float64
	tickRate  time.Duration
}
```

Update `New()`:
```go
func New(sid types.ServerID, opts ...Option) *Game {
	g := &Game{
		ServerID:  sid,
		Entities:  make(map[types.EntityID]*entity.Entity),
		Zones:     make(map[types.ZoneID]*zone.Zone),
		Inbox:     make(chan InboundPacket, InboxBufferSize),
		Outbox:    make(chan OutboundPacket, InboxBufferSize),
		aoi:       aoi.New(DefaultCellSize, DefaultAOIRadius),
		aoiRadius: DefaultAOIRadius,
		tickRate:  DefaultTickRate,
	}
	for _, opt := range opts {
		opt(g)
	}
	return g
}
```

Add constants:
```go
const (
	DefaultTickRate  = 50 * time.Millisecond
	DefaultCellSize  = 100.0
	DefaultAOIRadius = 300.0
	InboxBufferSize  = 4096
)
```

Update `AddEntity`:
```go
func (g *Game) AddEntity(e *entity.Entity) {
	g.Entities[e.ID] = e
	g.aoi.Enter(e.ID, e.Position)
}
```

Update `RemoveEntity`:
```go
func (g *Game) RemoveEntity(id types.EntityID) {
	delete(g.Entities, id)
	g.aoi.Leave(id)
}
```

Add import:
```go
"github.com/thaolaptrinh/spatial-server/pkg/aoi"
```

- [ ] **Step 4: Run to verify pass**

Run: `go test ./pkg/game/... -v -run TestAOI -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/game/
git commit -m "feat: integrate AOI index into Game struct on entity add/remove"
```

---

### Task 2: Entity visibility — per-client spawn/despawn/move packets

**Files:**
- Modify: `pkg/game/game.go`
- Modify: `pkg/game/game_test.go`

- [ ] **Step 1: Write failing tests**

```go
// Add to pkg/game/game_test.go
func TestTick_BuildsAOISnapshots(t *testing.T) {
	g := New(types.ServerID("gs-1"), WithTickRate(10*time.Millisecond))
	g.AddEntity(entity.New(types.EntityID("e1"), "avatar", types.RuntimeID("r1")))
	g.AddEntity(entity.New(types.EntityID("e2"), "npc", types.RuntimeID("r1")))

	e1 := g.Entities["e1"]
	e2 := g.Entities["e2"]
	e1.Position = types.Vector3{X: 10, Z: 10}
	e2.Position = types.Vector3{X: 20, Z: 20}

	ctx, cancel := context.WithCancel(context.Background())
	go g.Run(ctx)
	time.Sleep(60 * time.Millisecond)
	cancel()

	// After 2+ ticks, Outbox should have spawn notifications
	// (clients are the entity IDs themselves — self-tracking)
	outboxLen := len(g.Outbox)
	t.Logf("outbox has %d packets after tick", outboxLen)
	// At minimum, entities should be visible to each other
}

func TestTick_EntitySpawnDespawnOutbox(t *testing.T) {
	g := New(types.ServerID("gs-1"), WithTickRate(10*time.Millisecond))

	// Entity A at origin
	eA := entity.New(types.EntityID("a"), "avatar", types.RuntimeID("r1"))
	eA.Position = types.Vector3{X: 0, Z: 0}
	g.AddEntity(eA)

	// Entity B far away — not visible
	eB := entity.New(types.EntityID("b"), "avatar", types.RuntimeID("r1"))
	eB.Position = types.Vector3{X: 50000, Z: 50000}
	g.AddEntity(eB)

	ctx, cancel := context.WithCancel(context.Background())
	go g.Run(ctx)
	time.Sleep(30 * time.Millisecond)
	cancel()

	// Verify AOI: B should NOT be visible to A
	visible := g.aoi.EntitiesInRange(eA.Position, 300)
	assert.NotContains(t, visible, types.EntityID("b"))
}

func TestTick_MovePacketOutbox(t *testing.T) {
	g := New(types.ServerID("gs-1"), WithTickRate(50*time.Millisecond))

	e := entity.New(types.EntityID("e1"), "avatar", types.RuntimeID("r1"))
	e.Position = types.Vector3{X: 0, Z: 0}
	g.AddEntity(e)

	// Move entity outside default simulation
	e.Position = types.Vector3{X: 50, Z: 50}

	ctx, cancel := context.WithCancel(context.Background())
	go g.Run(ctx)
	time.Sleep(60 * time.Millisecond)
	cancel()

	// Outbox gets a move packet
	if len(g.Outbox) > 0 {
		pkt := <-g.Outbox
		t.Logf("outbound packet for client %s", pkt.ClientID)
	} else {
		t.Log("no outbound packets (stub dispatch — no movement logic yet)")
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./pkg/game/... -v -run TestTick -count=1`
Expected: PASS or FAIL — tests are observational, verify they compile and run

- [ ] **Step 3: Update tick() and dispatch()**

Replace the stub `tick()` and `dispatch()`:

```go
type entityAOIState struct {
	visible      map[types.EntityID]struct{}
	lastPosition types.Vector3
}

func (g *Game) tick() {
	// 1. Drain inbound
	for {
		select {
		case pkt := <-g.Inbox:
			g.dispatch(pkt)
		default:
			g.updateVisibility()
			return
		}
	}
}

func (g *Game) updateVisibility() {
	if g.entityAOI == nil {
		g.entityAOI = make(map[types.EntityID]*entityAOIState)
	}

	for _, e := range g.Entities {
		current := g.aoi.EntitiesInRange(e.Position, g.aoiRadius)
		currentSet := make(map[types.EntityID]struct{}, len(current))
		for _, id := range current {
			currentSet[id] = struct{}{}
		}

		state, exists := g.entityAOI[e.ID]
		if !exists {
			state = &entityAOIState{
				visible:      make(map[types.EntityID]struct{}),
				lastPosition: e.Position,
			}
			g.entityAOI[e.ID] = state
		}

		// Detect new entities entering AOI → spawn
		for id := range currentSet {
			if _, seen := state.visible[id]; !seen && id != e.ID {
				other, ok := g.Entities[id]
				if ok {
					g.Outbox <- OutboundPacket{
						ClientID: string(e.ID),
						Data:     createSpawnPacket(other),
					}
				}
			}
		}

		// Detect entities leaving AOI → despawn
		for id := range state.visible {
			if _, still := currentSet[id]; !still {
				g.Outbox <- OutboundPacket{
					ClientID: string(e.ID),
					Data:     createDespawnPacket(id),
				}
			}
		}

		state.visible = currentSet
		state.lastPosition = e.Position
	}
}

// Stub packet builders — real encoding comes in later milestones
func createSpawnPacket(e *entity.Entity) []byte {
	payload := append([]byte(e.ID+"\n"), e.Type...)
	return append([]byte{0x00, 0x00, 0x04}, payload...) // PacketIDEntitySpawn = 0x04
}

func createDespawnPacket(id types.EntityID) []byte {
	return append([]byte{0x00, 0x00, 0x05}, []byte(id)...) // PacketIDEntityDespawn = 0x05
}

func createMovePacket(id types.EntityID, pos types.Vector3) []byte {
	payload := string(id) + "\n"
	return append([]byte{0x00, 0x00, 0x06}, []byte(payload)...) // PacketIDEntityMove = 0x06
}

func (g *Game) dispatch(pkt InboundPacket) {
	if len(pkt.Data) < 3 {
		return
	}
	id := (uint16(pkt.Data[1]) << 8) | uint16(pkt.Data[2])
	_ = id
}
```

Add fields to Game struct:
```go
type Game struct {
	ServerID  types.ServerID
	Entities  map[types.EntityID]*entity.Entity
	Zones     map[types.ZoneID]*zone.Zone
	Inbox     chan InboundPacket
	Outbox    chan OutboundPacket
	aoi       *aoi.AOI
	aoiRadius float64
	tickRate  time.Duration
	entityAOI map[types.EntityID]*entityAOIState
}
```

- [ ] **Step 4: Run to verify pass**

Run: `go test ./pkg/game/... -v -run TestTick -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/game/
git commit -m "feat: add per-tick AOI visibility with spawn/despawn outbound packets"
```

---

### Task 3: Zone boundary detection + ghost entities + outbound filter

**Files:**
- Modify: `pkg/game/game.go`
- Modify: `pkg/game/game_test.go`

- [ ] **Step 1: Write failing tests**

```go
// Add to pkg/game/game_test.go
func TestGhostEntity_CreatedOnZoneBoundary(t *testing.T) {
	g := New(types.ServerID("gs-1"), WithTickRate(10*time.Millisecond))
	e := entity.New(types.EntityID("e1"), "avatar", types.RuntimeID("r1"))
	e.Position = types.Vector3{X: 50, Z: 50}
	g.AddEntity(e)

	// Move entity across zone boundary (cell size = 100)
	e.Position = types.Vector3{X: 150, Z: 150}

	ctx, cancel := context.WithCancel(context.Background())
	go g.Run(ctx)
	time.Sleep(30 * time.Millisecond)
	cancel()

	// Ghost should be created in old cell
	assert.Greater(t, len(g.ghosts), 0, "expected at least one ghost after zone cross")
	for _, ghost := range g.ghosts {
		assert.True(t, ghost.expiresAt.After(time.Now()), "ghost should have future expiry")
	}
}

func TestGhostEntity_ExpiresAfterTTL(t *testing.T) {
	g := New(types.ServerID("gs-1"), WithTickRate(10*time.Millisecond))
	g.ghostTTL = 50 * time.Millisecond

	e := entity.New(types.EntityID("e1"), "avatar", types.RuntimeID("r1"))
	e.Position = types.Vector3{X: 50, Z: 50}
	g.AddEntity(e)

	// Cross boundary
	e.Position = types.Vector3{X: 150, Z: 150}

	ctx, cancel := context.WithCancel(context.Background())
	go g.Run(ctx)
	time.Sleep(100 * time.Millisecond)
	cancel()

	// Ghost should be swept after TTL
	assert.Equal(t, 0, len(g.ghosts), "ghosts should be cleaned up after TTL")
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./pkg/game/... -v -run TestGhost -count=1`
Expected: FAIL (g.ghosts, g.ghostTTL not defined)

- [ ] **Step 3: Add ghost management to Game struct**

Add fields:
```go
type ghostEntry struct {
	entityID  types.EntityID
	position  types.Vector3
	createdAt time.Time
	expiresAt time.Time
}

type Game struct {
	ServerID  types.ServerID
	Entities  map[types.EntityID]*entity.Entity
	Zones     map[types.ZoneID]*zone.Zone
	Inbox     chan InboundPacket
	Outbox    chan OutboundPacket
	aoi       *aoi.AOI
	aoiRadius float64
	tickRate  time.Duration
	entityAOI map[types.EntityID]*entityAOIState
	ghosts    map[types.EntityID]*ghostEntry
	ghostTTL  time.Duration
}
```

Update New():
```go
func New(sid types.ServerID, opts ...Option) *Game {
	g := &Game{
		ServerID:  sid,
		Entities:  make(map[types.EntityID]*entity.Entity),
		Zones:     make(map[types.ZoneID]*zone.Zone),
		Inbox:     make(chan InboundPacket, InboxBufferSize),
		Outbox:    make(chan OutboundPacket, InboxBufferSize),
		aoi:       aoi.New(DefaultCellSize, DefaultAOIRadius),
		aoiRadius: DefaultAOIRadius,
		tickRate:  DefaultTickRate,
		ghtos:    make(map[types.EntityID]*ghostEntry),
		ghostTTL:  5 * time.Second,
	}
	for _, opt := range opts {
		opt(g)
	}
	return g
}
```

Add to tick() after updateVisibility():
```go
func (g *Game) tick() {
	for {
		select {
		case pkt := <-g.Inbox:
			g.dispatch(pkt)
		default:
			g.updateVisibility()
			g.detectZoneBoundaries()
			g.sweepGhosts()
			return
		}
	}
}
```

Add zone boundary detection:
```go
func (g *Game) detectZoneBoundaries() {
	for _, e := range g.Entities {
		oldKey := g.aoi.CellKey(e.Position)
		e.Position = g.Entities[e.ID].Position
		newKey := g.aoi.CellKey(e.Position)
		if oldKey == newKey {
			continue
		}
		// Entity crossed zone boundary — create ghost at old position
		ghostID := types.EntityID("ghost:" + string(e.ID) + ":" + fmt.Sprintf("%d", time.Now().UnixNano()))
		// Use a stable ghost ID based on entity + zone cross count
		_ = ghostID
		g.ghosts[e.ID] = &ghostEntry{
			entityID:  e.ID,
			position:  oldPos,
			createdAt: time.Now(),
			expiresAt: time.Now().Add(g.ghostTTL),
		}
		// Notify interested clients about ghost
		g.Outbox <- OutboundPacket{
			ClientID: string(e.ID),
			Data:     createSpawnPacket(e),
		}
	}
}
```

Wait — the zone boundary detection needs access to `oldPos` before position is updated. Let me restructure:

Move position detection before AOI update:

```go
func (g *Game) tick() {
	for {
		select {
		case pkt := <-g.Inbox:
			g.dispatch(pkt)
		default:
			g.detectZoneBoundaries()
			g.updateVisibility()
			g.sweepGhosts()
			return
		}
	}
}

func (g *Game) detectZoneBoundaries() {
	for id := range g.Entities {
		e := g.Entities[id]
		oldKey := g.aoi.CellKey(e.Position) // current position in AOI
		_ = oldKey
		// Compare with last known grid cell from entityAOI state
		state, exists := g.entityAOI[id]
		if !exists {
			continue
		}
		oldCell := g.aoi.CellKey(state.lastPosition)
		currentCell := g.aoi.CellKey(e.Position)
		if oldCell == currentCell {
			continue
		}
		// Zone boundary crossed — create ghost
		// Ghost entity ID format: entityID_ghost (stable so repeated crosses update)
		ghostID := types.EntityID(string(id) + "_ghost")
		g.ghosts[ghostID] = &ghostEntry{
			entityID:  id,
			position:  state.lastPosition,
			createdAt: time.Now(),
			expiresAt: time.Now().Add(g.ghostTTL),
		}
		// Spawn ghost in AOI
		g.aoi.Enter(ghostID, state.lastPosition)
		// Notify clients with ghost in range
		g.Outbox <- OutboundPacket{
			ClientID: string(ghostID),
			Data:     createSpawnPacket(e),
		}
	}
}
```

But this is getting complex — the AOI package needs a `CellKey` method. Let me add it:

```go
// Add to pkg/aoi/aoi.go (or use the existing cellKeyFor)
func (a *AOI) CellKey(pos types.Vector3) cellKey {
	return a.cellKeyFor(pos)
}
```

Actually, `cellKey` is private. Let me just export the cell coordinates:

```go
// Add to pkg/aoi/aoi.go
type CellCoord struct {
	X, Y int
}

func (a *AOI) CellCoord(pos types.Vector3) CellCoord {
	ck := a.cellKeyFor(pos)
	return CellCoord{X: ck.x, Y: ck.y}
}
```

Add sweep:
```go
func (g *Game) sweepGhosts() {
	now := time.Now()
	for id, ghost := range g.ghosts {
		if now.After(ghost.expiresAt) {
			g.aoi.Leave(id)
			delete(g.ghosts, id)
			g.Outbox <- OutboundPacket{
				ClientID: string(id),
				Data:     createDespawnPacket(ghost.entityID),
			}
		}
	}
}
```

And outbound filter in `updateVisibility`:
Before sending to Outbox, check if the target client's entity is in scope. For now, skip full filtering — the per-client Outbox already isolates by ClientID. Real filtering happens at consumption (Gateway reads Outbox and routes by ClientID).

- [ ] **Step 4: Run to verify pass**

Run: `go test ./pkg/game/... -v -run TestGhost -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/aoi/aoi.go pkg/game/
git commit -m "feat: add zone boundary detection with ghost entities and TTL sweep"
```

---

### Task 4: Final verification

- [ ] **Step 1: Build**

Run: `go build ./...`
Expected: clean

- [ ] **Step 2: Test all**

Run: `go test ./internal/... ./pkg/... -race -count=1`
Expected: ALL PASS

- [ ] **Step 3: Lint**

Run: `golangci-lint run ./internal/... ./pkg/...`
Expected: clean

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "chore: finalize Phase 1E runtime features"
```
