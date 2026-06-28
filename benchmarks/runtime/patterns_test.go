package runtime

import (
	"math"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/thaolaptrinh/spatial-server/internal/types"
)

func TestIdlePattern_DoesNotMove(t *testing.T) {
	p := idlePattern{}
	pos := types.Vector3{X: 5, Z: 7}
	p.Update(&pos, 1.0)
	assert.Equal(t, 5.0, pos.X)
	assert.Equal(t, 7.0, pos.Z)
}

func TestWalkPattern_MovesInDirection(t *testing.T) {
	p := &walkPattern{speed: 10, dir: 0} // +X
	pos := types.Vector3{}
	p.Update(&pos, 1.0)
	assert.InDelta(t, 10.0, pos.X, 1e-6, "should move +X by speed*dt")
	assert.InDelta(t, 0.0, pos.Z, 1e-6)

	p2 := &walkPattern{speed: 10, dir: math.Pi / 2} // +Z
	pos2 := types.Vector3{}
	p2.Update(&pos2, 0.5)
	assert.InDelta(t, 0.0, pos2.X, 1e-6)
	assert.InDelta(t, 5.0, pos2.Z, 1e-6)
}

func TestClusterPattern_MovesTowardCenter(t *testing.T) {
	center := types.Vector3{X: 100, Z: 100}
	p := &clusterPattern{center: center, speed: 10}
	pos := types.Vector3{}
	distBefore := dist(pos, center)
	p.Update(&pos, 1.0)
	distAfter := dist(pos, center)
	assert.Less(t, distAfter, distBefore, "cluster must move toward center")
}

func TestHotspotPattern_Converges(t *testing.T) {
	hot := types.Vector3{X: 50, Z: 50}
	p := &hotspotPattern{hot: hot, speed: 12}
	pos := types.Vector3{}
	prev := dist(pos, hot)
	for i := 0; i < 20; i++ {
		p.Update(&pos, 0.5)
	}
	assert.Less(t, dist(pos, hot), prev, "hotspot must converge on the hot point")
}

func TestBoundaryPattern_OscillatesAcrossEdge(t *testing.T) {
	p := &boundaryPattern{edge: 50, span: 10, speed: 5, phase: 1}
	pos := types.Vector3{X: 50}
	maxX := pos.X
	minX := pos.X
	for i := 0; i < 100; i++ {
		p.Update(&pos, 0.5)
		if pos.X > maxX {
			maxX = pos.X
		}
		if pos.X < minX {
			minX = pos.X
		}
	}
	// It must have crossed the edge in both directions (phase flipped).
	assert.Greater(t, maxX, 50.0, "must cross above the edge")
	assert.Less(t, minX, 50.0, "must cross below the edge after flipping")
}

func TestRandomPattern_DeterministicWithSeed(t *testing.T) {
	mk := func() *randomPattern {
		return &randomPattern{rng: rand.New(rand.NewSource(42)), speed: 5, dir: 0}
	}
	a, b := mk(), mk()
	pa, pb := types.Vector3{}, types.Vector3{}
	for i := 0; i < 50; i++ {
		a.Update(&pa, 0.1)
		b.Update(&pb, 0.1)
	}
	assert.Equal(t, pa, pb, "same seed must produce identical trajectories")
}

func dist(a, b types.Vector3) float64 {
	dx := a.X - b.X
	dz := a.Z - b.Z
	return math.Sqrt(dx*dx + dz*dz)
}
