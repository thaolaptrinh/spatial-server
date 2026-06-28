package validation

import (
	"context"
	"fmt"
	"time"
)

type ScenarioRunner struct{}

func NewRunner() *ScenarioRunner { return &ScenarioRunner{} }

func ValidateScenarios(scenarios []ScenarioDefinition) error {
	seen := make(map[string]int)
	for i, sc := range scenarios {
		id := sc.Metadata.ID
		if id == "" {
			return fmt.Errorf("scenario at index %d has empty ID", i)
		}
		if prev, ok := seen[id]; ok {
			return fmt.Errorf("duplicate scenario ID %q at indices %d and %d", id, prev, i)
		}
		seen[id] = i
	}
	return nil
}

func (r *ScenarioRunner) Run(ctx context.Context, sc ScenarioDefinition) *ValidationReport {
	report := &ValidationReport{
		Framework: FrameworkMeta{
			FrameworkVersion: frameworkVersion,
			ScenarioVersion:  fmt.Sprintf("%d", sc.Metadata.Version),
			ExecTimestamp:    time.Now(),
		},
		Scenario: sc.Metadata,
	}
	start := time.Now()
	defer func() { report.Duration = time.Since(start) }()

	defer func() {
		if rec := recover(); rec != nil {
			report.Outcome = OutcomeError
			if report.RootCause == "" {
				report.RootCause = fmt.Sprintf("panic: %v", rec)
			}
		}
	}()

	scopedCtx := ctx
	if sc.Metadata.Timeout > 0 {
		var cancel context.CancelFunc
		scopedCtx, cancel = context.WithTimeout(ctx, sc.Metadata.Timeout)
		defer cancel()
	}

	infra, teardown, err := sc.Setup()
	if err != nil {
		report.Outcome = OutcomeError
		report.RootCause = fmt.Sprintf("setup: %v", err)
		return report
	}
	defer teardown()

	met, reason := CheckRequirements(infra, sc.Metadata.Requirements)
	if !met {
		report.Outcome = OutcomeSkipped
		report.RootCause = reason
		return report
	}

	baseline, err := observeAll(scopedCtx, PhaseBaseline, sc.Observers, infra)
	if err != nil {
		report.Outcome = OutcomeError
		report.RootCause = fmt.Sprintf("baseline observe: %v", err)
		return report
	}
	report.Baseline = baseline

	defer func() { _ = sc.Injector.Recover(context.Background(), infra) }()
	if err := sc.Injector.Inject(scopedCtx, infra); err != nil {
		report.Outcome = OutcomeError
		report.RootCause = fmt.Sprintf("inject: %v", err)
		return report
	}

	recoverStart := time.Now()
	recoveryErr := sc.Recovery.Wait(scopedCtx, infra)
	report.Measurement.Recovery.Duration = time.Since(recoverStart)

	if recoveryErr != nil {
		if scopedCtx.Err() == context.DeadlineExceeded {
			report.Outcome = OutcomeTimedOut
			report.RootCause = "recovery timed out"
		} else {
			report.Outcome = OutcomeError
			report.RootCause = fmt.Sprintf("recovery: %v", recoveryErr)
		}
	}

	postRecovery, err := observeAll(scopedCtx, PhasePostRecovery, sc.Observers, infra)
	if err != nil {
		report.Outcome = OutcomeError
		report.RootCause = fmt.Sprintf("post-recovery observe: %v", err)
		return report
	}
	report.PostRecovery = postRecovery

	allPassed := true
	for _, v := range sc.Validators {
		result := v.Validate(baseline, postRecovery)
		report.Validations = append(report.Validations, result)
		if result.Status == StatusFail {
			allPassed = false
			if report.RootCause == "" {
				report.RootCause = fmt.Sprintf("%s: %s", result.Validator, result.Detail)
			}
		}
	}

	if report.Outcome == "" {
		if allPassed {
			report.Outcome = OutcomePassed
		} else {
			report.Outcome = OutcomeFailed
		}
	}

	evaluateAcceptance(report, sc.Acceptance)

	if sc.Reporter != nil {
		_ = sc.Reporter.Generate(report)
	}

	return report
}

func evaluateAcceptance(report *ValidationReport, ac AcceptanceCriteria) {
	check := func(name string, actual, threshold time.Duration, policy AcceptancePolicy) {
		if threshold == 0 {
			return
		}
		status := StatusPass
		if actual > threshold {
			if policy == AcceptFail {
				status = StatusFail
				report.Outcome = OutcomeFailed
			} else {
				status = StatusWarn
			}
		}
		report.Validations = append(report.Validations, ValidationResult{
			Validator: "acceptance/" + name,
			Status:    status,
			Detail:    fmt.Sprintf("%s actual=%v threshold=%v policy=%s", name, actual, threshold, policy),
		})
	}
	check("recovery-duration", report.Measurement.Recovery.Duration, ac.RecoveryDuration.Threshold, ac.RecoveryDuration.Policy)
	check("tick-p95", report.Measurement.Tick.P95, ac.TickP95.Threshold, ac.TickP95.Policy)
}

func observeAll(ctx context.Context, phase ObservationPhase, observers []Observer, infra Infrastructure) ([]Evidence, error) {
	var all []Evidence
	for _, o := range observers {
		e, err := o.Observe(ctx, phase, infra)
		if err != nil {
			if o.Policy() == PolicyRequired {
				return nil, fmt.Errorf("%s(%s): %w", o.Name(), phase, err)
			}
			all = append(all, Evidence{
				Timestamp: time.Now(), Phase: phase, Source: o.Name(),
				Kind: "error", Key: "observer-error", Value: err.Error(),
			})
			continue
		}
		all = append(all, e...)
	}
	return all, nil
}
