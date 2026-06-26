package game

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/thaolaptrinh/spatial-server/internal/types"
	"github.com/thaolaptrinh/spatial-server/pkg/aoi"
	"github.com/thaolaptrinh/spatial-server/pkg/entity"
	"github.com/thaolaptrinh/spatial-server/pkg/zone"
)

const (
	DefaultTickRate  = 50 * time.Millisecond
	InboxBufferSize  = 4096
	DefaultCellSize  = 100.0
	DefaultAOIRadius = 300.0
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
	mu        sync.Mutex
}

type Option func(*Game)

func WithTickRate(d time.Duration) Option {
	return func(g *Game) { g.tickRate = d }
}

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
		entityAOI: make(map[types.EntityID]*entityAOIState),
		ghosts:    make(map[types.EntityID]*ghostEntry),
		ghostTTL:  5 * time.Second,
	}
	for _, opt := range opts {
		opt(g)
	}
	return g
}

func (g *Game) AddEntity(e *entity.Entity) {
	g.Entities[e.ID] = e
	g.aoi.Enter(e.ID, e.Position)
	g.entityAOI[e.ID] = &entityAOIState{
		visible:      make(map[types.EntityID]struct{}),
		lastPosition: e.Position,
	}
}

func (g *Game) RemoveEntity(id types.EntityID) {
	g.aoi.Leave(id)
	delete(g.Entities, id)
}

func (g *Game) EntityCount() int {
	return len(g.Entities)
}

func (g *Game) AssignZone(z *zone.Zone) {
	g.Zones[z.ID] = z
}

func (g *Game) ReleaseZone(id types.ZoneID) {
	delete(g.Zones, id)
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
	g.mu.Lock()
	defer g.mu.Unlock()
	for id := range g.Entities {
		e := g.Entities[id]
		state, exists := g.entityAOI[id]
		if !exists {
			continue
		}
		oldCellX, oldCellY := g.aoi.CellCoord(state.lastPosition)
		currentCellX, currentCellY := g.aoi.CellCoord(e.Position)
		if oldCellX == currentCellX && oldCellY == currentCellY {
			continue
		}
		ghostID := types.EntityID(string(id) + "_ghost")
		g.ghosts[ghostID] = &ghostEntry{
			entityID:  id,
			position:  state.lastPosition,
			createdAt: time.Now(),
			expiresAt: time.Now().Add(g.ghostTTL),
		}
		g.aoi.Enter(ghostID, state.lastPosition)
	}
}

func (g *Game) sweepGhosts() {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := time.Now()
	for id, ghost := range g.ghosts {
		if now.After(ghost.expiresAt) {
			g.aoi.Leave(id)
			delete(g.ghosts, id)
		}
	}
}

func (g *Game) GhostCount() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.ghosts)
}

func (g *Game) updateVisibility() {
	if g.entityAOI == nil {
		return
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

		for id := range currentSet {
			if _, seen := state.visible[id]; !seen && id != e.ID {
				other, ok := g.Entities[id]
				if ok {
					g.Outbox <- OutboundPacket{
						ClientID: string(e.ID),
						Data:     []byte(fmt.Sprintf("spawn:%s", other.ID)),
					}
				}
			}
		}

		for id := range state.visible {
			if _, still := currentSet[id]; !still {
				g.Outbox <- OutboundPacket{
					ClientID: string(e.ID),
					Data:     []byte(fmt.Sprintf("despawn:%s", id)),
				}
			}
		}

		state.visible = currentSet
		state.lastPosition = e.Position
	}
}

func (g *Game) dispatch(pkt InboundPacket) {
	if len(pkt.Data) < 3 {
		return
	}
	packetID := (uint16(pkt.Data[1]) << 8) | uint16(pkt.Data[2])
	if packetID == 0x03 { // PositionUpdate
		for _, e := range g.Entities {
			if string(e.ID) == pkt.ClientID {
				g.aoi.Move(e.ID, e.Position)
				break
			}
		}
	}
}
