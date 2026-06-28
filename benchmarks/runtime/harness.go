// Package runtime provides an in-process benchmark harness that drives a real
// game.Game with a configurable number of simulated entities and movement
// patterns. It measures the runtime core (tick latency, AOI cost, queue
// utilization, drops, allocations, GC) without the network stack, so results
// are reproducible and CI-friendly. Cross-node end-to-end measurement is the
// job of the WebSocket framework.
package runtime

import (
	"fmt"
	"math/rand"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/thaolaptrinh/spatial-server/benchmarks/framework"
	"github.com/thaolaptrinh/spatial-server/internal/game"
	"github.com/thaolaptrinh/spatial-server/internal/game/entity"
	"github.com/thaolaptrinh/spatial-server/internal/game/zone"
	"github.com/thaolaptrinh/spatial-server/internal/types"
	"github.com/thaolaptrinh/spatial-server/pkg/protocol"
	v1 "github.com/thaolaptrinh/spatial-server/proto/gen/spatialserver/v1"
)

// Config configures a benchmark run.
type Config struct {
	Users       int           // simulated entities
	Pattern     string        // idle|walk|random|cluster|hotspot|boundary|mixed
	MovingFrac  float64       // fraction of users that move (realistic mix)
	CellSize    float64       // AOI/zone cell size (world units)
	Radius      float64       // AOI interest radius
	WorldSize   float64       // side length of the square world
	TickRate    time.Duration // simulation tick interval
	UpdateEvery int           // per-entity position update interval (ticks)
	Seed        int64         // rng seed for reproducibility
}

// DefaultConfig returns a realistic baseline configuration.
func DefaultConfig() Config {
	return Config{
		Pattern:     "mixed",
		MovingFrac:  0.6,
		CellSize:    100.0,
		Radius:      300.0,
		WorldSize:   1000.0,
		TickRate:    50 * time.Millisecond,
		UpdateEvery: 1,
		Seed:        1,
	}
}

// Stats holds the measured results of a benchmark run.
type Stats struct {
	Config        Config
	Ticks         int
	TickMean      time.Duration
	TickP50       time.Duration
	TickP95       time.Duration
	TickP99       time.Duration
	TickMax       time.Duration
	TickOverruns  int64
	AllocPerTick  uint64
	HeapBytes     uint64
	NumGC         uint32
	GCPauseTotal  time.Duration
	Goroutines    int
	Drops         map[string]int
	RuntimeEvents map[string]int
}

// benchMetrics implements game.Metrics to capture drops, overruns and event counts.
type benchMetrics struct {
	overruns      int64
	drops         map[string]int
	runtimeEvents map[string]int
}

func newBenchMetrics() *benchMetrics {
	return &benchMetrics{drops: map[string]int{}, runtimeEvents: map[string]int{}}
}

func (m *benchMetrics) TickDuration(time.Duration)     {}
func (m *benchMetrics) TickOverrun()                   { m.overruns++ }
func (m *benchMetrics) QueueDepth(string, int)         {}
func (m *benchMetrics) Dropped(q string, n int)        { m.drops[q] += n }
func (m *benchMetrics) EntityCount(string, int)        {}
func (m *benchMetrics) ActiveSpaces(int)               {}
func (m *benchMetrics) RuntimeEvent(k string)          { m.runtimeEvents[k]++ }

// Harness drives a game.Game with simulated entities.
type Harness struct {
	cfg       Config
	g         *game.Game
	metrics   *benchMetrics
	rng       *rand.Rand
	entities  []*entity.Entity
	patterns  []MovementPattern
	edgeX     float64
	done      chan struct{}
	drainerWg sync.WaitGroup
}

// New creates a harness for the given config (does not set up entities).
func New(cfg Config) *Harness {
	if cfg.UpdateEvery <= 0 {
		cfg.UpdateEvery = 1
	}
	m := newBenchMetrics()
	return &Harness{
		cfg:     cfg,
		g:       game.New(types.ServerID("bench-gs"), game.WithTickRate(cfg.TickRate), game.WithMetrics(m)),
		metrics: m,
		rng:     rand.New(rand.NewSource(cfg.Seed)),
	}
}

// Setup assigns the zone grid and spawns the configured entities.
func (h *Harness) Setup() {
	// Model the production relay adapter: a consumer must drain the Events
	// channel, otherwise it fills and publish drops artificially. The drainer
	// runs until Close.
	h.done = make(chan struct{})
	h.drainerWg.Add(1)
	go func() {
		defer h.drainerWg.Done()
		for {
			select {
			case <-h.g.Events:
			case <-h.done:
				return
			}
		}
	}()

	cell := h.cfg.CellSize
	perAxis := int(h.cfg.WorldSize/cell)
	if perAxis < 1 {
		perAxis = 1
	}
	rid := types.RuntimeID("space-bench")
	for gx := 0; gx < perAxis; gx++ {
		for gy := 0; gy < perAxis; gy++ {
			zid := types.ZoneID(fmt.Sprintf("z-%d-%d", gx, gy))
			_ = h.g.AssignZone(zone.New(zid, rid, gx, gy, cell))
		}
	}
	h.edgeX = float64(perAxis/2) * cell

	for i := 0; i < h.cfg.Users; i++ {
		x := h.cfg.WorldSize * h.rng.Float64()
		z := h.cfg.WorldSize * h.rng.Float64()
		gx := int(x / cell)
		gy := int(z / cell)
		if gx >= perAxis {
			gx = perAxis - 1
		}
		if gy >= perAxis {
			gy = perAxis - 1
		}
		eid := types.EntityID(fmt.Sprintf("u-%d", i))
		e := entity.New(eid, "avatar", rid)
		e.Position = types.Vector3{X: x, Z: z}
		e.ZoneID = types.ZoneID(fmt.Sprintf("z-%d-%d", gx, gy))
		h.g.AddEntity(e)
		h.entities = append(h.entities, e)
		h.patterns = append(h.patterns, h.assignPattern(i))
	}
}

func (h *Harness) assignPattern(i int) MovementPattern {
	moving := h.rng.Float64() < h.cfg.MovingFrac
	if !moving {
		return idlePattern{}
	}
	switch h.cfg.Pattern {
	case "walk":
		return &walkPattern{speed: 10, dir: h.rng.Float64() * 2 * 3.14159265}
	case "random":
		return &randomPattern{rng: rand.New(rand.NewSource(h.cfg.Seed + int64(i))), speed: 8, dir: h.rng.Float64() * 2 * 3.14159265}
	case "cluster":
		return &clusterPattern{center: types.Vector3{X: h.cfg.WorldSize / 2, Z: h.cfg.WorldSize / 2}, speed: 6}
	case "hotspot":
		return &hotspotPattern{hot: types.Vector3{X: h.cfg.WorldSize / 2, Z: h.cfg.WorldSize / 2}, speed: 12}
	case "boundary":
		return &boundaryPattern{edge: h.edgeX, span: h.cfg.CellSize, speed: 12}
	default: // mixed
		switch h.rng.Intn(4) {
		case 0:
			return idlePattern{}
		case 1:
			return &walkPattern{speed: 10, dir: h.rng.Float64() * 2 * 3.14159265}
		case 2:
			return &randomPattern{rng: rand.New(rand.NewSource(h.cfg.Seed + int64(i))), speed: 8}
		default:
			return &clusterPattern{center: types.Vector3{X: h.cfg.WorldSize / 2, Z: h.cfg.WorldSize / 2}, speed: 6}
		}
	}
}

// Step injects position updates for moving entities and runs one tick.
// Returns the wall-clock duration of the tick (the runtime cost being measured).
func (h *Harness) Step(tickIndex int) time.Duration {
	if tickIndex%h.cfg.UpdateEvery == 0 {
		dt := h.cfg.TickRate.Seconds() * float64(h.cfg.UpdateEvery)
		for i, e := range h.entities {
			if h.patterns[i] == nil {
				continue
			}
			if _, ok := h.patterns[i].(idlePattern); ok {
				continue
			}
			h.patterns[i].Update(&e.Position, dt)
			upd := &v1.EntityUpdate{
				EntityId:  string(e.ID),
				Position:  &v1.Vector3{X: e.Position.X, Y: e.Position.Y, Z: e.Position.Z},
				Timestamp: time.Now().UnixMilli(),
			}
			payload, _ := proto.Marshal(upd)
			frame := protocol.Encode(protocol.PacketIDPositionUpdate, payload, false, 0)
			select {
			case h.g.Inbox <- game.InboundPacket{ClientID: string(e.ID), Data: frame}:
			default:
			}
		}
	}
	start := time.Now()
	h.g.Tick()
	return time.Since(start)
}

// Close stops the Events drainer. Always call after Run.
func (h *Harness) Close() {
	if h.done != nil {
		close(h.done)
		h.drainerWg.Wait()
		h.done = nil
	}
}

// Run executes the benchmark for the given number of ticks and returns Stats.
func (h *Harness) Run(ticks int) Stats {
	defer h.Close()
	hist := framework.NewHistogram()
	var memBefore, memAfter runtimeMemStats
	snapshotMem(&memBefore)

	for i := 0; i < ticks; i++ {
		d := h.Step(i)
		hist.Observe(float64(d.Nanoseconds()))
	}
	snapshotMem(&memAfter)

	allocTotal := uint64(0)
	if memAfter.TotalAlloc > memBefore.TotalAlloc {
		allocTotal = memAfter.TotalAlloc - memBefore.TotalAlloc
	}
	gcPause := time.Duration(memAfter.PauseTotalNs - memBefore.PauseTotalNs)
	gcCount := uint32(0)
	if memAfter.NumGC >= memBefore.NumGC {
		gcCount = memAfter.NumGC - memBefore.NumGC
	}

	return Stats{
		Config:        h.cfg,
		Ticks:         ticks,
		TickMean:      time.Duration(hist.Mean()),
		TickP50:       time.Duration(hist.Percentile(50)),
		TickP95:       time.Duration(hist.Percentile(95)),
		TickP99:       time.Duration(hist.Percentile(99)),
		TickMax:       time.Duration(hist.Max()),
		TickOverruns:  h.metrics.overruns,
		AllocPerTick:  allocTotal / uint64(ticks),
		HeapBytes:     memAfter.HeapAlloc,
		NumGC:         gcCount,
		GCPauseTotal:  gcPause,
		Goroutines:    goroutineCount(),
		Drops:         h.metrics.drops,
		RuntimeEvents: h.metrics.runtimeEvents,
	}
}
