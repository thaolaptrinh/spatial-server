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

func TestNewGame(t *testing.T) {
	g := New(types.ServerID("gs-1"), WithTickRate(50*time.Millisecond))
	assert.Equal(t, types.ServerID("gs-1"), g.ServerID)
	assert.NotNil(t, g.Entities)
	assert.NotNil(t, g.Zones)
	assert.NotNil(t, g.Inbox)
	assert.NotNil(t, g.Outbox)
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

func TestAssignZone(t *testing.T) {
	g := New(types.ServerID("gs-1"))
	z := zone.New(types.ZoneID("z1"), types.RuntimeID("r1"), 0, 0, 100)
	g.AssignZone(z)
	assert.Equal(t, 1, len(g.Zones))
}

func TestReleaseZone(t *testing.T) {
	g := New(types.ServerID("gs-1"))
	z := zone.New(types.ZoneID("z1"), types.RuntimeID("r1"), 0, 0, 100)
	g.AssignZone(z)
	g.ReleaseZone(types.ZoneID("z1"))
	assert.Equal(t, 0, len(g.Zones))
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
	drain := make(chan OutboundPacket, 100)
	go func() {
		for {
			select {
			case pkt := <-g.Outbox:
				drain <- pkt
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
		case pkt := <-drain:
			if pkt.ClientID == "a" && len(pkt.Data) > 0 {
				foundSpawn = true
			}
		case <-time.After(100 * time.Millisecond):
			assert.True(t, foundSpawn, "entity a should receive spawn for entity b")
			return
		}
	}
}

func TestTick_EntityFarAwayNoSpawn(t *testing.T) {
	g := New(types.ServerID("gs-1"), WithTickRate(10*time.Millisecond))
	eA := entity.New(types.EntityID("a"), "avatar", types.RuntimeID("r1"))
	eA.Position = types.Vector3{X: 0, Z: 0}
	g.AddEntity(eA)
	eB := entity.New(types.EntityID("b"), "avatar", types.RuntimeID("r1"))
	eB.Position = types.Vector3{X: 50000, Z: 50000}
	g.AddEntity(eB)

	ctx, cancel := context.WithCancel(context.Background())
	go g.Run(ctx)
	time.Sleep(30 * time.Millisecond)
	cancel()

	visible := g.aoi.EntitiesInRange(types.Vector3{X: 0, Z: 0}, 300)
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
