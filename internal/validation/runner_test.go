package validation

import (
	"context"
	"testing"
	"time"
)

type testInjector struct {
	name     string
	injected bool
}

func (i *testInjector) Name() string                                          { return i.name }
func (i *testInjector) Inject(ctx context.Context, infra Infrastructure) error  { i.injected = true; return nil }
func (i *testInjector) Recover(ctx context.Context, infra Infrastructure) error { i.injected = false; return nil }

type testObserver struct {
	name   string
	fn     func(ObservationPhase) []Evidence
	policy ObserverPolicy
}

func (o *testObserver) Name() string          { return o.name }
func (o *testObserver) Policy() ObserverPolicy {
	if o.policy == "" {
		return PolicyRequired
	}
	return o.policy
}
func (o *testObserver) Observe(ctx context.Context, phase ObservationPhase, infra Infrastructure) ([]Evidence, error) {
	return o.fn(phase), nil
}

type testValidator struct {
	name string
	res  ValidationResult
}

func (v *testValidator) Name() string                                     { return v.name }
func (v *testValidator) Validate(baseline, postRecovery []Evidence) ValidationResult { return v.res }

type testRecovery struct{ delay time.Duration }

func (r *testRecovery) Wait(ctx context.Context, infra Infrastructure) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(r.delay):
		return nil
	}
}

func TestRunner_Pass(t *testing.T) {
	sc := ScenarioDefinition{
		Metadata: ScenarioMetadata{
			Name: "test", Mode: ModeProcess, Requirements: Requirements{MinRuntimeNodes: 1},
			Timeout: 2 * time.Second, ExpectedBehavior: "passes",
		},
		Setup:    func() (Infrastructure, func(), error) { return &stubInfra{nodes: 1}, func() {}, nil },
		Injector: &testInjector{name: "inj"},
		Recovery: &testRecovery{delay: time.Millisecond},
		Observers: []Observer{&testObserver{name: "obs", fn: func(p ObservationPhase) []Evidence {
			return []Evidence{{Timestamp: time.Now(), Phase: p, Source: "obs", Kind: "state", Key: "ok", Value: "true"}}
		}}},
		Validators: []Validator{&testValidator{
			name: "val", res: ValidationResult{Validator: "val", Status: StatusPass, Detail: "ok"},
		}},
	}
	report := NewRunner().Run(context.Background(), sc)
	if report.Outcome != OutcomePassed {
		t.Fatalf("expected passed, got %s: %s", report.Outcome, report.RootCause)
	}
	if len(report.Baseline) != 1 || len(report.PostRecovery) != 1 {
		t.Fatal("expected 1 baseline + 1 post-recovery evidence")
	}
}

func TestRunner_Skip(t *testing.T) {
	sc := ScenarioDefinition{
		Metadata: ScenarioMetadata{
			Name: "skip", Mode: ModeProcess, Requirements: Requirements{MinRuntimeNodes: 99}, Timeout: time.Second,
		},
		Setup:    func() (Infrastructure, func(), error) { return &stubInfra{nodes: 1}, func() {}, nil },
		Injector: &testInjector{name: "inj"},
		Recovery: &testRecovery{},
	}
	report := NewRunner().Run(context.Background(), sc)
	if report.Outcome != OutcomeSkipped {
		t.Fatalf("expected skipped, got %s", report.Outcome)
	}
}

func TestRunner_Fail(t *testing.T) {
	sc := ScenarioDefinition{
		Metadata: ScenarioMetadata{
			Name: "fail", Mode: ModeProcess, Requirements: Requirements{}, Timeout: 2 * time.Second,
		},
		Setup:    func() (Infrastructure, func(), error) { return &stubInfra{nodes: 1}, func() {}, nil },
		Injector: &testInjector{name: "inj"},
		Recovery: &testRecovery{delay: time.Millisecond},
		Validators: []Validator{&testValidator{
			name: "val", res: ValidationResult{Validator: "val", Status: StatusFail, Detail: "broken"},
		}},
	}
	report := NewRunner().Run(context.Background(), sc)
	if report.Outcome != OutcomeFailed {
		t.Fatalf("expected failed, got %s", report.Outcome)
	}
}

func TestRunner_Timeout(t *testing.T) {
	sc := ScenarioDefinition{
		Metadata: ScenarioMetadata{
			Name: "timeout", Mode: ModeProcess, Requirements: Requirements{}, Timeout: time.Millisecond,
		},
		Setup:    func() (Infrastructure, func(), error) { return &stubInfra{nodes: 1}, func() {}, nil },
		Injector: &testInjector{name: "inj"},
		Recovery: &testRecovery{delay: time.Second},
	}
	report := NewRunner().Run(context.Background(), sc)
	if report.Outcome != OutcomeTimedOut {
		t.Fatalf("expected timed_out, got %s", report.Outcome)
	}
}
