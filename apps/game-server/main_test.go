package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found from", dir)
		}
		dir = parent
	}
}

func buildBinary(t *testing.T, pkg string) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), filepath.Base(pkg))
	cmd := exec.Command("go", "build", "-o", bin, pkg)
	cmd.Dir = moduleRoot(t)
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build %s: %v\n%s", pkg, err, out)
	}
	return bin
}

func TestGameServerBinary_StartAndGracefulShutdown(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("SIGTERM handling differs on Windows")
	}
	bin := buildBinary(t, "./apps/game-server/")
	cmd := exec.Command(bin)
	cmd.Dir = moduleRoot(t)
	cmd.Env = append(os.Environ(), "SPATIAL_GRPC__PORT=9999")

	start := time.Now()
	if err := cmd.Start(); err != nil {
		t.Fatalf("start game-server: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	time.Sleep(200 * time.Millisecond)
	cmd.Process.Signal(os.Interrupt)

	select {
	case err := <-done:
		if err != nil {
			t.Logf("game-server exit: %v", err)
		}
	case <-time.After(5 * time.Second):
		cmd.Process.Kill()
		t.Fatal("game-server did not shut down within 5s")
	}
	t.Logf("game-server started and shut down in %v", time.Since(start))
}

func TestGameServerBinary_PortConflict(t *testing.T) {
	bin := buildBinary(t, "./apps/game-server/")

	// Start first instance on port 9997
	cmd1 := exec.Command(bin)
	cmd1.Dir = moduleRoot(t)
	cmd1.Env = append(os.Environ(), "SPATIAL_GRPC__PORT=9997")
	if err := cmd1.Start(); err != nil {
		t.Fatalf("start first instance: %v", err)
	}
	defer cmd1.Process.Kill()
	time.Sleep(100 * time.Millisecond)

	// Start second instance on same port — should fail
	cmd2 := exec.Command(bin)
	cmd2.Dir = moduleRoot(t)
	cmd2.Env = append(os.Environ(), "SPATIAL_GRPC__PORT=9997")
	out, err := cmd2.CombinedOutput()
	if err == nil {
		t.Fatal("expected port conflict error, got none")
	}
	t.Logf("port conflict error: %s", string(out))

	cmd1.Process.Signal(os.Interrupt)
	cmd1.Wait()
}
