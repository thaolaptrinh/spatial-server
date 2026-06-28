package room

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/thaolaptrinh/spatial-server/internal/types"
)

func TestSweeper_MarksShutdownAfterMissedHeartbeats(t *testing.T) {
	reg := NewServerRegistry()
	zo := NewZoneOwnership()
	require.NoError(t, reg.Register(&NodeDescriptor{NodeID: types.ServerID("gs-A"), Host: "a", Port: 9000, Capacity: NodeCapacity{MaxZones: 10}}))
	require.NoError(t, reg.Register(&NodeDescriptor{NodeID: types.ServerID("gs-B"), Host: "b", Port: 9000, Capacity: NodeCapacity{MaxZones: 10}}))
	require.NoError(t, reg.Heartbeat(types.ServerID("gs-B")))
	require.NoError(t, zo.Claim("z1", types.ServerID("gs-A")))
	reg.svr[types.ServerID("gs-A")].LastHeartbeat = time.Now().Add(-20 * time.Second)

	var reassigned []string
	s := NewSweeper(reg, zo, SweeperConfig{Interval: time.Second, MissThreshold: 15 * time.Second},
		func(zoneID string, target types.ServerID) { reassigned = append(reassigned, zoneID+"->"+string(target)) })
	s.sweep(time.Now())

	info, ok := reg.Get(types.ServerID("gs-A"))
	require.True(t, ok)
	assert.Equal(t, types.ServerStatusShutdown, info.Status)
	owner, ok := zo.Lookup("z1")
	assert.False(t, ok)
	assert.Equal(t, types.ServerID(""), owner)
	assert.NotEmpty(t, reassigned)
}

func TestSweeper_LeavesHealthyServersAlone(t *testing.T) {
	reg := NewServerRegistry()
	zo := NewZoneOwnership()
	require.NoError(t, reg.Register(&NodeDescriptor{NodeID: types.ServerID("gs-A"), Host: "a", Port: 9000, Capacity: NodeCapacity{MaxZones: 10}}))
	require.NoError(t, reg.Heartbeat(types.ServerID("gs-A")))
	require.NoError(t, zo.Claim("z1", types.ServerID("gs-A")))

	s := NewSweeper(reg, zo, SweeperConfig{Interval: time.Second, MissThreshold: 15 * time.Second}, nil)
	s.sweep(time.Now())

	info, ok := reg.Get(types.ServerID("gs-A"))
	assert.True(t, ok)
	assert.Equal(t, types.ServerStatusActive, info.Status)
}
