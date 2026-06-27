package storage

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/thaolaptrinh/spatial-server/internal/types"
	"github.com/thaolaptrinh/spatial-server/pkg/room"
)

func TestServerRepository_RegisterHeartbeatLeastLoaded(t *testing.T) {
	pool := testDB(t)
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
