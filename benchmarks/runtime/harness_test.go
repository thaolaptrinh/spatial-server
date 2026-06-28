package runtime

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHarness_SetupSpawnsEntities verifies Setup creates exactly cfg.Users
// entities and assigns each a movement pattern.
func TestHarness_SetupSpawnsEntities(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Users = 25
	cfg.WorldSize = 300
	h := New(cfg)
	h.Setup()
	require.Len(t, h.entities, cfg.Users)
	require.Len(t, h.patterns, cfg.Users)
	h.Close()
}

// TestHarness_Run_SaneStats runs a short benchmark and checks the returned
// Stats are well-formed — a wiring regression (e.g. ticks not counted, events
// not drained) would surface here.
func TestHarness_Run_SaneStats(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Users = 20
	cfg.WorldSize = 300
	h := New(cfg)
	h.Setup()
	stats := h.Run(60) // Run closes the Events drainer

	assert.Equal(t, 60, stats.Ticks)
	assert.Greater(t, stats.TickMean, time.Duration(0))
	assert.Greater(t, stats.TickP95, time.Duration(0))
	assert.Greater(t, stats.TickP99, time.Duration(0))
	assert.GreaterOrEqual(t, stats.TickP99, stats.TickP95, "p99 must be >= p95")
	assert.Greater(t, stats.HeapBytes, uint64(0))
}

// TestHarness_DeterministicWithSeed verifies two harnesses with the same seed
// produce identical entity placements (reproducibility).
func TestHarness_DeterministicWithSeed(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Users = 10
	cfg.WorldSize = 200

	h1 := New(cfg)
	h1.Setup()
	h1.Close()
	h2 := New(cfg)
	h2.Setup()
	h2.Close()

	require.Len(t, h1.entities, len(h2.entities))
	for i := range h1.entities {
		assert.Equal(t, h1.entities[i].Position, h2.entities[i].Position, "seeded placement must match")
	}
}
