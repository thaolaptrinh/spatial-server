package game

import "time"

// Metrics is the observation surface the simulation reports to. The
// simulation depends only on this contract; concrete implementations
// bridging to Prometheus live in the service binaries (dependency
// inversion: the consumer owns the interface).
type Metrics interface {
	TickDuration(d time.Duration)
	TickOverrun()
	QueueDepth(queue string, depth int)
	Dropped(queue string, n int)
	EntityCount(typ string, n int)
	ActiveSpaces(n int)
	RuntimeEvent(kind string)
}

// NoopMetrics is a no-op Metrics implementation used when no metrics are wired.
type NoopMetrics struct{}

func (NoopMetrics) TickDuration(time.Duration) {}
func (NoopMetrics) TickOverrun()               {}
func (NoopMetrics) QueueDepth(string, int)     {}
func (NoopMetrics) Dropped(string, int)        {}
func (NoopMetrics) EntityCount(string, int)    {}
func (NoopMetrics) ActiveSpaces(int)           {}
func (NoopMetrics) RuntimeEvent(string)        {}

// eventKindLabel maps an EventKind to a stable metric label.
func eventKindLabel(k EventKind) string {
	switch k {
	case EventSpawn:
		return "spawn"
	case EventDespawn:
		return "despawn"
	case EventMove:
		return "move"
	case EventState:
		return "state"
	default:
		return "unknown"
	}
}

// WithMetrics wires a Metrics implementation into the Game.
func WithMetrics(m Metrics) Option {
	return func(g *Game) {
		if m != nil {
			g.metrics = m
		}
	}
}
