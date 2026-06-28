//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// defaultAddrs points at a `docker compose up` stack on the host ports.
func defaultAddrs() StackAddrs {
	return StackAddrs{
		Gateway:     "127.0.0.1:8080",
		RoomService: "127.0.0.1:9001",
		PostgresDSN: "postgres://spatial:spatial@127.0.0.1:5432/spatial?sslmode=disable",
		JWTSecret:   "dev-secret-key-change-in-production",
	}
}

// TestE2E_DistributedBenchmark runs progressive client counts against the live
// stack and writes benchmarks/reports/e2e-distributed.md. Requires the stack:
// `docker compose -f deploy/docker-compose/docker-compose.yml up -d`.
// Run with: go test -tags=e2e -run=TestE2E_DistributedBenchmark ./benchmarks/e2e/
func TestE2E_DistributedBenchmark(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e benchmark is long-running")
	}
	if _, err := os.Stat(filepath.Join(reportsDir(), "..")); err != nil {
		t.Skipf("benchmarks dir not found (run from repo root): %v", err)
	}
	addrs := defaultAddrs()

	stages := []int{10, 25, 50}
	runID := time.Now().Unix()
	var results []*Result
	for _, clients := range stages {
		cfg := Config{
			Clients:      clients,
			Duration:     20 * time.Second,
			SendInterval: 100 * time.Millisecond, // 10 Hz per client
			Addrs:        addrs,
			RuntimeID:    fmt.Sprintf("e2e-bench-%d-%d", clients, runID),
			ZoneCount:    1,
		}
		t.Logf("stage clients=%d starting", clients)
		r, err := Run(context.Background(), cfg)
		if err != nil {
			t.Logf("stage clients=%d failed: %v (is the stack up?)", clients, err)
			continue
		}
		results = append(results, r)
		t.Logf("stage clients=%d: connect p95=%s round-trip p50=%s p95=%s p99=%s sends/s=%.0f frames/s=%.0f",
			clients, fmtDur(r.ConnectP95), fmtDur(r.RoundTripP50), fmtDur(r.RoundTripP95), fmtDur(r.RoundTripP99), r.SendsPerSec, r.FramesPerSec)
	}

	if len(results) == 0 {
		t.Fatal("no stages completed — ensure the stack is running")
	}
	if err := WriteReport(filepath.Join(reportsDir(), "e2e-distributed.md"), results); err != nil {
		t.Fatal(err)
	}
	t.Log("distributed e2e report written to benchmarks/reports/e2e-distributed.md")
}
