package game

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/thaolaptrinh/spatial-server/internal/types"
	"github.com/thaolaptrinh/spatial-server/internal/game/aoi"
	"github.com/thaolaptrinh/spatial-server/internal/game/entity"
	"github.com/thaolaptrinh/spatial-server/pkg/protocol"
	"github.com/thaolaptrinh/spatial-server/internal/game/zone"
	v1 "github.com/thaolaptrinh/spatial-server/proto/gen/spatialserver/v1"
	"google.golang.org/protobuf/proto"
)

const (
	DefaultTickRate  = 50 * time.Millisecond
	InboxBufferSize  = 4096
	DefaultCellSize  = 100.0
	DefaultAOIRadius = 300.0
	cmdChannelBuffer = 256
	DefaultZoneID    = types.ZoneID("default")

	// maxPacketsPerTick caps inbound processing per tick so a packet burst
	// cannot blow the tick budget and cause cascading overload. Remaining
	// packets are processed on subsequent ticks.
	maxPacketsPerTick = 1024
	// maxTickDelta caps the simulated delta after a pause (e.g. GC stall, cold
	// start) to prevent a spiral-of-death where a long dt causes a long tick
	// which causes an even longer dt.
	maxTickDelta = 250 * time.Millisecond
	// cmdEnqueueTimeout is how long a control-plane command (entity add/remove)
	// blocks waiting for queue space before it is dropped loudly. Control
	// commands must never be silently dropped: a drop indicates the tick loop
	// is stalled and is logged + counted.
	cmdEnqueueTimeout = 100 * time.Millisecond
)

type InboundPacket struct {
	ClientID string
	Data     []byte
}

type entityAOIState struct {
	visible      map[types.EntityID]struct{}
	lastPosition types.Vector3
}

type ghostKind int

const (
	ghostLocal ghostKind = iota
	ghostRemote
)

type ghostEntry struct {
	kind       ghostKind
	entityID   types.EntityID
	originZone types.ZoneID
	position   types.Vector3
	createdAt  time.Time
	expiresAt  time.Time
}

func (e ghostEntry) remote() (types.ZoneID, bool) {
	return e.originZone, e.kind == ghostRemote
}

type zoneSim struct {
	zone     *zone.Zone
	aoi      *aoi.AOI
	entities map[types.EntityID]*entity.Entity
}

type SnapshotWriter interface {
	Save(zoneID types.ZoneID, snapshot []byte, tick int64)
}

type Game struct {
	ServerID      types.ServerID
	Zones         map[types.ZoneID]*zoneSim
	Entities      map[types.EntityID]*entity.Entity
	entityZone    map[types.EntityID]types.ZoneID
	entityAOI     map[types.EntityID]*entityAOIState
	ghosts        map[types.EntityID]*ghostEntry
	aoiIndex      map[types.ZoneID]*aoi.AOI
	ghostStore    map[types.ZoneID]map[types.EntityID]*ghostEntry
	peers         *PeerRegistry
	peerZone      map[types.ZoneID]types.ServerID
	Inbox         chan InboundPacket
	Events        chan Event
	aoiRadius     float64
	tickRate      time.Duration
	ghostTTL      time.Duration
	cmds          chan func()
	mu            sync.RWMutex
	snapshotter   SnapshotWriter
	snapshotEvery int
	tickCount     int64

	sessionStates   map[types.EntityID]*sessionState
	reconnectWindow time.Duration
	lifecycleClock  func() time.Time
	metrics         Metrics
	lastTickAt      time.Time
}

type Option func(*Game)

func WithTickRate(d time.Duration) Option {
	return func(g *Game) { g.tickRate = d }
}

func WithSnapshotter(w SnapshotWriter, every int) Option {
	return func(g *Game) { g.snapshotter = w; g.snapshotEvery = every }
}

func New(sid types.ServerID, opts ...Option) *Game {
	g := &Game{
		ServerID:   sid,
		Zones:      make(map[types.ZoneID]*zoneSim),
		Entities:   make(map[types.EntityID]*entity.Entity),
		entityZone: make(map[types.EntityID]types.ZoneID),
		entityAOI:  make(map[types.EntityID]*entityAOIState),
		ghosts:     make(map[types.EntityID]*ghostEntry),
		aoiIndex:   make(map[types.ZoneID]*aoi.AOI),
		ghostStore: make(map[types.ZoneID]map[types.EntityID]*ghostEntry),
		peers:      NewPeerRegistry(),
		peerZone:   make(map[types.ZoneID]types.ServerID),
		Inbox:      make(chan InboundPacket, InboxBufferSize),
		Events:     make(chan Event, InboxBufferSize),
		aoiRadius:  DefaultAOIRadius,
		tickRate:   DefaultTickRate,
		ghostTTL:   5 * time.Second,
		cmds:       make(chan func(), cmdChannelBuffer),

		sessionStates:   make(map[types.EntityID]*sessionState),
		reconnectWindow: 30 * time.Second,
		lifecycleClock:  time.Now,
		metrics:         NoopMetrics{},
	}
	for _, opt := range opts {
		opt(g)
	}
	return g
}

func (g *Game) AssignZone(z *zone.Zone) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, exists := g.Zones[z.ID]; exists {
		return fmt.Errorf("zone %s: %w", z.ID, types.ErrConflict)
	}
	aoiGrid := aoi.New(DefaultCellSize, g.aoiRadius)
	g.Zones[z.ID] = &zoneSim{
		zone:     z,
		aoi:      aoiGrid,
		entities: make(map[types.EntityID]*entity.Entity),
	}
	g.aoiIndex[z.ID] = aoiGrid
	g.ghostStore[z.ID] = make(map[types.EntityID]*ghostEntry)
	return nil
}

func (g *Game) RegisterPeerZone(zoneID types.ZoneID, serverID types.ServerID) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.peerZone[zoneID] = serverID
}

func (g *Game) ReleaseZone(id types.ZoneID) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	sim, exists := g.Zones[id]
	if !exists {
		return fmt.Errorf("zone %s: %w", id, types.ErrNotFound)
	}
	for eid, zid := range g.entityZone {
		if zid != id {
			continue
		}
		delete(g.Entities, eid)
		delete(g.entityAOI, eid)
		delete(g.entityZone, eid)
	}
	for gid, ghost := range g.ghosts {
		if ghost.originZone == id {
			delete(g.ghosts, gid)
		}
	}
	delete(g.Zones, id)
	delete(g.aoiIndex, id)
	delete(g.ghostStore, id)
	delete(g.peerZone, id)
	_ = sim
	return nil
}

func (g *Game) AOIFor(zid types.ZoneID) *aoi.AOI {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if sim := g.Zones[zid]; sim != nil {
		return sim.aoi
	}
	return nil
}

func (g *Game) ZoneOf(id types.EntityID) types.ZoneID {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.entityZone[id]
}

func (g *Game) addEntity(e *entity.Entity) {
	g.mu.Lock()
	defer g.mu.Unlock()
	zid := e.ZoneID
	if zid == "" {
		zid = DefaultZoneID
		e.ZoneID = zid
	}
	sim := g.Zones[zid]
	if sim == nil {
		sim = &zoneSim{
			aoi:      aoi.New(DefaultCellSize, g.aoiRadius),
			entities: make(map[types.EntityID]*entity.Entity),
		}
		g.Zones[zid] = sim
	}
	// Enforce Space isolation: an entity's Space is always the Space of its
	// zone. This is the single source of truth for Space membership and
	// guarantees entities from different Spaces can never share state, even
	// when zones overlap by grid coordinate. Migration and snapshot load rely
	// on this derivation rather than trusting caller-supplied IDs.
	if sim.zone != nil && sim.zone.RuntimeID != "" {
		e.RuntimeID = sim.zone.RuntimeID
	}
	g.Entities[e.ID] = e
	g.entityZone[e.ID] = zid
	sim.entities[e.ID] = e
	sim.aoi.Enter(e.ID, e.Position)
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
	if sim := g.Zones[zid]; sim != nil {
		sim.aoi.Leave(id)
		delete(sim.entities, id)
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

// EntitiesNearGrid returns entities within radius of the given grid cell,
// scoped to a single Space. The Space parameter is mandatory: grid
// coordinates are relative per-Space, so without it the query would be
// ambiguous and could mix entities from different Spaces.
func (g *Game) EntitiesNearGrid(space types.RuntimeID, gridX, gridY int, radius float64) []*entity.Entity {
	g.mu.RLock()
	defer g.mu.RUnlock()
	var result []*entity.Entity
	radiusSq := radius * radius
	for _, sim := range g.Zones {
		if sim.zone == nil || sim.zone.RuntimeID != space {
			continue
		}
		if sim.zone.Grid.X != gridX || sim.zone.Grid.Y != gridY {
			continue
		}
		size := sim.zone.Size
		if size <= 0 {
			size = DefaultCellSize
		}
		center := types.Vector3{
			X: float64(gridX)*size + size/2,
			Z: float64(gridY)*size + size/2,
		}
		for _, e := range sim.entities {
			dx := e.Position.X - center.X
			dz := e.Position.Z - center.Z
			if dx*dx+dz*dz <= radiusSq {
				result = append(result, e)
			}
		}
	}
	return result
}

func (g *Game) Run(ctx context.Context) error {
	ticker := time.NewTicker(g.tickRate)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			g.tick()
		}
	}
}

// Tick runs a single simulation tick. It enables step-mode driving for
// benchmarks and deterministic replay (the production loop uses Run, which is
// wall-clock driven).
func (g *Game) Tick() { g.tick() }

func (g *Game) EnqueueAddEntity(e *entity.Entity) {
	g.enqueueCmd(func() { g.addEntity(e) }, "cmds_add")
}

func (g *Game) EnqueueRemoveEntity(id types.EntityID) {
	g.enqueueCmd(func() { g.removeEntity(id) }, "cmds_remove")
}

// enqueueCmd submits a control-plane command. Unlike data-plane packets,
// control commands block briefly for queue space and are never dropped
// silently: a drop is logged and counted as it indicates a stalled tick loop.
func (g *Game) enqueueCmd(fn func(), label string) {
	timer := time.NewTimer(cmdEnqueueTimeout)
	defer timer.Stop()
	select {
	case g.cmds <- fn:
	case <-timer.C:
		g.metrics.Dropped(label, 1)
		slog.Warn("control command dropped: command queue full", slog.String("queue", label))
	}
}

func (g *Game) applyCmds() {
	for i := 0; i < cmdChannelBuffer; i++ {
		select {
		case cmd := <-g.cmds:
			cmd()
		default:
			return
		}
	}
}

func (g *Game) tick() {
	now := g.lifecycleClock()
	if g.lastTickAt.IsZero() {
		g.lastTickAt = now
	}
	dt := now.Sub(g.lastTickAt)
	g.lastTickAt = now
	if dt > maxTickDelta {
		dt = maxTickDelta
	}

	g.applyCmds()

	// Bounded inbound drain: process at most maxPacketsPerTick packets per
	// tick so a burst cannot blow the tick budget. The rest wait until the
	// next tick (bounded by the Inbox channel capacity, drops counted by the
	// relay that enqueues them).
	processed := 0
drain:
	for {
		select {
		case pkt := <-g.Inbox:
			g.dispatch(pkt)
			processed++
			if processed >= maxPacketsPerTick {
				break drain
			}
		default:
			break drain
		}
	}

	g.tickCount++
	if g.snapshotter != nil && g.snapshotEvery > 0 && g.tickCount%int64(g.snapshotEvery) == 0 {
		g.snapshotAllZones()
	}
	g.simulate(dt)
	zids := g.zoneIDs()
	for _, zid := range zids {
		g.detectZoneBoundaries(zid)
		g.updateVisibility(zid)
	}
	g.sweepGhosts()
	g.SweepDisconnected()

	elapsed := g.lifecycleClock().Sub(now)
	g.metrics.TickDuration(elapsed)
	if elapsed > g.tickRate {
		g.metrics.TickOverrun()
	}
	g.reportGauges()
}

// reportGauges emits the per-tick gauge snapshot: queue depths, active
// spaces, and entity counts by type.
func (g *Game) reportGauges() {
	g.mu.RLock()
	defer g.mu.RUnlock()
	g.metrics.QueueDepth("inbox", len(g.Inbox))
	g.metrics.QueueDepth("events", len(g.Events))
	g.metrics.QueueDepth("cmds", len(g.cmds))

	entitiesByType := make(map[string]int, 8)
	spaces := make(map[types.RuntimeID]struct{})
	for _, e := range g.Entities {
		entitiesByType[e.Type]++
	}
	for _, sim := range g.Zones {
		if sim.zone != nil && sim.zone.RuntimeID != "" {
			spaces[sim.zone.RuntimeID] = struct{}{}
		}
	}
	g.metrics.ActiveSpaces(len(spaces))
	for typ, n := range entitiesByType {
		g.metrics.EntityCount(typ, n)
	}
}

func (g *Game) zoneIDs() []types.ZoneID {
	g.mu.RLock()
	defer g.mu.RUnlock()
	zids := make([]types.ZoneID, 0, len(g.Zones))
	for zid := range g.Zones {
		zids = append(zids, zid)
	}
	return zids
}

func (g *Game) snapshotAllZones() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.snapshotter == nil {
		return
	}
	for zid, sim := range g.Zones {
		var rows []map[string]any
		for _, e := range sim.entities {
			rows = append(rows, map[string]any{
				"id":       string(e.ID),
				"type":     e.Type,
				"behavior": string(e.Attrs["behavior"]),
				"x":        e.Position.X,
				"y":        e.Position.Y,
				"z":        e.Position.Z,
			})
		}
		data, err := json.Marshal(rows)
		if err != nil {
			slog.Warn("marshal snapshot", slog.String("error", err.Error()))
			continue
		}
		g.snapshotter.Save(zid, data, g.tickCount)
	}
}

func (g *Game) detectZoneBoundaries(zid types.ZoneID) {
	g.mu.Lock()
	defer g.mu.Unlock()
	sim := g.Zones[zid]
	if sim == nil {
		return
	}
	for id, e := range sim.entities {
		state, exists := g.entityAOI[id]
		if !exists {
			continue
		}
		oldCellX, oldCellY := sim.aoi.CellCoord(state.lastPosition)
		currentCellX, currentCellY := sim.aoi.CellCoord(e.Position)
		if oldCellX == currentCellX && oldCellY == currentCellY {
			continue
		}
		ghostID := types.EntityID(string(id) + "_ghost")
		g.ghosts[ghostID] = &ghostEntry{
			kind:       ghostLocal,
			entityID:   id,
			originZone: zid,
			position:   state.lastPosition,
			createdAt:  time.Now(),
			expiresAt:  time.Now().Add(g.ghostTTL),
		}
		sim.aoi.Enter(ghostID, state.lastPosition)
	}
}

func (g *Game) sweepGhosts() {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := time.Now()
	for id, ghost := range g.ghosts {
		if !now.After(ghost.expiresAt) {
			continue
		}
		if sim := g.Zones[ghost.originZone]; sim != nil {
			sim.aoi.Leave(id)
		}
		delete(g.ghosts, id)
	}
}

func (g *Game) GhostCount() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return len(g.ghosts)
}

func (g *Game) updateVisibility(zid types.ZoneID) {
	g.mu.Lock()
	defer g.mu.Unlock()
	sim := g.Zones[zid]
	if sim == nil {
		return
	}
	for _, e := range sim.entities {
		current := sim.aoi.EntitiesInRange(e.Position, g.aoiRadius)
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

		for id := range currentSet {
			if _, seen := state.visible[id]; !seen && id != e.ID {
				if other, ok := g.Entities[id]; ok {
					g.publish(Event{
						Kind:     EventSpawn,
						Space:    e.RuntimeID,
						Observer: e.ID,
						EntityID: other.ID,
						Type:     other.Type,
						Position: other.Position,
					})
				}
			}
		}

		for id := range state.visible {
			if _, still := currentSet[id]; !still {
				g.publish(Event{
					Kind:     EventDespawn,
					Space:    e.RuntimeID,
					Observer: e.ID,
					EntityID: id,
				})
			}
		}

		state.visible = currentSet
		state.lastPosition = e.Position
	}
}

func (g *Game) dispatch(pkt InboundPacket) {
	_, id, payload, _, _, err := protocol.Decode(pkt.Data)
	if err != nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if id == protocol.PacketIDPositionUpdate {
		var upd v1.EntityUpdate
		if err := proto.Unmarshal(payload, &upd); err != nil {
			return
		}
		e, ok := g.Entities[types.EntityID(pkt.ClientID)]
		if !ok {
			return
		}
		e.Position.X = upd.GetPosition().GetX()
		e.Position.Y = upd.GetPosition().GetY()
		e.Position.Z = upd.GetPosition().GetZ()
		if zid, ok := g.entityZone[e.ID]; ok {
			if sim := g.Zones[zid]; sim != nil {
				sim.aoi.Move(e.ID, e.Position)
			}
		}
		// Replicate the client-driven move to AOI observers. Without this, other
		// clients never see continuous movement (only spawn/despawn on cell
		// changes) — the core realtime position-sync path would be broken.
		g.enqueueMove(e.ID, e.Position)
	}
	if id == protocol.PacketIDEntityAction {
		var act v1.EntityAction
		if err := proto.Unmarshal(payload, &act); err != nil {
			return
		}
		e, ok := g.Entities[types.EntityID(act.GetEntityId())]
		if !ok {
			return
		}
		if e.OwnerID != types.OwnerID(pkt.ClientID) {
			return
		}
		if e.Lifecycle != nil {
			e.Lifecycle.OnAction(act.GetAction(), act.GetPayload())
		}
	}
	if id == protocol.PacketIDEntityState {
		var st v1.EntityState
		if err := proto.Unmarshal(payload, &st); err != nil {
			return
		}
		e, ok := g.Entities[types.EntityID(st.GetEntityId())]
		if !ok {
			return
		}
		zid, ok := g.entityZone[e.ID]
		if !ok {
			return
		}
		sim := g.Zones[zid]
		if sim == nil {
			return
		}
		frame := payload
		for _, obsID := range sim.aoi.EntitiesInRange(e.Position, g.aoiRadius) {
			if obsID == e.ID {
				continue
			}
			g.publish(Event{
				Kind:     EventState,
				Space:    e.RuntimeID,
				Observer: obsID,
				EntityID: e.ID,
				Payload:  frame,
			})
		}
	}
}

func (g *Game) simulate(dt time.Duration) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for id, e := range g.Entities {
		if e.Lifecycle == nil {
			continue
		}
		prev := e.Position
		e.Lifecycle.OnSimulate(e, dt)
		if e.Position == prev {
			continue
		}
		if zid, ok := g.entityZone[id]; ok {
			if sim := g.Zones[zid]; sim != nil {
				sim.aoi.Move(id, e.Position)
			}
		}
		g.enqueueMove(id, e.Position)
	}
}

func (g *Game) enqueueMove(id types.EntityID, pos types.Vector3) {
	zid, ok := g.entityZone[id]
	if !ok {
		return
	}
	sim := g.Zones[zid]
	if sim == nil {
		return
	}
	space := types.RuntimeID("")
	if e, ok := g.Entities[id]; ok {
		space = e.RuntimeID
	}
	for _, obsID := range sim.aoi.EntitiesInRange(pos, g.aoiRadius) {
		if obsID == id {
			continue
		}
		g.publish(Event{
			Kind:     EventMove,
			Space:    space,
			Observer: obsID,
			EntityID: id,
			Position: pos,
		})
	}
}
