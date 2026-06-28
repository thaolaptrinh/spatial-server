package framework

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReport_AssertP95 verifies the p95 gate: passing when p95 is within budget
// (and recording it), failing when over budget.
func TestReport_AssertP95(t *testing.T) {
	h := NewHistogram()
	for i := 1; i <= 100; i++ {
		h.Observe(float64(i))
	}

	r := NewReport("unit")
	require.NoError(t, r.AssertP95(h, 200)) // p95 ~95 < 200 → pass
	assert.InDelta(t, 95.0, r.P95, 1.0)

	err := r.AssertP95(h, 50) // p95 ~95 > 50 → fail
	require.Error(t, err)
}

// TestReport_WriteTo verifies the report serializes to valid JSON at the given
// path and round-trips.
func TestReport_WriteTo(t *testing.T) {
	r := NewReport("unit")
	r.P95 = 42
	r.Packets = 7
	r.Pass = true

	path := filepath.Join(t.TempDir(), "report.json")
	require.NoError(t, r.WriteTo(path))

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var got Report
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, "unit", got.Scenario)
	assert.Equal(t, 42.0, got.P95)
	assert.Equal(t, 7, got.Packets)
	assert.True(t, got.Pass)
}
