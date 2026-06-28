package runtime

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModuleRoot_FindsGoMod(t *testing.T) {
	root := moduleRoot()
	require.NotEmpty(t, root, "moduleRoot must locate the go.mod directory")
	_, err := os.Stat(filepath.Join(root, "go.mod"))
	assert.NoError(t, err, "located root must contain go.mod")
}

func TestWriteCapacityReport_OverBudgetWithDrops(t *testing.T) {
	stages := []Stats{
		{Config: Config{Users: 100, TickRate: 50 * time.Millisecond}, Ticks: 300,
			TickMean: time.Millisecond, TickP50: time.Millisecond, TickP95: 2 * time.Millisecond,
			TickP99: 3 * time.Millisecond, TickMax: 4 * time.Millisecond,
			AllocPerTick: 60_000, HeapBytes: 2 * 1024 * 1024, NumGC: 8, Goroutines: 3},
		{Config: Config{Users: 1000, TickRate: 50 * time.Millisecond}, Ticks: 300,
			TickMean: 10 * time.Millisecond, TickP95: 60 * time.Millisecond, // exceeds 50ms budget
			TickP99: 70 * time.Millisecond, TickMax: 80 * time.Millisecond,
			AllocPerTick: 2_860_000, HeapBytes: 4 * 1024 * 1024, NumGC: 256,
			GCPauseTotal: 19 * time.Millisecond, Goroutines: 3,
			Drops:        map[string]int{"inbox": 100},
			RuntimeEvents: map[string]int{"spawn": 5000, "move": 12000}},
	}
	path := filepath.Join(t.TempDir(), "capacity.md")
	require.NoError(t, WriteCapacityReport(path, stages))

	body, err := os.ReadFile(path)
	require.NoError(t, err)
	s := string(body)
	assert.Contains(t, s, "| 100 |")
	assert.Contains(t, s, "| 1000 |")
	assert.Contains(t, s, "Limiting stage", "over-budget stage must be flagged")
	assert.Contains(t, s, "**YES**", "over-budget marker must appear")
	assert.Contains(t, s, "Backpressure engaged", "drops must be reported")
	assert.Contains(t, s, "spawn", "runtime events table must include event kinds")
	assert.Contains(t, s, "move")
}

func TestWriteCapacityReport_WithinBudget(t *testing.T) {
	stages := []Stats{
		{Config: Config{Users: 50, TickRate: 50 * time.Millisecond}, Ticks: 100,
			TickP95: time.Millisecond, AllocPerTick: 30_000},
	}
	path := filepath.Join(t.TempDir(), "capacity.md")
	require.NoError(t, WriteCapacityReport(path, stages))
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	s := string(body)
	assert.NotContains(t, s, "Limiting stage")
	assert.Contains(t, s, "sustained the tick budget")
}
