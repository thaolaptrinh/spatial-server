package runtime

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/thaolaptrinh/spatial-server/benchmarks/framework"
)

// stageTicks is the number of ticks each capacity-report stage runs.
const stageTicks = 300

// benchStage drives the harness for b.N ticks and reports tick percentiles +
// overruns. The reported duration per op is the pure g.Tick() cost (injection
// happens before the measured region); allocs/op includes simulated-client
// encoding — see the AOI/dispatch micro-benchmarks for clean runtime allocation
// numbers.
func benchStage(b *testing.B, users int) {
	cfg := DefaultConfig()
	cfg.Users = users
	h := New(cfg)
	h.Setup()
	b.ReportAllocs()
	b.ResetTimer()
	hist := framework.NewHistogram()
	for i := 0; i < b.N; i++ {
		d := h.Step(i)
		hist.Observe(float64(d.Nanoseconds()))
	}
	b.ReportMetric(hist.Percentile(50)/1e6, "tick_p50_ms")
	b.ReportMetric(hist.Percentile(95)/1e6, "tick_p95_ms")
	b.ReportMetric(hist.Percentile(99)/1e6, "tick_p99_ms")
	b.ReportMetric(float64(h.metrics.overruns), "overruns")
}

func BenchmarkStage_50(b *testing.B)   { benchStage(b, 50) }
func BenchmarkStage_100(b *testing.B)  { benchStage(b, 100) }
func BenchmarkStage_250(b *testing.B)  { benchStage(b, 250) }
func BenchmarkStage_500(b *testing.B)  { benchStage(b, 500) }
func BenchmarkStage_1000(b *testing.B) { benchStage(b, 1000) }

// TestCapacityReport runs all five stages progressively and writes a markdown
// capacity report plus goroutine/mutex/block profiles. It is long-running and
// skipped in -short mode so it does not slow the default test suite.
func TestCapacityReport(t *testing.T) {
	if testing.Short() {
		t.Skip("capacity report is long-running; skipped in -short")
	}
	reportsDir := filepath.Join(moduleRoot(), "benchmarks", "reports")
	if err := os.MkdirAll(reportsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Enable synchronization profiles for the run.
	framework.EnableMutexProfile(1)
	framework.EnableBlockProfile(1)

	var stages []Stats
	for _, users := range []int{50, 100, 250, 500, 1000} {
		cfg := DefaultConfig()
		cfg.Users = users
		h := New(cfg)
		h.Setup()
		stages = append(stages, h.Run(stageTicks))
		t.Logf("stage users=%d done: tick mean=%s p95=%s overruns=%d allocs/tick=%s gc=%d goroutines=%d",
			users, stages[len(stages)-1].TickMean, stages[len(stages)-1].TickP95,
			stages[len(stages)-1].TickOverruns, humanCount(stages[len(stages)-1].AllocPerTick),
			stages[len(stages)-1].NumGC, stages[len(stages)-1].Goroutines)
	}

	if err := WriteCapacityReport(filepath.Join(reportsDir, "capacity.md"), stages); err != nil {
		t.Fatal(err)
	}
	framework.WriteHeap(filepath.Join(reportsDir, "heap.pprof"))
	framework.WriteProfile("goroutine", filepath.Join(reportsDir, "goroutine.pprof"))
	framework.WriteProfile("mutex", filepath.Join(reportsDir, "mutex.pprof"))
	framework.WriteProfile("block", filepath.Join(reportsDir, "block.pprof"))
	framework.EnableMutexProfile(0)
	framework.EnableBlockProfile(0)

	t.Log("capacity report + profiles written to benchmarks/reports/")
}
