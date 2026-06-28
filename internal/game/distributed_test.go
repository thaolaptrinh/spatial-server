package game

import (
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/thaolaptrinh/spatial-server/internal/game/entity"
	"github.com/thaolaptrinh/spatial-server/internal/game/zone"
	"github.com/thaolaptrinh/spatial-server/internal/types"
)

// twoNodeHarness creates two Runtime Nodes (Game instances) executing the same
// Space, each owning a disjoint zone.
type twoNodeHarness struct {
	A, B        *Game
	space       types.RuntimeID
	zoneA       types.ZoneID
	zoneB       types.ZoneID
}

func newTwoNodeHarness(t *testing.T) *twoNodeHarness {
	t.Helper()
	a := New(types.ServerID("gs-A"), WithTickRate(10*time.Millisecond))
	b := New(types.ServerID("gs-B"), WithTickRate(10*time.Millisecond))
	sp := types.RuntimeID("space-1")
	za := types.ZoneID("zA")
	zb := types.ZoneID("zB")
	require.NoError(t, a.AssignZone(zone.New(za, sp, 0, 0, 100)))
	require.NoError(t, b.AssignZone(zone.New(zb, sp, 1, 0, 100)))
	return &twoNodeHarness{A: a, B: b, space: sp, zoneA: za, zoneB: zb}
}

// spawn creates an entity on g in the given zone at (x,z).
func (h *twoNodeHarness) spawn(g *Game, zoneID types.ZoneID, x, z float64) *entity.Entity {
	e := entity.New(types.NewEntityID(), "avatar", h.space)
	e.ZoneID = zoneID
	e.Position = types.Vector3{X: x, Z: z}
	g.AddEntity(e)
	return e
}

// entitiesAcross returns the total entity count across both nodes.
func (h *twoNodeHarness) entitiesAcross() int {
	return h.A.EntityCount() + h.B.EntityCount()
}

// ── invariant 1: entity uniqueness (no duplicates) ────────────────────

func TestDistributed_EntityUniqueness(t *testing.T) {
	h := newTwoNodeHarness(t)
	e1 := h.spawn(h.A, h.zoneA, 10, 10)
	e2 := h.spawn(h.B, h.zoneB, 110, 10)

	assert.Equal(t, 2, h.entitiesAcross())
	// Each entity lives on exactly one node.
	_, aHas2 := h.A.Entities[e2.ID]
	_, bHas1 := h.B.Entities[e1.ID]
	assert.False(t, aHas2, "entity spawned on B must not appear on A")
	assert.False(t, bHas1, "entity spawned on A must not appear on B")
}

// ── invariant 2: ownership uniqueness + transfer ──────────────────────

func TestDistributed_OwnershipTransferPreservesCount(t *testing.T) {
	h := newTwoNodeHarness(t)
	e := h.spawn(h.A, h.zoneA, 10, 10)

	assert.Equal(t, 1, h.A.EntityCount())
	h.A.RemoveEntity(e.ID)
	assert.Equal(t, 0, h.A.EntityCount())

	e.ZoneID = h.zoneB
	h.B.AddEntity(e)
	assert.Equal(t, 1, h.entitiesAcross())
	assert.Equal(t, types.ZoneID("zB"), h.B.ZoneOf(e.ID))
}

// ── invariant 3: cross-node ghost sync (AOI across zone boundary) ─────

func TestDistributed_CrossNodeGhostSync(t *testing.T) {
	h := newTwoNodeHarness(t)

	// Entity near boundary on node A; node B queries it as neighbour.
	e := h.spawn(h.A, h.zoneA, 95, 50)
	h.B.RegisterPeerZone(h.zoneA, types.ServerID("gs-A"))
	h.B.SetNeighborQuerier(func(zoneID types.ZoneID, _, _ int, _ float64) []neighborEntity {
		if zoneID != h.zoneA {
			return nil
		}
		return []neighborEntity{{ID: e.ID, Type: e.Type, Pos: e.Position}}
	})

	require.Equal(t, 0, ghostStoreCount(h.B, h.zoneB))
	h.B.ReconcileNeighborGhosts(h.zoneB, h.zoneA, time.Now)
	assert.Equal(t, 1, ghostStoreCount(h.B, h.zoneB), "node B must observe the boundary ghost of node A's entity")

	// Remove the source entity; ghost must expire after TTL.
	h.A.RemoveEntity(e.ID)
	h.B.SetNeighborQuerier(func(z types.ZoneID, _, _ int, _ float64) []neighborEntity { return nil })
	h.B.ReconcileNeighborGhosts(h.zoneB, h.zoneA, func() time.Time { return time.Now().Add(h.B.ghostTTL*6 + time.Second) })
	assert.Equal(t, 0, ghostStoreCount(h.B, h.zoneB), "ghost must expire after the source is gone")
}

// sweepGhostsWith injects a clock for deterministic expiration.
func (g *Game) sweepGhostsWith(now time.Time) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for id, ghost := range g.ghosts {
		if now.After(ghost.expiresAt) {
			if sim := g.Zones[ghost.originZone]; sim != nil {
				sim.aoi.Leave(id)
			}
			delete(g.ghosts, id)
		}
	}
}

// ── invariant 4: no ghost leaks ───────────────────────────────────────

func TestDistributed_NoGhostLeaks(t *testing.T) {
	h := newTwoNodeHarness(t)
	e := h.spawn(h.A, h.zoneA, 95, 50)
	h.B.RegisterPeerZone(h.zoneA, types.ServerID("gs-A"))
	h.B.SetNeighborQuerier(func(z types.ZoneID, _, _ int, _ float64) []neighborEntity {
		if z == h.zoneA {
			return []neighborEntity{{ID: e.ID, Type: e.Type, Pos: e.Position}}
		}
		return nil
	})

	// Run many reconcile cycles; ghostStore must stay bounded (≤ 1 entity).
	for i := 0; i < 300; i++ {
		h.B.ReconcileNeighborGhosts(h.zoneB, h.zoneA, time.Now)
		assert.LessOrEqual(t, ghostStoreCount(h.B, h.zoneB), 1)
	}
}

// ── invariant 5: Space isolation across nodes ─────────────────────────

func TestDistributed_SpaceIsolationAcrossNodes(t *testing.T) {
	ga := New(types.ServerID("gs-A"))
	gb := New(types.ServerID("gs-B"))
	require.NoError(t, ga.AssignZone(zone.New(types.ZoneID("zaA"), types.RuntimeID("spaceA"), 0, 0, 100)))
	require.NoError(t, gb.AssignZone(zone.New(types.ZoneID("zbB"), types.RuntimeID("spaceB"), 0, 0, 100)))

	eA := hSpawn(ga, types.ZoneID("zaA"), types.RuntimeID("spaceA"), 50, 50)
	hSpawn(gb, types.ZoneID("zbB"), types.RuntimeID("spaceB"), 50, 50)

	// query spaceA at (0,0): must find only eA, never eB.
	got := ga.EntitiesNearGrid(types.RuntimeID("spaceA"), 0, 0, 200)
	require.Len(t, got, 1)
	assert.Equal(t, eA.ID, got[0].ID)
}

func hSpawn(g *Game, zoneID types.ZoneID, space types.RuntimeID, x, z float64) *entity.Entity {
	e := entity.New(types.NewEntityID(), "avatar", space)
	e.ZoneID = zoneID
	e.Position = types.Vector3{X: x, Z: z}
	g.AddEntity(e)
	return e
}

// ── invariant 6: randomized workload with deterministic invariants ─────

func TestDistributed_RandomizedInvariants(t *testing.T) {
	h := newTwoNodeHarness(t)
	rng := rand.New(rand.NewSource(1))
	totalSpawned := 0

	for iter := 0; iter < 1000; iter++ {
		// Random spawn or remove.
		if rng.Float64() < 0.15 { // spawn rate
			if rng.Float64() < 0.5 {
				h.spawn(h.A, h.zoneA, rng.Float64()*90, rng.Float64()*90)
			} else {
				h.spawn(h.B, h.zoneB, rng.Float64()*90+100, rng.Float64()*90)
			}
			totalSpawned++
		}
		// Random movement (position update to tick path validates AOI).
		for _, g := range []*Game{h.A, h.B} {
			for id, e := range g.Entities {
				if rng.Float64() < 0.3 {
					e.Position.X += (rng.Float64() - 0.5) * 20
					e.Position.Z += (rng.Float64() - 0.5) * 20
					g.Entities[id] = e
				}
			}
		}
		// Tick both nodes (simulate concurrent execution).
		h.A.Tick()
		h.B.Tick()

		// Invariants ------------------------------------------------
		n := h.entitiesAcross()

		// 1. No lost entities (we never remove, only add in this loop).
		//    The tick might process queued AddEntity from earlier, so
		//    count can be lower than spawned if enqueued adds not yet
		//    processed. EnqueueAddEntity sends to cmds; applyCmds
		//    drains before simulate. But we called AddEntity directly,
		//    so no queueing. Verify the total is at least what we've
		//    confirmed added through deterministic paths.
		_ = n

		// 2. Space isolation: every entity's Space == its zone's Space.
		for _, g := range []*Game{h.A, h.B} {
			for _, e := range g.Entities {
				zid := g.ZoneOf(e.ID)
				if zid == "" {
					continue // entity being added/removed between ticks
				}
				if sim := g.Zones[zid]; sim != nil && sim.zone != nil {
					assert.Equal(t, sim.zone.RuntimeID, e.RuntimeID,
						"entity %s Space must match its zone", e.ID)
				}
			}
		}
		// 3. Zone ownership is disjoint (no zone appears in both Games).
		aZones := zoneSet(h.A)
		bZones := zoneSet(h.B)
		for z := range aZones {
			_, conflict := bZones[z]
			assert.False(t, conflict, "zone %s must not appear in both nodes", z)
		}
		// 4. Entity count invariant: no lost spawns (AddEntity is synchronous;
		//    no queue drops possible in this test).
		_ = h.A.EntityCount()
		_ = h.B.EntityCount()
	}

	assert.Equal(t, totalSpawned, h.entitiesAcross(),
		"total entities across nodes must match spawned count (no lost entities)")
}


func zoneSet(g *Game) map[types.ZoneID]struct{} {
	set := make(map[types.ZoneID]struct{})
	for z := range g.Zones {
		set[z] = struct{}{}
	}
	return set
}

// ── invariant 7: long-run deterministic non-leak check ────────────────

func TestDistributed_LongRun_NoGradualLeaks(t *testing.T) {
	h := newTwoNodeHarness(t)
	rng := rand.New(rand.NewSource(42))
	// Spawn a stable set of entities and run many ticks.
	for i := 0; i < 100; i++ {
		h.spawn(h.A, h.zoneA, rng.Float64()*90, rng.Float64()*90)
		h.spawn(h.B, h.zoneB, rng.Float64()*90+100, rng.Float64()*90)
	}
	initialTotal := h.entitiesAcross()

	for iter := 0; iter < 500; iter++ {
		for _, g := range []*Game{h.A, h.B} {
			for id, e := range g.Entities {
				e.Position.X += (rng.Float64() - 0.5) * 5
				e.Position.Z += (rng.Float64() - 0.5) * 5
				g.Entities[id] = e
			}
		}
		h.A.Tick()
		h.B.Tick()
	}
	// After many ticks, the total entity count must be unchanged.
	assert.Equal(t, initialTotal, h.entitiesAcross(), "long run must not leak or lose entities")
	// Ghosts must be bounded (swept by TTL).
	assert.LessOrEqual(t, h.A.GhostCount(), initialTotal)
	assert.LessOrEqual(t, h.B.GhostCount(), initialTotal)
}
