package storage

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/thaolaptrinh/spatial-server/internal/migration"
)

func TestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("SPATIAL_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set SPATIAL_TEST_POSTGRES_DSN")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	require.NoError(t, err)
	_, _ = pool.Exec(context.Background(), "TRUNCATE game_servers, zones, runtimes CASCADE")
	require.NoError(t, migration.Run(pool, "internal/storage/migrations"))
	t.Cleanup(pool.Close)
	return pool
}
