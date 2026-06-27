package gateway

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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

func TestRouterCache_Overwrite(t *testing.T) {
	rc := NewRouterCache(5 * time.Second)
	rc.Set("zone-1", "gs-1", "old-host", 9001)
	rc.Set("zone-1", "gs-2", "new-host", 9002)

	entry, ok := rc.Get("zone-1")
	assert.True(t, ok)
	assert.Equal(t, "gs-2", entry.ServerID)
	assert.Equal(t, "new-host", entry.Host)
}
