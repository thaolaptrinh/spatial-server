package aoi

import (
	"fmt"
	"testing"

	"github.com/thaolaptrinh/spatial-server/internal/types"
)

// populate fills an AOI grid with n entities spread across cells.
func populate(b *testing.B, n int) *AOI {
	b.Helper()
	a := New(100, 300)
	for i := 0; i < n; i++ {
		x := float64(i%50) * 25
		z := float64(i/50) * 25
		a.Enter(types.EntityID(fmt.Sprintf("e%d", i)), types.Vector3{X: x, Z: z})
	}
	return a
}

// BenchmarkAOI_Query measures the broadphase query cost (the hot path called
// per entity per tick via EntitiesInRange).
func BenchmarkAOI_Query(b *testing.B) {
	for _, n := range []int{100, 500, 1000} {
		b.Run(fmt.Sprintf("entities=%d", n), func(b *testing.B) {
			a := populate(b, n)
			pos := types.Vector3{X: 600, Z: 600}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = a.Query(pos)
			}
		})
	}
}

// BenchmarkAOI_EntitiesInRange measures the full range query (Query + distance
// filter). This is what updateVisibility calls for every entity each tick.
func BenchmarkAOI_EntitiesInRange(b *testing.B) {
	for _, n := range []int{100, 500, 1000} {
		b.Run(fmt.Sprintf("entities=%d", n), func(b *testing.B) {
			a := populate(b, n)
			pos := types.Vector3{X: 600, Z: 600}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = a.EntitiesInRange(pos, 300)
			}
		})
	}
}

// BenchmarkAOI_Move measures cell-update cost (called on every position change).
func BenchmarkAOI_Move(b *testing.B) {
	a := populate(b, 500)
	id := types.EntityID("e0")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		a.Move(id, types.Vector3{X: float64(i % 1000), Z: float64(i % 700)})
	}
}
