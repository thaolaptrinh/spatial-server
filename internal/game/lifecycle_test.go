package game

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/thaolaptrinh/spatial-server/internal/game/entity"
	"github.com/thaolaptrinh/spatial-server/internal/types"
)

type fakeClock struct{ t time.Time }

func newFakeClock() *fakeClock { return &fakeClock{t: time.Unix(1000, 0)} }
func (f *fakeClock) now() time.Time { return f.t }
func (f *fakeClock) advance(d time.Duration) { f.t = f.t.Add(d) }

func TestMarkDisconnected_DoesNotDespawnEntity(t *testing.T) {
	g := New(types.ServerID("gs-1"))
	clock := newFakeClock()
	g.lifecycleClock = clock.now
	g.reconnectWindow = 30 * time.Second

	g.AddEntity(entity.New(types.EntityID("e1"), "avatar", types.RuntimeID("r1")))

	g.MarkDisconnected(types.EntityID("e1"))
	assert.Equal(t, 1, g.EntityCount())
	st := g.sessionStates[types.EntityID("e1")]
	require.NotNil(t, st)
	assert.Equal(t, SessionDisconnected, st.status)
}

func TestMarkReconnected_ReturnsToActive(t *testing.T) {
	g := New(types.ServerID("gs-1"))
	clock := newFakeClock()
	g.lifecycleClock = clock.now
	g.reconnectWindow = 30 * time.Second
	g.AddEntity(entity.New(types.EntityID("e1"), "avatar", types.RuntimeID("r1")))
	g.MarkDisconnected(types.EntityID("e1"))

	g.MarkReconnected(types.EntityID("e1"))
	st := g.sessionStates[types.EntityID("e1")]
	require.NotNil(t, st)
	assert.Equal(t, SessionActive, st.status)
}

func TestSweepDisconnected_DespawnsAfterWindow(t *testing.T) {
	g := New(types.ServerID("gs-1"))
	clock := newFakeClock()
	g.lifecycleClock = clock.now
	g.reconnectWindow = 30 * time.Second
	g.AddEntity(entity.New(types.EntityID("e1"), "avatar", types.RuntimeID("r1")))
	g.MarkDisconnected(types.EntityID("e1"))
	require.Equal(t, 1, g.EntityCount())

	clock.advance(31 * time.Second)
	g.SweepDisconnected()
	assert.Equal(t, 0, g.EntityCount())
}

func TestTick_CallsSweepDisconnected_EntityDespawnedAfterReconnectWindow(t *testing.T) {
	g := New(types.ServerID("gs-1"), WithTickRate(10*time.Millisecond))
	clock := newFakeClock()
	g.lifecycleClock = clock.now
	g.reconnectWindow = 30 * time.Second
	g.AddEntity(entity.New(types.EntityID("e1"), "avatar", types.RuntimeID("r1")))

	g.MarkDisconnected(types.EntityID("e1"))
	require.Equal(t, 1, g.EntityCount())

	clock.advance(31 * time.Second)
	g.Tick()
	assert.Equal(t, 0, g.EntityCount(), "tick() must call SweepDisconnected() and despawn expired entities")
}

func TestTick_DoesNotDespawnRecentlyDisconnectedEntities(t *testing.T) {
	g := New(types.ServerID("gs-1"), WithTickRate(10*time.Millisecond))
	clock := newFakeClock()
	g.lifecycleClock = clock.now
	g.reconnectWindow = 30 * time.Second
	g.AddEntity(entity.New(types.EntityID("e1"), "avatar", types.RuntimeID("r1")))

	g.MarkDisconnected(types.EntityID("e1"))

	clock.advance(10 * time.Second)
	g.Tick()
	assert.Equal(t, 1, g.EntityCount(), "entity still within reconnect window must not be despawned")
}
