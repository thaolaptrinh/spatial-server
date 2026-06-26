package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/thaolaptrinh/spatial-server/internal/types"
)

func TestNewSession(t *testing.T) {
	s := NewSession("client-1", "player-1", types.ZoneID("zone-1"), types.ServerID("gs-1"))
	assert.Equal(t, "client-1", s.ClientID)
	assert.Equal(t, "player-1", s.PlayerID)
	assert.Equal(t, types.ZoneID("zone-1"), s.ZoneID)
	assert.Equal(t, types.ServerID("gs-1"), s.ServerID)
	assert.False(t, s.Closed)
}

func TestSessionClose(t *testing.T) {
	s := NewSession("c1", "p1", types.ZoneID("z1"), types.ServerID("gs1"))
	s.Close()
	assert.True(t, s.Closed)
}

func TestPool_AddGetRemove(t *testing.T) {
	pool := NewPool()
	s1 := NewSession("c1", "p1", types.ZoneID("z1"), types.ServerID("gs1"))
	s2 := NewSession("c2", "p2", types.ZoneID("z1"), types.ServerID("gs1"))

	pool.Add(s1)
	pool.Add(s2)
	assert.Equal(t, 2, pool.Count())

	got, ok := pool.Get("c1")
	assert.True(t, ok)
	assert.Equal(t, "c1", got.ClientID)

	pool.Remove("c1")
	_, ok = pool.Get("c1")
	assert.False(t, ok)
	assert.Equal(t, 1, pool.Count())
}

func TestPool_GetMissing(t *testing.T) {
	pool := NewPool()
	_, ok := pool.Get("non-existent")
	assert.False(t, ok)
}

func TestPool_ConcurrentSafe(t *testing.T) {
	pool := NewPool()
	for i := 0; i < 100; i++ {
		pool.Add(NewSession(string(rune(i)), "p", types.ZoneID("z"), types.ServerID("gs")))
	}
	assert.Equal(t, 100, pool.Count())

	for i := 0; i < 50; i++ {
		pool.Remove(string(rune(i)))
	}
	assert.Equal(t, 50, pool.Count())
}
