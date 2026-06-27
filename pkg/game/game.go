package game

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/thaolaptrinh/spatial-server/internal/types"
	"github.com/thaolaptrinh/spatial-server/pkg/aoi"
	"github.com/thaolaptrinh/spatial-server/pkg/entity"
	"github.com/thaolaptrinh/spatial-server/pkg/protocol"
	"github.com/thaolaptrinh/spatial-server/pkg/zone"
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
)

type InboundPacket struct {
	ClientID string
	Data     []byte
}

type OutboundPacket struct {
	ClientID string
	Data     []byte
}

type entityAOIState struct {
	visible      map[types.EntityID]struct{}
	lastPosition types.Vector3
}

type ghostEntry struct {
	entityID  types.EntityID
	zoneID    types.ZoneID
	position  types.Vector3
	createdAt time.Time
	expiresAt time.Time
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
	Peers         *PeerRegistry
	entityZone    map[types.EntityID]types.ZoneID
	entityAOI     map[types.EntityID]*entityAOIState
	ghosts        map[types.EntityID]*ghostEntry
	Inbox         chan InboundPacket
	Outbox        chan OutboundPacket
	aoiRadius     float64
	tickRate      time.Duration
	ghostTTL      time.Duration
	cmds          chan func()
	mu            sync.RWMutex
	snapshotter   SnapshotWriter
	snapshotEvery int
	tickCount     int64
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
		Peers:      NewPeerRegistry(),
		entityZone: make(map[types.EntityID]types.ZoneID),
		entityAOI:  make(map[types.EntityID]*entityAOIState),
		ghosts:     make(map[types.EntityID]*ghostEntry),
		Inbox:      make(chan InboundPacket, InboxBufferSize),
		Outbox:     make(chan OutboundPacket, InboxBufferSize),
		aoiRadius:  DefaultAOIRadius,
		tickRate:   DefaultTickRate,
		ghostTTL:   5 * time.Second,
		cmds:       make(chan func(), cmdChannelBuffer),
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
	g.Zones[z.ID] = &zoneSim{
		zone:     z,
		aoi:      aoi.New(DefaultCellSize, g.aoiRadius),
		entities: make(map[types.EntityID]*entity.Entity),
	}
	return nil
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
		if ghost.zoneID == id {
			delete(g.ghosts, gid)
		}
	}
	delete(g.Zones, id)
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

func (g *Game) EntitiesNearGrid(gridX, gridY int, radius float64) []*entity.Entity {
	g.mu.RLock()
	defer g.mu.RUnlock()
	var result []*entity.Entity
	radiusSq := radius * radius
	for _, sim := range g.Zones {
		if sim.zone == nil || sim.zone.Grid.X != gridX || sim.zone.Grid.Y != gridY {
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

func (g *Game) EnqueueAddEntity(e *entity.Entity) {
	select {
	case g.cmds <- func() { g.addEntity(e) }:
	default:
	}
}

func (g *Game) EnqueueRemoveEntity(id types.EntityID) {
	select {
	case g.cmds <- func() { g.removeEntity(id) }:
	default:
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
	g.applyCmds()
	for {
		select {
		case pkt := <-g.Inbox:
			g.dispatch(pkt)
		default:
			g.tickCount++
			if g.snapshotter != nil && g.snapshotEvery > 0 && g.tickCount%int64(g.snapshotEvery) == 0 {
				g.snapshotAllZones()
			}
			g.simulate(g.tickRate)
			zids := g.zoneIDs()
			for _, zid := range zids {
				g.detectZoneBoundaries(zid)
				g.updateVisibility(zid)
			}
			g.sweepGhosts()
			return
		}
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
				"behavior": e.Behavior,
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
			entityID:  id,
			zoneID:    zid,
			position:  state.lastPosition,
			createdAt: time.Now(),
			expiresAt: time.Now().Add(g.ghostTTL),
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
		if sim := g.Zones[ghost.zoneID]; sim != nil {
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
				other, ok := g.Entities[id]
				if ok {
					select {
					case g.Outbox <- OutboundPacket{
						ClientID: string(e.ID),
						Data:     encodeSpawnFrame(other),
					}:
					default:
					}
				}
			}
		}

		for id := range state.visible {
			if _, still := currentSet[id]; !still {
				select {
				case g.Outbox <- OutboundPacket{
					ClientID: string(e.ID),
					Data:     encodeDespawnFrame(id),
				}:
				default:
				}
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
		if e.OwnerID != types.ServerID(pkt.ClientID) {
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
		frame := encodeStateFrame(e.ID, st.GetAnimation(), st.GetHealth())
		for _, obsID := range sim.aoi.EntitiesInRange(e.Position, g.aoiRadius) {
			if obsID == e.ID {
				continue
			}
			select {
			case g.Outbox <- OutboundPacket{ClientID: string(obsID), Data: frame}:
			default:
			}
		}
	}
}

func (g *Game) simulate(dt time.Duration) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for id, e := range g.Entities {
		lc, ok := e.Lifecycle.(*NPCLifecycle)
		if !ok || lc == nil || lc.Behavior == nil {
			continue
		}
		if lc.Behavior.Step(e, dt) {
			if zid, ok := g.entityZone[id]; ok {
				if sim := g.Zones[zid]; sim != nil {
					sim.aoi.Move(id, e.Position)
				}
			}
			g.enqueueMove(id, e.Position)
		}
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
	frame := encodeMoveFrame(id, pos)
	for _, obsID := range sim.aoi.EntitiesInRange(pos, g.aoiRadius) {
		if obsID == id {
			continue
		}
		select {
		case g.Outbox <- OutboundPacket{ClientID: string(obsID), Data: frame}:
		default:
		}
	}
}
