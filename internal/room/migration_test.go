package room

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/thaolaptrinh/spatial-server/internal/types"
)

func TestPrepareTransfer_TransitionsToTransferring(t *testing.T) {
	zo2 := newZoneTable()
	require.NoError(t, zo2.set("z1", types.ServerID("gs-A"), types.ZoneStatusActive))
	m := NewMigrationCoordinator()
	m.table = zo2

	accepted, err := m.PrepareTransfer("z1", types.ServerID("gs-B"))
	require.NoError(t, err)
	assert.True(t, accepted)

	row, ok := zo2.get("z1")
	require.True(t, ok)
	assert.Equal(t, types.ZoneStatusTransferring, row.status)
}

func TestPrepareTransfer_RejectsWhenNotActive(t *testing.T) {
	zo2 := newZoneTable()
	require.NoError(t, zo2.set("z1", types.ServerID("gs-A"), types.ZoneStatusUnowned))
	m := NewMigrationCoordinator()
	m.table = zo2

	accepted, err := m.PrepareTransfer("z1", types.ServerID("gs-B"))
	require.Error(t, err)
	assert.False(t, accepted)
}

func TestCompleteTransfer_UpdatesOwnerAndStatus(t *testing.T) {
	zo2 := newZoneTable()
	require.NoError(t, zo2.set("z1", types.ServerID("gs-A"), types.ZoneStatusActive))
	m := NewMigrationCoordinator()
	m.table = zo2
	_, _ = m.PrepareTransfer("z1", types.ServerID("gs-B"))

	require.NoError(t, m.CompleteTransfer("z1", types.ServerID("gs-B")))
	row, ok := zo2.get("z1")
	require.True(t, ok)
	assert.Equal(t, types.ZoneStatusActive, row.status)
	assert.Equal(t, types.ServerID("gs-B"), row.owner)
}
