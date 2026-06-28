package room

import (
	"context"
	"testing"
	"time"

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

func TestServerRepository_ListDead_ReturnsTimeoutServers(t *testing.T) {
	pool := storage.TestDB(t)
	repo := NewServerRepository(pool)
	ctx := context.Background()

	n1 := &room.NodeDescriptor{
		NodeID:   types.ServerID("alive"),
		Host:     "h", Port: 1,
		Capacity: room.NodeCapacity{MaxZones: 5},
	}
	require.NoError(t, repo.Register(ctx, n1))
	require.NoError(t, repo.Heartbeat(ctx, "alive", room.NodeLoad{}))

	n2 := &room.NodeDescriptor{
		NodeID:   types.ServerID("dead"),
		Host:     "h", Port: 2,
		Capacity: room.NodeCapacity{MaxZones: 5},
	}
	require.NoError(t, repo.Register(ctx, n2))
	require.NoError(t, repo.Heartbeat(ctx, "dead", room.NodeLoad{}))

	_, err := pool.Exec(ctx,
		`UPDATE game_servers SET last_heartbeat = NOW() - INTERVAL '60 seconds' WHERE id='dead'`)
	require.NoError(t, err)

	deadIDs, err := repo.ListDead(ctx, 15*time.Second)
	require.NoError(t, err)
	assert.Len(t, deadIDs, 1)
	assert.Equal(t, types.ServerID("dead"), deadIDs[0])

	require.NoError(t, repo.MarkShutdown(ctx, "dead"))

	deadIDs2, err := repo.ListDead(ctx, 15*time.Second)
	require.NoError(t, err)
	assert.Len(t, deadIDs2, 0)
}
