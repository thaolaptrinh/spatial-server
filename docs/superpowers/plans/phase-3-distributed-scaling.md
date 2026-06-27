# Phase 3 — Distributed Scaling Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Transform the single-server Runtime into a multi-server cluster where zones are distributed across Game Servers, entities migrate across boundaries, Room Service is highly available, and Gateways receive push-based routing updates.

**Architecture:** The `Game` struct becomes zone-aware (one AOI grid per owned zone) with a `PeerRegistry` of gRPC clients to neighbor Game Servers. Room Service gains a heartbeat sweeper, K3s Lease leader election, and an ownership-change push stream. Gateways subscribe to that stream to invalidate their routing cache. Zone migration streams state P2P (source GS → target GS) while Room Service only coordinates metadata.

**Tech Stack:** Go 1.25, gRPC streaming (`google.golang.org/grpc`), `github.com/redis/go-redis/v9`, `github.com/jackc/pgx/v5`, `k8s.io/client-go` (K3s Lease), protobuf, Testcontainers for integration tests.

**Pre-existing files (checked before writing):**
- `pkg/game/game.go` — single `aoi *aoi.AOI`, flat `Entities`, stub `AssignZone`/`ReleaseZone` at lines 122-128, `detectZoneBoundaries()` at line 183, `ghostTTL = 5s` at line 81
- `pkg/aoi/aoi.go` — `AOI` grid with `Enter`/`Leave`/`Move`/`EntitiesInRange`/`CellCoord`/`Count`, no serialization
- `pkg/gateway/gateway.go` — `RouterCache` TTL cache, `Set`/`Get` only
- `pkg/gateway/handler.go` — `Handler` with `handleWS`/`relayWS`, dials game-server per connection
- `pkg/room/room.go` — `ServerRegistry` (`Register`/`Heartbeat`/`LeastLoaded`/`Get`), `ZoneOwnership` (`Claim`/`Release`/`Lookup`), `ResolveZone`. `ServerInfo.LastBeat` set but never checked
- `apps/game-server/main.go` — `gameServerServer` implements only `Relay` (line 98), `clientRegistry` with 64-slot `chan []byte` (line 119)
- `proto/spatialserver/v1/game_server.proto` — all RPCs declared (`AssignZone`, `ReleaseZone`, `MigrateEntity`, `ZoneStateSync`, `NotifyEntityEnter/Leave`, `SendEntityUpdate`, `QueryEntities`, `Relay`)
- `proto/spatialserver/v1/room_service.proto` — has `PrepareTransfer`/`TransferZone`, no `WatchOwnership`
- `proto/spatialserver/v1/common.proto` — `EntitySnapshot`, `ZoneSnapshot{zone_id, entities, aoi_state}`, `EntityEnterLeave`, `EntityUpdate`, `ZoneStatus` enum (`UNOWNED=1`, `ACTIVE=2`, `TRANSFERRING=3`, `ORPHAN=4`)
- `internal/types/types.go` — `ZoneStatus` Go enum (`Unowned=0`,`Active=1`,`Transferring=2`,`Orphan=3`) with `ValidTransition`; sentinel errors `ErrNotFound`/`ErrConflict`/`ErrInvalidArg`/`ErrNotOwned`
- Module path: `github.com/thaolaptrinh/spatial-server`; proto gen import: `spatialserverv1 "github.com/thaolaptrinh/spatial-server/proto/gen/spatialserver/v1"` (aliased `v1` in `pkg/game`)

---

### Task 1: Multi-Zone Game Server Refactor

**Files:**
- Create: `pkg/game/peer.go`
- Modify: `pkg/game/game.go`
- Modify: `pkg/game/game_test.go`

- [ ] **Step 1: Write the failing test**

  Add to `pkg/game/game_test.go`:

  ```go
  func TestMultiZone_SeparateAOIGrids(t *testing.T) {
  	g := New(types.ServerID("gs-1"))
  	z1 := zone.New(types.ZoneID("z1"), types.RuntimeID("r1"), 0, 0, 100)
  	z2 := zone.New(types.ZoneID("z2"), types.RuntimeID("r1"), 1, 0, 100)
  	require.NoError(t, g.AssignZone(z1))
  	require.NoError(t, g.AssignZone(z2))

  	e1 := entity.New(types.EntityID("e1"), "avatar", types.RuntimeID("r1"))
  	e1.ZoneID = types.ZoneID("z1")
  	e1.Position = types.Vector3{X: 10, Z: 10}
  	g.AddEntity(e1)

  	e2 := entity.New(types.EntityID("e2"), "avatar", types.RuntimeID("r1"))
  	e2.ZoneID = types.ZoneID("z2")
  	e2.Position = types.Vector3{X: 10, Z: 10}
  	g.AddEntity(e2)

  	assert.Equal(t, 2, g.EntityCount())
  	assert.Equal(t, 1, g.AOIFor(types.ZoneID("z1")).Count())
  	assert.Equal(t, 1, g.AOIFor(types.ZoneID("z2")).Count())
  	assert.Equal(t, types.ZoneID("z1"), g.ZoneOf(types.EntityID("e1")))
  	assert.Equal(t, types.ZoneID("z2"), g.ZoneOf(types.EntityID("e2")))
  }

  func TestReleaseZone_TeardownAOIGrid(t *testing.T) {
  	g := New(types.ServerID("gs-1"))
  	z1 := zone.New(types.ZoneID("z1"), types.RuntimeID("r1"), 0, 0, 100)
  	require.NoError(t, g.AssignZone(z1))
  	assert.NotNil(t, g.AOIFor(types.ZoneID("z1")))
  	require.NoError(t, g.ReleaseZone(types.ZoneID("z1")))
  	assert.Nil(t, g.AOIFor(types.ZoneID("z1")))
  }
  ```

- [ ] **Step 2: Run test to verify it fails**

  Run: `go test ./pkg/game/... -run TestMultiZone_SeparateAOIGrids -v`
  Expected: FAIL — `g.AssignZone` returns no error currently (returns nothing) and `AOIFor`/`ZoneOf` undefined.

- [ ] **Step 3: Create `pkg/game/peer.go`**

  ```go
  package game

  import (
  	"fmt"
  	"sync"

  	"github.com/thaolaptrinh/spatial-server/internal/types"
  	"google.golang.org/grpc"
  	"google.golang.org/grpc/credentials/insecure"

  	spatialserverv1 "github.com/thaolaptrinh/spatial-server/proto/gen/spatialserver/v1"
  )

  type peerConn struct {
  	target types.ServerID
  	addr   string
  	conn   *grpc.ClientConn
  	client spatialserverv1.GameServerClient
  }

  type PeerRegistry struct {
  	mu    sync.RWMutex
  	conns map[types.ServerID]*peerConn
  }

  func NewPeerRegistry() *PeerRegistry {
  	return &PeerRegistry{conns: make(map[types.ServerID]*peerConn)}
  }

  func (p *PeerRegistry) Upsert(serverID types.ServerID, addr string) error {
  	p.mu.Lock()
  	defer p.mu.Unlock()
  	if pc, ok := p.conns[serverID]; ok {
  		if pc.addr == addr {
  			return nil
  		}
  		_ = pc.conn.Close()
  		delete(p.conns, serverID)
  	}
  	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
  	if err != nil {
  		return fmt.Errorf("dial peer %s: %w", addr, err)
  	}
  	p.conns[serverID] = &peerConn{
  		target: serverID,
  		addr:   addr,
  		conn:   conn,
  		client: spatialserverv1.NewGameServerClient(conn),
  	}
  	return nil
  }

  func (p *PeerRegistry) Client(serverID types.ServerID) (spatialserverv1.GameServerClient, bool) {
  	p.mu.RLock()
  	defer p.mu.RUnlock()
  	pc, ok := p.conns[serverID]
  	if !ok {
  		return nil, false
  	}
  	return pc.client, true
  }

  func (p *PeerRegistry) Close() {
  	p.mu.Lock()
  	defer p.mu.Unlock()
  	for _, pc := range p.conns {
  		_ = pc.conn.Close()
  	}
  	p.conns = make(map[types.ServerID]*peerConn)
  }
  ```

- [ ] **Step 4: Refactor `pkg/game/game.go` for multi-zone**

  Replace the `Game` struct and the zone/entity/tick methods. The full new top section (imports stay, add `"fmt"`):

  ```go
  const (
  	DefaultZoneID    = types.ZoneID("default")
  	GhostTTLDefault  = 500 * time.Millisecond
  )

  type Game struct {
  	ServerID   types.ServerID
  	Zones      map[types.ZoneID]*zone.Zone
  	Entities   map[types.EntityID]*entity.Entity
  	entityZone map[types.EntityID]types.ZoneID
  	aoiIndex   map[types.ZoneID]*aoi.AOI
  	entityAOI  map[types.EntityID]*entityAOIState
  	ghostStore map[types.ZoneID]map[types.EntityID]*ghostEntry
  	peers      *PeerRegistry
  	peerZones  map[types.ZoneID]types.ServerID
  	crossCh    chan crossZoneEvent
  	Inbox      chan InboundPacket
  	Outbox     chan OutboundPacket
  	aoiRadius  float64
  	tickRate   time.Duration
  	ghostTTL   time.Duration
  	cmds       chan func()
  	mu         sync.RWMutex
  }

  type crossZoneEvent struct {
  	kind     string
  	zoneID   types.ZoneID
  	entityID types.EntityID
  	position types.Vector3
  }

  func New(sid types.ServerID, opts ...Option) *Game {
  	g := &Game{
  		ServerID:   sid,
  		Zones:      make(map[types.ZoneID]*zone.Zone),
  		Entities:   make(map[types.EntityID]*entity.Entity),
  		entityZone: make(map[types.EntityID]types.ZoneID),
  		aoiIndex:   make(map[types.ZoneID]*aoi.AOI),
  		entityAOI:  make(map[types.EntityID]*entityAOIState),
  		ghostStore: make(map[types.ZoneID]map[types.EntityID]*ghostEntry),
  		peers:      NewPeerRegistry(),
  		peerZones:  make(map[types.ZoneID]types.ServerID),
  		crossCh:    make(chan crossZoneEvent, cmdChannelBuffer),
  		Inbox:      make(chan InboundPacket, InboxBufferSize),
  		Outbox:     make(chan OutboundPacket, InboxBufferSize),
  		aoiRadius:  DefaultAOIRadius,
  		tickRate:   DefaultTickRate,
  		ghostTTL:   GhostTTLDefault,
  		cmds:       make(chan func(), cmdChannelBuffer),
  	}
  	for _, opt := range opts {
  		opt(g)
  	}
  	return g
  }

  func WithGhostTTL(d time.Duration) Option {
  	return func(g *Game) { g.ghostTTL = d }
  }

  func (g *Game) Peers() *PeerRegistry { return g.peers }

  func (g *Game) AOIFor(zid types.ZoneID) *aoi.AOI {
  	g.mu.RLock()
  	defer g.mu.RUnlock()
  	return g.aoiIndex[zid]
  }

  func (g *Game) ZoneOf(id types.EntityID) types.ZoneID {
  	g.mu.RLock()
  	defer g.mu.RUnlock()
  	return g.entityZone[id]
  }

  func (g *Game) AssignZone(z *zone.Zone) error {
  	g.mu.Lock()
  	defer g.mu.Unlock()
  	if _, exists := g.aoiIndex[z.ID]; exists {
  		return fmt.Errorf("zone %s: %w", z.ID, types.ErrConflict)
  	}
  	g.Zones[z.ID] = z
  	g.aoiIndex[z.ID] = aoi.New(DefaultCellSize, g.aoiRadius)
  	g.ghostStore[z.ID] = make(map[types.EntityID]*ghostEntry)
  	return nil
  }

  func (g *Game) ReleaseZone(id types.ZoneID) error {
  	g.mu.Lock()
  	defer g.mu.Unlock()
  	if _, exists := g.aoiIndex[id]; !exists {
  		return fmt.Errorf("zone %s: %w", id, types.ErrNotFound)
  	}
  	for eid, zid := range g.entityZone {
  		if zid == id {
  			delete(g.Entities, eid)
  			delete(g.entityAOI, eid)
  			delete(g.entityZone, eid)
  		}
  	}
  	delete(g.aoiIndex, id)
  	delete(g.ghostStore, id)
  	delete(g.Zones, id)
  	return nil
  }

  func (g *Game) RegisterPeerZone(zoneID types.ZoneID, serverID types.ServerID) {
  	g.mu.Lock()
  	g.peerZones[zoneID] = serverID
  	g.mu.Unlock()
  }

  func (g *Game) addEntity(e *entity.Entity) {
  	g.mu.Lock()
  	defer g.mu.Unlock()
  	zid := e.ZoneID
  	if zid == "" {
  		for id := range g.aoiIndex {
  			zid = id
  			break
  		}
  		e.ZoneID = zid
  	}
  	grid, ok := g.aoiIndex[zid]
  	if !ok {
  		return
  	}
  	g.Entities[e.ID] = e
  	g.entityZone[e.ID] = zid
  	grid.Enter(e.ID, e.Position)
  	g.entityAOI[e.ID] = &entityAOIState{
  		visible:      make(map[types.EntityID]struct{}),
  		lastPosition: e.Position,
  	}
  }

  func (g *Game) AddEntity(e *entity.Entity) { g.addEntity(e) }

  func (g *Game) removeEntity(id types.EntityID) {
  	g.mu.Lock()
  	defer g.mu.Unlock()
  	zid, ok := g.entityZone[id]
  	if !ok {
  		return
  	}
  	if grid, ok := g.aoiIndex[zid]; ok {
  		grid.Leave(id)
  	}
  	delete(g.Entities, id)
  	delete(g.entityAOI, id)
  	delete(g.entityZone, id)
  }

  func (g *Game) RemoveEntity(id types.EntityID) { g.removeEntity(id) }

  func (g *Game) EntityCount() int {
  	g.mu.RLock()
  	defer g.mu.RUnlock()
  	return len(g.Entities)
  }
  ```

  Replace `tick()` and the AOI-bound helpers to operate per-zone:

  ```go
  func (g *Game) tick() {
  	g.applyCmds()
  	for {
  		select {
  		case pkt := <-g.Inbox:
  			g.dispatch(pkt)
  		default:
  			g.mu.RLock()
  			zones := make([]types.ZoneID, 0, len(g.aoiIndex))
  			for zid := range g.aoiIndex {
  				zones = append(zones, zid)
  			}
  			g.mu.RUnlock()
  			for _, zid := range zones {
  				g.detectZoneBoundaries(zid)
  				g.updateVisibility(zid)
  				g.sweepGhosts(zid)
  			}
  			return
  		}
  	}
  }

  func (g *Game) detectZoneBoundaries(zid types.ZoneID) {
  	g.mu.Lock()
  	defer g.mu.Unlock()
  	grid := g.aoiIndex[zid]
  	if grid == nil {
  		return
  	}
  	for id, z := range g.entityZone {
  		if z != zid {
  			continue
  		}
  		e := g.Entities[id]
  		state, exists := g.entityAOI[id]
  		if !exists {
  			continue
  		}
  		oldCellX, oldCellY := grid.CellCoord(state.lastPosition)
  		curCellX, curCellY := grid.CellCoord(e.Position)
  		if oldCellX == curCellX && oldCellY == curCellY {
  			continue
  		}
  		ghostID := types.EntityID(string(id) + "_ghost")
  		g.ghostStore[zid][ghostID] = &ghostEntry{
  			entityID:  id,
  			position:  state.lastPosition,
  			createdAt: time.Now(),
  			expiresAt: time.Now().Add(g.ghostTTL),
  		}
  		grid.Enter(ghostID, state.lastPosition)
  	}
  }

  func (g *Game) sweepGhosts(zid types.ZoneID) {
  	g.mu.Lock()
  	defer g.mu.Unlock()
  	grid := g.aoiIndex[zid]
  	store := g.ghostStore[zid]
  	if grid == nil || store == nil {
  		return
  	}
  	now := time.Now()
  	for id, ghost := range store {
  		if now.After(ghost.expiresAt) {
  			grid.Leave(id)
  			delete(store, id)
  		}
  	}
  }

  func (g *Game) GhostCount() int {
  	g.mu.RLock()
  	defer g.mu.RUnlock()
  	n := 0
  	for _, store := range g.ghostStore {
  		n += len(store)
  	}
  	return n
  }

  func (g *Game) updateVisibility(zid types.ZoneID) {
  	g.mu.Lock()
  	defer g.mu.Unlock()
  	grid := g.aoiIndex[zid]
  	if grid == nil {
  		return
  	}
  	for id, z := range g.entityZone {
  		if z != zid {
  			continue
  		}
  		e := g.Entities[id]
  		if e == nil {
  			continue
  		}
  		current := grid.EntitiesInRange(e.Position, g.aoiRadius)
  		currentSet := make(map[types.EntityID]struct{}, len(current))
  		for _, other := range current {
  			currentSet[other] = struct{}{}
  		}
  		state, exists := g.entityAOI[id]
  		if !exists {
  			state = &entityAOIState{visible: make(map[types.EntityID]struct{}), lastPosition: e.Position}
  			g.entityAOI[id] = state
  		}
  		for other := range currentSet {
  			if _, seen := state.visible[other]; !seen && other != id {
  				if neighbor, ok := g.Entities[other]; ok {
  					select {
  					case g.Outbox <- OutboundPacket{ClientID: string(id), Data: encodeSpawn(neighbor)}:
  					default:
  					}
  				}
  			}
  		}
  		for other := range state.visible {
  			if _, still := currentSet[other]; !still {
  				select {
  				case g.Outbox <- OutboundPacket{ClientID: string(id), Data: encodeDespawn(other)}:
  				default:
  				}
  			}
  		}
  		state.visible = currentSet
  		state.lastPosition = e.Position
  	}
  }

  func (g *Game) dispatch(pkt InboundPacket) {
  	id, payload, _, err := protocol.Decode(pkt.Data)
  	if err != nil {
  		return
  	}
  	if id != protocol.PacketIDPositionUpdate {
  		return
  	}
  	var upd v1.EntityUpdate
  	if err := proto.Unmarshal(payload, &upd); err != nil {
  		return
  	}
  	g.mu.Lock()
  	defer g.mu.Unlock()
  	e, ok := g.Entities[types.EntityID(pkt.ClientID)]
  	if !ok {
  		return
  	}
  	e.Position.X = upd.GetPosition().GetX()
  	e.Position.Y = upd.GetPosition().GetY()
  	e.Position.Z = upd.GetPosition().GetZ()
  	if grid, ok := g.aoiIndex[g.entityZone[e.ID]]; ok {
  		grid.Move(e.ID, e.Position)
  	}
  }
  ```

- [ ] **Step 5: Update the two existing tests that referenced the removed `g.aoi` field**

  In `pkg/game/game_test.go`, change `g.aoi.EntitiesInRange(...)` usages:
  - In `TestAOI_AddEntityRegistersInAOI`: assign a zone first and use `g.AOIFor(types.ZoneID("z1")).EntitiesInRange(...)`. Set `e.ZoneID = types.ZoneID("z1")` and call `g.AssignZone(zone.New(types.ZoneID("z1"), types.RuntimeID("r1"), 0, 0, 100))` before `g.AddEntity(e)`.
  - In `TestAOI_RemoveEntityRemovesFromAOI`: same zone setup, use `g.AOIFor(types.ZoneID("z1"))`.
  - In `TestTick_EntityFarAwayNoSpawn`: same change for the `g.aoi.EntitiesInRange` line.

- [ ] **Step 6: Run tests to verify pass**

  Run: `go test ./pkg/game/... -v -race -cover`
  Expected: PASS — all existing tests plus the two new multi-zone tests green.

- [ ] **Step 7: Commit**

  ```bash
  git add pkg/game/peer.go pkg/game/game.go pkg/game/game_test.go
  git commit -m "refactor: make game server zone-aware with per-zone aoi grids"
  ```

---

### Task 2: AssignZone/ReleaseZone gRPC Handlers

**Files:**
- Modify: `apps/game-server/main.go`

- [ ] **Step 1: Write the failing test**

  Create `apps/game-server/main_test.go`:

  ```go
  package main

  import (
  	"context"
  	"testing"

  	"github.com/stretchr/testify/require"

  	"github.com/thaolaptrinh/spatial-server/internal/types"
  	"github.com/thaolaptrinh/spatial-server/pkg/game"
  	spatialserverv1 "github.com/thaolaptrinh/spatial-server/proto/gen/spatialserver/v1"
  )

  func TestAssignZoneRPC_CreatesZone(t *testing.T) {
  	g := game.New(types.ServerID("gs-1"))
  	srv := newGameServerServer(g)

  	resp, err := srv.AssignZone(context.Background(), &spatialserverv1.AssignZoneRequest{
  		ZoneId:    "z1",
  		RuntimeId: "r1",
  		GridX:     0,
  		GridY:     0,
  		ZoneSize:  100,
  	})
  	require.NoError(t, err)
  	require.True(t, resp.GetSuccess())
  	require.NotNil(t, g.AOIFor(types.ZoneID("z1")))
  }

  func TestReleaseZoneRPC_TeardownZone(t *testing.T) {
  	g := game.New(types.ServerID("gs-1"))
  	srv := newGameServerServer(g)
  	_, _ = srv.AssignZone(context.Background(), &spatialserverv1.AssignZoneRequest{ZoneId: "z1", RuntimeId: "r1"})
  	require.NotNil(t, g.AOIFor(types.ZoneID("z1")))

  	resp, err := srv.ReleaseZone(context.Background(), &spatialserverv1.ReleaseZoneRequest{ZoneId: "z1", RuntimeId: "r1"})
  	require.NoError(t, err)
  	require.True(t, resp.GetSuccess())
  	require.Nil(t, g.AOIFor(types.ZoneID("z1")))
  }
  ```

- [ ] **Step 2: Run test to verify it fails**

  Run: `go test ./apps/game-server/... -run TestAssignZoneRPC -v`
  Expected: FAIL — `srv.AssignZone` undefined (`UnimplementedGameServerServer` returns `Unimplemented` error).

- [ ] **Step 3: Implement handlers in `apps/game-server/main.go`**

  Add imports (`pkg/zone` already used in tests; add `"github.com/thaolaptrinh/spatial-server/pkg/zone"` and `types`). Append handler methods to `gameServerServer`:

  ```go
  func (s *gameServerServer) AssignZone(ctx context.Context, req *spatialserverv1.AssignZoneRequest) (*spatialserverv1.AssignZoneResponse, error) {
  	z := zone.New(
  		types.ZoneID(req.GetZoneId()),
  		types.RuntimeID(req.GetRuntimeId()),
  		int(req.GetGridX()),
  		int(req.GetGridY()),
  		req.GetZoneSize(),
  	)
  	if err := s.game.AssignZone(z); err != nil {
  		return &spatialserverv1.AssignZoneResponse{Success: false}, nil
  	}
  	return &spatialserverv1.AssignZoneResponse{Success: true}, nil
  }

  func (s *gameServerServer) ReleaseZone(ctx context.Context, req *spatialserverv1.ReleaseZoneRequest) (*spatialserverv1.ReleaseZoneResponse, error) {
  	if err := s.game.ReleaseZone(types.ZoneID(req.GetZoneId())); err != nil {
  		return &spatialserverv1.ReleaseZoneResponse{Success: false}, nil
  	}
  	return &spatialserverv1.ReleaseZoneResponse{Success: true}, nil
  }
  ```

  Add `"github.com/thaolaptrinh/spatial-server/pkg/zone"` to the import block.

- [ ] **Step 4: Run tests to verify pass**

  Run: `go test ./apps/game-server/... -v -race`
  Expected: PASS.

- [ ] **Step 5: Commit**

  ```bash
  git add apps/game-server/main.go apps/game-server/main_test.go
  git commit -m "feat: implement assignzone and releasezone rpc handlers"
  ```

---

### Task 3: Cross-Zone AOI Boundary + QueryEntities

**Files:**
- Modify: `pkg/game/peer.go`
- Create: `pkg/game/boundary.go`
- Create: `pkg/game/boundary_test.go`
- Modify: `apps/game-server/main.go`

- [ ] **Step 1: Write the failing test**

  Create `pkg/game/boundary_test.go`:

  ```go
  package game

  import (
  	"testing"
  	"time"

  	"github.com/stretchr/testify/assert"
  	"github.com/stretchr/testify/require"

  	"github.com/thaolaptrinh/spatial-server/internal/types"
  	"github.com/thaolaptrinh/spatial-server/pkg/entity"
  	"github.com/thaolaptrinh/spatial-server/pkg/zone"
  )

  func TestReconcileGhosts_QueriesNeighborAndStoresGhosts(t *testing.T) {
  	g := New(types.ServerID("gs-A"))
  	require.NoError(t, g.AssignZone(zone.New(types.ZoneID("z1"), types.RuntimeID("r1"), 0, 0, 100)))
  	require.NoError(t, g.AssignZone(zone.New(types.ZoneID("z2"), types.RuntimeID("r1"), 1, 0, 100)))

  	// z2 is owned by a neighbor server (stubbed querier returns one entity near boundary)
  	g.RegisterPeerZone(types.ZoneID("z2"), types.ServerID("gs-B"))
  	g.SetNeighborQuerier(func(zoneID types.ZoneID, gridX, gridY int, radius float64) []neighborEntity {
  		if zoneID != types.ZoneID("z2") {
  			return nil
  		}
  		return []neighborEntity{{ID: types.EntityID("remote1"), Type: "avatar", Pos: types.Vector3{X: 105, Z: 10}}}
  	})

  	require.Equal(t, 0, ghostStoreCount(g, types.ZoneID("z1")))
  	g.ReconcileNeighborGhosts(types.ZoneID("z1"), types.ZoneID("z2"), time.Now)
  	// remote1 registered as ghost in z1's AOI
  	assert.Equal(t, 1, ghostStoreCount(g, types.ZoneID("z1")))
  }
  ```

- [ ] **Step 2: Run test to verify it fails**

  Run: `go test ./pkg/game/... -run TestReconcileGhosts -v`
  Expected: FAIL — `SetNeighborQuerier`, `ReconcileNeighborGhosts`, `neighborEntity`, `ghostStoreCount` undefined.

- [ ] **Step 3: Add the neighbor-querier seam to `pkg/game/peer.go`**

  Append to `peer.go`:

  ```go
  type neighborEntity struct {
  	ID   types.EntityID
  	Type string
  	Pos  types.Vector3
  }

  type NeighborQuerier func(zoneID types.ZoneID, gridX, gridY int, radius float64) []neighborEntity

  func (p *PeerRegistry) SetQuerier(q NeighborQuerier) {
  	p.mu.Lock()
  	p.querier = q
  	p.mu.Unlock()
  }

  func (p *PeerRegistry) Querier() NeighborQuerier {
  	p.mu.RLock()
  	defer p.mu.RUnlock()
  	return p.querier
  }
  ```

  Add a `querier NeighborQuerier` field to the `PeerRegistry` struct.

- [ ] **Step 4: Create `pkg/game/boundary.go`**

  ```go
  package game

  import (
  	"time"

  	"github.com/thaolaptrinh/spatial-server/internal/types"
  	spatialserverv1 "github.com/thaolaptrinh/spatial-server/proto/gen/spatialserver/v1"
  )

  type ghostKind int

  const (
  	ghostLocal ghostKind = iota
  	ghostRemote
  )

  type remoteGhost struct {
  	entityID  types.EntityID
  	zoneID    types.ZoneID
  	typ       string
  	position  types.Vector3
  	expiresAt time.Time
  }

  func (g *Game) SetNeighborQuerier(q NeighborQuerier) {
  	g.peers.SetQuerier(q)
  }

  func (g *Game) ReconcileNeighborGhosts(ownerZone, neighborZone types.ZoneID, now func() time.Time) {
  	g.mu.Lock()
  	defer g.mu.Unlock()
  	grid := g.aoiIndex[ownerZone]
  	store := g.ghostStore[ownerZone]
  	if grid == nil || store == nil {
  		return
  	}
  	z := g.Zones[ownerZone]
  	if z == nil {
  		return
  	}
  	querier := g.peers.Querier()
  	if querier == nil {
  		return
  	}
  	// Query the band along the shared edge with the neighbor.
  	results := querier(neighborZone, z.Grid.X, z.Grid.Y, g.aoiRadius)
  	seen := make(map[types.EntityID]struct{}, len(results))
  	for _, n := range results {
  		ghostID := types.EntityID(string(n.ID) + "@ghost")
  		seen[ghostID] = struct{}{}
  		if _, exists := store[ghostID]; !exists {
  			grid.Enter(ghostID, n.Pos)
  		} else {
  			grid.Move(ghostID, n.Pos)
  		}
  		store[ghostID] = &remoteGhost{
  			entityID:  n.ID,
  			zoneID:    neighborZone,
  			typ:       n.Type,
  			position:  n.Pos,
  			expiresAt: now().Add(g.ghostTTL * 6), // remote ghosts live longer than local ones
  		}.toEntry(now(), g.ghostTTL*6)
  	}
  	// Evict remote ghosts no longer returned by the neighbor.
  	for gid, entry := range store {
  		if _, ok := seen[gid]; ok {
  			continue
  		}
  		if _, isRemote := entry.remote(); isRemote && now().After(entry.expiresAt) {
  			grid.Leave(gid)
  			delete(store, gid)
  		}
  	}
  }

  func (g *Game) queryLocal(zoneID types.ZoneID, gridX, gridY int, radius float64) []*spatialserverv1.EntitySnapshot {
  	g.mu.RLock()
  	defer g.mu.RUnlock()
  	grid := g.aoiIndex[zoneID]
  	if grid == nil {
  		return nil
  	}
  	center := types.Vector3{X: float64(gridX) * DefaultCellSize, Z: float64(gridY) * DefaultCellSize}
  	ids := grid.EntitiesInRange(center, radius)
  	var snaps []*spatialserverv1.EntitySnapshot
  	for _, id := range ids {
  		e, ok := g.Entities[id]
  		if !ok {
  			continue
  		}
  		snaps = append(snaps, &spatialserverv1.EntitySnapshot{
  			EntityId: string(e.ID),
  			Type:     e.Type,
  			Position: &spatialserverv1.Vector3{X: e.Position.X, Y: e.Position.Y, Z: e.Position.Z},
  		})
  	}
  	return snaps
  }

  func ghostStoreCount(g *Game, zoneID types.ZoneID) int {
  	g.mu.RLock()
  	defer g.mu.RUnlock()
  	return len(g.ghostStore[zoneID])
  }
  ```

  Add `remote()`/`toEntry()` helpers and a `kind ghostKind` field on `ghostEntry`:

  ```go
  type ghostEntry struct {
  	kind      ghostKind
  	entityID  types.EntityID
  	originZone types.ZoneID
  	position  types.Vector3
  	createdAt time.Time
  	expiresAt time.Time
  }

  func (e ghostEntry) remote() (types.ZoneID, bool) {
  	return e.originZone, e.kind == ghostRemote
  }

  func (r remoteGhost) toEntry(now time.Time, ttl time.Duration) *ghostEntry {
  	return &ghostEntry{
  		kind:       ghostRemote,
  		entityID:   r.entityID,
  		originZone: r.zoneID,
  		position:   r.position,
  		createdAt:  now,
  		expiresAt:  r.expiresAt,
  	}
  }
  ```

  Replace the existing `ghostEntry` definition in `game.go` with the version above (add `kind` and `originZone` fields; local ghosts created in `detectZoneBoundaries` set `kind: ghostLocal`).

- [ ] **Step 5: Implement `QueryEntities` RPC in `apps/game-server/main.go`**

  ```go
  func (s *gameServerServer) QueryEntities(ctx context.Context, req *spatialserverv1.QueryEntitiesRequest) (*spatialserverv1.QueryEntitiesResponse, error) {
  	// Delegate to the first owned zone (single-zone query semantics).
  	var snaps []*spatialserverv1.EntitySnapshot
  	for zid := range s.game.Zones {
  		snaps = s.game.QueryLocal(types.ZoneID(zid), int(req.GetGridX()), int(req.GetGridY()), req.GetRadius())
  		break
  	}
  	return &spatialserverv1.QueryEntitiesResponse{Entities: snaps}, nil
  }
  ```

  Expose `QueryLocal` on `Game` as a public wrapper around `queryLocal`:

  ```go
  func (g *Game) QueryLocal(zoneID types.ZoneID, gridX, gridY int, radius float64) []*v1.EntitySnapshot {
  	return g.queryLocal(zoneID, gridX, gridY, radius)
  }
  ```

- [ ] **Step 6: Run tests to verify pass**

  Run: `go test ./pkg/game/... ./apps/game-server/... -v -race`
  Expected: PASS.

- [ ] **Step 7: Commit**

  ```bash
  git add pkg/game/peer.go pkg/game/boundary.go pkg/game/boundary_test.go pkg/game/game.go apps/game-server/main.go
  git commit -m "feat: cross-zone aoi boundary with neighbor ghost reconciliation"
  ```

---

### Task 4: NotifyEntityEnter / NotifyEntityLeave / SendEntityUpdate

**Files:**
- Modify: `pkg/game/boundary.go`
- Modify: `pkg/game/boundary_test.go`
- Modify: `pkg/game/peer.go`
- Modify: `apps/game-server/main.go`

- [ ] **Step 1: Write the failing test**

  Append to `pkg/game/boundary_test.go`:

  ```go
  func TestNotifyEntityEnter_StoresRemoteGhost(t *testing.T) {
  	g := New(types.ServerID("gs-B"))
  	require.NoError(t, g.AssignZone(zone.New(types.ZoneID("z2"), types.RuntimeID("r1"), 1, 0, 100)))
  	require.Equal(t, 0, ghostStoreCount(g, types.ZoneID("z2")))

  	g.ApplyEntityEnter(types.ZoneID("z2"), types.EntityID("e1"), "avatar", types.Vector3{X: 95, Z: 10})
  	assert.Equal(t, 1, ghostStoreCount(g, types.ZoneID("z2")))

  	g.ApplyEntityLeave(types.ZoneID("z2"), types.EntityID("e1"))
  	assert.Equal(t, 0, ghostStoreCount(g, types.ZoneID("z2")))
  }

  func TestSendEntityUpdate_MovesRemoteGhost(t *testing.T) {
  	g := New(types.ServerID("gs-B"))
  	require.NoError(t, g.AssignZone(zone.New(types.ZoneID("z2"), types.RuntimeID("r1"), 1, 0, 100)))
  	g.ApplyEntityEnter(types.ZoneID("z2"), types.EntityID("e1"), "avatar", types.Vector3{X: 95, Z: 10})

  	g.ApplyEntityUpdate(types.ZoneID("z2"), types.EntityID("e1"), types.Vector3{X: 97, Z: 12})
  	store := g.ghostStoreFor(types.ZoneID("z2"))
  	entry := store[types.EntityID("e1@ghost")]
  	require.NotNil(t, entry)
  	assert.InDelta(t, 97.0, entry.position.X, 0.001)
  }
  ```

- [ ] **Step 2: Run test to verify it fails**

  Run: `go test ./pkg/game/... -run TestNotifyEntityEnter -v`
  Expected: FAIL — `ApplyEntityEnter`/`ApplyEntityLeave`/`ApplyEntityUpdate`/`ghostStoreFor` undefined.

- [ ] **Step 3: Implement the apply methods in `pkg/game/boundary.go`**

  ```go
  func (g *Game) ghostStoreFor(zoneID types.ZoneID) map[types.EntityID]*ghostEntry {
  	g.mu.RLock()
  	defer g.mu.RUnlock()
  	return g.ghostStore[zoneID]
  }

  func (g *Game) ApplyEntityEnter(zoneID types.ZoneID, entityID types.EntityID, typ string, pos types.Vector3) {
  	g.mu.Lock()
  	defer g.mu.Unlock()
  	grid := g.aoiIndex[zoneID]
  	store := g.ghostStore[zoneID]
  	if grid == nil || store == nil {
  		return
  	}
  	ghostID := types.EntityID(string(entityID) + "@ghost")
  	grid.Enter(ghostID, pos)
  	store[ghostID] = &ghostEntry{
  		kind:       ghostRemote,
  		entityID:   entityID,
  		originZone: zoneID,
  		position:   pos,
  		createdAt:  time.Now(),
  		expiresAt:  time.Now().Add(g.ghostTTL * 6),
  	}
  }

  func (g *Game) ApplyEntityLeave(zoneID types.ZoneID, entityID types.EntityID) {
  	g.mu.Lock()
  	defer g.mu.Unlock()
  	grid := g.aoiIndex[zoneID]
  	store := g.ghostStore[zoneID]
  	if grid == nil || store == nil {
  		return
  	}
  	ghostID := types.EntityID(string(entityID) + "@ghost")
  	grid.Leave(ghostID)
  	delete(store, ghostID)
  }

  func (g *Game) ApplyEntityUpdate(zoneID types.ZoneID, entityID types.EntityID, pos types.Vector3) {
  	g.mu.Lock()
  	defer g.mu.Unlock()
  	grid := g.aoiIndex[zoneID]
  	store := g.ghostStore[zoneID]
  	if grid == nil || store == nil {
  		return
  	}
  	ghostID := types.EntityID(string(entityID) + "@ghost")
  	entry, ok := store[ghostID]
  	if !ok {
  		return
  	}
  	grid.Move(ghostID, pos)
  	entry.position = pos
  	entry.expiresAt = time.Now().Add(g.ghostTTL * 6)
  }
  ```

- [ ] **Step 4: Implement the three RPC handlers in `apps/game-server/main.go`**

  ```go
  func (s *gameServerServer) NotifyEntityEnter(ctx context.Context, req *spatialserverv1.EntityEnterLeave) (*spatialserverv1.NotifyResponse, error) {
  	pos := req.GetPosition()
  	s.game.ApplyEntityEnter(types.ZoneID(req.GetZoneId()), types.EntityID(req.GetEntityId()), "", types.Vector3{X: pos.GetX(), Y: pos.GetY(), Z: pos.GetZ()})
  	return &spatialserverv1.NotifyResponse{Acknowledged: true}, nil
  }

  func (s *gameServerServer) NotifyEntityLeave(ctx context.Context, req *spatialserverv1.EntityEnterLeave) (*spatialserverv1.NotifyResponse, error) {
  	s.game.ApplyEntityLeave(types.ZoneID(req.GetZoneId()), types.EntityID(req.GetEntityId()))
  	return &spatialserverv1.NotifyResponse{Acknowledged: true}, nil
  }

  func (s *gameServerServer) SendEntityUpdate(ctx context.Context, req *spatialserverv1.EntityUpdate) (*spatialserverv1.Ack, error) {
  	// Determine the zone hosting the ghost for this entity by scanning owned grids.
  	for zid := range s.game.Zones {
  		s.game.ApplyEntityUpdate(types.ZoneID(zid), types.EntityID(req.GetEntityId()), types.Vector3{X: req.GetPosition().GetX(), Y: req.GetPosition().GetY(), Z: req.GetPosition().GetZ()})
  	}
  	return &spatialserverv1.Ack{Success: true}, nil
  }
  ```

- [ ] **Step 5: Run tests to verify pass**

  Run: `go test ./pkg/game/... ./apps/game-server/... -v -race`
  Expected: PASS.

- [ ] **Step 6: Commit**

  ```bash
  git add pkg/game/boundary.go pkg/game/boundary_test.go apps/game-server/main.go
  git commit -m "feat: notify entity enter/leave and cross-zone entity update handlers"
  ```

---

### Task 5: Zone Migration — PrepareTransfer + Status Machine

**Files:**
- Modify: `pkg/room/room.go`
- Create: `pkg/room/migration.go`
- Create: `pkg/room/migration_test.go`

- [ ] **Step 1: Write the failing test**

  Create `pkg/room/migration_test.go`:

  ```go
  package room

  import (
  	"testing"

  	"github.com/stretchr/testify/assert"
  	"github.com/stretchr/testify/require"

  	"github.com/thaolaptrinh/spatial-server/internal/types"
  )

  func TestPrepareTransfer_TransitionsToTransferring(t *testing.T) {
  	zo := NewZoneOwnership()
  	zo2 := newZoneTable()
  	require.NoError(t, zo2.set("z1", types.ServerID("gs-A"), types.ZoneStatusActive))
  	m := NewMigrationCoordinator(zo2)

  	accepted, err := m.PrepareTransfer("z1", types.ServerID("gs-B"))
  	require.NoError(t, err)
  	assert.True(t, accepted)

  	row, ok := zo2.get("z1")
  	require.True(t, ok)
  	assert.Equal(t, types.ZoneStatusTransferring, row.status)
  }

  func TestPrepareTransfer_RejectsWhenNotActive(t *testing.T) {
  	zo2 := newZoneTable()
  	require.NoError(t, zo2.set("z1", types.ServerID("gs-A"), types.ZoneStatusUnowned))
  	m := NewMigrationCoordinator(zo2)

  	accepted, err := m.PrepareTransfer("z1", types.ServerID("gs-B"))
  	require.Error(t, err)
  	assert.False(t, accepted)
  }

  func TestCompleteTransfer_UpdatesOwnerAndStatus(t *testing.T) {
  	zo2 := newZoneTable()
  	require.NoError(t, zo2.set("z1", types.ServerID("gs-A"), types.ZoneStatusActive))
  	m := NewMigrationCoordinator(zo2)
  	_, _ = m.PrepareTransfer("z1", types.ServerID("gs-B"))

  	require.NoError(t, m.CompleteTransfer("z1", types.ServerID("gs-B")))
  	row, _ := zo2.get("z1")
  	assert.Equal(t, types.ZoneStatusActive, row.status)
  	assert.Equal(t, types.ServerID("gs-B"), row.owner)
  }
  ```

- [ ] **Step 2: Run test to verify it fails**

  Run: `go test ./pkg/room/... -run TestPrepareTransfer -v`
  Expected: FAIL — `newZoneTable`, `MigrationCoordinator` undefined.

- [ ] **Step 3: Extend `pkg/room/room.go` with a status-aware ownership table**

  Append to `room.go`:

  ```go
  type zoneRow struct {
  	owner  types.ServerID
  	status types.ZoneStatus
  }

  type ZoneTable struct {
   	mu    sync.RWMutex
   	rows  map[string]zoneRow
   }

   func newZoneTable() *ZoneTable {
   	return &ZoneTable{rows: make(map[string]zoneRow)}
   }

   func (t *ZoneTable) set(zoneID string, owner types.ServerID, status types.ZoneStatus) error {
   	t.mu.Lock()
   	defer t.mu.Unlock()
   	t.rows[zoneID] = zoneRow{owner: owner, status: status}
   	return nil
   }

   func (t *ZoneTable) get(zoneID string) (zoneRow, bool) {
   	t.mu.RLock()
   	defer t.mu.RUnlock()
   	r, ok := t.rows[zoneID]
   	return r, ok
   }

   func (t *ZoneTable) update(zoneID string, owner types.ServerID, status types.ZoneStatus) {
   	t.mu.Lock()
   	defer t.mu.Unlock()
   	t.rows[zoneID] = zoneRow{owner: owner, status: status}
   }
  ```

- [ ] **Step 4: Create `pkg/room/migration.go`**

  ```go
  package room

  import (
  	"fmt"

  	"github.com/thaolaptrinh/spatial-server/internal/types"
  )

  type OwnershipChangeListener func(zoneID string, owner types.ServerID, status types.ZoneStatus)

  type MigrationCoordinator struct {
   	table   *ZoneTable
   	listeners []OwnershipChangeListener
   }

   func NewMigrationCoordinator(t *ZoneTable) *MigrationCoordinator {
   	if t == nil {
   		t = newZoneTable()
   	}
   	return &MigrationCoordinator{table: t}
   }

   func (m *MigrationCoordinator) WithListener(l OwnershipChangeListener) *MigrationCoordinator {
   	m.listeners = append(m.listeners, l)
   	return m
   }

   func (m *MigrationCoordinator) PrepareTransfer(zoneID string, target types.ServerID) (bool, error) {
   	row, ok := m.table.get(zoneID)
   	if !ok {
   		return false, fmt.Errorf("zone %s: %w", zoneID, types.ErrNotFound)
   	}
   	if row.status != types.ZoneStatusActive {
   		return false, fmt.Errorf("zone %s status %s: %w", zoneID, row.status, types.ErrConflict)
   	}
   	m.table.update(zoneID, row.owner, types.ZoneStatusTransferring)
   	return true, nil
   }

   func (m *MigrationCoordinator) CompleteTransfer(zoneID string, target types.ServerID) error {
   	row, ok := m.table.get(zoneID)
   	if !ok {
   		return fmt.Errorf("zone %s: %w", zoneID, types.ErrNotFound)
   	}
   	if row.status != types.ZoneStatusTransferring {
   		return fmt.Errorf("zone %s status %s: %w", zoneID, row.status, types.ErrInvalidArg)
   	}
   	m.table.update(zoneID, target, types.ZoneStatusActive)
   	for _, l := range m.listeners {
   		l(zoneID, target, types.ZoneStatusActive)
   	}
   	return nil
   }

   func (m *MigrationCoordinator) Status(zoneID string) (types.ZoneStatus, bool) {
   	row, ok := m.table.get(zoneID)
   	return row.status, ok
   }
  ```

- [ ] **Step 5: Run tests to verify pass**

  Run: `go test ./pkg/room/... -v -race`
  Expected: PASS.

- [ ] **Step 6: Commit**

  ```bash
  git add pkg/room/room.go pkg/room/migration.go pkg/room/migration_test.go
  git commit -m "feat: zone migration coordinator with prepare/complete transfer"
  ```

---

### Task 6: Zone Migration — ZoneStateSync Streaming + AOI Serialize

**Files:**
- Modify: `pkg/aoi/aoi.go`
- Modify: `pkg/aoi/aoi_test.go`
- Modify: `pkg/game/game.go`
- Modify: `apps/game-server/main.go`

- [ ] **Step 1: Write the failing test**

  Append to `pkg/aoi/aoi_test.go`:

  ```go
  func TestSerializeDeserialize_RoundTrip(t *testing.T) {
  	a := New(100, 300)
  	a.Enter("e1", types.Vector3{X: 10, Z: 10})
  	a.Enter("e2", types.Vector3{X: 250, Z: 250})

  	data, cellSize, radius := a.Serialize()
  	b := Deserialize(cellSize, radius, data)

  	ids := b.EntitiesInRange(types.Vector3{X: 10, Z: 10}, 300)
  	assert.Contains(t, ids, types.EntityID("e1"))
  	assert.Contains(t, ids, types.EntityID("e2"))
  }
  ```

- [ ] **Step 2: Run test to verify it fails**

  Run: `go test ./pkg/aoi/... -run TestSerializeDeserialize -v`
  Expected: FAIL — `Serialize`/`Deserialize` undefined.

- [ ] **Step 3: Implement serialization in `pkg/aoi/aoi.go`**

  Add an import for `encoding/gob` and `bytes`. The serialization captures positions (cells are derived):

  ```go
  type serializedCell struct {
  	X, Y int
  	IDs  []types.EntityID
  }
  type serializedAOI struct {
  	CellSize  float64
  	Radius    float64
  	Positions map[types.EntityID]types.Vector3
  }

  func (a *AOI) Serialize() ([]byte, float64, float64) {
  	a2 := serializedAOI{
  		CellSize:  a.cellSize,
  		Radius:    a.radius,
  		Positions: make(map[types.EntityID]types.Vector3, len(a.positions)),
  	}
  	for id, pos := range a.positions {
  		a2.Positions[id] = pos
  	}
  	var buf bytes.Buffer
  	_ = gob.NewEncoder(&buf).Encode(a2)
  	return buf.Bytes(), a.cellSize, a.radius
  }

  func Deserialize(cellSize, radius float64, data []byte) *AOI {
  	var s serializedAOI
  	if err := gob.NewDecoder(bytes.NewReader(data)).Decode(&s); err != nil {
  		return New(cellSize, radius)
  	}
  	a := New(s.CellSize, s.Radius)
  	for id, pos := range s.Positions {
  		a.Enter(id, pos)
  	}
  	return a
  }
  ```

- [ ] **Step 4: Add `SnapshotZone` to `Game` in `pkg/game/game.go`**

  ```go
  func (g *Game) SnapshotZone(zoneID types.ZoneID) *v1.ZoneSnapshot {
  	g.mu.RLock()
  	defer g.mu.RUnlock()
  	grid := g.aoiIndex[zoneID]
  	if grid == nil {
  		return nil
  	}
  	var snaps []*v1.EntitySnapshot
  	for id, z := range g.entityZone {
  		if z != zoneID {
  			continue
  		}
  		e := g.Entities[id]
  		if e == nil {
  			continue
  		}
  		snaps = append(snaps, &v1.EntitySnapshot{
  			EntityId: string(e.ID),
  			Type:     e.Type,
  			Position: &v1.Vector3{X: e.Position.X, Y: e.Position.Y, Z: e.Position.Z},
  		})
  	}
  	state, _, _ := grid.Serialize()
  	return &v1.ZoneSnapshot{
  		ZoneId:   &v1.ZoneID{Id: string(zoneID)},
  		Entities: snaps,
  		AoiState: state,
  	}
  }

  func (g *Game) LoadZoneSnapshot(snap *v1.ZoneSnapshot) {
  	zoneID := types.ZoneID(snap.GetZoneId().GetId())
  	g.mu.Lock()
  	if _, ok := g.aoiIndex[zoneID]; !ok {
  		g.aoiIndex[zoneID] = Deserialize(DefaultCellSize, g.aoiRadius, snap.GetAoiState())
  		g.ghostStore[zoneID] = make(map[types.EntityID]*ghostEntry)
  	}
  	g.mu.Unlock()
  	for _, esnap := range snap.GetEntities() {
  		e := entity.New(types.EntityID(esnap.GetEntityId()), esnap.GetType(), types.RuntimeID(""))
  		e.ZoneID = zoneID
  		e.Position = types.Vector3{X: esnap.GetPosition().GetX(), Y: esnap.GetPosition().GetY(), Z: esnap.GetPosition().GetZ()}
  		g.AddEntity(e)
  	}
  }
  ```

  (`(*AOI).Serialize` returns `(bytes, cellSize, radius)`; only the bytes are embedded in the snapshot — the receiver reuses its own `DefaultCellSize`/`aoiRadius` on `Deserialize`.)


- [ ] **Step 5: Implement the `ZoneStateSync` RPC in `apps/game-server/main.go`**

  This is a server-stream receiver: source GS sends `ZoneSnapshot` chunks; this target GS loads them.

  ```go
  func (s *gameServerServer) ZoneStateSync(stream spatialserverv1.GameServer_ZoneStateSyncServer) error {
  	ctx := stream.Context()
  	for {
  		snap, err := stream.Recv()
  		if err != nil {
  			return err
  		}
  		select {
  		case <-ctx.Done():
  			return ctx.Err()
  		default:
  		}
  		s.game.LoadZoneSnapshot(snap)
  	}
  }
  ```

  The source-side send helper (used by the orchestrator in Task 5's caller in `apps/room-service`) lives on `Game`:

  ```go
  func (g *Game) StreamZoneState(zoneID types.ZoneID, send func(*v1.ZoneSnapshot) error) error {
  	snap := g.SnapshotZone(zoneID)
  	if snap == nil {
  		return fmt.Errorf("zone %s: %w", zoneID, types.ErrNotFound)
  	}
  	return send(snap)
  }
  ```

  Add `"fmt"` to game.go imports if not present.

- [ ] **Step 6: Run tests to verify pass**

  Run: `go test ./pkg/aoi/... ./pkg/game/... ./apps/game-server/... -v -race`
  Expected: PASS.

- [ ] **Step 7: Commit**

  ```bash
  git add pkg/aoi/aoi.go pkg/aoi/aoi_test.go pkg/game/game.go apps/game-server/main.go
  git commit -m "feat: zone state sync streaming with aoi serialization"
  ```

---

### Task 7: Entity Migration — MigrateEntity

**Files:**
- Modify: `pkg/game/game.go`
- Modify: `pkg/game/game_test.go`
- Modify: `apps/game-server/main.go`

- [ ] **Step 1: Write the failing test**

  Append to `pkg/game/game_test.go`:

  ```go
  func TestMigrateEntityIn_RemovesFromSourceAndAddsToTarget(t *testing.T) {
  	g := New(types.ServerID("gs-B"))
  	require.NoError(t, g.AssignZone(zone.New(types.ZoneID("z2"), types.RuntimeID("r1"), 1, 0, 100)))

  	g.MigrateEntityIn(&v1.EntitySnapshot{
  		EntityId: "e1",
  		Type:     "avatar",
  		Position: &v1.Vector3{X: 120, Z: 10},
  	}, types.ZoneID("z2"))

  	assert.Equal(t, 1, g.EntityCount())
  	assert.Equal(t, types.ZoneID("z2"), g.ZoneOf(types.EntityID("e1")))
  }

  func TestMigrateEntityOut_RemovesEntity(t *testing.T) {
  	g := New(types.ServerID("gs-A"))
  	require.NoError(t, g.AssignZone(zone.New(types.ZoneID("z1"), types.RuntimeID("r1"), 0, 0, 100)))
  	e := entity.New(types.EntityID("e1"), "avatar", types.RuntimeID("r1"))
  	e.ZoneID = types.ZoneID("z1")
  	g.AddEntity(e)
  	require.Equal(t, 1, g.EntityCount())

  	g.MigrateEntityOut(types.EntityID("e1"))
  	assert.Equal(t, 0, g.EntityCount())
  }
  ```

- [ ] **Step 2: Run test to verify it fails**

  Run: `go test ./pkg/game/... -run TestMigrateEntity -v`
  Expected: FAIL — `MigrateEntityIn`/`MigrateEntityOut` undefined.

- [ ] **Step 3: Implement migration methods in `pkg/game/game.go`**

  ```go
  func (g *Game) MigrateEntityIn(snap *v1.EntitySnapshot, targetZone types.ZoneID) {
  	e := entity.New(types.EntityID(snap.GetEntityId()), snap.GetType(), types.RuntimeID(""))
  	e.ZoneID = targetZone
  	pos := snap.GetPosition()
  	e.Position = types.Vector3{X: pos.GetX(), Y: pos.GetY(), Z: pos.GetZ()}
  	g.AddEntity(e)
  	// Notify relay clients in range via a spawn packet on next visibility tick.
  }

  func (g *Game) MigrateEntityOut(id types.EntityID) {
  	g.removeEntity(id)
  }
  ```

- [ ] **Step 4: Implement the `MigrateEntity` RPC in `apps/game-server/main.go`**

  ```go
  func (s *gameServerServer) MigrateEntity(ctx context.Context, req *spatialserverv1.MigrateEntityRequest) (*spatialserverv1.MigrateEntityResponse, error) {
  	s.game.MigrateEntityIn(req.GetEntity(), types.ZoneID(req.GetTargetZoneId()))
  	return &spatialserverv1.MigrateEntityResponse{Success: true}, nil
  }
  ```

- [ ] **Step 5: Run tests to verify pass**

  Run: `go test ./pkg/game/... ./apps/game-server/... -v -race`
  Expected: PASS.

- [ ] **Step 6: Commit**

  ```bash
  git add pkg/game/game.go pkg/game/game_test.go apps/game-server/main.go
  git commit -m "feat: migrate entity across zone boundaries"
  ```

---

### Task 8: Room Service HA via Leader Election

**Files:**
- Create: `pkg/room/leaderelection.go`
- Create: `pkg/room/leaderelection_test.go`
- Modify: `apps/room-service/main.go` (placeholder wiring note — cluster required to run)

- [ ] **Step 1: Write the failing test**

  Create `pkg/room/leaderelection_test.go`:

  ```go
  package room

  import (
  	"sync/atomic"
  	"testing"
  	"time"

  	"github.com/stretchr/testify/assert"
  )

  func TestLeadershipGate_OnlyLeaderServesWrites(t *testing.T) {
  	gate := NewLeadershipGate(&fakeLease{leader: false})
  	assert.False(t, gate.IsLeader())

  	var writes atomic.Int32
  	gate.DoIfLeader(func() { writes.Add(1) })
  	assert.Equal(t, int32(0), writes.Load())

  	gate.SetLease(&fakeLease{leader: true})
  	assert.True(t, gate.IsLeader())
  	gate.DoIfLeader(func() { writes.Add(1) })
  	assert.Equal(t, int32(1), writes.Load())
  }

  type fakeLease struct{ leader bool }

  func (f *fakeLease) Acquire() bool      { return f.leader }
  func (f *fakeLease) Renew() bool        { return f.leader }
  func (f *fakeLease) Release()           {}
  func (f *fakeLease) IsHeld() bool       { return f.leader }
  func (f *fakeLease) Run(_ time.Duration) {}
  ```

- [ ] **Step 2: Run test to verify it fails**

  Run: `go test ./pkg/room/... -run TestLeadershipGate -v`
  Expected: FAIL — `NewLeadershipGate`, `LeadershipGate` undefined.

- [ ] **Step 3: Create `pkg/room/leaderelection.go`**

  ```go
  package room

  import (
  	"sync"
  	"time"
  )

  type Lease interface {
  	Acquire() bool
  	Renew() bool
  	Release()
  	IsHeld() bool
  	Run(renewInterval time.Duration)
  }

  type LeadershipGate struct {
  	mu   sync.RWMutex
  	lease Lease
  }

  func NewLeadershipGate(l Lease) *LeadershipGate {
  	return &LeadershipGate{lease: l}
  }

  func (g *LeadershipGate) SetLease(l Lease) {
  	g.mu.Lock()
  	g.lease = l
  	g.mu.Unlock()
  }

  func (g *LeadershipGate) IsLeader() bool {
  	g.mu.RLock()
  	defer g.mu.RUnlock()
  	return g.lease != nil && g.lease.IsHeld()
  }

  func (g *LeadershipGate) DoIfLeader(fn func()) {
  	if !g.IsLeader() {
  		return
  	}
  	fn()
  }
  ```

  Add a `k8sLease` implementation (compile-gated behind build, or runtime import) in the same file guarded so unit tests don't require `client-go`:

  ```go
  // NOTE: the real K3s Lease implementation lives behind the "k8sledelection"
  // build tag in leaderelection_k8s.go to keep client-go out of unit tests.
  ```

  Create `pkg/room/leaderelection_k8s.go` (build-tagged):

  ```go
  //go:build k8sledelection

  package room

  import (
  	"context"
  	"time"

  	coordinationv1 "k8s.io/api/coordination/v1"
  	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
  	"k8s.io/client-go/kubernetes"
  	"k8s.io/client-go/rest"
  )

  type K8sLease struct {
  	client     *kubernetes.Clientset
  	leaseName  string
  	holder     string
  	namespace  string
  	held       bool
  }

  func NewInClusterK8sLease(leaseName, holderIdentity, namespace string) (*K8sLease, error) {
  	cfg, err := rest.InClusterConfig()
  	if err != nil {
  		return nil, err
  	}
  	cs, err := kubernetes.NewForConfig(cfg)
  	if err != nil {
  		return nil, err
  	}
  	return &K8sLease{client: cs, leaseName: leaseName, holder: holderIdentity, namespace: namespace}, nil
  }

  func (l *K8sLease) Acquire() bool {
  	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
  	defer cancel()
  	_, err := l.client.CoordinationV1().Leases(l.namespace).Get(ctx, l.leaseName, metav1.GetOptions{})
  	if err == nil {
  		return l.Renew()
  	}
  	_, err = l.client.CoordinationV1().Leases(l.namespace).Create(ctx, &coordinationv1.Lease{
  		ObjectMeta: metav1.ObjectMeta{Name: l.leaseName},
  		Spec: coordinationv1.LeaseSpec{
  			HolderIdentity: &l.holder,
  			LeaseDurationSeconds: ptrInt32(15),
  		},
  	}, metav1.CreateOptions{})
  	l.held = err == nil
  	return l.held
  }

  func (l *K8sLease) Renew() bool { return l.Acquire() }
  func (l *K8sLease) Release()    { l.held = false }
  func (l *K8sLease) IsHeld() bool { return l.held }
  func (l *K8sLease) Run(renewInterval time.Duration) {
  	t := time.NewTicker(renewInterval)
  	defer t.Stop()
  	for range t.C {
  		l.Acquire()
  	}
  }

  func ptrInt32(v int32) *int32 { return &v }
  ```

- [ ] **Step 4: Wire leadership gating in `apps/room-service/main.go`**

  Add a `gate *LeadershipGate` field to the room service server. Gate `PrepareTransfer`/`TransferZone`/`Register` writes with `gate.DoIfLeader(...)`. Health check returns `SERVING` only when `gate.IsLeader()`, else `NOT_SERVING`. (Full wiring requires a cluster; this is the integration seam.)

- [ ] **Step 5: Run tests to verify pass**

  Run: `go test ./pkg/room/... -v -race`
  Expected: PASS (the k8s file is excluded by build tag, so no client-go needed).

  Add `k8s.io/client-go` to `go.mod` only when building with the tag:
  ```bash
  go get -t -tags k8sledelection k8s.io/client-go@latest
  ```

- [ ] **Step 6: Commit**

  ```bash
  git add pkg/room/leaderelection.go pkg/room/leaderelection_k8s.go pkg/room/leaderelection_test.go apps/room-service/main.go go.mod go.sum
  git commit -m "feat: room service ha leadership gate with k3s lease"
  ```

---

### Task 9: Heartbeat-Timeout Sweeper

**Files:**
- Create: `pkg/room/sweeper.go`
- Create: `pkg/room/sweeper_test.go`

- [ ] **Step 1: Write the failing test**

  Create `pkg/room/sweeper_test.go`:

  ```go
  package room

  import (
  	"testing"
  	"time"

  	"github.com/stretchr/testify/assert"
  	"github.com/stretchr/testify/require"

  	"github.com/thaolaptrinh/spatial-server/internal/types"
  )

  func TestSweeper_MarksShutdownAfterMissedHeartbeats(t *testing.T) {
  	reg := NewServerRegistry()
  	zo := NewZoneOwnership()
  	// gs-A owns z1 but will go silent; gs-B is healthy and eligible to receive it.
  	require.NoError(t, reg.Register(&ServerInfo{ID: types.ServerID("gs-A"), Host: "a", Port: 9000, MaxZones: 10}))
  	require.NoError(t, reg.Register(&ServerInfo{ID: types.ServerID("gs-B"), Host: "b", Port: 9000, MaxZones: 10}))
  	require.NoError(t, reg.Heartbeat(types.ServerID("gs-B"))) // flip gs-B to ACTIVE
  	require.NoError(t, zo.Claim("z1", types.ServerID("gs-A")))
  	reg.svr[types.ServerID("gs-A")].LastBeat = time.Now().Add(-20 * time.Second)

  	var reassigned []string
  	s := NewSweeper(reg, zo, Config{Interval: time.Second, MissThreshold: 15 * time.Second},
  		func(zoneID string, target types.ServerID) { reassigned = append(reassigned, zoneID+"->"+string(target)) })
  	s.sweep(time.Now())

  	info, ok := reg.Get(types.ServerID("gs-A"))
  	require.True(t, ok)
  	assert.Equal(t, types.ServerStatusShutdown, info.Status)
  	owner, _ := zo.Lookup("z1")
  	assert.Equal(t, types.ServerID(""), owner) // orphaned zones released
  	assert.NotEmpty(t, reassigned)
  }

  func TestSweeper_LeavesHealthyServersAlone(t *testing.T) {
  	reg := NewServerRegistry()
  	zo := NewZoneOwnership()
  	require.NoError(t, reg.Register(&ServerInfo{ID: types.ServerID("gs-A"), Host: "a", Port: 9000, MaxZones: 10}))
  	require.NoError(t, reg.Heartbeat(types.ServerID("gs-A"))) // fresh heartbeat + ACTIVE
  	require.NoError(t, zo.Claim("z1", types.ServerID("gs-A")))

  	s := NewSweeper(reg, zo, Config{Interval: time.Second, MissThreshold: 15 * time.Second}, nil)
  	s.sweep(time.Now())

  	info, _ := reg.Get(types.ServerID("gs-A"))
  	assert.Equal(t, types.ServerStatusActive, info.Status)
  }
  ```

- [ ] **Step 2: Run test to verify it fails**

  Run: `go test ./pkg/room/... -run TestSweeper -v`
  Expected: FAIL — `NewSweeper`, `Config`, `sweep` undefined.

- [ ] **Step 3: Create `pkg/room/sweeper.go`**

  ```go
  package room

  import (
  	"context"
  	"log/slog"
  	"time"

  	"github.com/thaolaptrinh/spatial-server/internal/types"
  )

  type Config struct {
  	Interval       time.Duration
  	MissThreshold  time.Duration
  }

  type ReassignFunc func(zoneID string, target types.ServerID)

  type Sweeper struct {
  	reg      *ServerRegistry
  	zo       *ZoneOwnership
  	cfg      Config
  	reassign ReassignFunc
  }

  func NewSweeper(reg *ServerRegistry, zo *ZoneOwnership, cfg Config, reassign ReassignFunc) *Sweeper {
  	if cfg.Interval == 0 {
  		cfg.Interval = 5 * time.Second
  	}
  	if cfg.MissThreshold == 0 {
  		cfg.MissThreshold = 15 * time.Second
  	}
  	return &Sweeper{reg: reg, zo: zo, cfg: cfg, reassign: reassign}
  }

  func (s *Sweeper) Run(ctx context.Context) {
  	t := time.NewTicker(s.cfg.Interval)
  	defer t.Stop()
  	for {
  		select {
  		case <-ctx.Done():
  			return
  		case <-t.C:
  			s.sweep(time.Now())
  		}
  	}
  }

  func (s *Sweeper) sweep(now time.Time) {
  	s.reg.mu.RLock()
  	dead := make([]types.ServerID, 0)
  	for id, info := range s.reg.svr {
  		if info.Status == types.ServerStatusShutdown {
  			continue
  		}
  		if now.Sub(info.LastBeat) > s.cfg.MissThreshold {
  			dead = append(dead, id)
  		}
  	}
  	s.reg.mu.RUnlock()

  	for _, id := range dead {
  		s.reg.mu.Lock()
  		if info, ok := s.reg.svr[id]; ok {
  			info.Status = types.ServerStatusShutdown
  		}
  		s.reg.mu.Unlock()
  		s.reassignZonesOf(id)
  		slog.Warn("server marked shutdown after heartbeat timeout", slog.String("server_id", string(id)))
  	}
  }

  func (s *Sweeper) reassignZonesOf(dead types.ServerID) {
  	s.zo.mu.Lock()
  	orphaned := make([]string, 0)
  	for zoneID, owner := range s.zo.zones {
  		if owner == dead {
  			orphaned = append(orphaned, zoneID)
  			delete(s.zo.zones, zoneID)
  		}
  	}
  	s.zo.mu.Unlock()

  	for _, zoneID := range orphaned {
  		target, ok := s.reg.LeastLoaded()
  		if !ok {
  			slog.Error("no active server to receive orphan zone", slog.String("zone_id", zoneID))
  			continue
  		}
  		if s.reassign != nil {
  			s.reassign(zoneID, target.ID)
  		}
  	}
  }
  ```

- [ ] **Step 4: Run tests to verify pass**

  Run: `go test ./pkg/room/... -v -race`
  Expected: PASS.

- [ ] **Step 5: Commit**

  ```bash
  git add pkg/room/sweeper.go pkg/room/sweeper_test.go
  git commit -m "feat: heartbeat timeout sweeper with orphan zone reassignment"
  ```

---

### Task 10: Push-Based Routing-Cache Invalidation

**Files:**
- Modify: `proto/spatialserver/v1/room_service.proto`
- Modify: `pkg/room/room.go`
- Modify: `pkg/gateway/gateway.go`
- Modify: `pkg/gateway/handler.go`
- Modify: `pkg/gateway/gateway_test.go`
- Modify: `apps/room-service/main.go`

- [ ] **Step 1: Write the failing test**

  Append to `pkg/gateway/gateway_test.go`:

  ```go
  func TestRouterCache_Invalidate(t *testing.T) {
  	rc := NewRouterCache(5 * time.Minute)
  	rc.Set("zone-1", "gs-1", "h1", 9001)
  	require.True(t, func() bool { _, ok := rc.Get("zone-1"); return ok }())

  	rc.Invalidate("zone-1")
  	_, ok := rc.Get("zone-1")
  	assert.False(t, ok)
  }

  func TestRouterCache_ApplyChange(t *testing.T) {
  	rc := NewRouterCache(5 * time.Minute)
  	rc.Set("zone-1", "gs-old", "h-old", 9001)
  	rc.ApplyChange(OwnershipChange{ZoneID: "zone-1", ServerID: "gs-new", Host: "h-new", Port: 9002})
  	entry, ok := rc.Get("zone-1")
  	require.True(t, ok)
  	assert.Equal(t, "gs-new", entry.ServerID)
  	assert.Equal(t, "h-new", entry.Host)
  }
  ```

  Add the `OwnershipChange` type and import `require` in the test file:

  ```go
  import "github.com/stretchr/testify/require"
  type OwnershipChange = spatialserverv1.OwnershipChange
  ```
  (plus the proto import).

- [ ] **Step 2: Run test to verify it fails**

  Run: `go test ./pkg/gateway/... -run "TestRouterCache_Invalidate|TestRouterCache_ApplyChange" -v`
  Expected: FAIL — `Invalidate`/`ApplyChange` undefined, `OwnershipChange` type not generated yet.

- [ ] **Step 3: Extend the proto**

  Add to `proto/spatialserver/v1/room_service.proto` inside the `RoomService` service:

  ```proto
    rpc WatchOwnership(WatchRequest) returns (stream OwnershipChange);
  ```

  and append the messages:

  ```proto
  message WatchRequest {}

  message OwnershipChange {
    string zone_id = 1;
    string server_id = 2;
    string host = 3;
    int32 port = 4;
  }
  ```

  Regenerate: `make proto`

- [ ] **Step 4: Add fan-out hooks in `pkg/room/room.go`**

  Append a watcher manager:

  ```go
  type WatcherFanout struct {
  	mu       sync.Mutex
  	channels map[string]chan *spatialserverv1.OwnershipChange
  	nextID   int
  }

  func NewWatcherFanout() *WatcherFanout {
  	return &WatcherFanout{channels: make(map[string]chan *spatialserverv1.OwnershipChange)}
  }

  func (f *WatcherFanout) Subscribe() (string, <-chan *spatialserverv1.OwnershipChange) {
  	f.mu.Lock()
  	defer f.mu.Unlock()
  	f.nextID++
  	id := fmt.Sprintf("w%d", f.nextID)
  	ch := make(chan *spatialserverv1.OwnershipChange, 16)
  	f.channels[id] = ch
  	return id, ch
  }

  func (f *WatcherFanout) Unsubscribe(id string) {
  	f.mu.Lock()
  	defer f.mu.Unlock()
  	if ch, ok := f.channels[id]; ok {
  		close(ch)
  		delete(f.channels, id)
  	}
  }

  func (f *WatcherFanout) Broadcast(change *spatialserverv1.OwnershipChange) {
  	f.mu.Lock()
  	defer f.mu.Unlock()
  	for _, ch := range f.channels {
  		select {
  		case ch <- change:
  		default:
  		}
  	}
  }
  ```

  Add `spatialserverv1` and `"fmt"` imports to room.go.

- [ ] **Step 5: Add `Invalidate`/`ApplyChange` to `pkg/gateway/gateway.go`**

  ```go
  func (rc *RouterCache) Invalidate(zoneID string) {
  	rc.mu.Lock()
  	delete(rc.entries, zoneID)
  	rc.mu.Unlock()
  }

  func (rc *RouterCache) ApplyChange(c OwnershipChange) {
  	rc.Set(c.GetZoneId(), c.GetServerId(), c.GetHost(), int(c.GetPort()))
  }
  ```

  Add a type alias in `pkg/gateway/gateway.go`:

  ```go
  import spatialserverv1 "github.com/thaolaptrinh/spatial-server/proto/gen/spatialserver/v1"

  type OwnershipChange = spatialserverv1.OwnershipChange
  ```

- [ ] **Step 6: Start the `WatchOwnership` subscriber in `pkg/gateway/handler.go`**

  Add a field `watcher ZoneWatcher` to `Handler` and a `StartOwnershipWatch(ctx)` method:

  ```go
  type ZoneWatcher interface {
  	WatchOwnership(ctx context.Context, in *spatialserverv1.WatchRequest, opts ...grpc.CallOption) (spatialserverv1.RoomService_WatchOwnershipClient, error)
  }

  func (h *Handler) StartOwnershipWatch(ctx context.Context, w ZoneWatcher) {
  	go func() {
  		for {
  			stream, err := w.WatchOwnership(ctx, &spatialserverv1.WatchRequest{})
  			if err != nil {
  				select {
  				case <-ctx.Done():
  					return
  				case <-time.After(2 * time.Second):
  				}
  				continue
  			}
  			for {
  				change, err := stream.Recv()
  				if err != nil {
  					select {
  					case <-ctx.Done():
  						return
  					case <-time.After(2 * time.Second):
  					}
  					break
  				}
  				h.cache.ApplyChange(*change)
  			}
  		}
  	}()
  }
  ```

  Add `time` import and the grpc import to handler.go.

- [ ] **Step 7: Implement `WatchOwnership` server in `apps/room-service/main.go`**

  ```go
  func (s *roomServiceServer) WatchOwnership(req *spatialserverv1.WatchRequest, stream spatialserverv1.RoomService_WatchOwnershipServer) error {
  	id, ch := s.fanout.Subscribe()
  	defer s.fanout.Unsubscribe(id)
  	ctx := stream.Context()
  	for {
  		select {
  		case <-ctx.Done():
  			return ctx.Err()
  		case change, ok := <-ch:
  			if !ok {
  				return nil
  			}
  			if err := stream.Send(change); err != nil {
  				return err
  			}
  		}
  	}
  }
  ```

  Add `fanout *WatcherFanout` to `roomServiceServer` and call `fanout.Broadcast(...)` from `PrepareTransfer` completion and the sweeper's reassign callback.

- [ ] **Step 8: Run tests to verify pass**

  Run: `go test ./pkg/gateway/... ./pkg/room/... -v -race`
  Expected: PASS.

- [ ] **Step 9: Commit**

  ```bash
  git add proto/spatialserver/v1/room_service.proto proto/gen/ pkg/room/room.go pkg/gateway/gateway.go pkg/gateway/handler.go pkg/gateway/gateway_test.go apps/room-service/main.go
  git commit -m "feat: push-based routing cache invalidation via watch ownership"
  ```

---

## Self-Review Checklist

### Spec coverage

| Spec section | Task |
|---|---|
| Multi-zone Game Server (`aoiIndex`, `zoneEntities`, `ghostStore`, `peers`) | Task 1 |
| `AssignZone`/`ReleaseZone` RPCs | Task 2 |
| Cross-zone AOI boundary + `QueryEntities` | Task 3 |
| `NotifyEntityEnter`/`Leave`, `SendEntityUpdate` | Task 4 |
| Zone migration — `PrepareTransfer` + status machine | Task 5 |
| Zone migration — `ZoneStateSync` streaming + AOI serialize | Task 6 |
| Entity migration — `MigrateEntity` | Task 7 |
| Room Service HA (K3s Lease) | Task 8 |
| Heartbeat sweeper (ADR-011) | Task 9 |
| Push-based routing-cache invalidation (`WatchOwnership`) | Task 10 |

### Placeholder scan
- No "TBD"/"TODO"/"implement later" — each step shows full code ✅
- All RPC handlers delegate to real `Game`/`MigrationCoordinator` methods defined in earlier tasks ✅
- AOI serialization is complete gob round-trip ✅

### Type consistency
- `types.ZoneStatus` Go enum values (`Unowned=0`,`Active=1`,`Transferring=2`,`Orphan=3`) used in Task 5 ✅
- `types.ServerStatusShutdown`/`Active` used in Task 9 ✅
- Proto getters match generated names (`GetZoneId`, `GetEntityId`, `GetPosition`, `GetSuccess`) ✅
- `v1` alias for proto gen package used consistently in `pkg/game` ✅
- `PeerRegistry.Client(serverID)` / `Upsert(serverID, addr)` signatures stable across tasks ✅
- `ghostEntry` extended with `kind`/`originZone` in Task 3; `detectZoneBoundaries` updated to set `ghostLocal` ✅

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/phase-3-distributed-scaling.md`. Two execution options:

**1. Subagent-Driven (recommended)** — dispatch a fresh subagent per task (10 tasks, mostly sequential — Task 1 unblocks 2-7), review between tasks, fast iteration.

**2. Inline Execution** — execute tasks in this session with checkpoints.

Which approach?
