//go:build validation

package injectors

import (
	"context"
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/thaolaptrinh/spatial-server/internal/validation"
)

type ProcessCrash struct {
	Target validation.ResourceID
}

func (i *ProcessCrash) Name() string { return fmt.Sprintf("process-crash(%s)", i.Target) }

func (i *ProcessCrash) Inject(ctx context.Context, infra validation.Infrastructure) error {
	procs, err := infra.Processes(i.Target)
	if err != nil {
		return fmt.Errorf("process-crash inject: %w", err)
	}
	for _, p := range procs {
		if err := p.Signal(syscall.SIGKILL); err != nil {
			return fmt.Errorf("process-crash inject SIGKILL: %w", err)
		}
	}
	return nil
}

func (i *ProcessCrash) Recover(ctx context.Context, infra validation.Infrastructure) error { return nil }

type ProcessFreeze struct {
	Target   validation.ResourceID
	Duration time.Duration
	frozen   []*os.Process
}

func NewProcessFreeze(target validation.ResourceID, dur time.Duration) *ProcessFreeze {
	return &ProcessFreeze{Target: target, Duration: dur}
}

func (i *ProcessFreeze) Name() string {
	return fmt.Sprintf("process-freeze(%s, %v)", i.Target, i.Duration)
}

func (i *ProcessFreeze) Inject(ctx context.Context, infra validation.Infrastructure) error {
	procs, err := infra.Processes(i.Target)
	if err != nil {
		return fmt.Errorf("process-freeze inject: %w", err)
	}
	i.frozen = procs
	for _, p := range procs {
		if err := p.Signal(syscall.SIGSTOP); err != nil {
			return fmt.Errorf("process-freeze inject SIGSTOP: %w", err)
		}
	}
	return nil
}

func (i *ProcessFreeze) Recover(ctx context.Context, infra validation.Infrastructure) error {
	for _, p := range i.frozen {
		if err := p.Signal(syscall.SIGCONT); err != nil {
			return fmt.Errorf("process-freeze recover SIGCONT: %w", err)
		}
	}
	return nil
}
