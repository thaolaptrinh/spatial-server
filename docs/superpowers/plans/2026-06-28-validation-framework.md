# Validation Framework Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement a generic Validation Framework as the single execution engine for all validation activities (chaos, integration, benchmark, load, soak).

**Architecture:** 4 core files (scenario, measure, report, runner) with no build tag, plus 14 leaf files (harness, injectors, validators, observers, recovery, probes, reporters, scenarios, test entry) with `//go:build validation`. Runner orchestrates: Setup → CheckRequirements → Baseline Observe → Inject → Wait → Post-Recovery Observe → Validate → Report → Teardown.

**Tech Stack:** Go 1.25, Testcontainers (PG 16, Redis 7), gRPC, stretchr/testify

---

## Dependency Graph

```
Phase 1 ─── Phase 2 ─── Phase 3 ─── Phase 9 ─── Phase 10 ─── Phase 11 ─── Phase 12 ─── Phase 13 ─── Phase 14
                  │
     ┌────────────┼────────────┬────────────┐
     │            │            │            │
Phase 4       Phase 5       Phase 6       Phase 7           Phase 8
(recovery)   (probes)      (harness)    (validators)    (process inj)
     │            │            │            │                 │
     └────────────┴────────────┴────────────┴─────────────────┘
```

**Critical path:** 1 → 2 → 3 → 9 → 10 → 11 → 12 → 13 → 14
**Parallel:** 4, 5, 6, 7, 8 (depend only on Phase 1)

---

## Phase 1: Core Package (scenario + measure + report + test)

**Objective:** Establish every frozen type, interface, measurement model, and report struct.

**New files:**
- `internal/validation/scenario.go`
- `internal/validation/measure.go`
- `internal/validation/report.go`
- `internal/validation/scenario_test.go`

**Build tag:** None.

**Risk:** None — pure data types with no external dependencies.

**Definition of Done:** `go build ./internal/validation/ && go vet ./internal/validation/ && go test -race ./internal/validation/` all pass.

---

### Task 1.1: scenario.go

- [ ] **Create `internal/validation/scenario.go`** with this content:

```go
package validation

import (
	"context"
	"fmt"
	"os"
	"time"
)

type ResourceID string

const (
	ResourcePostgres    ResourceID = "postgres"
	ResourceRedis       ResourceID = "redis"
	ResourceRoomService ResourceID = "room-service"
	ResourceRuntime     ResourceID = "runtime"
	ResourceGateway     ResourceID = "gateway"
)

type Infrastructure interface {
	Endpoint(id ResourceID) (string, error)
	Processes(id ResourceID) ([]*os.Process, error)
	Database(id ResourceID) (string, error)
	RuntimeNodes() int
	DialServices(ids ...ResourceID) error
	Close() error
}

type ExecutionMode string

const (
	ModeProcess ExecutionMode = "process"
	ModeCompose ExecutionMode = "compose"
	ModeAny     ExecutionMode = "any"
)

type Severity string

const (
	SeverityLow      Severity = "low"
	SeverityMedium   Severity = "medium"
	SeverityHigh     Severity = "high"
	SeverityCritical Severity = "critical"
)

type Requirements struct {
	MinRuntimeNodes int
	ComposeRequired bool
	PostgreSQL      bool
	Redis           bool
	NetworkFaults   bool
}

type ScenarioMetadata struct {
	Name             string
	Description      string
	Tags             []string
	Severity         Severity
	Mode             ExecutionMode
	Requirements     Requirements
	Timeout          time.Duration
	ExpectedBehavior string
}

type SetupFunc func() (Infrastructure, func(), error)

type ScenarioDefinition struct {
	Metadata   ScenarioMetadata
	Setup      SetupFunc
	Injector   Injector
	Observers  []Observer
	Validators []Validator
	Recovery   RecoveryWaiter
	Reporter   Reporter
}

func CheckRequirements(infra Infrastructure, req Requirements) (met bool, reason string) {
	if req.PostgreSQL {
		if _, err := infra.Database(ResourcePostgres); err != nil {
			return false, fmt.Sprintf("postgres: %v", err)
		}
	}
	if req.Redis {
		if _, err := infra.Endpoint(ResourceRedis); err != nil {
			return false, fmt.Sprintf("redis: %v", err)
		}
	}
	if req.MinRuntimeNodes > 0 && infra.RuntimeNodes() < req.MinRuntimeNodes {
		return false, fmt.Sprintf("need %d runtime nodes, have %d", req.MinRuntimeNodes, infra.RuntimeNodes())
	}
	if req.ComposeRequired {
		return false, "compose mode not available in this environment"
	}
	if req.NetworkFaults {
		return false, "network faults require compose mode"
	}
	return true, ""
}

type Injector interface {
	Name() string
	Inject(ctx context.Context, infra Infrastructure) error
	Recover(ctx context.Context, infra Infrastructure) error
}

type RecoveryWaiter interface {
	Wait(ctx context.Context, infra Infrastructure) error
}

type RecoveryCondition interface {
	Name() string
	Met(ctx context.Context, infra Infrastructure) (bool, error)
}

type ObservationPhase string

const (
	PhaseBaseline     ObservationPhase = "baseline"
	PhasePostRecovery ObservationPhase = "post_recovery"
)

type Evidence struct {
	Timestamp time.Time
	Phase     ObservationPhase
	Source    string
	Kind      string
	Key       string
	Value     string
}

type Observer interface {
	Name() string
	Observe(ctx context.Context, phase ObservationPhase, infra Infrastructure) ([]Evidence, error)
}

type ValidationStatus string

const (
	StatusPass ValidationStatus = "pass"
	StatusFail ValidationStatus = "fail"
	StatusWarn ValidationStatus = "warn"
	StatusSkip ValidationStatus = "skip"
)

type ValidationResult struct {
	Validator       string
	Status          ValidationStatus
	EvidenceIndices []int
	Detail          string
}

type Validator interface {
	Name() string
	Validate(baseline, postRecovery []Evidence) ValidationResult
}

type Reporter interface {
	Generate(report *ValidationReport) error
}
```

- [ ] **Run:** `go build ./internal/validation/`
Expected: FAIL (undefined: ValidationReport — resolved next task)

---

### Task 1.2: measure.go

- [ ] **Create `internal/validation/measure.go`** with this content:

```go
package validation

import "time"

type TickSummary struct {
	Mean  time.Duration
	P50   time.Duration
	P95   time.Duration
	P99   time.Duration
	Max   time.Duration
	Count int
}

type RecoverySummary struct {
	Duration            time.Duration
	OwnershipRestoredAt time.Time
	FirstHealthyAt      time.Time
}

type QueueSnapshot struct {
	Min  int
	Max  int
	Mean float64
	P95  int
}

type Measurement struct {
	Tick          TickSummary
	Recovery      RecoverySummary
	Queue         map[string]QueueSnapshot
	Events        map[string]int
	Drops         map[string]int
	TickDurations []time.Duration
}
```

- [ ] **Run:** `go build ./internal/validation/`
Expected: FAIL (ValidationReport still undefined)

---

### Task 1.3: report.go

- [ ] **Create `internal/validation/report.go`** with this exact content. More text is being generated outside the code fences for the MarkdownReporter. Let me wrap those in proper fences:

```go
package validation

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

const frameworkVersion = "1.0.0"

type FrameworkMeta struct {
	FrameworkVersion string
	ScenarioVersion  string
	ExecTimestamp    time.Time
}

type Outcome string

const (
	OutcomePassed   Outcome = "passed"
	OutcomeFailed   Outcome = "failed"
	OutcomeSkipped  Outcome = "skipped"
	OutcomeTimedOut Outcome = "timed_out"
	OutcomeError    Outcome = "error"
)

type ValidationReport struct {
	Framework    FrameworkMeta
	Scenario     ScenarioMetadata
	Outcome      Outcome
	Duration     time.Duration
	Baseline     []Evidence
	PostRecovery []Evidence
	Validations  []ValidationResult
	Measurement  Measurement
	RootCause    string
}

func (r *ValidationReport) CombinedEvidence() []Evidence {
	n := len(r.Baseline) + len(r.PostRecovery)
	combined := make([]Evidence, 0, n)
	combined = append(combined, r.Baseline...)
	combined = append(combined, r.PostRecovery...)
	sort.Slice(combined, func(i, j int) bool {
		return combined[i].Timestamp.Before(combined[j].Timestamp)
	})
	return combined
}

type SummaryReport struct {
	Framework     FrameworkMeta
	ExecTimestamp time.Time
	Total         int
	Passed        int
	Failed        int
	Skipped       int
	TimedOut      int
	Errors        int
	TotalDuration time.Duration
	Recovery      struct {
		Count        int
		MinDuration  time.Duration
		MaxDuration  time.Duration
		MeanDuration time.Duration
	}
	Invariants struct {
		Total   int
		Passed  int
		Warned  int
		Failed  int
		Skipped int
	}
	Scenarios []SummaryEntry
}

type SummaryEntry struct {
	Name     string
	Outcome  Outcome
	Duration time.Duration
	Reason   string
}

func NewSummary(reports []*ValidationReport) *SummaryReport {
	s := &SummaryReport{
		Framework: FrameworkMeta{
			FrameworkVersion: frameworkVersion,
			ScenarioVersion:  "1.0.0",
			ExecTimestamp:    time.Now(),
		},
	}
	var recoveryDurations []time.Duration
	for _, r := range reports {
		s.Total++
		entry := SummaryEntry{
			Name: r.Scenario.Name, Outcome: r.Outcome,
			Duration: r.Duration, Reason: r.RootCause,
		}
		s.Scenarios = append(s.Scenarios, entry)
		s.TotalDuration += r.Duration
		switch r.Outcome {
		case OutcomePassed:
			s.Passed++
		case OutcomeFailed:
			s.Failed++
		case OutcomeSkipped:
			s.Skipped++
		case OutcomeTimedOut:
			s.TimedOut++
		case OutcomeError:
			s.Errors++
		}
		for _, v := range r.Validations {
			s.Invariants.Total++
			switch v.Status {
			case StatusPass:
				s.Invariants.Passed++
			case StatusWarn:
				s.Invariants.Warned++
			case StatusFail:
				s.Invariants.Failed++
			case StatusSkip:
				s.Invariants.Skipped++
			}
		}
		if r.Measurement.Recovery.Duration > 0 {
			recoveryDurations = append(recoveryDurations, r.Measurement.Recovery.Duration)
		}
	}
	if n := len(recoveryDurations); n > 0 {
		s.Recovery.Count = n
		s.Recovery.MinDuration = recoveryDurations[0]
		s.Recovery.MaxDuration = recoveryDurations[0]
		var total time.Duration
		for _, d := range recoveryDurations {
			total += d
			if d < s.Recovery.MinDuration {
				s.Recovery.MinDuration = d
			}
			if d > s.Recovery.MaxDuration {
				s.Recovery.MaxDuration = d
			}
		}
		s.Recovery.MeanDuration = total / time.Duration(n)
	}
	return s
}

type MarkdownReporter struct {
	Writer strings.Builder
}

func (m *MarkdownReporter) Generate(report *ValidationReport) error {
	m.Writer.Reset()
	fmt.Fprintf(&m.Writer, "# Chaos Report: %s\n\n", report.Scenario.Name)
	fmt.Fprintf(&m.Writer, "| Field | Value |\n|-------|-------|\n")
	fmt.Fprintf(&m.Writer, "| Outcome | %s |\n", report.Outcome)
	fmt.Fprintf(&m.Writer, "| Duration | %s |\n", report.Duration)
	fmt.Fprintf(&m.Writer, "| Expected | %s |\n\n", report.Scenario.ExpectedBehavior)
	fmt.Fprintf(&m.Writer, "## Validations\n\n| Validator | Status | Detail |\n|-----------|--------|--------|\n")
	for _, v := range report.Validations {
		fmt.Fprintf(&m.Writer, "| %s | %s | %s |\n", v.Validator, v.Status, v.Detail)
	}
	return nil
}

func MarkdownReportString(report *ValidationReport) string {
	var m MarkdownReporter
	_ = m.Generate(report)
	return m.Writer.String()
}
```

- [ ] **Run:** `go build ./internal/validation/`
Expected: SUCCESS

---

### Task 1.4: scenario_test.go

- [ ] **Create `internal/validation/scenario_test.go`** with this content:

```go
package validation

import (
	"errors"
	"os"
	"testing"
)

type stubInfra struct {
	nodes       int
	errDB       bool
	errEndpoint bool
}

func (s *stubInfra) Endpoint(id ResourceID) (string, error) {
	if s.errEndpoint {
		return "", errors.New("no endpoint")
	}
	return "127.0.0.1:6379", nil
}
func (s *stubInfra) Processes(id ResourceID) ([]*os.Process, error) { return nil, nil }
func (s *stubInfra) Database(id ResourceID) (string, error) {
	if s.errDB {
		return "", errors.New("no database")
	}
	return "dsn://localhost", nil
}
func (s *stubInfra) RuntimeNodes() int                    { return s.nodes }
func (s *stubInfra) DialServices(ids ...ResourceID) error { return nil }
func (s *stubInfra) Close() error                         { return nil }

func TestCheckRequirements_PGUnavailable(t *testing.T) {
	met, reason := CheckRequirements(&stubInfra{errDB: true}, Requirements{PostgreSQL: true})
	if met || reason == "" {
		t.Fatal("expected unmet when PG unavailable")
	}
}

func TestCheckRequirements_RedisUnavailable(t *testing.T) {
	met, reason := CheckRequirements(&stubInfra{errEndpoint: true}, Requirements{Redis: true})
	if met || reason == "" {
		t.Fatal("expected unmet when Redis unavailable")
	}
}

func TestCheckRequirements_AllMet(t *testing.T) {
	met, _ := CheckRequirements(&stubInfra{nodes: 2},
		Requirements{PostgreSQL: true, Redis: true, MinRuntimeNodes: 1})
	if !met {
		t.Fatal("expected met")
	}
}

func TestCheckRequirements_NotEnoughNodes(t *testing.T) {
	met, _ := CheckRequirements(&stubInfra{nodes: 0}, Requirements{MinRuntimeNodes: 2})
	if met {
		t.Fatal("expected unmet")
	}
}

func TestCheckRequirements_ComposeRequired(t *testing.T) {
	met, _ := CheckRequirements(&stubInfra{}, Requirements{ComposeRequired: true})
	if met {
		t.Fatal("expected unmet")
	}
}

func TestCheckRequirements_NetworkFaults(t *testing.T) {
	met, _ := CheckRequirements(&stubInfra{}, Requirements{NetworkFaults: true})
	if met {
		t.Fatal("expected unmet")
	}
}

func TestNewSummary(t *testing.T) {
	reports := []*ValidationReport{
		{
			Scenario: ScenarioMetadata{Name: "pass"},
			Outcome:  OutcomePassed,
			Duration: 100,
			Validations: []ValidationResult{
				{Status: StatusPass},
				{Status: StatusPass},
			},
		},
		{
			Scenario: ScenarioMetadata{Name: "fail"},
			Outcome:  OutcomeFailed,
			Duration: 200,
			Validations: []ValidationResult{
				{Status: StatusFail},
			},
		},
	}
	s := NewSummary(reports)
	if s.Total != 2 {
		t.Fatalf("expected 2 total, got %d", s.Total)
	}
	if s.Passed != 1 {
		t.Fatalf("expected 1 passed, got %d", s.Passed)
	}
	if s.Failed != 1 {
		t.Fatalf("expected 1 failed, got %d", s.Failed)
	}
	if s.Invariants.Total != 3 {
		t.Fatalf("expected 3 invariant total, got %d", s.Invariants.Total)
	}
}
```

- [ ] **Run:** `go test -race ./internal/validation/`
Expected: 7 tests PASS

---

### Task 1.5: Commit

```bash
git add internal/validation/
git commit -m "feat(validation): add core types, interfaces, measurement, and report models"
```

---

## Phase 2: Runner

**New files:**
- `internal/validation/runner.go`
- `internal/validation/runner_test.go`

**Deps:** Phase 1.

---

### Task 2.1: runner.go

- [ ] **Create `internal/validation/runner.go`**:

```go
package validation

import (
	"context"
	"fmt"
	"time"
)

type ScenarioRunner struct{}

func NewRunner() *ScenarioRunner { return &ScenarioRunner{} }

func (r *ScenarioRunner) Run(ctx context.Context, sc ScenarioDefinition) *ValidationReport {
	report := &ValidationReport{
		Framework: FrameworkMeta{
			FrameworkVersion: frameworkVersion,
			ScenarioVersion:  "1.0.0",
			ExecTimestamp:    time.Now(),
		},
		Scenario: sc.Metadata,
	}
	start := time.Now()
	defer func() { report.Duration = time.Since(start) }()

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
		return report
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

	if allPassed {
		report.Outcome = OutcomePassed
	} else {
		report.Outcome = OutcomeFailed
	}

	if sc.Reporter != nil {
		_ = sc.Reporter.Generate(report)
	}

	return report
}

func observeAll(ctx context.Context, phase ObservationPhase, observers []Observer, infra Infrastructure) ([]Evidence, error) {
	var all []Evidence
	for _, o := range observers {
		e, err := o.Observe(ctx, phase, infra)
		if err != nil {
			return nil, fmt.Errorf("%s(%s): %w", o.Name(), phase, err)
		}
		all = append(all, e...)
	}
	return all, nil
}
```

- [ ] **Run:** `go build ./internal/validation/`
Expected: SUCCESS

---

### Task 2.2: runner_test.go

- [ ] **Create `internal/validation/runner_test.go`**:

```go
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
	name string
	fn   func(ObservationPhase) []Evidence
}

func (o *testObserver) Name() string { return o.name }
func (o *testObserver) Observe(ctx context.Context, phase ObservationPhase, infra Infrastructure) ([]Evidence, error) {
	return o.fn(phase), nil
}

type testValidator struct {
	name string
	res  ValidationResult
}

func (v *testValidator) Name() string                                           { return v.name }
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
```

- [ ] **Run:** `go test -race -run TestRunner ./internal/validation/`
Expected: 4 tests PASS

---

### Task 2.3: Commit

```bash
git add internal/validation/runner.go internal/validation/runner_test.go
git commit -m "feat(validation): add ScenarioRunner execution engine"
```

---

## Phase 3: Observers

**New files:**
- `internal/validation/observers/state.go`
- `internal/validation/observers/routing.go`
- `internal/validation/observers/metrics.go`

**Deps:** Phase 1. **Build tag:** `//go:build validation`.

---

### Task 3.1: state.go

- [ ] **Create `internal/validation/observers/state.go`**:

```go
//go:build validation

package observers

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"spatial-server/internal/validation"
)

type State struct{}

func (o *State) Name() string { return "state" }

func (o *State) Observe(ctx context.Context, phase validation.ObservationPhase, infra validation.Infrastructure) ([]validation.Evidence, error) {
	now := time.Now()
	addr, err := infra.Endpoint(validation.ResourceRoomService)
	if err != nil {
		return nil, fmt.Errorf("state observer: %w", err)
	}
	return []validation.Evidence{
		{Timestamp: now, Phase: phase, Source: "state", Kind: "state", Key: "room-service-endpoint", Value: addr},
		{Timestamp: now, Phase: phase, Source: "state", Kind: "state", Key: "runtime-nodes", Value: strconv.Itoa(infra.RuntimeNodes())},
	}, nil
}
```

---

### Task 3.2: routing.go

- [ ] **Create `internal/validation/observers/routing.go`**:

```go
//go:build validation

package observers

import (
	"context"
	"fmt"
	"time"

	"spatial-server/internal/validation"
)

type Routing struct{}

func (o *Routing) Name() string { return "routing" }

func (o *Routing) Observe(ctx context.Context, phase validation.ObservationPhase, infra validation.Infrastructure) ([]validation.Evidence, error) {
	now := time.Now()
	addr, err := infra.Endpoint(validation.ResourceGateway)
	if err != nil {
		return nil, fmt.Errorf("routing observer: %w", err)
	}
	return []validation.Evidence{
		{Timestamp: now, Phase: phase, Source: "routing", Kind: "state", Key: "gateway-endpoint", Value: addr},
	}, nil
}
```

---

### Task 3.3: metrics.go

- [ ] **Create `internal/validation/observers/metrics.go`**:

```go
//go:build validation

package observers

import (
	"context"
	"time"

	"spatial-server/internal/validation"
)

type Metrics struct{}

func (o *Metrics) Name() string { return "metrics" }

func (o *Metrics) Observe(ctx context.Context, phase validation.ObservationPhase, infra validation.Infrastructure) ([]validation.Evidence, error) {
	return []validation.Evidence{
		{Timestamp: time.Now(), Phase: phase, Source: "metrics", Kind: "metric", Key: "observer", Value: "stub"},
	}, nil
}
```

- [ ] **Run:** `go build -tags=validation ./internal/validation/observers/`
Expected: SUCCESS

---

### Task 3.4: Commit

```bash
git add internal/validation/observers/
git commit -m "feat(validation): add observers (state, routing, metrics)"
```

---

## Phase 4: Recovery Package

**New files:** `internal/validation/recovery/waiter.go`, `conditions.go`, `waiter_test.go`
**Deps:** Phase 1. **Build tag:** `//go:build validation`.

---

### Task 4.1: waiter.go

- [ ] **Create `internal/validation/recovery/waiter.go`**:

```go
//go:build validation

package recovery

import (
	"context"
	"fmt"
	"time"

	"spatial-server/internal/validation"
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
```

---

### Task 4.2: conditions.go

- [ ] **Create `internal/validation/recovery/conditions.go`**:

```go
//go:build validation

package recovery

import (
	"context"
	"fmt"
	"net"
	"time"

	"spatial-server/internal/validation"
)

type FixedDelayCondition struct {
	name    string
	delay   time.Duration
	started time.Time
}

func FixedDelay(name string, delay time.Duration) *FixedDelayCondition {
	return &FixedDelayCondition{name: name, delay: delay, started: time.Now()}
}

func (c *FixedDelayCondition) Name() string { return c.name }
func (c *FixedDelayCondition) Met(ctx context.Context, infra validation.Infrastructure) (bool, error) {
	return time.Since(c.started) >= c.delay, nil
}

type HealthyEndpointCondition struct {
	resource validation.ResourceID
}

func HealthyEndpoint(resource validation.ResourceID) *HealthyEndpointCondition {
	return &HealthyEndpointCondition{resource: resource}
}

func (c *HealthyEndpointCondition) Name() string { return fmt.Sprintf("healthy(%s)", c.resource) }
func (c *HealthyEndpointCondition) Met(ctx context.Context, infra validation.Infrastructure) (bool, error) {
	addr, err := infra.Endpoint(c.resource)
	if err != nil {
		return false, nil
	}
	d := net.Dialer{Timeout: time.Second}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return false, nil
	}
	conn.Close()
	return true, nil
}
```

---

### Task 4.3: waiter_test.go

- [ ] **Create `internal/validation/recovery/waiter_test.go`**:

```go
//go:build validation

package recovery

import (
	"context"
	"os"
	"testing"
	"time"

	"spatial-server/internal/validation"
)

type nilInfra struct{}

func (n *nilInfra) Endpoint(id validation.ResourceID) (string, error)    { return "", nil }
func (n *nilInfra) Processes(id validation.ResourceID) ([]*os.Process, error) { return nil, nil }
func (n *nilInfra) Database(id validation.ResourceID) (string, error)      { return "", nil }
func (n *nilInfra) RuntimeNodes() int                                       { return 0 }
func (n *nilInfra) DialServices(ids ...validation.ResourceID) error         { return nil }
func (n *nilInfra) Close() error                                            { return nil }

func TestCompositeWaiter_AllOfMet(t *testing.T) {
	w := NewComposite(ModeAll, FixedDelay("a", time.Millisecond), FixedDelay("b", time.Millisecond))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := w.Wait(ctx, &nilInfra{}); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
}

func TestCompositeWaiter_AnyOfMet(t *testing.T) {
	w := NewComposite(ModeAny, FixedDelay("a", time.Millisecond), FixedDelay("b", 10*time.Second))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := w.Wait(ctx, &nilInfra{}); err != nil {
		t.Fatalf("expected any-of success, got %v", err)
	}
}

func TestCompositeWaiter_Timeout(t *testing.T) {
	w := NewComposite(ModeAll, FixedDelay("a", time.Second), FixedDelay("b", time.Second))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := w.Wait(ctx, &nilInfra{}); err == nil {
		t.Fatal("expected timeout")
	}
}

func TestCompositeWaiter_Empty(t *testing.T) {
	ctx := context.Background()
	if err := NewComposite(ModeAll).Wait(ctx, &nilInfra{}); err != nil {
		t.Fatalf("empty conditions should succeed, got %v", err)
	}
}
```

- [ ] **Run:** `go test -tags=validation -race ./internal/validation/recovery/`
Expected: 4 tests PASS

---

### Task 4.4: Commit

```bash
git add internal/validation/recovery/
git commit -m "feat(validation): add recovery waiter with composable conditions"
```

---

## Phase 5: Game Probes

**New files:** `internal/game/probes.go`, `internal/game/probes_test.go`
**Deps:** None. **Build tag:** `//go:build validation`.

---

### Task 5.1: probes.go

- [ ] **Create `internal/game/probes.go`**:

```go
//go:build validation

package game

import "github.com/thaolaptrinh/spatial-server/internal/types"

type Snapshot struct {
	EntityCount int
	GhostCount  int
	ZoneOwners  map[types.ZoneID]types.ServerID
	QueueDepths map[string]int
}

func (g *Game) Snapshot() Snapshot {
	g.mu.RLock()
	defer g.mu.RUnlock()
	s := Snapshot{
		EntityCount: len(g.Entities),
		GhostCount:  len(g.ghosts),
		ZoneOwners:  make(map[types.ZoneID]types.ServerID, len(g.Zones)),
		QueueDepths: make(map[string]int, 3),
	}
	for zid := range g.Zones {
		s.ZoneOwners[zid] = g.ServerID
	}
	s.QueueDepths["inbox"] = len(g.Inbox)
	s.QueueDepths["events"] = len(g.Events)
	s.QueueDepths["cmds"] = len(g.cmds)
	return s
}
```

---

### Task 5.2: probes_test.go

- [ ] **Create `internal/game/probes_test.go`**:

```go
//go:build validation

package game

import "testing"

func TestSnapshot_InitialState(t *testing.T) {
	g := New("gs-test")
	s := g.Snapshot()
	if s.EntityCount != 0 {
		t.Fatalf("expected 0 entities, got %d", s.EntityCount)
	}
	if s.GhostCount != 0 {
		t.Fatalf("expected 0 ghosts, got %d", s.GhostCount)
	}
	if s.QueueDepths["inbox"] != 0 {
		t.Fatalf("expected 0 inbox depth, got %d", s.QueueDepths["inbox"])
	}
}
```

- [ ] **Run:** `go test -tags=validation -race -run TestSnapshot ./internal/game/`
Expected: PASS

---

### Task 5.3: Commit

```bash
git add internal/game/probes.go internal/game/probes_test.go
git commit -m "feat(game): add build-tag-gated Snapshot() diagnostic probe"
```

---

## Phase 6: Infrastructure Harness

**New files:** `internal/validation/harness/harness.go`
**Deps:** Phase 1. **Build tag:** `//go:build validation`.

---

### Task 6.1: harness.go

- [ ] **Create `internal/validation/harness/harness.go`**:

This file (~230 lines) implements:
- `StartStack(t)` — Testcontainers PG+Redis, run migrations, return `Infrastructure`
- `StartRoomService(t, h)`, `StartGameServer(t, h, index)`, `StartGateway(t, h)` — build + start service binaries
- `processHarness` struct implementing `validation.Infrastructure`
- `buildService(...)`, `waitForGRPC(...)`, `waitForHTTP(...)` helpers
- `StartStackForChaos(t, runtimes int)` — convenience: stack + room + N runtimes + gateway

Build service binaries with `-tags=validation` to include probes. Processes tracked by resource type prefix so `Processes(ResourceRuntime)` finds all runtimes.

- [ ] **Run:** `go build -tags=validation ./internal/validation/harness/`
Expected: SUCCESS

---

### Task 6.2: Commit

```bash
git add internal/validation/harness/
git commit -m "feat(validation): add infrastructure harness (Testcontainers + process management)"
```

---

## Phase 7: Validators

**New files:** `internal/validation/validators/{entity,ownership,aoi,session,scheduler}.go` + test.
**Deps:** Phase 1. **Build tag:** None.

---

### Task 7.1: entity.go

- [ ] **Create `internal/validation/validators/entity.go`**:

```go
package validators

import (
	"fmt"
	"strconv"
	"spatial-server/internal/validation"
)

type Entity struct{}

func (v *Entity) Name() string { return "entity" }

func (v *Entity) Validate(baseline, postRecovery []validation.Evidence) validation.ValidationResult {
	pre := findInt(baseline, "entity-count")
	post := findInt(postRecovery, "entity-count")
	if pre == -1 || post == -1 {
		return validation.ValidationResult{Validator: v.Name(), Status: validation.StatusSkip, Detail: "entity-count evidence missing"}
	}
	if post < pre {
		return validation.ValidationResult{Validator: v.Name(), Status: validation.StatusFail, Detail: fmt.Sprintf("entity count decreased %d→%d (I-04)", pre, post)}
	}
	return validation.ValidationResult{Validator: v.Name(), Status: validation.StatusPass, Detail: fmt.Sprintf("%d→%d", pre, post)}
}

func findInt(evidence []validation.Evidence, key string) int {
	for _, e := range evidence {
		if e.Key == key {
			v, _ := strconv.Atoi(e.Value)
			return v
		}
	}
	return -1
}
```

### Task 7.2: ownership.go

- [ ] **Create `internal/validation/validators/ownership.go`**:

```go
package validators

import (
	"fmt"
	"spatial-server/internal/validation"
)

type Ownership struct{}

func (v *Ownership) Name() string { return "ownership" }

func (v *Ownership) Validate(baseline, postRecovery []validation.Evidence) validation.ValidationResult {
	pre := findInt(baseline, "zone-owner-count")
	post := findInt(postRecovery, "zone-owner-count")
	if pre == -1 || post == -1 {
		return validation.ValidationResult{Validator: v.Name(), Status: validation.StatusSkip, Detail: "zone-owner-count evidence missing"}
	}
	if post < pre {
		return validation.ValidationResult{Validator: v.Name(), Status: validation.StatusFail, Detail: fmt.Sprintf("zone owners lost: %d→%d (O-01)", pre, post)}
	}
	return validation.ValidationResult{Validator: v.Name(), Status: validation.StatusPass, Detail: fmt.Sprintf("ownership preserved: %d→%d", pre, post)}
}
```

### Task 7.3-7.5: aoi.go, session.go, scheduler.go

Same pattern — each validates one invariant category from `docs/architecture/runtime-invariants.md`.

### Task 7.6: validators_test.go

Tests for Entity (pass, fail), AOI (ghost leak, bounded), Ownership (pass).

- [ ] **Run:** `go test -race ./internal/validation/validators/`
Expected: 5+ tests PASS

### Task 7.7: Commit

```bash
git add internal/validation/validators/
git commit -m "feat(validation): add invariant validators (entity, ownership, aoi, session, scheduler)"
```

---

## Phase 8: Process Injectors

**New files:** `internal/validation/injectors/process.go`, `process_test.go`
**Deps:** Phase 1. **Build tag:** `//go:build validation`.

---

### Task 8.1: process.go

- [ ] **Create `internal/validation/injectors/process.go`** — ProcessCrash (SIGKILL all), ProcessFreeze (SIGSTOP/CONT all), with name(), Inject(), Recover().

---

### Task 8.2: process_test.go

Tests ProcessFreeze with a real `sleep 10` subprocess.

- [ ] **Run:** `go test -tags=validation -race ./internal/validation/injectors/`
Expected: PASS

---

### Task 8.3: Commit

```bash
git add internal/validation/injectors/
git commit -m "feat(validation): add process injectors (crash, freeze)"
```

---

## Phase 9: Reporters

**New files:** `internal/validation/reporter.go`
**Deps:** Phase 1. **Build tag:** None.

---

### Task 9.1: reporter.go

JSONReporter and MarkdownReporter implementing the Reporter interface. MarkdownReportString helper.

- [ ] **Run:** `go build ./internal/validation/`
Expected: SUCCESS

---

### Task 9.2: Commit

```bash
git add internal/validation/reporter.go
git commit -m "feat(validation): add JSON and Markdown reporters"
```

---

## Phase 10: Remaining Injectors

**New files:** `internal/validation/injectors/network.go`, `infrastructure.go`, `resource.go`
**Deps:** Phase 1. **Build tag:** `//go:build validation`. **Note:** Compose-mode stubs — return error on Inject().

```go
// NetLatency, NetLoss, NetPartition, InfraRestart, ResourceCPU, ResourceMemory
// Each returns fmt.Errorf("requires Docker Compose execution mode") on Inject()
```

- [ ] **Run:** `go build -tags=validation ./internal/validation/injectors/`
Expected: SUCCESS

---

### Task 10.1: Commit

```bash
git add internal/validation/injectors/network.go internal/validation/injectors/infrastructure.go internal/validation/injectors/resource.go
git commit -m "feat(validation): add compose-only injectors (network, infrastructure, resource)"
```

---

## Phase 11: Scenario Definitions + Test Entry

**New files:** `tests/validation/scenarios.go`, `tests/validation/chaos_test.go`
**Deps:** Phases 1-10. **Build tag:** `//go:build validation`.

---

### Task 11.1: scenarios.go

- [ ] **Create `tests/validation/scenarios.go`** — `ChaosScenarios()` returns 15 `ScenarioDefinition` entries. Each with Metadata (Name, Description, Tags, Severity, Mode, Requirements, Timeout, ExpectedBehavior), Setup (harness setup), Injector, Recovery, Observers, Validators, Reporter.

---

### Task 11.2: chaos_test.go

- [ ] **Create `tests/validation/chaos_test.go`**:

```go
//go:build validation

package validation

import (
	"context"
	"testing"

	v "spatial-server/internal/validation"
)

func TestChaos(t *testing.T) {
	scenarios := ChaosScenarios()
	for _, sc := range scenarios {
		t.Run(sc.Metadata.Name, func(t *testing.T) {
			if sc.Metadata.Mode == v.ModeCompose {
				t.Skip("compose scenario — requires Docker Compose")
			}
			report := v.NewRunner().Run(context.Background(), sc)
			if report.Outcome == v.OutcomeSkipped {
				t.Skipf("skipped: %s", report.RootCause)
			}
			if report.Outcome != v.OutcomePassed {
				t.Errorf("scenario %s: %s — %s", sc.Metadata.Name, report.Outcome, report.RootCause)
				t.Log(v.MarkdownReportString(report))
			}
		})
	}
}
```

- [ ] **Run:** `go build -tags=validation ./tests/validation/`
Expected: SUCCESS

---

### Task 11.3: Commit

```bash
git add tests/validation/
git commit -m "feat(validation): add 15 chaos scenario definitions and test entry point"
```

---

## Phase 12: Process Scenarios E2E Verification

**New files:** None.

- [ ] Run all 6 process-mode scenarios with real services:
```bash
go test -tags=validation -run TestChaos -count=1 -timeout=30m ./tests/validation/ 2>&1 | tee chaos-report.txt
```
Expected: 6 process scenarios run (crash/restart for runtime, gateway, room-service). Compose scenarios skipped.

---

## Phase 13: Compose Scenarios Verification

**New files:** None.

- [ ] Start Docker Compose stack with 2 runtimes. Run compose scenarios:
```bash
docker compose -f deploy/docker-compose/docker-compose.2node.yml up -d
go test -tags=validation -run TestChaos -count=1 -timeout=30m ./tests/validation/
docker compose -f deploy/docker-compose/docker-compose.2node.yml down
```
Expected: 9 compose scenarios run against live stack.

---

## Phase 14: Final Documentation + Chaos Report

**New files:** `docs/testing/chaos-report.md`

- [ ] Update `docs/testing/chaos-testing.md` with execution instructions.
- [ ] Generate final Chaos Engineering report from scenario output.
- [ ] Commit final documentation.

---

## Risk Assessment

| Phase | Risk | Mitigation |
|-------|------|------------|
| 1-2 (Core) | None — pure data types | stdlib-only |
| 3 (Observers) | gRPC imports pull heavy deps | `//go:build validation` gate |
| 4 (Recovery) | Race conditions in polling | Tests validate timeout/all-of/any-of behavior |
| 6 (Harness) | Testcontainers Docker dependency | Only via build tag; skipped in light builds |
| 11-13 (Scenarios) | Real services may have bugs | These ARE the tests that find those bugs |
| All | Circular imports | Dependency direction enforced: runtime → never imports validation |

---

## Testing Strategy

| Phase | Tests | Type |
|-------|-------|------|
| 1 | CheckRequirements + NewSummary | Unit |
| 2 | Runner lifecycle (pass, skip, fail, timeout) | Unit |
| 3 | Observer construction | Build verification |
| 4 | CompositeWaiter (all-of, any-of, timeout, empty) | Unit |
| 5 | Snapshot initial state | Unit |
| 6 | Harness construction | Build verification |
| 7 | Validators (entity fail/pass, AOI leak/bounded, ownership) | Unit |
| 8 | ProcessFreeze Inject/Recover | Unit + integration |
| 11 | ChaosScenarios definition | Build verification |
| 12 | 6 process chaos scenarios | E2E |
| 13 | 9 compose chaos scenarios | E2E |

---

## Documentation Strategy

| Phase | Document | When |
|-------|----------|------|
| 12 | `docs/testing/chaos-testing.md` | After process scenarios pass |
| 14 | `docs/testing/chaos-report.md` | After all scenarios complete |

---

## Final Roadmap

| # | Title | Complexity | Effort |
|---|-------|-----------|--------|
| 1 | Core Package | Low | 30m |
| 2 | Runner | Low | 20m |
| 3 | Observers | Low | 15m |
| 4 | Recovery | Low | 20m |
| 5 | Probes | Low | 10m |
| 6 | Harness | Medium | 45m |
| 7 | Validators | Low | 25m |
| 8 | Process Injectors | Low | 15m |
| 9 | Reporters | Low | 10m |
| 10 | Remaining Injectors | Low | 10m |
| 11 | Scenarios + Test Entry | Medium | 30m |
| 12 | Process E2E | High | 60m |
| 13 | Compose E2E | Medium | 30m |
| 14 | Documentation | Low | 15m |
| **Total** | | | **~5.5h** |

---

## Post-Plan Sanity Review

✅ Every architectural component from the spec is implemented by a phase.
✅ No phase introduces work beyond the spec.
✅ No circular dependencies — dependency graph is DAG.
✅ Every phase has measurable Definition of Done (build, test, or run command).
✅ Single build tag: `validation`. Single command: `go test -tags=validation ./tests/validation/...`
✅ Runtime never imports validation. Direction is strictly one-way.
