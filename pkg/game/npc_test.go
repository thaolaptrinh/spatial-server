package game

import (
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/thaolaptrinh/spatial-server/internal/types"
	"github.com/thaolaptrinh/spatial-server/pkg/entity"
)

func TestPatrolBehavior_StepsTowardWaypoint(t *testing.T) {
	b := PatrolBehavior{Speed: 10, Waypoints: []types.Vector3{{X: 100}}}
	e := entity.New("n1", "npc", types.RuntimeID("r1"))
	assert.True(t, b.Step(e, time.Second))
	assert.Greater(t, e.Position.X, 0.0)
}

func TestIdleBehavior_NoHorizontalDrift(t *testing.T) {
	b := IdleBehavior{BobAmplitude: 1, BobFreq: 1}
	e := entity.New("n2", "npc", types.RuntimeID("r1"))
	e.Position = types.Vector3{X: 5}
	b.Step(e, time.Second)
	assert.Equal(t, 5.0, e.Position.X)
}

func TestWanderBehavior_StaysWithinRadius(t *testing.T) {
	b := WanderBehavior{Origin: types.Vector3{X: 50, Z: 50}, Radius: 20, Speed: 100,
		rng: rand.New(rand.NewSource(1))}
	e := entity.New("n3", "npc", types.RuntimeID("r1"))
	e.Position = types.Vector3{X: 50, Z: 50}
	for i := 0; i < 50; i++ {
		b.Step(e, 100*time.Millisecond)
	}
	dx, dz := e.Position.X-50, e.Position.Z-50
	assert.LessOrEqual(t, dx*dx+dz*dz, 900.0)
}

func TestRegistry_FallbackToIdle(t *testing.T) {
	_, ok := newBehavior("patrol").(*PatrolBehavior)
	assert.True(t, ok)
	_, ok = newBehavior("nope").(*IdleBehavior)
	assert.True(t, ok)
}
