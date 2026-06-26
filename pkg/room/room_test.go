package room

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/thaolaptrinh/spatial-server/internal/types"
)

func TestServerRegistry_Register(t *testing.T) {
	reg := NewServerRegistry()
	svr := &ServerInfo{ID: "gs-1", Host: "localhost", Port: 9001, Status: types.ServerStatusJoining}
	err := reg.Register(svr)
	assert.NoError(t, err)

	got, ok := reg.Get("gs-1")
	assert.True(t, ok)
	assert.Equal(t, types.ServerID("gs-1"), got.ID)
}

func TestServerRegistry_RegisterDuplicate(t *testing.T) {
	reg := NewServerRegistry()
	reg.Register(&ServerInfo{ID: "gs-1", Host: "localhost", Port: 9001})
	err := reg.Register(&ServerInfo{ID: "gs-1", Host: "other", Port: 9002})
	assert.Error(t, err)
}

func TestServerRegistry_Heartbeat(t *testing.T) {
	reg := NewServerRegistry()
	reg.Register(&ServerInfo{ID: "gs-1", Host: "localhost", Port: 9001})
	err := reg.Heartbeat("gs-1")
	assert.NoError(t, err)

	svr, _ := reg.Get("gs-1")
	assert.True(t, svr.LastBeat.After(time.Now().Add(-time.Second)))
}

func TestServerRegistry_HeartbeatNotFound(t *testing.T) {
	reg := NewServerRegistry()
	err := reg.Heartbeat("no-such-server")
	assert.Error(t, err)
}

func TestServerRegistry_LeastLoaded(t *testing.T) {
	reg := NewServerRegistry()
	reg.Register(&ServerInfo{ID: "gs-1", Host: "localhost", Port: 9001, ZoneCount: 5, MaxZones: 10, Status: types.ServerStatusActive})
	reg.Register(&ServerInfo{ID: "gs-2", Host: "localhost", Port: 9002, ZoneCount: 2, MaxZones: 10, Status: types.ServerStatusActive})
	reg.Register(&ServerInfo{ID: "gs-3", Host: "localhost", Port: 9003, ZoneCount: 8, MaxZones: 10, Status: types.ServerStatusActive})

	svr, ok := reg.LeastLoaded()
	assert.True(t, ok)
	assert.Equal(t, types.ServerID("gs-2"), svr.ID)
}

func TestServerRegistry_LeastLoadedSkipsInactive(t *testing.T) {
	reg := NewServerRegistry()
	reg.Register(&ServerInfo{ID: "gs-1", Host: "localhost", Port: 9001, ZoneCount: 1, MaxZones: 10, Status: types.ServerStatusJoining})
	reg.Register(&ServerInfo{ID: "gs-2", Host: "localhost", Port: 9002, ZoneCount: 1, MaxZones: 10, Status: types.ServerStatusDraining})

	_, ok := reg.LeastLoaded()
	assert.False(t, ok)
}

func TestZoneOwnership_ClaimAndLookup(t *testing.T) {
	zo := NewZoneOwnership()
	err := zo.Claim("zone-1", "gs-1")
	assert.NoError(t, err)

	owner, ok := zo.Lookup("zone-1")
	assert.True(t, ok)
	assert.Equal(t, types.ServerID("gs-1"), owner)
}

func TestZoneOwnership_DoubleClaim(t *testing.T) {
	zo := NewZoneOwnership()
	zo.Claim("zone-1", "gs-1")
	err := zo.Claim("zone-1", "gs-2")
	assert.Error(t, err)
}

func TestZoneOwnership_Release(t *testing.T) {
	zo := NewZoneOwnership()
	zo.Claim("zone-1", "gs-1")

	err := zo.Release("zone-1", "gs-1")
	assert.NoError(t, err)

	_, ok := zo.Lookup("zone-1")
	assert.False(t, ok)
}

func TestZoneOwnership_ReleaseWrongOwner(t *testing.T) {
	zo := NewZoneOwnership()
	zo.Claim("zone-1", "gs-1")
	err := zo.Release("zone-1", "gs-2")
	assert.Error(t, err)
}

func TestLookupZone(t *testing.T) {
	reg := NewServerRegistry()
	zo := NewZoneOwnership()

	reg.Register(&ServerInfo{ID: "gs-1", Host: "192.168.1.1", Port: 9001, Status: types.ServerStatusActive})
	zo.Claim("zone-42", "gs-1")

	serverID, host, port, err := ResolveZone(zo, reg, "zone-42")
	assert.NoError(t, err)
	assert.Equal(t, types.ServerID("gs-1"), serverID)
	assert.Equal(t, "192.168.1.1", host)
	assert.Equal(t, 9001, port)
}

func TestLookupZone_NotFound(t *testing.T) {
	reg := NewServerRegistry()
	zo := NewZoneOwnership()

	_, _, _, err := ResolveZone(zo, reg, "non-existent-zone")
	assert.Error(t, err)
}
