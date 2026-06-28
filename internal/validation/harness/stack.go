//go:build validation

package harness

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/thaolaptrinh/spatial-server/internal/migration"
	"github.com/thaolaptrinh/spatial-server/internal/validation"
)

type processHarness struct {
	pgDSN     string
	redisAddr string
	endpoints map[validation.ResourceID]string
	processes map[validation.ResourceID][]*os.Process
	cleanups  []func()
	mu        sync.Mutex
}

func StartStack(t *testing.T) (*processHarness, func()) {
	t.Helper()
	ctx := context.Background()
	h := &processHarness{
		endpoints: make(map[validation.ResourceID]string),
		processes: make(map[validation.ResourceID][]*os.Process),
	}
	pgC, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("spatial"),
		postgres.WithUsername("spatial"),
		postgres.WithPassword("spatial"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(60*time.Second),
		),
	)
	require.NoError(t, err)
	h.pgDSN, err = pgC.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	redisC, err := redis.Run(ctx, "redis:7-alpine")
	require.NoError(t, err)
	h.redisAddr, err = redisC.Endpoint(ctx, "")
	require.NoError(t, err)
	h.endpoints[validation.ResourcePostgres] = h.pgDSN
	h.endpoints[validation.ResourceRedis] = h.redisAddr
	pool, err := pgxpool.New(ctx, h.pgDSN)
	require.NoError(t, err)
	root := moduleRoot()
	require.NotEmpty(t, root)
	require.NoError(t, migration.Run(pool, filepath.Join(root, "internal", "storage", "migrations")))
	h.cleanups = append(h.cleanups, func() { pgC.Terminate(ctx); redisC.Terminate(ctx) })
	t.Logf("postgres ready: %s", h.pgDSN)
	t.Logf("redis ready: %s", h.redisAddr)
	return h, func() {
		for i := len(h.cleanups) - 1; i >= 0; i-- {
			h.cleanups[i]()
		}
	}
}

func (h *processHarness) Endpoint(id validation.ResourceID) (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if addr, ok := h.endpoints[id]; ok {
		return addr, nil
	}
	return "", fmt.Errorf("no endpoint for %s", id)
}

func (h *processHarness) Processes(id validation.ResourceID) ([]*os.Process, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.processes[id], nil
}

func (h *processHarness) Database(id validation.ResourceID) (string, error) {
	if id == validation.ResourcePostgres {
		return h.pgDSN, nil
	}
	return "", fmt.Errorf("no database for %s", id)
}

func (h *processHarness) RuntimeNodes() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.processes[validation.ResourceRuntime])
}

func (h *processHarness) DialServices(ids ...validation.ResourceID) error {
	for _, id := range ids {
		if _, err := h.Endpoint(id); err != nil {
			return fmt.Errorf("dial %s: %w", id, err)
		}
	}
	return nil
}

func (h *processHarness) Close() error { return nil }

func StartStackForChaos(t *testing.T, runtimes int) (*processHarness, func()) {
	h, teardown := StartStack(t)
	h.StartRoomService(t)
	for i := 0; i < runtimes; i++ {
		h.StartGameServer(t, i)
	}
	h.StartGateway(t)
	return h, teardown
}

func ResetForChaos(t *testing.T, runtimes int) (*processHarness, func()) {
	return StartStackForChaos(t, runtimes)
}

func NewStackForScenario(t *testing.T, runtimes int) (*processHarness, func()) {
	return StartStackForChaos(t, runtimes)
}
