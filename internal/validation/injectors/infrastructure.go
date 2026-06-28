//go:build validation

package injectors

import (
	"context"
	"fmt"

	"github.com/thaolaptrinh/spatial-server/internal/validation"
)

type InfraRestart struct {
	Target validation.ResourceID
}

func (i *InfraRestart) Name() string { return fmt.Sprintf("infra-restart(%s)", i.Target) }

func (i *InfraRestart) Inject(ctx context.Context, infra validation.Infrastructure) error {
	return fmt.Errorf("infra-restart requires Docker Compose execution mode")
}

func (i *InfraRestart) Recover(ctx context.Context, infra validation.Infrastructure) error { return nil }
