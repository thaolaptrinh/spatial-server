package game

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/thaolaptrinh/spatial-server/internal/types"
	"github.com/thaolaptrinh/spatial-server/internal/game/entity"
	"github.com/thaolaptrinh/spatial-server/internal/game/zone"
	spatialserverv1 "github.com/thaolaptrinh/spatial-server/proto/gen/spatialserver/v1"
)

func TestReconcileGhosts_QueriesNeighborAndStoresGhosts(t *testing.T) {
	g := New(types.ServerID("gs-A"))
	require.NoError(t, g.AssignZone(zone.New(types.ZoneID("z1"), types.RuntimeID("r1"), 0, 0, 100)))
	require.NoError(t, g.AssignZone(zone.New(types.ZoneID("z2"), types.RuntimeID("r1"), 1, 0, 100)))

	g.RegisterPeerZone(types.ZoneID("z2"), types.ServerID("gs-B"))
	g.SetNeighborQuerier(func(zoneID types.ZoneID, gridX, gridY int, radius float64) []neighborEntity {
		if zoneID != types.ZoneID("z2") {
			return nil
		}
		return []neighborEntity{{ID: types.EntityID("remote1"), Type: "avatar", Pos: types.Vector3{X: 105, Z: 10}}}
	})

	require.Equal(t, 0, ghostStoreCount(g, types.ZoneID("z1")))
	g.ReconcileNeighborGhosts(types.ZoneID("z1"), types.ZoneID("z2"), time.Now)
	assert.Equal(t, 1, ghostStoreCount(g, types.ZoneID("z1")))
}

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

func TestMigrateEntityIn_RemovesFromSourceAndAddsToTarget(t *testing.T) {
	g := New(types.ServerID("gs-B"))
	require.NoError(t, g.AssignZone(zone.New(types.ZoneID("z2"), types.RuntimeID("r1"), 1, 0, 100)))

	g.MigrateEntityIn(&spatialserverv1.EntitySnapshot{
		EntityId: "e1",
		Type:     "avatar",
		Position: &spatialserverv1.Vector3{X: 120, Z: 10},
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
