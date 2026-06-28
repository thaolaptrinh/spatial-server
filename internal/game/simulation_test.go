package game

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/thaolaptrinh/spatial-server/internal/types"
	"github.com/thaolaptrinh/spatial-server/internal/game/entity"
	"github.com/thaolaptrinh/spatial-server/pkg/protocol"
	"github.com/thaolaptrinh/spatial-server/internal/game/zone"
	v1 "github.com/thaolaptrinh/spatial-server/proto/gen/spatialserver/v1"
	"google.golang.org/protobuf/proto"
)

func TestNewGame(t *testing.T) {
	g := New(types.ServerID("gs-1"), WithTickRate(50*time.Millisecond))
	assert.Equal(t, types.ServerID("gs-1"), g.ServerID)
	assert.NotNil(t, g.Entities)
	assert.NotNil(t, g.Zones)
	assert.NotNil(t, g.Inbox)
	assert.NotNil(t, g.Events)
}

func TestAddEntity(t *testing.T) {
	g := New(types.ServerID("gs-1"))
	e := entity.New(types.EntityID("e1"), "avatar", types.RuntimeID("r1"))
	g.AddEntity(e)
	assert.Equal(t, 1, g.EntityCount())
	got, ok := g.Entities[types.EntityID("e1")]
	require.True(t, ok)
	assert.Equal(t, "e1", string(got.ID))
}

func TestRemoveEntity(t *testing.T) {
	g := New(types.ServerID("gs-1"))
	g.AddEntity(entity.New(types.EntityID("e1"), "avatar", types.RuntimeID("r1")))
	g.AddEntity(entity.New(types.EntityID("e2"), "avatar", types.RuntimeID("r1")))
	g.RemoveEntity(types.EntityID("e1"))
	assert.Equal(t, 1, g.EntityCount())
}

func TestRemoveEntity_NotFound(t *testing.T) {
	g := New(types.ServerID("gs-1"))
	g.RemoveEntity(types.EntityID("no-such"))
	assert.Equal(t, 0, g.EntityCount())
}

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

func TestAssignZone(t *testing.T) {
	g := New(types.ServerID("gs-1"))
	z := zone.New(types.ZoneID("z1"), types.RuntimeID("r1"), 0, 0, 100)
	require.NoError(t, g.AssignZone(z))
	assert.Equal(t, 1, len(g.Zones))
}

func TestAssignZone_Duplicate(t *testing.T) {
	g := New(types.ServerID("gs-1"))
	z := zone.New(types.ZoneID("z1"), types.RuntimeID("r1"), 0, 0, 100)
	require.NoError(t, g.AssignZone(z))
	err := g.AssignZone(zone.New(types.ZoneID("z1"), types.RuntimeID("r1"), 0, 0, 100))
	assert.Error(t, err)
}

func TestReleaseZone(t *testing.T) {
	g := New(types.ServerID("gs-1"))
	z := zone.New(types.ZoneID("z1"), types.RuntimeID("r1"), 0, 0, 100)
	require.NoError(t, g.AssignZone(z))
	require.NoError(t, g.ReleaseZone(types.ZoneID("z1")))
	assert.Equal(t, 0, len(g.Zones))
}

func TestReleaseZone_NotFound(t *testing.T) {
	g := New(types.ServerID("gs-1"))
	err := g.ReleaseZone(types.ZoneID("no-such"))
	assert.Error(t, err)
}

func TestEntitiesNearGrid_ReturnsEntitiesInMatchingZone(t *testing.T) {
	g := New(types.ServerID("gs-1"))
	require.NoError(t, g.AssignZone(zone.New(types.ZoneID("z1"), types.RuntimeID("r1"), 0, 0, 100)))

	near := entity.New(types.EntityID("near"), "avatar", types.RuntimeID("r1"))
	near.ZoneID = types.ZoneID("z1")
	near.Position = types.Vector3{X: 50, Z: 50}
	g.AddEntity(near)

	far := entity.New(types.EntityID("far"), "avatar", types.RuntimeID("r1"))
	far.ZoneID = types.ZoneID("z1")
	far.Position = types.Vector3{X: 5000, Z: 5000}
	g.AddEntity(far)

	got := g.EntitiesNearGrid(types.RuntimeID("r1"), 0, 0, 200)
	require.Len(t, got, 1)
	assert.Equal(t, types.EntityID("near"), got[0].ID)
}

func TestEntitiesNearGrid_NoMatchingGridReturnsNil(t *testing.T) {
	g := New(types.ServerID("gs-1"))
	require.NoError(t, g.AssignZone(zone.New(types.ZoneID("z1"), types.RuntimeID("r1"), 0, 0, 100)))

	e := entity.New(types.EntityID("e1"), "avatar", types.RuntimeID("r1"))
	e.ZoneID = types.ZoneID("z1")
	e.Position = types.Vector3{X: 50, Z: 50}
	g.AddEntity(e)

	assert.Nil(t, g.EntitiesNearGrid(types.RuntimeID("r1"), 9, 9, 200))
}

func TestSpaceIsolation_EntitiesNearGridDoesNotCrossSpaces(t *testing.T) {
	g := New(types.ServerID("gs-1"))
	// Two distinct Spaces, each with a zone at the same grid coordinate (0,0).
	require.NoError(t, g.AssignZone(zone.New(types.ZoneID("za"), types.RuntimeID("spaceA"), 0, 0, 100)))
	require.NoError(t, g.AssignZone(zone.New(types.ZoneID("zb"), types.RuntimeID("spaceB"), 0, 0, 100)))

	a := entity.New(types.EntityID("ea"), "avatar", types.RuntimeID("spaceA"))
	a.ZoneID = types.ZoneID("za")
	a.Position = types.Vector3{X: 50, Z: 50}
	g.AddEntity(a)

	b := entity.New(types.EntityID("eb"), "avatar", types.RuntimeID("spaceB"))
	b.ZoneID = types.ZoneID("zb")
	b.Position = types.Vector3{X: 50, Z: 50}
	g.AddEntity(b)

	// Querying spaceA at (0,0) must return only the spaceA entity, even
	// though spaceB has an entity at the same grid coordinate and position.
	gotA := g.EntitiesNearGrid(types.RuntimeID("spaceA"), 0, 0, 200)
	require.Len(t, gotA, 1)
	assert.Equal(t, types.EntityID("ea"), gotA[0].ID)

	gotB := g.EntitiesNearGrid(types.RuntimeID("spaceB"), 0, 0, 200)
	require.Len(t, gotB, 1)
	assert.Equal(t, types.EntityID("eb"), gotB[0].ID)
}

func TestSpaceIsolation_EntitySpaceDerivedFromZone(t *testing.T) {
	g := New(types.ServerID("gs-1"))
	require.NoError(t, g.AssignZone(zone.New(types.ZoneID("z1"), types.RuntimeID("spaceX"), 0, 0, 100)))

	// MigrateEntityIn and snapshot load previously created entities with an
	// empty Space. The entity's Space must now be derived from its zone so it
	// is bound to the correct Space regardless of caller-supplied input.
	g.MigrateEntityIn(&v1.EntitySnapshot{
		EntityId: "em", Type: "avatar",
		Position: &v1.Vector3{X: 10, Z: 10},
	}, types.ZoneID("z1"))

	e, ok := g.Entities[types.EntityID("em")]
	require.True(t, ok)
	assert.Equal(t, types.RuntimeID("spaceX"), e.RuntimeID)
}

func TestInboxSend(t *testing.T) {
	g := New(types.ServerID("gs-1"))
	g.Inbox <- InboundPacket{ClientID: "c1", Data: []byte{0x00, 0x03, 0x48, 0x69}}
	select {
	case pkt := <-g.Inbox:
		assert.Equal(t, "c1", pkt.ClientID)
	case <-time.After(time.Second):
		t.Fatal("timeout reading inbox")
	}
}

func TestRun_Lifecycle(t *testing.T) {
	g := New(types.ServerID("gs-1"), WithTickRate(10*time.Millisecond))
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		g.Run(ctx)
		close(done)
	}()

	time.Sleep(30 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not exit after context cancel")
	}
}

func TestGhostEntity_CreatedOnZoneBoundary(t *testing.T) {
	g := New(types.ServerID("gs-1"), WithTickRate(10*time.Millisecond))
	e := entity.New(types.EntityID("e1"), "avatar", types.RuntimeID("r1"))
	e.Position = types.Vector3{X: 50, Z: 50}
	g.AddEntity(e)

	e.Position = types.Vector3{X: 150, Z: 150}

	ctx, cancel := context.WithCancel(context.Background())
	go g.Run(ctx)
	time.Sleep(30 * time.Millisecond)
	cancel()

	assert.Greater(t, g.GhostCount(), 0, "expected at least one ghost after zone cross")
}

func TestGhostEntity_ExpiresAfterTTL(t *testing.T) {
	g := New(types.ServerID("gs-1"), WithTickRate(10*time.Millisecond))
	g.ghostTTL = 50 * time.Millisecond

	e := entity.New(types.EntityID("e1"), "avatar", types.RuntimeID("r1"))
	e.Position = types.Vector3{X: 50, Z: 50}
	g.AddEntity(e)

	e.Position = types.Vector3{X: 150, Z: 150}

	ctx, cancel := context.WithCancel(context.Background())
	go g.Run(ctx)
	time.Sleep(100 * time.Millisecond)
	cancel()

	assert.Equal(t, 0, g.GhostCount(), "ghosts should be cleaned up after TTL")
}

func TestAOI_AddEntityRegistersInAOI(t *testing.T) {
	g := New(types.ServerID("gs-1"))
	require.NoError(t, g.AssignZone(zone.New(types.ZoneID("z1"), types.RuntimeID("r1"), 0, 0, 100)))
	e := entity.New(types.EntityID("e1"), "avatar", types.RuntimeID("r1"))
	e.ZoneID = types.ZoneID("z1")
	e.Position = types.Vector3{X: 10, Z: 10}
	g.AddEntity(e)

	visible := g.AOIFor(types.ZoneID("z1")).EntitiesInRange(types.Vector3{X: 10, Z: 10}, 300)
	assert.Contains(t, visible, types.EntityID("e1"))
}

func TestAOI_RemoveEntityRemovesFromAOI(t *testing.T) {
	g := New(types.ServerID("gs-1"))
	require.NoError(t, g.AssignZone(zone.New(types.ZoneID("z1"), types.RuntimeID("r1"), 0, 0, 100)))
	e := entity.New(types.EntityID("e1"), "avatar", types.RuntimeID("r1"))
	e.ZoneID = types.ZoneID("z1")
	e.Position = types.Vector3{X: 10, Z: 10}
	g.AddEntity(e)
	g.RemoveEntity(types.EntityID("e1"))

	visible := g.AOIFor(types.ZoneID("z1")).EntitiesInRange(types.Vector3{X: 10, Z: 10}, 300)
	assert.NotContains(t, visible, types.EntityID("e1"))
}

func TestRun_TickProcessesInbox(t *testing.T) {
	g := New(types.ServerID("gs-1"), WithTickRate(10*time.Millisecond))
	g.AddEntity(entity.New(types.EntityID("target"), "avatar", types.RuntimeID("r1")))

	ctx, cancel := context.WithCancel(context.Background())
	go g.Run(ctx)

	g.Inbox <- InboundPacket{ClientID: "c1", Data: []byte{0x00, 0xFF, 0x00}}

	time.Sleep(30 * time.Millisecond)
	cancel()
}

func TestTick_EntityInRangeSpawns(t *testing.T) {
	g := New(types.ServerID("gs-1"), WithTickRate(10*time.Millisecond))
	eA := entity.New(types.EntityID("a"), "avatar", types.RuntimeID("r1"))
	eA.Position = types.Vector3{X: 0, Z: 0}
	g.AddEntity(eA)
	eB := entity.New(types.EntityID("b"), "avatar", types.RuntimeID("r1"))
	eB.Position = types.Vector3{X: 50, Z: 50}
	g.AddEntity(eB)

	ctx, cancel := context.WithCancel(context.Background())
	drain := make(chan Event, 100)
	go func() {
		for {
			select {
			case evt := <-g.Events:
				drain <- evt
			case <-ctx.Done():
				return
			}
		}
	}()
	go g.Run(ctx)
	time.Sleep(30 * time.Millisecond)
	cancel()

	var foundSpawn bool
	for {
		select {
		case evt := <-drain:
			if evt.Kind == EventSpawn && evt.Observer == types.EntityID("a") && evt.EntityID == types.EntityID("b") {
				foundSpawn = true
			}
		case <-time.After(100 * time.Millisecond):
			assert.True(t, foundSpawn, "entity a should receive spawn for entity b")
			return
		}
	}
}

func TestTick_EntityFarAwayNoSpawn(t *testing.T) {
	g := New(types.ServerID("gs-1"))
	require.NoError(t, g.AssignZone(zone.New(types.ZoneID("z1"), types.RuntimeID("r1"), 0, 0, 100)))
	eA := entity.New(types.EntityID("a"), "avatar", types.RuntimeID("r1"))
	eA.ZoneID = types.ZoneID("z1")
	eA.Position = types.Vector3{X: 0, Z: 0}
	g.AddEntity(eA)
	eB := entity.New(types.EntityID("b"), "avatar", types.RuntimeID("r1"))
	eB.ZoneID = types.ZoneID("z1")
	eB.Position = types.Vector3{X: 50000, Z: 50000}
	g.AddEntity(eB)

	ctx, cancel := context.WithCancel(context.Background())
	go g.Run(ctx)
	time.Sleep(30 * time.Millisecond)
	cancel()

	visible := g.AOIFor(types.ZoneID("z1")).EntitiesInRange(types.Vector3{X: 0, Z: 0}, 300)
	assert.NotContains(t, visible, types.EntityID("b"))
}

func TestEnqueueAddEntity_ExecutesOnTick(t *testing.T) {
	g := New(types.ServerID("gs-1"), WithTickRate(10*time.Millisecond))
	g.EnqueueAddEntity(entity.New(types.EntityID("e1"), "avatar", types.RuntimeID("r1")))

	ctx, cancel := context.WithCancel(context.Background())
	go g.Run(ctx)
	time.Sleep(30 * time.Millisecond)
	cancel()

	assert.Equal(t, 1, g.EntityCount())
}

func TestEnqueueRemoveEntity_ExecutesOnTick(t *testing.T) {
	g := New(types.ServerID("gs-1"), WithTickRate(10*time.Millisecond))
	e := entity.New(types.EntityID("e1"), "avatar", types.RuntimeID("r1"))
	g.AddEntity(e)
	assert.Equal(t, 1, g.EntityCount())

	g.EnqueueRemoveEntity(types.EntityID("e1"))

	ctx, cancel := context.WithCancel(context.Background())
	go g.Run(ctx)
	time.Sleep(30 * time.Millisecond)
	cancel()

	assert.Equal(t, 0, g.EntityCount())
}

func TestDispatch_DecodesPositionUpdateProto(t *testing.T) {
	g := New(types.ServerID("gs-1"), WithTickRate(50*time.Millisecond))
	e := entity.New(types.EntityID("p1"), "avatar", types.RuntimeID("r1"))
	e.Position = types.Vector3{X: 0, Z: 0}
	g.AddEntity(e)

	newPos := &v1.Vector3{X: 100, Y: 0, Z: 200}
	upd := &v1.EntityUpdate{EntityId: "p1", Position: newPos}
	payload, _ := proto.Marshal(upd)
	frame := protocol.Encode(protocol.PacketIDPositionUpdate, payload, false, 0)

	g.Inbox <- InboundPacket{ClientID: "p1", Data: frame}

	ctx, cancel := context.WithCancel(context.Background())
	go g.Run(ctx)
	time.Sleep(60 * time.Millisecond)
	cancel()

	g.mu.Lock()
	assert.Equal(t, 100.0, e.Position.X)
	assert.Equal(t, 200.0, e.Position.Z)
	g.mu.Unlock()
}

func TestEvents_DropOnFull(t *testing.T) {
	g := New(types.ServerID("gs-1"))
	for i := 0; i < InboxBufferSize; i++ {
		select {
		case g.Events <- Event{Kind: EventSpawn, Observer: types.EntityID("c")}:
		default:
			t.Log("buffer full at", i)
		}
	}

	e1 := entity.New(types.EntityID("e1"), "avatar", types.RuntimeID("r1"))
	e1.Position = types.Vector3{X: 0, Z: 0}
	g.AddEntity(e1)
	e2 := entity.New(types.EntityID("e2"), "avatar", types.RuntimeID("r1"))
	e2.Position = types.Vector3{X: 5, Z: 5}
	g.AddEntity(e2)

	g.tick()
}

func TestDispatch_EntityAction_RoutesToLifecycle(t *testing.T) {
	g := New(types.ServerID("s1"))
	e := entity.New(types.EntityID("p1"), "avatar", types.RuntimeID("r1"))
	e.OwnerID = types.OwnerID("p1")
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

func TestOutbound_EmitsSpawnEvent(t *testing.T) {
	g := New(types.ServerID("gs-1"))
	e := entity.New(types.EntityID("npc1"), "npc", types.RuntimeID("r1"))
	e.Position = types.Vector3{X: 50, Z: 50}
	g.AddEntity(e)

	e2 := entity.New(types.EntityID("p1"), "avatar", types.RuntimeID("r1"))
	e2.Position = types.Vector3{X: 55, Z: 55}
	g.AddEntity(e2)

	g.tick()

	for len(g.Events) > 0 {
		evt := <-g.Events
		if evt.Kind == EventSpawn && evt.EntityID == types.EntityID("npc1") {
			assert.Equal(t, "npc", evt.Type)
			assert.Equal(t, 50.0, evt.Position.X)
			assert.Equal(t, 50.0, evt.Position.Z)
			return
		}
	}
	t.Error("never received a spawn event for npc1")
}
