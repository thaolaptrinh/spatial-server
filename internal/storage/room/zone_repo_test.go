package room

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/thaolaptrinh/spatial-server/internal/storage"
	"github.com/thaolaptrinh/spatial-server/internal/types"
)

func TestZoneRepository_ClaimLookupRelease(t *testing.T) {
	pool := storage.TestDB(t)
	repo := NewZoneRepository(pool)
	ctx := context.Background()

	_, _ = pool.Exec(ctx, `INSERT INTO runtimes (id) VALUES ('r1') ON CONFLICT DO NOTHING`)
	require.NoError(t, repo.Claim(ctx, "z1", "r1", types.ServerID("s1")))
	err := repo.Claim(ctx, "z1", "r1", types.ServerID("s2"))
	assert.ErrorIs(t, err, types.ErrConflict)
	owner, err := repo.Lookup(ctx, "z1")
	require.NoError(t, err)
	assert.Equal(t, types.ServerID("s1"), owner)
	require.ErrorIs(t, repo.Release(ctx, "z1", types.ServerID("s2")), types.ErrNotOwned)
	require.NoError(t, repo.Release(ctx, "z1", types.ServerID("s1")))
	_, err = repo.Lookup(ctx, "z1")
	assert.ErrorIs(t, err, types.ErrNotFound)
}
