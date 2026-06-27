package game

import (
	"fmt"
	"time"

	"github.com/thaolaptrinh/spatial-server/internal/types"
	"github.com/thaolaptrinh/spatial-server/internal/game/aoi"
	"github.com/thaolaptrinh/spatial-server/internal/game/entity"
	spatialserverv1 "github.com/thaolaptrinh/spatial-server/proto/gen/spatialserver/v1"
)

type remoteGhost struct {
	entityID  types.EntityID
	zoneID    types.ZoneID
	typ       string
	position  types.Vector3
	expiresAt time.Time
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
	if z == nil || z.zone == nil {
		return
	}
	querier := g.peers.Querier()
	if querier == nil {
		return
	}
	results := querier(neighborZone, z.zone.Grid.X, z.zone.Grid.Y, g.aoiRadius)
	seen := make(map[types.EntityID]struct{}, len(results))
	for _, n := range results {
		ghostID := types.EntityID(string(n.ID) + "@ghost")
		seen[ghostID] = struct{}{}
		if _, exists := store[ghostID]; !exists {
			grid.Enter(ghostID, n.Pos)
		} else {
			grid.Move(ghostID, n.Pos)
		}
		store[ghostID] = &ghostEntry{
			kind:       ghostRemote,
			entityID:   n.ID,
			originZone: neighborZone,
			position:   n.Pos,
			createdAt:  now(),
			expiresAt:  now().Add(g.ghostTTL * 6),
		}
	}
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

func (g *Game) QueryLocal(zoneID types.ZoneID, gridX, gridY int, radius float64) []*spatialserverv1.EntitySnapshot {
	return g.queryLocal(zoneID, gridX, gridY, radius)
}

func ghostStoreCount(g *Game, zoneID types.ZoneID) int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return len(g.ghostStore[zoneID])
}

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

func (g *Game) MigrateEntityIn(snap *spatialserverv1.EntitySnapshot, targetZone types.ZoneID) {
	e := entity.New(types.EntityID(snap.GetEntityId()), snap.GetType(), types.RuntimeID(""))
	e.ZoneID = targetZone
	pos := snap.GetPosition()
	e.Position = types.Vector3{X: pos.GetX(), Y: pos.GetY(), Z: pos.GetZ()}
	g.AddEntity(e)
}

func (g *Game) MigrateEntityOut(id types.EntityID) {
	g.removeEntity(id)
}

func (g *Game) SnapshotZone(zoneID types.ZoneID) *spatialserverv1.ZoneSnapshot {
	g.mu.RLock()
	defer g.mu.RUnlock()
	grid := g.aoiIndex[zoneID]
	if grid == nil {
		return nil
	}
	var snaps []*spatialserverv1.EntitySnapshot
	for id, z := range g.entityZone {
		if z != zoneID {
			continue
		}
		e := g.Entities[id]
		if e == nil {
			continue
		}
		snaps = append(snaps, &spatialserverv1.EntitySnapshot{
			EntityId: string(e.ID),
			Type:     e.Type,
			Position: &spatialserverv1.Vector3{X: e.Position.X, Y: e.Position.Y, Z: e.Position.Z},
		})
	}
	state, _, _ := grid.Serialize()
	return &spatialserverv1.ZoneSnapshot{
		ZoneId:   &spatialserverv1.ZoneID{Id: string(zoneID)},
		Entities: snaps,
		AoiState: state,
	}
}

func (g *Game) LoadZoneSnapshot(snap *spatialserverv1.ZoneSnapshot) {
	zoneID := types.ZoneID(snap.GetZoneId().GetId())
	g.mu.Lock()
	if _, ok := g.aoiIndex[zoneID]; !ok {
		g.aoiIndex[zoneID] = aoi.Deserialize(DefaultCellSize, g.aoiRadius, snap.GetAoiState())
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

func (g *Game) StreamZoneState(zoneID types.ZoneID, send func(*spatialserverv1.ZoneSnapshot) error) error {
	snap := g.SnapshotZone(zoneID)
	if snap == nil {
		return fmt.Errorf("zone %s: %w", zoneID, types.ErrNotFound)
	}
	return send(snap)
}
