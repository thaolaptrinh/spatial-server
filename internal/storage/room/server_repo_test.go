package room

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/thaolaptrinh/spatial-server/internal/room"
	"github.com/thaolaptrinh/spatial-server/internal/storage"
	"github.com/thaolaptrinh/spatial-server/internal/types"
)

func TestServerRepository_CRUD(t *testing.T) {
	pool := storage.TestDB(t)
	repo := NewServerRepository(pool)
	ctx := context.Background()

	n := &room.NodeDescriptor{
		NodeID:       types.ServerID("s1"),
		Host:         "h",
		Port:         9,
		AdvertiseAddr: "h:9",
		Capacity:     room.NodeCapacity{MaxZones: 5},
	}
	require.NoError(t, repo.Register(ctx, n))
	require.NoError(t, repo.Heartbeat(ctx, "s1", room.NodeLoad{}))
	got, err := repo.Get(ctx, "s1")
	require.NoError(t, err)
	assert.Equal(t, "h", got.Host)

	nodes, err := repo.List(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, nodes)
	assert.Equal(t, types.ServerID("s1"), nodes[0].NodeID)

	require.NoError(t, repo.Remove(ctx, "s1"))
	_, err = repo.Get(ctx, "s1")
	assert.ErrorIs(t, err, types.ErrNotFound)
}
