//go:build validation

package injectors

import (
	"context"
	"fmt"

	"github.com/thaolaptrinh/spatial-server/internal/validation"
)

type NetLatency struct {
	Target   validation.ResourceID
	DelayMS  int
	JitterMS int
}

func (i *NetLatency) Name() string {
	return fmt.Sprintf("net-latency(%s, %dms)", i.Target, i.DelayMS)
}

func (i *NetLatency) Inject(ctx context.Context, infra validation.Infrastructure) error {
	return fmt.Errorf("net-latency requires Docker Compose execution mode")
}

func (i *NetLatency) Recover(ctx context.Context, infra validation.Infrastructure) error { return nil }

type NetLoss struct {
	Target  validation.ResourceID
	Percent int
}

func (i *NetLoss) Name() string {
	return fmt.Sprintf("net-loss(%s, %d%%)", i.Target, i.Percent)
}

func (i *NetLoss) Inject(ctx context.Context, infra validation.Infrastructure) error {
	return fmt.Errorf("net-loss requires Docker Compose execution mode")
}

func (i *NetLoss) Recover(ctx context.Context, infra validation.Infrastructure) error { return nil }

type NetPartition struct{}

func (i *NetPartition) Name() string { return "net-partition" }

func (i *NetPartition) Inject(ctx context.Context, infra validation.Infrastructure) error {
	return fmt.Errorf("net-partition requires Docker Compose execution mode")
}

func (i *NetPartition) Recover(ctx context.Context, infra validation.Infrastructure) error { return nil }
