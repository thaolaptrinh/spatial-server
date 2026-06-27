package framework

import "testing"

func TestHistogram_Percentiles(t *testing.T) {
	h := NewHistogram()
	for i := 1; i <= 100; i++ {
		h.Observe(float64(i))
	}
	p95 := h.Percentile(95)
	if p95 < 94 || p95 > 96 {
		t.Fatalf("expected ~95, got %.2f", p95)
	}
}
