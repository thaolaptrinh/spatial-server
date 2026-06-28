//go:build integration

package integration

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/thaolaptrinh/spatial-server/internal/migration"
)

type stack struct {
	pgDSN     string
	redisAddr string
	cleanup   func()
}

func moduleRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func startStack(t *testing.T) *stack {
	t.Helper()
	ctx := context.Background()

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

	pgDSN, err := pgC.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	redisC, err := redis.Run(ctx, "redis:7-alpine")
	require.NoError(t, err)

	redisAddr, err := redisC.Endpoint(ctx, "")
	require.NoError(t, err)

	pool, err := pgxpool.New(ctx, pgDSN)
	require.NoError(t, err)

	root := moduleRoot()
	require.NotEmpty(t, root)
	require.NoError(t, migration.Run(pool, filepath.Join(root, "internal", "storage", "migrations")))

	cleanup := func() {
		pgC.Terminate(ctx)
		redisC.Terminate(ctx)
	}

	t.Logf("postgres ready: %s", pgDSN)
	t.Logf("redis ready: %s", redisAddr)

	return &stack{pgDSN: pgDSN, redisAddr: redisAddr, cleanup: cleanup}
}

func buildService(t *testing.T, name string) string {
	t.Helper()
	root := moduleRoot()
	require.NotEmpty(t, root)
	binPath := filepath.Join(os.TempDir(), fmt.Sprintf("spatial-%s-%d", name, time.Now().UnixNano()))
	cmd := exec.Command("go", "build", "-o", binPath, fmt.Sprintf("./apps/%s/", name))
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "build %s failed:\n%s", name, string(out))
	return binPath
}

func startService(t *testing.T, name, binPath string, extraEnv ...string) func() {
	t.Helper()
	var env []string
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "SPATIAL_") {
			env = append(env, e)
		}
	}
	env = append(env, extraEnv...)

	logPath := filepath.Join(os.TempDir(), fmt.Sprintf("spatial-%s-%d.log", name, time.Now().UnixNano()))
	f, err := os.Create(logPath)
	require.NoError(t, err)
	t.Logf("%s log: %s", name, logPath)

	root := moduleRoot()
	cmd := exec.Command(binPath)
	cmd.Dir = root
	cmd.Env = env
	cmd.Stdout = f
	cmd.Stderr = f
	require.NoError(t, cmd.Start())

	t.Logf("started %s (pid=%d)", name, cmd.Process.Pid)

	cleanup := func() {
		cmd.Process.Signal(syscall.SIGTERM)
		done := make(chan struct{})
		go func() {
			cmd.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			cmd.Process.Kill()
		}
		os.Remove(binPath)
		f.Close()
	}

	return cleanup
}

func waitForGRPC(t *testing.T, addr string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, time.Second)
		if err == nil {
			conn.Close()
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for gRPC %s", addr)
}

func waitForActiveServer(t *testing.T, pgDSN string, timeout time.Duration) {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), pgDSN)
	if err != nil {
		t.Fatalf("connect for waitForActiveServer: %v", err)
	}
	defer pool.Close()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var status string
		err := pool.QueryRow(context.Background(),
			`SELECT status FROM game_servers ORDER BY last_heartbeat DESC LIMIT 1`).Scan(&status)
		if err == nil && status == "active" {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for active game-server")
}

func waitForHTTP(t *testing.T, url string, timeout time.Duration) {
	t.Helper()
	client := &http.Client{Timeout: time.Second}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for HTTP %s", url)
}
