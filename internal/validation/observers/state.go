//go:build validation

package observers

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/thaolaptrinh/spatial-server/internal/validation"
)

type State struct{}

func (o *State) Name() string                     { return "state" }
func (o *State) Policy() validation.ObserverPolicy { return validation.PolicyRequired }

func (o *State) Observe(ctx context.Context, phase validation.ObservationPhase, infra validation.Infrastructure) ([]validation.Evidence, error) {
	now := time.Now()
	addr, err := infra.Endpoint(validation.ResourceRoomService)
	if err != nil {
		return nil, fmt.Errorf("state observer: %w", err)
	}
	return []validation.Evidence{
		{Timestamp: now, Phase: phase, Source: "state", Kind: "state", Key: validation.EvKeyRoomServiceAddr, Value: addr},
		{Timestamp: now, Phase: phase, Source: "state", Kind: "state", Key: validation.EvKeyRuntimeNodes, Value: strconv.Itoa(infra.RuntimeNodes())},
	}, nil
}
