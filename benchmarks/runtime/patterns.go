package runtime

import (
	"math"
	"math/rand"

	"github.com/thaolaptrinh/spatial-server/internal/types"
)

// MovementPattern updates an entity position for a time step (seconds).
// Patterns are pure functions of (position, dt) so benchmarks are
// reproducible given a fixed rng seed.
type MovementPattern interface {
	Update(pos *types.Vector3, dt float64)
}

// idlePattern does not move (represents AFK/static observers).
type idlePattern struct{}

func (idlePattern) Update(*types.Vector3, float64) {}

// walkPattern moves in a fixed direction at a fixed speed.
type walkPattern struct {
	speed float64
	dir   float64
}

func (w *walkPattern) Update(p *types.Vector3, dt float64) {
	p.X += math.Cos(w.dir) * w.speed * dt
	p.Z += math.Sin(w.dir) * w.speed * dt
}

// randomPattern performs a correlated random walk.
type randomPattern struct {
	rng   *rand.Rand
	speed float64
	dir   float64
}

func (r *randomPattern) Update(p *types.Vector3, dt float64) {
	r.dir += (r.rng.Float64() - 0.5) * 0.4
	p.X += math.Cos(r.dir) * r.speed * dt
	p.Z += math.Sin(r.dir) * r.speed * dt
}

// clusterPattern gravitates toward a center point (clustered workload).
type clusterPattern struct {
	center types.Vector3
	speed  float64
}

func (c *clusterPattern) Update(p *types.Vector3, dt float64) {
	dx := c.center.X - p.X
	dz := c.center.Z - p.Z
	dist := math.Sqrt(dx*dx + dz*dz)
	if dist < 1 {
		return
	}
	p.X += (dx / dist) * c.speed * dt
	p.Z += (dz / dist) * c.speed * dt
}

// hotspotPattern converges on a single hot point (extreme density).
type hotspotPattern struct {
	hot   types.Vector3
	speed float64
}

func (h *hotspotPattern) Update(p *types.Vector3, dt float64) {
	dx := h.hot.X - p.X
	dz := h.hot.Z - p.Z
	dist := math.Sqrt(dx*dx + dz*dz) + 1e-6
	p.X += (dx / dist) * h.speed * dt
	p.Z += (dz / dist) * h.speed * dt
}

// boundaryPattern oscillates across a zone boundary to stress ghost/migration
// code paths.
type boundaryPattern struct {
	edge float64
	span float64
	speed float64
	phase int
}

func (b *boundaryPattern) Update(p *types.Vector3, dt float64) {
	p.X += float64(b.phase) * b.speed * dt
	if p.X > b.edge+b.span {
		b.phase = -1
	} else if p.X < b.edge-b.span {
		b.phase = 1
	}
}
