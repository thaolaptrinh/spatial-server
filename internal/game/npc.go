package game

import (
	"log/slog"
	"math"
	"math/rand"
	"time"

	"github.com/thaolaptrinh/spatial-server/internal/types"
	"github.com/thaolaptrinh/spatial-server/internal/game/entity"
)

type Behavior interface {
	Step(e *entity.Entity, dt time.Duration) (moved bool)
}

type PatrolBehavior struct {
	Speed     float64
	Waypoints []types.Vector3
	target    int
}

func (p *PatrolBehavior) Step(e *entity.Entity, dt time.Duration) bool {
	if len(p.Waypoints) == 0 {
		return false
	}
	wp := p.Waypoints[p.target]
	delta := p.Speed * dt.Seconds()
	moveAxis(&e.Position.X, wp.X, delta)
	moveAxis(&e.Position.Z, wp.Z, delta)
	if e.Position.X == wp.X && e.Position.Z == wp.Z {
		p.target = (p.target + 1) % len(p.Waypoints)
	}
	return true
}

type IdleBehavior struct {
	BobAmplitude, BobFreq, phase float64
}

func (i *IdleBehavior) Step(e *entity.Entity, dt time.Duration) bool {
	i.phase += i.BobFreq * dt.Seconds()
	e.Position.Y = i.BobAmplitude * math.Sin(i.phase)
	return false
}

type WanderBehavior struct {
	Origin            types.Vector3
	Radius, Speed     float64
	rng               *rand.Rand
	target            types.Vector3
	pause             time.Duration
}

func (w *WanderBehavior) Step(e *entity.Entity, dt time.Duration) bool {
	if w.rng == nil {
		w.rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	if w.pause > 0 {
		w.pause -= dt
		return false
	}
	if w.target == (types.Vector3{}) {
		angle := w.rng.Float64() * 2 * math.Pi
		r := w.rng.Float64() * w.Radius
		w.target = types.Vector3{
			X: w.Origin.X + r*math.Cos(angle),
			Z: w.Origin.Z + r*math.Sin(angle),
		}
	}
	delta := w.Speed * dt.Seconds()
	moved := moveAxis(&e.Position.X, w.target.X, delta) || moveAxis(&e.Position.Z, w.target.Z, delta)
	if !moved {
		w.target = types.Vector3{}
		w.pause = 500 * time.Millisecond
	}
	return true
}

func moveAxis(pos *float64, target, delta float64) bool {
	if *pos == target {
		return false
	}
	if math.Abs(target-*pos) <= delta {
		*pos = target
		return true
	}
	if target > *pos {
		*pos += delta
	} else {
		*pos -= delta
	}
	return true
}

var behaviorFactories = map[string]func() Behavior{
	"patrol": func() Behavior { return &PatrolBehavior{Speed: 10} },
	"idle":   func() Behavior { return &IdleBehavior{BobAmplitude: 0.5, BobFreq: 2} },
	"wander": func() Behavior { return &WanderBehavior{Radius: 20, Speed: 10} },
}

func newBehavior(tag string) Behavior {
	if f, ok := behaviorFactories[tag]; ok {
		return f()
	}
	slog.Warn("unknown NPC behavior, falling back to idle", slog.String("behavior", tag))
	return behaviorFactories["idle"]()
}

type NPCLifecycle struct {
	entity.BaseLifecycle
	Behavior Behavior
}

// OnSimulate steps the entity's autonomous behavior. The simulation loop
// invokes this via the generic Lifecycle contract; it never type-switches on
// NPCLifecycle, so new autonomous lifecycles can be added without modifying
// the simulation core.
func (n *NPCLifecycle) OnSimulate(e *entity.Entity, dt time.Duration) {
	if n.Behavior == nil {
		return
	}
	n.Behavior.Step(e, dt)
}
