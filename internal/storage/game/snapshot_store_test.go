package game

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/thaolaptrinh/spatial-server/internal/storage"
)

func TestSnapshotStore_SaveAndLoad(t *testing.T) {
	pool := storage.TestDB(t)
	store := NewSnapshotStore(pool)
	ctx := context.Background()

	_, _ = pool.Exec(ctx, `INSERT INTO runtimes (id) VALUES ('r1') ON CONFLICT DO NOTHING`)

	require.NoError(t, store.Save(ctx, "z1", "r1", []byte(`{"tick":1}`), 1))
	require.NoError(t, store.Save(ctx, "z1", "r1", []byte(`{"tick":2}`), 2))

	snap, tick, err := store.Latest(ctx, "z1")
	require.NoError(t, err)
	assert.Equal(t, int64(2), tick)
	assert.JSONEq(t, `{"tick":2}`, string(snap))
}
