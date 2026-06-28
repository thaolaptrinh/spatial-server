//go:build validation

package observers

import (
	"context"
	"fmt"
	"time"

	"github.com/thaolaptrinh/spatial-server/internal/validation"
)

type Routing struct{}

func (o *Routing) Name() string                     { return "routing" }
func (o *Routing) Policy() validation.ObserverPolicy { return validation.PolicyOptional }

func (o *Routing) Observe(ctx context.Context, phase validation.ObservationPhase, infra validation.Infrastructure) ([]validation.Evidence, error) {
	now := time.Now()
	addr, err := infra.Endpoint(validation.ResourceGateway)
	if err != nil {
		return nil, fmt.Errorf("routing observer: %w", err)
	}
	return []validation.Evidence{
		{Timestamp: now, Phase: phase, Source: "routing", Kind: "state", Key: validation.EvKeyGatewayAddr, Value: addr},
	}, nil
}
