//go:build validation

package harness

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/thaolaptrinh/spatial-server/internal/validation"
)

func (h *processHarness) StartRoomService(t *testing.T) {
	t.Helper()
	binPath := buildService(t, "room-service")
	root := moduleRoot()
	logPath := filepath.Join(os.TempDir(), fmt.Sprintf("spatial-room-service-%d.log", time.Now().UnixNano()))
	f, err := os.Create(logPath)
	require.NoError(t, err)
	cmd := exec.Command(binPath)
	cmd.Dir = root
	cmd.Env = append(cleanEnv(), "SPATIAL_POSTGRES__DSN="+h.pgDSN, "SPATIAL_REDIS__ADDR="+h.redisAddr, "SPATIAL_GRPC__HOST=127.0.0.1", "SPATIAL_GRPC__PORT=19000")
	cmd.Stdout, cmd.Stderr = f, f
	require.NoError(t, cmd.Start())
	h.mu.Lock()
	h.processes[validation.ResourceRoomService] = append(h.processes[validation.ResourceRoomService], cmd.Process)
	h.endpoints[validation.ResourceRoomService] = "127.0.0.1:19000"
	h.mu.Unlock()
	h.cleanups = append(h.cleanups, func() { killProcess(cmd); os.Remove(binPath); f.Close() })
	waitForGRPC(t, "127.0.0.1:19000", 30*time.Second)
}

func (h *processHarness) StartGameServer(t *testing.T, index int) {
	t.Helper()
	binPath := buildService(t, "game-server")
	port := 19001 + index
	root := moduleRoot()
	logPath := filepath.Join(os.TempDir(), fmt.Sprintf("spatial-game-server-%d-%d.log", index, time.Now().UnixNano()))
	f, err := os.Create(logPath)
	require.NoError(t, err)
	cmd := exec.Command(binPath)
	cmd.Dir = root
	cmd.Env = append(cleanEnv(), "SPATIAL_GRPC__HOST=127.0.0.1", fmt.Sprintf("SPATIAL_GRPC__PORT=%d", port), "SPATIAL_ROOM_SERVICE__ADDR=127.0.0.1:19000")
	cmd.Stdout, cmd.Stderr = f, f
	require.NoError(t, cmd.Start())
	h.mu.Lock()
	h.processes[validation.ResourceRuntime] = append(h.processes[validation.ResourceRuntime], cmd.Process)
	h.mu.Unlock()
	h.cleanups = append(h.cleanups, func() { killProcess(cmd); os.Remove(binPath); f.Close() })
	waitForGRPC(t, fmt.Sprintf("127.0.0.1:%d", port), 30*time.Second)
}

func (h *processHarness) StartGateway(t *testing.T) {
	t.Helper()
	binPath := buildService(t, "gateway")
	root := moduleRoot()
	logPath := filepath.Join(os.TempDir(), fmt.Sprintf("spatial-gateway-%d.log", time.Now().UnixNano()))
	f, err := os.Create(logPath)
	require.NoError(t, err)
	cmd := exec.Command(binPath)
	cmd.Dir = root
	cmd.Env = append(cleanEnv(), "SPATIAL_GATEWAY__WS_PORT=18080", "SPATIAL_ROOM_SERVICE__ADDR=127.0.0.1:19000")
	cmd.Stdout, cmd.Stderr = f, f
	require.NoError(t, cmd.Start())
	h.mu.Lock()
	h.processes[validation.ResourceGateway] = append(h.processes[validation.ResourceGateway], cmd.Process)
	h.endpoints[validation.ResourceGateway] = "127.0.0.1:18080"
	h.mu.Unlock()
	h.cleanups = append(h.cleanups, func() { killProcess(cmd); os.Remove(binPath); f.Close() })
	waitForHTTP(t, "http://127.0.0.1:18080/health", 30*time.Second)
}

func cleanEnv() []string {
	var env []string
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "SPATIAL_") {
			env = append(env, e)
		}
	}
	return env
}

func killProcess(cmd *exec.Cmd) {
	cmd.Process.Signal(syscall.SIGTERM)
	done := make(chan struct{})
	go func() { cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		cmd.Process.Kill()
	}
}
