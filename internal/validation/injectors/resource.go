//go:build validation

package injectors

import (
	"context"
	"fmt"

	"github.com/thaolaptrinh/spatial-server/internal/validation"
)

type ResourceCPU struct {
	Target   validation.ResourceID
	Cores    int
	Duration string
}

func (i *ResourceCPU) Name() string {
	return fmt.Sprintf("resource-cpu(%s, %d cores)", i.Target, i.Cores)
}

func (i *ResourceCPU) Inject(ctx context.Context, infra validation.Infrastructure) error {
	return fmt.Errorf("resource-cpu requires Docker Compose execution mode")
}

func (i *ResourceCPU) Recover(ctx context.Context, infra validation.Infrastructure) error { return nil }

type ResourceMemory struct {
	Target   validation.ResourceID
	LimitMB  int
	Duration string
}

func (i *ResourceMemory) Name() string {
	return fmt.Sprintf("resource-memory(%s, %dMB)", i.Target, i.LimitMB)
}

func (i *ResourceMemory) Inject(ctx context.Context, infra validation.Infrastructure) error {
	return fmt.Errorf("resource-memory requires Docker Compose execution mode")
}

func (i *ResourceMemory) Recover(ctx context.Context, infra validation.Infrastructure) error { return nil }
