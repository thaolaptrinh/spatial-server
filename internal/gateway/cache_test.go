package gateway

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRouterCache_SetGet(t *testing.T) {
	rc := NewRouterCache(5 * time.Second)
	rc.Set("zone-1", "gs-1", "localhost", 9001)
	entry, ok := rc.Get("zone-1")
	assert.True(t, ok)
	assert.Equal(t, "gs-1", entry.ServerID)
	assert.Equal(t, "localhost", entry.Host)
	assert.Equal(t, 9001, entry.Port)
}

func TestRouterCache_Miss(t *testing.T) {
	rc := NewRouterCache(5 * time.Second)
	_, ok := rc.Get("non-existent")
	assert.False(t, ok)
}

func TestRouterCache_Expired(t *testing.T) {
	rc := NewRouterCache(50 * time.Millisecond)
	rc.Set("zone-1", "gs-1", "localhost", 9001)
	time.Sleep(100 * time.Millisecond)
	_, ok := rc.Get("zone-1")
	assert.False(t, ok)
}

func TestRouterCache_Invalidate(t *testing.T) {
	rc := NewRouterCache(5 * time.Minute)
	rc.Set("zone-1", "gs-1", "h1", 9001)
	_, ok := rc.Get("zone-1")
	require.True(t, ok)

	rc.Invalidate("zone-1")
	_, ok = rc.Get("zone-1")
	assert.False(t, ok)
}

func TestRouterCache_ApplyChange(t *testing.T) {
	rc := NewRouterCache(5 * time.Minute)
	rc.Set("zone-1", "gs-old", "h-old", 9001)
	rc.ApplyChange(&OwnershipChange{ZoneId: "zone-1", ServerId: "gs-new", Host: "h-new", Port: 9002})
	entry, ok := rc.Get("zone-1")
	require.True(t, ok)
	assert.Equal(t, "gs-new", entry.ServerID)
	assert.Equal(t, "h-new", entry.Host)
}

func TestRouterCache_Overwrite(t *testing.T) {
	rc := NewRouterCache(5 * time.Second)
	rc.Set("zone-1", "gs-1", "old-host", 9001)
	rc.Set("zone-1", "gs-2", "new-host", 9002)

	entry, ok := rc.Get("zone-1")
	assert.True(t, ok)
	assert.Equal(t, "gs-2", entry.ServerID)
	assert.Equal(t, "new-host", entry.Host)
}
