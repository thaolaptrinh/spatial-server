//go:build validation

package observers

import (
	"context"
	"time"

	"github.com/thaolaptrinh/spatial-server/internal/validation"
)

type Metrics struct{}

func (o *Metrics) Name() string                     { return "metrics" }
func (o *Metrics) Policy() validation.ObserverPolicy { return validation.PolicyOptional }

func (o *Metrics) Observe(ctx context.Context, phase validation.ObservationPhase, infra validation.Infrastructure) ([]validation.Evidence, error) {
	return []validation.Evidence{
		{Timestamp: time.Now(), Phase: phase, Source: "metrics", Kind: "metric", Key: "observer", Value: "stub"},
	}, nil
}
