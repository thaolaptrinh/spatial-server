package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// WriteReport writes a markdown distributed-benchmark report.
func WriteReport(path string, results []*Result) error {
	var b strings.Builder
	b.WriteString("# Distributed End-to-End Benchmark Report\n\n")
	b.WriteString(fmt.Sprintf("> Generated: %s\n\n", time.Now().Format(time.RFC3339)))
	b.WriteString("Full data path exercised: Client → WebSocket → Gateway → gRPC → Runtime Node → AOI → events → gRPC → Gateway → WebSocket → Client.\n")
	b.WriteString("`round-trip` = client A sends a position update, a different client receives the resulting EntityMove.\n\n")
	b.WriteString("| Clients | Connect p50 | Connect p95 | Round-trip p50 | Round-trip p95 | Round-trip p99 | Round-trip max | Sends/s | Frames/s | Duration |\n")
	b.WriteString("|---|---|---|---|---|---|---|---|---|---|\n")
	for _, r := range results {
		b.WriteString(fmt.Sprintf("| %d | %s | %s | %s | %s | %s | %s | %.0f | %.0f | %s |\n",
			r.Clients, fmtDur(r.ConnectP50), fmtDur(r.ConnectP95),
			fmtDur(r.RoundTripP50), fmtDur(r.RoundTripP95), fmtDur(r.RoundTripP99), fmtDur(r.RoundTripMax),
			r.SendsPerSec, r.FramesPerSec, r.Duration))
	}
	b.WriteString("\n## Notes\n")
	b.WriteString("- Multi-node scaling, failure injection, and long-duration (30m–3h) runs are framework-supported but executed in CI/long-running environments (see README).\n")
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func fmtDur(d time.Duration) string {
	switch {
	case d <= 0:
		return "0"
	case d < 10*time.Microsecond:
		return fmt.Sprintf("%.2fµs", float64(d.Nanoseconds())/1000)
	case d < time.Millisecond:
		return fmt.Sprintf("%.0fµs", float64(d.Nanoseconds())/1000)
	default:
		return fmt.Sprintf("%.2fms", float64(d.Microseconds())/1000.0)
	}
}

// reportsDir resolves the module-root benchmarks/reports directory so reports
// land at a stable path regardless of the test working directory.
func reportsDir() string {
	dir, err := os.Getwd()
	if err != nil {
		return "benchmarks/reports"
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return filepath.Join(dir, "benchmarks", "reports")
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "benchmarks/reports"
		}
		dir = parent
	}
}
