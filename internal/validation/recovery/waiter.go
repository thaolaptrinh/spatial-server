//go:build validation

package recovery

import (
	"context"
	"fmt"
	"time"

	"github.com/thaolaptrinh/spatial-server/internal/validation"
)

type CompositeMode string

const (
	ModeAll CompositeMode = "all"
	ModeAny CompositeMode = "any"
)

type CompositeWaiter struct {
	mode       CompositeMode
	conditions []validation.RecoveryCondition
	pollEvery  time.Duration
}

func NewComposite(mode CompositeMode, conditions ...validation.RecoveryCondition) *CompositeWaiter {
	return &CompositeWaiter{mode: mode, conditions: conditions, pollEvery: 500 * time.Millisecond}
}

func NewCompositeWithPoll(mode CompositeMode, pollEvery time.Duration, conditions ...validation.RecoveryCondition) *CompositeWaiter {
	return &CompositeWaiter{mode: mode, conditions: conditions, pollEvery: pollEvery}
}

func (w *CompositeWaiter) Wait(ctx context.Context, infra validation.Infrastructure) error {
	if len(w.conditions) == 0 {
		return nil
	}
	ticker := time.NewTicker(w.pollEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("recovery timed out: %w", ctx.Err())
		case <-ticker.C:
			met, unmet := w.check(infra)
			switch w.mode {
			case ModeAll:
				if len(unmet) == 0 && len(met) > 0 {
					return nil
				}
			case ModeAny:
				if len(met) > 0 {
					return nil
				}
			}
		}
	}
}

func (w *CompositeWaiter) check(infra validation.Infrastructure) (met, unmet []string) {
	for _, c := range w.conditions {
		ok, _ := c.Met(context.Background(), infra)
		if ok {
			met = append(met, c.Name())
		} else {
			unmet = append(unmet, c.Name())
		}
	}
	return
}
