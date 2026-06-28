package game

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/thaolaptrinh/spatial-server/internal/types"
	"github.com/thaolaptrinh/spatial-server/internal/game/entity"
	"github.com/thaolaptrinh/spatial-server/internal/game/zone"
)

// recordingMetrics is a Metrics implementation that records calls for
// assertion in tests.
type recordingMetrics struct {
	tickDurations []time.Duration
	tickOverruns  int
	queueDepths   map[string]int
	drops         map[string]int
	entityCounts  map[string]int
	activeSpaces  int
	runtimeEvents map[string]int
}

func newRecordingMetrics() *recordingMetrics {
	return &recordingMetrics{
		queueDepths:   map[string]int{},
		drops:         map[string]int{},
		entityCounts:  map[string]int{},
		runtimeEvents: map[string]int{},
	}
}

func (m *recordingMetrics) TickDuration(d time.Duration) { m.tickDurations = append(m.tickDurations, d) }
func (m *recordingMetrics) TickOverrun()                { m.tickOverruns++ }
func (m *recordingMetrics) QueueDepth(q string, d int)  { m.queueDepths[q] = d }
func (m *recordingMetrics) Dropped(q string, n int)     { m.drops[q] += n }
func (m *recordingMetrics) EntityCount(t string, n int) { m.entityCounts[t] = n }
func (m *recordingMetrics) ActiveSpaces(n int)          { m.activeSpaces = n }
func (m *recordingMetrics) RuntimeEvent(k string)       { m.runtimeEvents[k]++ }

// TestTick_RecordsMetrics verifies the per-tick observability surface is
// wired: tick duration, queue depths, entity counts and active spaces.
func TestTick_RecordsMetrics(t *testing.T) {
	m := newRecordingMetrics()
	g := New(types.ServerID("gs-1"), WithMetrics(m), WithTickRate(10*time.Millisecond))
	require.NoError(t, g.AssignZone(zone.New(types.ZoneID("z1"), types.RuntimeID("s1"), 0, 0, 100)))
	e := entity.New(types.EntityID("e1"), "avatar", types.RuntimeID("s1"))
	e.ZoneID = types.ZoneID("z1")
	g.AddEntity(e)

	g.tick()

	require.NotEmpty(t, m.tickDurations, "tick duration must be recorded")
	assert.Contains(t, m.queueDepths, "inbox")
	assert.Contains(t, m.queueDepths, "events")
	assert.Contains(t, m.queueDepths, "cmds")
	assert.Equal(t, 1, m.activeSpaces, "active spaces must be reported")
	assert.Equal(t, 1, m.entityCounts["avatar"], "entity count by type must be reported")
}

// TestTick_MeasuredDelta verifies the simulation receives a measured wall-clock
// delta, not the nominal tick rate. Without this, movement becomes
// framerate-dependent under load.
func TestTick_MeasuredDelta(t *testing.T) {
	rec := &deltaLifecycle{}
	g := New(types.ServerID("gs-1"), WithTickRate(50*time.Millisecond))

	t0 := time.Unix(1000000, 0)
	clk := t0
	g.lifecycleClock = func() time.Time { return clk }

	require.NoError(t, g.AssignZone(zone.New(types.ZoneID("z1"), types.RuntimeID("s1"), 0, 0, 100)))
	e := entity.New(types.EntityID("m"), "agent", types.RuntimeID("s1"))
	e.ZoneID = types.ZoneID("z1")
	e.Lifecycle = rec
	g.AddEntity(e)

	g.tick() // establishes lastTickAt
	clk = t0.Add(33 * time.Millisecond)
	g.tick() // dt must be ~33ms

	require.NotZero(t, rec.lastDT)
	assert.InDelta(t, 33, rec.lastDT.Milliseconds(), 2, "simulate must receive measured elapsed delta")
}

// TestTick_BoundedInboundDrain verifies a packet burst cannot blow the tick
// budget: at most maxPacketsPerTick packets are processed per tick and the
// rest remain queued.
func TestTick_BoundedInboundDrain(t *testing.T) {
	m := newRecordingMetrics()
	g := New(types.ServerID("gs-1"), WithMetrics(m))

	burst := maxPacketsPerTick + 10
	for i := 0; i < burst; i++ {
		g.Inbox <- InboundPacket{ClientID: "c", Data: []byte{}} // dispatch returns early on short packet
	}
	g.tick()

	// Exactly 10 packets should remain unprocessed after one bounded tick.
	assert.Equal(t, 10, m.queueDepths["inbox"], "inbound drain must be bounded per tick")
}

type deltaLifecycle struct {
	entity.BaseLifecycle
	lastDT time.Duration
}

func (d *deltaLifecycle) OnSimulate(_ *entity.Entity, dt time.Duration) { d.lastDT = dt }

// TestControlCommand_DropCounted verifies that control-plane commands are
// never dropped silently: when the command queue is full the drop is counted.
func TestControlCommand_DropCounted(t *testing.T) {
	m := newRecordingMetrics()
	g := New(types.ServerID("gs-1"), WithMetrics(m))

	// Fill the command queue so the next enqueue cannot proceed immediately.
	for i := 0; i < cmdChannelBuffer; i++ {
		g.cmds <- func() {}
	}

	g.EnqueueAddEntity(entity.New(types.EntityID("e1"), "avatar", types.RuntimeID("s1")))

	// enqueueCmd blocks up to cmdEnqueueTimeout before dropping.
	deadline := time.Now().Add(cmdEnqueueTimeout + 200*time.Millisecond)
	for time.Now().Before(deadline) {
		if m.drops["cmds_add"] > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	assert.Greater(t, m.drops["cmds_add"], 0, "dropped control command must be counted, never silent")
}
