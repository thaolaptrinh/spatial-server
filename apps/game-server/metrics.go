package main

import (
	"time"

	"github.com/thaolaptrinh/spatial-server/internal/game"
	"github.com/thaolaptrinh/spatial-server/internal/metrics"
)

// gameMetricsAdapter bridges the simulation's game.Metrics contract to the
// Prometheus registry. This is the only place that maps simulation
// observations to Prometheus instruments, keeping internal/game free of any
// metrics-library dependency.
type gameMetricsAdapter struct {
	reg *metrics.Registry
}

func (a gameMetricsAdapter) TickDuration(d time.Duration) {
	a.reg.TickDurationSeconds.Observe(d.Seconds())
}

func (a gameMetricsAdapter) TickOverrun() { a.reg.TickOverruns.Inc() }

func (a gameMetricsAdapter) QueueDepth(queue string, depth int) {
	a.reg.QueueDepth.WithLabelValues(queue).Set(float64(depth))
}

func (a gameMetricsAdapter) Dropped(queue string, n int) {
	a.reg.DroppedTotal.WithLabelValues(queue).Add(float64(n))
}

func (a gameMetricsAdapter) EntityCount(typ string, n int) {
	a.reg.EntityCount.WithLabelValues(typ).Set(float64(n))
}

func (a gameMetricsAdapter) ActiveSpaces(n int) { a.reg.ActiveSpaces.Set(float64(n)) }

func (a gameMetricsAdapter) RuntimeEvent(kind string) {
	a.reg.RuntimeEvents.WithLabelValues(kind).Inc()
}

var _ game.Metrics = gameMetricsAdapter{}
