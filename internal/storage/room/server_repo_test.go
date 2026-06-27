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

func TestServerRepository_RegisterHeartbeatLeastLoaded(t *testing.T) {
	pool := storage.TestDB(t)
	repo := NewServerRepository(pool)
	ctx := context.Background()

	require.NoError(t, repo.Register(ctx, &room.ServerInfo{ID: "s1", Host: "h", Port: 9, MaxZones: 5}))
	require.NoError(t, repo.Heartbeat(ctx, "s1"))
	got, err := repo.Get(ctx, "s1")
	require.NoError(t, err)
	assert.Equal(t, "h", got.Host)
	best, err := repo.LeastLoaded(ctx)
	require.NoError(t, err)
	assert.Equal(t, types.ServerID("s1"), best.ID)
	require.NoError(t, repo.Remove(ctx, "s1"))
	_, err = repo.Get(ctx, "s1")
	assert.ErrorIs(t, err, types.ErrNotFound)
}
