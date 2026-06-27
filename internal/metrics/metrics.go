package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Registry struct {
	ActiveConnections   prometheus.Gauge
	PacketsPerSec       *prometheus.CounterVec
	EntityCount         *prometheus.GaugeVec
	TickDurationSeconds prometheus.Histogram
	GRPCRequestDuration *prometheus.HistogramVec
	reg                 *prometheus.Registry
}

func NewRegistry() *Registry {
	r := prometheus.NewRegistry()
	m := &Registry{
		reg: r,
		ActiveConnections: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "spatial",
			Name:      "active_connections",
		}),
		PacketsPerSec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "spatial",
			Name:      "packets_per_sec_total",
		}, []string{"direction", "packet_id"}),
		EntityCount: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "spatial",
			Name:      "entity_count",
		}, []string{"type"}),
		TickDurationSeconds: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "spatial",
			Name:      "tick_duration_seconds",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1},
		}),
		GRPCRequestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "spatial",
			Name:      "grpc_request_duration_seconds",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.5},
		}, []string{"service", "method"}),
	}
	r.MustRegister(
		m.ActiveConnections,
		m.PacketsPerSec,
		m.EntityCount,
		m.TickDurationSeconds,
		m.GRPCRequestDuration,
	)
	return m
}

func (r *Registry) Handler() http.Handler {
	return promhttp.HandlerFor(r.reg, promhttp.HandlerOpts{})
}
