package game

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/thaolaptrinh/spatial-server/internal/types"
	"github.com/thaolaptrinh/spatial-server/internal/game/entity"
	"github.com/thaolaptrinh/spatial-server/internal/game/zone"
)

// TestMultiNode_OneSpaceAcrossTwoNodes validates that a single Space can be
// executed across multiple Runtime Nodes (Game instances), each owning a
// disjoint zone of that Space.
func TestMultiNode_OneSpaceAcrossTwoNodes(t *testing.T) {
	gsA := New(types.ServerID("gs-A"))
	gsB := New(types.ServerID("gs-B"))
	require.NoError(t, gsA.AssignZone(zone.New(types.ZoneID("zA"), types.RuntimeID("s1"), 0, 0, 100)))
	require.NoError(t, gsB.AssignZone(zone.New(types.ZoneID("zB"), types.RuntimeID("s1"), 1, 0, 100)))

	eA := entity.New(types.EntityID("a"), "avatar", types.RuntimeID("s1"))
	eA.ZoneID = types.ZoneID("zA")
	eA.Position = types.Vector3{X: 50, Z: 50}
	gsA.AddEntity(eA)

	eB := entity.New(types.EntityID("b"), "avatar", types.RuntimeID("s1"))
	eB.ZoneID = types.ZoneID("zB")
	eB.Position = types.Vector3{X: 150, Z: 50}
	gsB.AddEntity(eB)

	// Both nodes serve the same Space; each owns a disjoint zone of it.
	assert.Equal(t, types.RuntimeID("s1"), gsA.Entities[types.EntityID("a")].RuntimeID)
	assert.Equal(t, types.RuntimeID("s1"), gsB.Entities[types.EntityID("b")].RuntimeID)
	assert.Equal(t, types.ZoneID("zA"), gsA.ZoneOf(types.EntityID("a")))
	assert.Equal(t, types.ZoneID("zB"), gsB.ZoneOf(types.EntityID("b")))
	assert.Equal(t, 1, gsA.EntityCount())
	assert.Equal(t, 1, gsB.EntityCount())
}

// TestCrossNodeAOI_GhostSynchronization validates AOI synchronization across
// Runtime Nodes: an entity on gs-B near the boundary is observed by gs-A as a
// remote ghost via the neighbor querier.
func TestCrossNodeAOI_GhostSynchronization(t *testing.T) {
	gsA := New(types.ServerID("gs-A"))
	gsB := New(types.ServerID("gs-B"))
	require.NoError(t, gsA.AssignZone(zone.New(types.ZoneID("zA"), types.RuntimeID("s1"), 0, 0, 100)))
	require.NoError(t, gsB.AssignZone(zone.New(types.ZoneID("zB"), types.RuntimeID("s1"), 1, 0, 100)))

	near := entity.New(types.EntityID("nearB"), "avatar", types.RuntimeID("s1"))
	near.ZoneID = types.ZoneID("zB")
	near.Position = types.Vector3{X: 105, Z: 50}
	gsB.AddEntity(near)

	// gs-A queries gs-B's zone for boundary neighbors.
	gsA.SetNeighborQuerier(func(zoneID types.ZoneID, _, _ int, _ float64) []neighborEntity {
		if zoneID != types.ZoneID("zB") {
			return nil
		}
		return []neighborEntity{{ID: near.ID, Type: near.Type, Pos: near.Position}}
	})
	gsA.RegisterPeerZone(types.ZoneID("zB"), types.ServerID("gs-B"))

	require.Equal(t, 0, ghostStoreCount(gsA, types.ZoneID("zA")))
	gsA.ReconcileNeighborGhosts(types.ZoneID("zA"), types.ZoneID("zB"), time.Now)
	assert.Equal(t, 1, ghostStoreCount(gsA, types.ZoneID("zA")), "gs-A should observe gs-B's entity as a cross-node ghost")
}

// TestRuntimeEvent_PropagationSpawnToObserver validates that when two entities
// share AOI range the simulation publishes a Spawn event addressed to the
// observer, scoped to the correct Space.
func TestRuntimeEvent_PropagationSpawnToObserver(t *testing.T) {
	g := New(types.ServerID("gs-1"))
	require.NoError(t, g.AssignZone(zone.New(types.ZoneID("z1"), types.RuntimeID("s1"), 0, 0, 100)))

	obs := entity.New(types.EntityID("o"), "avatar", types.RuntimeID("s1"))
	obs.ZoneID = types.ZoneID("z1")
	obs.Position = types.Vector3{X: 50, Z: 50}
	g.AddEntity(obs)

	other := entity.New(types.EntityID("x"), "avatar", types.RuntimeID("s1"))
	other.ZoneID = types.ZoneID("z1")
	other.Position = types.Vector3{X: 55, Z: 50}
	g.AddEntity(other)

	g.tick()

	var saw bool
	for len(g.Events) > 0 {
		evt := <-g.Events
		if evt.Kind == EventSpawn && evt.Observer == types.EntityID("o") && evt.EntityID == types.EntityID("x") && evt.Space == types.RuntimeID("s1") {
			saw = true
		}
	}
	assert.True(t, saw, "observer should receive a Space-scoped Spawn event for the entering entity")
}

// movingLifecycle is a custom Lifecycle (not NPCLifecycle) that moves its
// entity. It proves the simulation core drives arbitrary lifecycles through
// the OnSimulate contract without type-switching (Open/Closed).
type movingLifecycle struct {
	entity.BaseLifecycle
}

func (movingLifecycle) OnSimulate(e *entity.Entity, _ time.Duration) { e.Position.X += 10 }

// TestLifecycleExtension_CustomBehaviorPropagatesMove validates that a custom
// (non-NPC) lifecycle driving entity movement is picked up by the core and
// propagated as a Move event to observers.
func TestLifecycleExtension_CustomBehaviorPropagatesMove(t *testing.T) {
	g := New(types.ServerID("gs-1"))
	require.NoError(t, g.AssignZone(zone.New(types.ZoneID("z1"), types.RuntimeID("s1"), 0, 0, 100)))

	mover := entity.New(types.EntityID("m"), "agent", types.RuntimeID("s1"))
	mover.ZoneID = types.ZoneID("z1")
	mover.Position = types.Vector3{X: 50, Z: 50}
	mover.Lifecycle = movingLifecycle{}
	g.AddEntity(mover)

	obs := entity.New(types.EntityID("o"), "avatar", types.RuntimeID("s1"))
	obs.ZoneID = types.ZoneID("z1")
	obs.Position = types.Vector3{X: 55, Z: 50}
	g.AddEntity(obs)

	for len(g.Events) > 0 {
		<-g.Events
	}

	g.simulate(time.Millisecond)

	var sawMove bool
	for len(g.Events) > 0 {
		evt := <-g.Events
		if evt.Kind == EventMove && evt.EntityID == types.EntityID("m") && evt.Observer == types.EntityID("o") {
			sawMove = true
		}
	}
	assert.True(t, sawMove, "custom lifecycle movement should propagate a Move event to the observer")
	assert.Equal(t, 60.0, mover.Position.X, "the custom lifecycle should have moved the entity")
}
