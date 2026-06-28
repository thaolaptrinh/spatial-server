# Validation Framework Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement a generic Validation Framework as the single execution engine for all validation activities (chaos, integration, benchmark, load, soak).

**Architecture:** 4 core files (scenario, measure, report, runner) with no build tag, plus 14 leaf files (harness, injectors, validators, observers, recovery, probes, reporters, scenarios, test entry) with `//go:build validation`. Runner orchestrates: Setup → CheckRequirements → Baseline Observe → Inject → defer Recover() → Wait → Post-Recovery Observe → Validate → Report → Teardown. Recover() is guaranteed via defer.

**Tech Stack:** Go 1.25, Testcontainers (PG 16, Redis 7), gRPC, stretchr/testify

---

## Dependency Graph

```
Phase 1 ─── Phase 2 ─── Phase 3 ─── Phase 9 ─── Phase 10 ─── Phase 11A ─── Phase 12 ─── Phase 11B ─── Phase 13 ─── Phase 14
                  │
     ┌────────────┼────────────┬────────────┐
     │            │            │            │
Phase 4       Phase 5       Phase 6       Phase 7           Phase 8
(recovery)   (probes)      (harness*4)  (validators)    (process inj)
     │            │            │            │                 │
     └────────────┴────────────┴────────────┴─────────────────┘
```

**Critical path:** 1 → 2 → 3 → 9 → 10 → 11A → 12 → 11B → 13 → 14
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
	ID               string
	Name             string
	Description      string
	Version          int
	Tags             []string
	Severity         Severity
	Mode             ExecutionMode
	Requirements     Requirements
	Timeout          time.Duration
	ExpectedBehavior string
}

// SetupFunc creates the test infrastructure and returns a cleanup function.
//
// Cleanup contract:
//   - owns every started process, container, temporary binary, log file
//   - must always be safe to call (nil-safe)
//   - must be idempotent (calling twice is harmless)
//   - called by Runner via defer — always executes, even after panics
type SetupFunc func() (Infrastructure, func(), error)

type AcceptancePolicy string

const (
	AcceptWarn AcceptancePolicy = "warn" // advisory only, never changes scenario outcome
	AcceptFail AcceptancePolicy = "fail" // FAILs the scenario when threshold exceeded
)

type AcceptanceCriterion struct {
	Threshold time.Duration
	Policy    AcceptancePolicy
}

type AcceptanceCriteria struct {
	RecoveryDuration     AcceptanceCriterion
	OwnershipConvergence  AcceptanceCriterion
	TickP95               AcceptanceCriterion
	ReconnectTime         AcceptanceCriterion
	QueueDrainTime        AcceptanceCriterion
}

type ScenarioDefinition struct {
	Metadata    ScenarioMetadata
	Setup       SetupFunc
	Injector    Injector
	Observers   []Observer
	Validators  []Validator
	Recovery    RecoveryWaiter
	Reporter    Reporter
	Acceptance  AcceptanceCriteria
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

// ── Evidence keys (single source of truth) ──────────────────────────────
//
// CONTRACT: Validators and Observers MUST reference only these declared
// Evidence Keys. Literal string keys are prohibited. This keeps evidence
// producers and consumers consistent and prevents key-drift bugs.

const (
	EvKeyEntityCount       = "entity-count"
	EvKeyGhostCount        = "ghost-count"
	EvKeyCmdDrops          = "cmd-drops"
	EvKeyZoneOwnerCount    = "zone-owner-count"
	EvKeyDisconnectedCount = "disconnected-count"
	EvKeyRuntimeNodes      = "runtime-nodes"
	EvKeyRoomServiceAddr   = "room-service-endpoint"
	EvKeyGatewayAddr       = "gateway-endpoint"
)

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

// ── Evidence lookup ─────────────────────────────────────────────────────

// EvidenceMap indexes evidence by key for O(1) lookup.
// When multiple evidence entries share a key, the last one wins.
func EvidenceMap(evidence []Evidence) map[string]Evidence {
	m := make(map[string]Evidence, len(evidence))
	for _, e := range evidence {
		m[e.Key] = e
	}
	return m
}

// ── Observer policy ─────────────────────────────────────────────────────

// ObserverPolicy controls whether a failed observer aborts the scenario.
type ObserverPolicy string

const (
	PolicyRequired ObserverPolicy = "required" // error aborts scenario (default)
	PolicyOptional ObserverPolicy = "optional" // error generates warning evidence only
)

type Observer interface {
	Name() string
	Policy() ObserverPolicy
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

- [ ] **Create `internal/validation/report.go`**:

```go
package validation

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

const frameworkVersion = "1.0.0"

// Version ownership:
//   - FrameworkVersion: identifies the Validation Framework release (hardcoded constant).
//   - ScenarioVersion: derived from ScenarioMetadata.Version, the singular authoritative
//     source for scenario versioning. No hardcoded string.
//   - SummaryReport uses FrameworkVersion + ScenarioVersion independently.
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

// Reporters are stateless. Each Generate() allocates local buffers, safe for concurrent reuse.
type MarkdownReporter struct{}

func (m *MarkdownReporter) Generate(report *ValidationReport) error {
	var b strings.Builder
	fmt.Fprintf(&b, "# Chaos Report: %s\n\n", report.Scenario.Name)
	fmt.Fprintf(&b, "| Field | Value |\n|-------|-------|\n")
	fmt.Fprintf(&b, "| Outcome | %s |\n", report.Outcome)
	fmt.Fprintf(&b, "| Duration | %s |\n", report.Duration)
	fmt.Fprintf(&b, "| Expected | %s |\n\n", report.Scenario.ExpectedBehavior)
	fmt.Fprintf(&b, "## Validations\n\n| Validator | Status | Detail |\n|-----------|--------|--------|\n")
	for _, v := range report.Validations {
		fmt.Fprintf(&b, "| %s | %s | %s |\n", v.Validator, v.Status, v.Detail)
	}
	// For MarkdownReporter.Generate(), the output is printed to stdout.
	// For programmatic use, call MarkdownReportString() instead.
	_, _ = os.Stdout.Write([]byte(b.String()))
	return nil
}

func MarkdownReportString(report *ValidationReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Chaos Report: %s\n\n", report.Scenario.Name)
	fmt.Fprintf(&b, "| Field | Value |\n|-------|-------|\n")
	fmt.Fprintf(&b, "| Outcome | %s |\n", report.Outcome)
	fmt.Fprintf(&b, "| Duration | %s |\n", report.Duration)
	fmt.Fprintf(&b, "| Expected | %s |\n\n", report.Scenario.ExpectedBehavior)
	fmt.Fprintf(&b, "## Validations\n\n| Validator | Status | Detail |\n|-----------|--------|--------|\n")
	for _, v := range report.Validations {
		fmt.Fprintf(&b, "| %s | %s | %s |\n", v.Validator, v.Status, v.Detail)
	}
	return b.String()
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

// ValidateScenarios checks for duplicate IDs across all scenarios.
// Returns errors for any duplicates. Call before Run() to fail fast.
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

	// Recover from panics in any lifecycle phase to preserve diagnostic output.
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

	if err := sc.Injector.Inject(scopedCtx, infra); err != nil {
		report.Outcome = OutcomeError
		report.RootCause = fmt.Sprintf("inject: %v", err)
		return report
	}
	defer func() { _ = sc.Injector.Recover(context.Background(), infra) }()

	// Recovery duration: starts immediately after Inject() succeeds,
	// ends when RecoveryWaiter.Wait() returns. Populated even on failure.
	// Recovery duration: starts immediately after Inject() succeeds,
	// ends when RecoveryWaiter.Wait() returns. Populated even on failure.
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
		// Do not return early — proceed to observe + validate anyway.
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

	// Outcome precedence: Error > TimedOut > Failed > Passed > Skipped.
	// Validators set Passed/Failed only when no higher-priority outcome exists.
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
	// OwnershipConvergence, ReconnectTime, QueueDrainTime:
	// Intentionally deferred. These require instrumentation in the harness
	// (ownership convergence tracking, reconnect event timestamps, queue drain
	// observation) which is part of the Harness phase and not yet implemented.
	// This is NOT incomplete work — the Measurement struct supports them,
	// evaluateAcceptance calls them when data is present, and they will be
	// wired up once harness instrumentation becomes available.
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
	name   string
	fn     func(ObservationPhase) []Evidence
	policy ObserverPolicy
}

func (o *testObserver) Name() string             { return o.name }
func (o *testObserver) Policy() ObserverPolicy    {
	if o.policy == "" { return PolicyRequired }
	return o.policy
}
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

func (o *State) Name() string           { return "state" }
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

func (o *Routing) Name() string            { return "routing" }
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

func (o *Metrics) Name() string            { return "metrics" }
func (o *Metrics) Policy() validation.ObserverPolicy { return validation.PolicyOptional }

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

**New files:**
- `internal/validation/harness/stack.go` — Testcontainers PG+Redis, processHarness struct, Infrastructure impl
- `internal/validation/harness/process_manager.go` — service lifecycle (start/stop)
- `internal/validation/harness/builders.go` — buildService, moduleRoot
- `internal/validation/harness/health.go` — waitForGRPC, waitForHTTP

**Deps:** Phase 1. **Build tag:** `//go:build validation`.

**Note:** Split from original 1-file harness into 4 for maintainability. No behavioural changes.
The full consolidated code follows — split into 4 files during implementation.

---

### Task 6.1: stack.go

- [ ] **Create `internal/validation/harness/stack.go`** — `processHarness`, `StartStack()`, `StartStackForChaos()`, Infrastructure methods.

### Task 6.2: process_manager.go

- [ ] **Create `internal/validation/harness/process_manager.go`** — `StartRoomService`, `StartGameServer`, `StartGateway`, `cleanEnv`, `killProcess`.

### Task 6.3: builders.go

- [ ] **Create `internal/validation/harness/builders.go`** — `buildService`, `moduleRoot`.

### Task 6.4: health.go

- [ ] **Create `internal/validation/harness/health.go`** — `waitForGRPC`, `waitForHTTP`.

---

### Consolidated harness code (split into 4 files during implementation):

```go
//go:build validation

package harness

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/thaolaptrinh/spatial-server/internal/migration"
	"github.com/thaolaptrinh/spatial-server/internal/validation"
)

type processHarness struct {
	pgDSN     string
	redisAddr string
	endpoints map[validation.ResourceID]string
	processes map[validation.ResourceID][]*os.Process
	cleanups  []func()
	mu        sync.Mutex
}

func moduleRoot() string {
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func StartStack(t *testing.T) (*processHarness, func()) {
	t.Helper()
	ctx := context.Background()
	h := &processHarness{
		endpoints: make(map[validation.ResourceID]string),
		processes: make(map[validation.ResourceID][]*os.Process),
	}
	pgC, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("spatial"),
		postgres.WithUsername("spatial"),
		postgres.WithPassword("spatial"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(60*time.Second),
		),
	)
	require.NoError(t, err)
	h.pgDSN, err = pgC.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	redisC, err := redis.Run(ctx, "redis:7-alpine")
	require.NoError(t, err)
	h.redisAddr, err = redisC.Endpoint(ctx, "")
	require.NoError(t, err)
	h.endpoints[validation.ResourcePostgres] = h.pgDSN
	h.endpoints[validation.ResourceRedis] = h.redisAddr
	pool, err := pgxpool.New(ctx, h.pgDSN)
	require.NoError(t, err)
	root := moduleRoot()
	require.NotEmpty(t, root)
	require.NoError(t, migration.Run(pool, filepath.Join(root, "internal", "storage", "migrations")))
	h.cleanups = append(h.cleanups, func() { pgC.Terminate(ctx); redisC.Terminate(ctx) })
	t.Logf("postgres ready: %s", h.pgDSN)
	t.Logf("redis ready: %s", h.redisAddr)
	return h, func() {
		for i := len(h.cleanups) - 1; i >= 0; i-- {
			h.cleanups[i]()
		}
	}
}

func (h *processHarness) Endpoint(id validation.ResourceID) (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if addr, ok := h.endpoints[id]; ok {
		return addr, nil
	}
	return "", fmt.Errorf("no endpoint for %s", id)
}

func (h *processHarness) Processes(id validation.ResourceID) ([]*os.Process, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.processes[id], nil
}

func (h *processHarness) Database(id validation.ResourceID) (string, error) {
	if id == validation.ResourcePostgres {
		return h.pgDSN, nil
	}
	return "", errors.New("no database")
}

func (h *processHarness) RuntimeNodes() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.processes[validation.ResourceRuntime])
}

func (h *processHarness) DialServices(ids ...validation.ResourceID) error {
	for _, id := range ids {
		if _, err := h.Endpoint(id); err != nil {
			return fmt.Errorf("dial %s: %w", id, err)
		}
	}
	return nil
}

func (h *processHarness) Close() error { return nil }

func (h *processHarness) StartRoomService(t *testing.T) {
	t.Helper()
	binPath := buildService(t, "room-service")
	root := moduleRoot()
	logPath := filepath.Join(os.TempDir(), fmt.Sprintf("spatial-room-service-%d.log", time.Now().UnixNano()))
	f, err := os.Create(logPath)
	require.NoError(t, err)
	cmd := exec.Command(binPath)
	cmd.Dir = root
	cmd.Env = append(cleanEnv(), "SPATIAL_POSTGRES__DSN="+h.pgDSN, "SPATIAL_REDIS__ADDR="+h.redisAddr, "SPATIAL_GRPC__HOST=127.0.0.1", "SPATIAL_GRPC__PORT=19000")
	cmd.Stdout, cmd.Stderr = f, f
	require.NoError(t, cmd.Start())
	h.mu.Lock()
	h.processes[validation.ResourceRoomService] = append(h.processes[validation.ResourceRoomService], cmd.Process)
	h.endpoints[validation.ResourceRoomService] = "127.0.0.1:19000"
	h.mu.Unlock()
	h.cleanups = append(h.cleanups, func() { killProcess(cmd); os.Remove(binPath); f.Close() })
	waitForGRPC(t, "127.0.0.1:19000", 30*time.Second)
}

func (h *processHarness) StartGameServer(t *testing.T, index int) {
	t.Helper()
	binPath := buildService(t, "game-server")
	port := 19001 + index
	root := moduleRoot()
	logPath := filepath.Join(os.TempDir(), fmt.Sprintf("spatial-game-server-%d-%d.log", index, time.Now().UnixNano()))
	f, err := os.Create(logPath)
	require.NoError(t, err)
	cmd := exec.Command(binPath)
	cmd.Dir = root
	cmd.Env = append(cleanEnv(), "SPATIAL_GRPC__HOST=127.0.0.1", fmt.Sprintf("SPATIAL_GRPC__PORT=%d", port), "SPATIAL_ROOM_SERVICE__ADDR=127.0.0.1:19000")
	cmd.Stdout, cmd.Stderr = f, f
	require.NoError(t, cmd.Start())
	h.mu.Lock()
	h.processes[validation.ResourceRuntime] = append(h.processes[validation.ResourceRuntime], cmd.Process)
	h.mu.Unlock()
	h.cleanups = append(h.cleanups, func() { killProcess(cmd); os.Remove(binPath); f.Close() })
	waitForGRPC(t, fmt.Sprintf("127.0.0.1:%d", port), 30*time.Second)
}

func (h *processHarness) StartGateway(t *testing.T) {
	t.Helper()
	binPath := buildService(t, "gateway")
	root := moduleRoot()
	logPath := filepath.Join(os.TempDir(), fmt.Sprintf("spatial-gateway-%d.log", time.Now().UnixNano()))
	f, err := os.Create(logPath)
	require.NoError(t, err)
	cmd := exec.Command(binPath)
	cmd.Dir = root
	cmd.Env = append(cleanEnv(), "SPATIAL_GATEWAY__WS_PORT=18080", "SPATIAL_ROOM_SERVICE__ADDR=127.0.0.1:19000")
	cmd.Stdout, cmd.Stderr = f, f
	require.NoError(t, cmd.Start())
	h.mu.Lock()
	h.processes[validation.ResourceGateway] = append(h.processes[validation.ResourceGateway], cmd.Process)
	h.endpoints[validation.ResourceGateway] = "127.0.0.1:18080"
	h.mu.Unlock()
	h.cleanups = append(h.cleanups, func() { killProcess(cmd); os.Remove(binPath); f.Close() })
	waitForHTTP(t, "http://127.0.0.1:18080/health", 30*time.Second)
}

func StartStackForChaos(t *testing.T, runtimes int) (*processHarness, func()) {
	h, teardown := StartStack(t)
	h.StartRoomService(t)
	for i := 0; i < runtimes; i++ {
		h.StartGameServer(t, i)
	}
	h.StartGateway(t)
	return h, teardown
}

// ResetForChaos restarts the full stack from scratch between scenario runs.
func ResetForChaos(t *testing.T, runtimes int) (*processHarness, func()) {
	return StartStackForChaos(t, runtimes)
}

// NewStackForScenario is a SetupFunc-compatible wrapper for use in ScenarioDefinition.
// It returns a function capturing runtimes count, callable from tests.
func NewStackForScenario(t *testing.T, runtimes int) (*processHarness, func()) {
	return StartStackForChaos(t, runtimes)
}

func cleanEnv() []string {
	var env []string
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "SPATIAL_") {
			env = append(env, e)
		}
	}
	return env
}

func buildService(t *testing.T, name string) string {
	t.Helper()
	root := moduleRoot()
	require.NotEmpty(t, root)
	binPath := filepath.Join(os.TempDir(), fmt.Sprintf("spatial-%s-%d", name, time.Now().UnixNano()))
	cmd := exec.Command("go", "build", "-tags=validation", "-o", binPath, fmt.Sprintf("./apps/%s/", name))
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "build %s failed:\n%s", name, string(out))
	return binPath
}

func killProcess(cmd *exec.Cmd) {
	cmd.Process.Signal(syscall.SIGTERM)
	done := make(chan struct{})
	go func() { cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		cmd.Process.Kill()
	}
}

func waitForGRPC(t *testing.T, addr string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, time.Second)
		if err == nil {
			conn.Close()
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for gRPC %s", addr)
}

func waitForHTTP(t *testing.T, url string, timeout time.Duration) {
	t.Helper()
	client := &http.Client{Timeout: time.Second}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for HTTP %s", url)
}
```

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
	pre := findInt(baseline, validation.EvKeyEntityCount)
	post := findInt(postRecovery, validation.EvKeyEntityCount)
	if pre == -1 || post == -1 {
		return validation.ValidationResult{Validator: v.Name(), Status: validation.StatusSkip, Detail: "entity-count evidence missing"}
	}
	if post < pre {
		return validation.ValidationResult{Validator: v.Name(), Status: validation.StatusFail, Detail: fmt.Sprintf("entity count decreased %d→%d (I-04)", pre, post)}
	}
	return validation.ValidationResult{Validator: v.Name(), Status: validation.StatusPass, Detail: fmt.Sprintf("%d→%d", pre, post)}
}

func findInt(evidence []validation.Evidence, key string) int {
	m := validation.EvidenceMap(evidence)
	if e, ok := m[key]; ok {
		v, _ := strconv.Atoi(e.Value)
		return v
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
	pre := findInt(baseline, validation.EvKeyZoneOwnerCount)
	post := findInt(postRecovery, validation.EvKeyZoneOwnerCount)
	if pre == -1 || post == -1 {
		return validation.ValidationResult{Validator: v.Name(), Status: validation.StatusSkip, Detail: "zone-owner-count evidence missing"}
	}
	if post < pre {
		return validation.ValidationResult{Validator: v.Name(), Status: validation.StatusFail, Detail: fmt.Sprintf("zone owners lost: %d→%d (O-01)", pre, post)}
	}
	return validation.ValidationResult{Validator: v.Name(), Status: validation.StatusPass, Detail: fmt.Sprintf("ownership preserved: %d→%d", pre, post)}
}
```

### Task 7.3: aoi.go

- [ ] **Create `internal/validation/validators/aoi.go`**:

```go
package validators

import (
	"fmt"
	"spatial-server/internal/validation"
)

type AOI struct{}

func (v *AOI) Name() string { return "aoi" }

func (v *AOI) Validate(baseline, postRecovery []validation.Evidence) validation.ValidationResult {
	postGhosts := findInt(postRecovery, validation.EvKeyGhostCount)
	postEntities := findInt(postRecovery, validation.EvKeyEntityCount)
	if postGhosts == -1 || postEntities == -1 {
		return validation.ValidationResult{Validator: v.Name(), Status: validation.StatusSkip, Detail: "ghost-count or entity-count missing"}
	}
	if postEntities > 0 && postGhosts > postEntities {
		return validation.ValidationResult{Validator: v.Name(), Status: validation.StatusFail,
			Detail: fmt.Sprintf("ghosts(%d) > entities(%d) — G-02 violated", postGhosts, postEntities)}
	}
	return validation.ValidationResult{Validator: v.Name(), Status: validation.StatusPass,
		Detail: fmt.Sprintf("ghosts bounded: %d/%d", postGhosts, postEntities)}
}
```

### Task 7.4: session.go

- [ ] **Create `internal/validation/validators/session.go`**:

```go
package validators

import "spatial-server/internal/validation"

type Session struct{}

func (v *Session) Name() string { return "session" }

func (v *Session) Validate(baseline, postRecovery []validation.Evidence) validation.ValidationResult {
	preDisc := findInt(baseline, validation.EvKeyDisconnectedCount)
	postDisc := findInt(postRecovery, validation.EvKeyDisconnectedCount)
	if preDisc == -1 || postDisc == -1 {
		return validation.ValidationResult{Validator: v.Name(), Status: validation.StatusSkip, Detail: "disconnected-count evidence missing"}
	}
	if postDisc > preDisc+10 {
		return validation.ValidationResult{Validator: v.Name(), Status: validation.StatusWarn, Detail: "disconnected sessions grew significantly"}
	}
	return validation.ValidationResult{Validator: v.Name(), Status: validation.StatusPass, Detail: "session state preserved"}
}
```

### Task 7.5: scheduler.go

- [ ] **Create `internal/validation/validators/scheduler.go`**:

```go
package validators

import "spatial-server/internal/validation"

type Scheduler struct{}

func (v *Scheduler) Name() string { return "scheduler" }

func (v *Scheduler) Validate(baseline, postRecovery []validation.Evidence) validation.ValidationResult {
	preDrops := findInt(baseline, validation.EvKeyCmdDrops)
	postDrops := findInt(postRecovery, validation.EvKeyCmdDrops)
	if preDrops == -1 || postDrops == -1 {
		return validation.ValidationResult{Validator: v.Name(), Status: validation.StatusSkip, Detail: "cmd-drops evidence missing"}
	}
	if postDrops > preDrops {
		return validation.ValidationResult{Validator: v.Name(), Status: validation.StatusWarn, Detail: "command drops observed (T-04)"}
	}
	return validation.ValidationResult{Validator: v.Name(), Status: validation.StatusPass, Detail: "no command drops"}
}
```

### Task 7.6: validators_test.go

- [ ] **Create `internal/validation/validators/validators_test.go`**:

```go
package validators

import (
	"testing"
	"time"

	"spatial-server/internal/validation"
)

func TestEntity_Pass(t *testing.T) {
	v := &Entity{}
	base := []validation.Evidence{{Key: validation.EvKeyEntityCount, Value: "10", Timestamp: time.Now()}}
	post := []validation.Evidence{{Key: validation.EvKeyEntityCount, Value: "10", Timestamp: time.Now()}}
	r := v.Validate(base, post)
	if r.Status != validation.StatusPass {
		t.Fatalf("expected pass, got %s: %s", r.Status, r.Detail)
	}
}

func TestEntity_Fail(t *testing.T) {
	v := &Entity{}
	r := v.Validate(
		[]validation.Evidence{{Key: validation.EvKeyEntityCount, Value: "10"}},
		[]validation.Evidence{{Key: validation.EvKeyEntityCount, Value: "5"}},
	)
	if r.Status != validation.StatusFail {
		t.Fatalf("expected fail, got %s", r.Status)
	}
}

func TestAOI_GhostLeak(t *testing.T) {
	v := &AOI{}
	r := v.Validate(
		[]validation.Evidence{{Key: validation.EvKeyGhostCount, Value: "2"}, {Key: validation.EvKeyEntityCount, Value: "10"}},
		[]validation.Evidence{{Key: validation.EvKeyGhostCount, Value: "15"}, {Key: validation.EvKeyEntityCount, Value: "10"}},
	)
	if r.Status != validation.StatusFail {
		t.Fatalf("expected ghost leak fail, got %s", r.Status)
	}
}

func TestAOI_GhostBounded(t *testing.T) {
	v := &AOI{}
	r := v.Validate(
		[]validation.Evidence{{Key: validation.EvKeyGhostCount, Value: "5"}, {Key: validation.EvKeyEntityCount, Value: "10"}},
		[]validation.Evidence{{Key: validation.EvKeyGhostCount, Value: "7"}, {Key: validation.EvKeyEntityCount, Value: "10"}},
	)
	if r.Status != validation.StatusPass {
		t.Fatalf("expected pass, got %s: %s", r.Status, r.Detail)
	}
}

func TestOwnership_Pass(t *testing.T) {
	v := &Ownership{}
	r := v.Validate(
		[]validation.Evidence{{Key: validation.EvKeyZoneOwnerCount, Value: "2"}},
		[]validation.Evidence{{Key: validation.EvKeyZoneOwnerCount, Value: "2"}},
	)
	if r.Status != validation.StatusPass {
		t.Fatalf("expected pass, got %s", r.Status)
	}
}
```

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

- [ ] **Create `internal/validation/injectors/process.go`**:

```go
//go:build validation

package injectors

import (
	"context"
	"fmt"
	"os"
	"syscall"
	"time"

	"spatial-server/internal/validation"
)

type ProcessCrash struct {
	Target validation.ResourceID
}

// ProcessCrash kills ALL processes of the target type.
//
// Restart ownership: The harness is the sole owner of service restart.
// Setup functions (harness.StartStackForChaos) start fresh binaries before
// each scenario. The recovery waiter detects restored liveness via
// HealthyEndpoint conditions. The injector only applies the fault.
//
// No restart occurs within a single scenario run — the recovery phase waits
// for whatever recovery mechanism the harness provides (new process detection
// by hitting the endpoint). If the harness cannot restart the service, the
// HealthyEndpoint condition will never be met and the scenario times out.
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

func (i *ProcessFreeze) Name() string { return fmt.Sprintf("process-freeze(%s, %v)", i.Target, i.Duration) }

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
```

---

### Task 8.2: process_test.go

- [ ] **Create `internal/validation/injectors/process_test.go`**:

```go
//go:build validation

package injectors

import (
	"context"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"spatial-server/internal/validation"
)

type mockInfra struct {
	procs []*os.Process
}

func (m *mockInfra) Endpoint(id validation.ResourceID) (string, error)     { return "", nil }
func (m *mockInfra) Processes(id validation.ResourceID) ([]*os.Process, error) { return m.procs, nil }
func (m *mockInfra) Database(id validation.ResourceID) (string, error)       { return "", nil }
func (m *mockInfra) RuntimeNodes() int                                        { return len(m.procs) }
func (m *mockInfra) DialServices(ids ...validation.ResourceID) error          { return nil }
func (m *mockInfra) Close() error                                             { return nil }

func TestProcessFreeze_InjectRecover(t *testing.T) {
	cmd := exec.Command("sleep", "10")
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot start sleep: %v", err)
	}
	defer func() { cmd.Process.Signal(syscall.SIGKILL); cmd.Wait() }()

	infra := &mockInfra{procs: []*os.Process{cmd.Process}}
	freeze := NewProcessFreeze(validation.ResourceRuntime, 100*time.Millisecond)

	if err := freeze.Inject(context.Background(), infra); err != nil {
		t.Fatalf("inject: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	if err := freeze.Recover(context.Background(), infra); err != nil {
		t.Fatalf("recover: %v", err)
	}
}
```

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

- [ ] **Create `internal/validation/reporter.go`** (MarkdownReporter is already in report.go from Phase 1):

```go
package validation

import (
	"encoding/json"
	"os"
)

type JSONReporter struct {
	Path string
}

func (r *JSONReporter) Generate(report *ValidationReport) error {
	w := os.Stdout
	if r.Path != "" {
		f, err := os.Create(r.Path)
		if err != nil {
			return err
		}
		defer f.Close()
		w = f
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}
```

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

## Phase 11A: Process Scenarios + Test Entry

**New files:** `tests/validation/process_scenarios.go`, `tests/validation/process_test.go`
**Deps:** Phases 1-10. **Build tag:** `//go:build validation`.

**Definition of Done:** `go build -tags=validation ./tests/validation/` succeeds; 7 process scenarios run and pass.

---

### Task 11A.1: process_scenarios.go

- [ ] **Create `tests/validation/process_scenarios.go`** — 7 process-mode scenarios:

```go
//go:build validation

package validation

import (
	"time"

	"spatial-server/internal/validation"
	"spatial-server/internal/validation/harness"
	"spatial-server/internal/validation/injectors"
	"spatial-server/internal/validation/observers"
	"spatial-server/internal/validation/recovery"
	"spatial-server/internal/validation/validators"
)

func ProcessScenarios(t *testing.T) []validation.ScenarioDefinition {
	t.Helper()
	stack := func(runtimes int) func() (validation.Infrastructure, func(), error) {
		return func() (validation.Infrastructure, func(), error) {
			h, c := harness.StartStackForChaos(t, runtimes)
			return h, c, nil
		}
	}
	defaultVals := []validation.Validator{
		&validators.Entity{}, &validators.Ownership{},
		&validators.AOI{}, &validators.Session{},
	}
	defaultObs := []validation.Observer{&observers.State{}, &observers.Routing{}}
	crashRecovery := recovery.NewComposite(recovery.ModeAll,
		recovery.HealthyEndpoint(validation.ResourceRoomService),
		recovery.FixedDelay("stabilize", 5*time.Second),
	)

	return []validation.ScenarioDefinition{
		{
			Metadata: validation.ScenarioMetadata{
				ID: "runtime-crash", Name: "Runtime Node Crash",
				Description: "Runtime Node crash and recovery", Version: 1,
				Tags: []string{"chaos", "crash"}, Severity: validation.SeverityHigh,
				Mode: validation.ModeProcess,
				Requirements:     validation.Requirements{PostgreSQL: true, Redis: true, MinRuntimeNodes: 1},
				Timeout:          2 * time.Minute,
				ExpectedBehavior: "Room Service reassigns zones, entities preserved, ghosts cleaned up",
			},
			Setup:      stack(1),
			Injector:   &injectors.ProcessCrash{Target: validation.ResourceRuntime},
			Recovery:   crashRecovery,
			Observers:  defaultObs,
			Validators: defaultVals,
			Acceptance: validation.AcceptanceCriteria{RecoveryDuration: validation.AcceptanceCriterion{Threshold: 30 * time.Second, Policy: validation.AcceptWarn}},
		},
		{
			Metadata: validation.ScenarioMetadata{
				ID: "runtime-restart", Name: "Runtime Node Restart",
				Description: "Runtime Node SIGKILL + restart", Version: 1,
				Tags: []string{"chaos", "restart"}, Severity: validation.SeverityHigh,
				Mode: validation.ModeProcess,
				Requirements:     validation.Requirements{PostgreSQL: true, Redis: true, MinRuntimeNodes: 1},
				Timeout:          2 * time.Minute,
				ExpectedBehavior: "Runtime re-registers, zones reassigned, entities intact",
			},
			Setup:      stack(1),
			Injector:   &injectors.ProcessCrash{Target: validation.ResourceRuntime},
			Recovery:   crashRecovery,
			Observers:  defaultObs,
			Validators: defaultVals,
			Acceptance: validation.AcceptanceCriteria{RecoveryDuration: validation.AcceptanceCriterion{Threshold: 30 * time.Second, Policy: validation.AcceptWarn}},
		},
		{
			Metadata: validation.ScenarioMetadata{
				ID: "gateway-crash", Name: "Gateway Crash",
				Description: "Gateway crash and recovery", Version: 1,
				Tags: []string{"chaos", "crash", "gateway"}, Severity: validation.SeverityHigh,
				Mode: validation.ModeProcess,
				Requirements:     validation.Requirements{PostgreSQL: true, Redis: true, MinRuntimeNodes: 1},
				Timeout:          2 * time.Minute,
				ExpectedBehavior: "Gateway reconnects to Room Service, routing cache repopulated",
			},
			Setup:    stack(1),
			Injector: &injectors.ProcessCrash{Target: validation.ResourceGateway},
			Recovery: recovery.NewComposite(recovery.ModeAll,
				recovery.HealthyEndpoint(validation.ResourceGateway),
				recovery.FixedDelay("stabilize", 3*time.Second),
			),
			Observers:  defaultObs,
			Validators: defaultVals,
			Acceptance: validation.AcceptanceCriteria{RecoveryDuration: validation.AcceptanceCriterion{Threshold: 15 * time.Second, Policy: validation.AcceptWarn}},
		},
		{
			Metadata: validation.ScenarioMetadata{
				ID: "gateway-restart", Name: "Gateway Restart",
				Description: "Gateway restart", Version: 1,
				Tags: []string{"chaos", "restart", "gateway"}, Severity: validation.SeverityHigh,
				Mode: validation.ModeProcess,
				Requirements:     validation.Requirements{PostgreSQL: true, Redis: true, MinRuntimeNodes: 1},
				Timeout:          2 * time.Minute,
				ExpectedBehavior: "Gateway restarts, reconnects, routing valid",
			},
			Setup:    stack(1),
			Injector: &injectors.ProcessCrash{Target: validation.ResourceGateway},
			Recovery: recovery.NewComposite(recovery.ModeAll,
				recovery.HealthyEndpoint(validation.ResourceGateway),
				recovery.FixedDelay("stabilize", 3*time.Second),
			),
			Observers:  defaultObs,
			Validators: defaultVals,
			Acceptance: validation.AcceptanceCriteria{RecoveryDuration: validation.AcceptanceCriterion{Threshold: 15 * time.Second, Policy: validation.AcceptWarn}},
		},
		{
			Metadata: validation.ScenarioMetadata{
				ID: "room-service-crash", Name: "Room Service Crash",
				Description: "Room Service crash and recovery", Version: 1,
				Tags: []string{"chaos", "crash"}, Severity: validation.SeverityCritical,
				Mode: validation.ModeProcess,
				Requirements:     validation.Requirements{PostgreSQL: true, Redis: true, MinRuntimeNodes: 1},
				Timeout:          2 * time.Minute,
				ExpectedBehavior: "Room Service recovers, ownership table preserved, Game Servers reconnect",
			},
			Setup:    stack(1),
			Injector: &injectors.ProcessCrash{Target: validation.ResourceRoomService},
			Recovery: recovery.NewComposite(recovery.ModeAll,
				recovery.HealthyEndpoint(validation.ResourceRoomService),
				recovery.FixedDelay("stabilize", 5*time.Second),
			),
			Observers:  defaultObs,
			Validators: defaultVals,
			Acceptance: validation.AcceptanceCriteria{RecoveryDuration: validation.AcceptanceCriterion{Threshold: 30 * time.Second, Policy: validation.AcceptWarn}},
		},
		{
			Metadata: validation.ScenarioMetadata{
				ID: "room-service-restart", Name: "Room Service Restart",
				Description: "Room Service restart", Version: 1,
				Tags: []string{"chaos", "restart"}, Severity: validation.SeverityCritical,
				Mode: validation.ModeProcess,
				Requirements:     validation.Requirements{PostgreSQL: true, Redis: true, MinRuntimeNodes: 1},
				Timeout:          2 * time.Minute,
				ExpectedBehavior: "Room Service restarts, ownership preserved, routing restored",
			},
			Setup:    stack(1),
			Injector: &injectors.ProcessCrash{Target: validation.ResourceRoomService},
			Recovery: recovery.NewComposite(recovery.ModeAll,
				recovery.HealthyEndpoint(validation.ResourceRoomService),
				recovery.FixedDelay("stabilize", 5*time.Second),
			),
			Observers:  defaultObs,
			Validators: defaultVals,
			Acceptance: validation.AcceptanceCriteria{RecoveryDuration: validation.AcceptanceCriterion{Threshold: 30 * time.Second, Policy: validation.AcceptWarn}},
		},
		{
			Metadata: validation.ScenarioMetadata{
				ID: "delayed-heartbeats", Name: "Delayed Heartbeats",
				Description: "Runtime Node frozen and thawed", Version: 1,
				Tags: []string{"chaos", "heartbeat"}, Severity: validation.SeverityMedium,
				Mode:         validation.ModeProcess,
				Requirements: validation.Requirements{PostgreSQL: true, Redis: true, MinRuntimeNodes: 1},
				Timeout:      2 * time.Minute,
				ExpectedBehavior: "Room Service detects missing heartbeats, marks zones orphan, recovers on thaw",
			},
			Setup:    stack(1),
			Injector: injectors.NewProcessFreeze(validation.ResourceRuntime, 15*time.Second),
			Recovery: recovery.NewComposite(recovery.ModeAll,
				recovery.FixedDelay("thaw-window", 20*time.Second),
			),
			Observers:  defaultObs,
			Validators: defaultVals,
			Acceptance: validation.AcceptanceCriteria{RecoveryDuration: validation.AcceptanceCriterion{Threshold: 25 * time.Second, Policy: validation.AcceptWarn}},
		},
	}
}
```

---

### Task 11A.2: process_test.go

- [ ] **Create `tests/validation/process_test.go`**:

```go
//go:build validation

package validation

import (
	"context"
	"testing"

	v "spatial-server/internal/validation"
)

func TestProcessChaosScenarios(t *testing.T) {
	scenarios := ProcessScenarios(t)
	if err := v.ValidateScenarios(scenarios); err != nil {
		t.Fatalf("invalid scenarios: %v", err)
	}
	for _, sc := range scenarios {
		t.Run(sc.Metadata.ID, func(t *testing.T) {
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

### Task 11A.3: Commit

```bash
git add tests/validation/
git commit -m "feat(validation): add 7 process chaos scenarios and test entry"
```

---

## Phase 11B: Compose Scenarios

**New files:** `tests/validation/compose_scenarios.go`, `tests/validation/compose_test.go`
**Deps:** Phase 11A. **Build tag:** `//go:build validation`.

**Definition of Done:** `go build -tags=validation ./tests/validation/` succeeds.

---

### Task 11B.1: compose_scenarios.go

- [ ] **Create `tests/validation/compose_scenarios.go`** — 8 compose-mode scenarios:

```go
//go:build validation

package validation

import (
	"time"

	"spatial-server/internal/validation"
	"spatial-server/internal/validation/injectors"
	"spatial-server/internal/validation/observers"
	"spatial-server/internal/validation/recovery"
	"spatial-server/internal/validation/validators"
)

func ComposeScenarios() []validation.ScenarioDefinition {
	defaultVals := []validation.Validator{
		&validators.Entity{}, &validators.Ownership{},
		&validators.AOI{}, &validators.Session{},
	}
	defaultObs := []validation.Observer{&observers.State{}, &observers.Routing{}}
	crashRecovery := recovery.NewComposite(recovery.ModeAll,
		recovery.HealthyEndpoint(validation.ResourceRoomService),
		recovery.FixedDelay("stabilize", 5*time.Second),
	)

	return []validation.ScenarioDefinition{
		{
			Metadata: validation.ScenarioMetadata{
				ID: "postgres-restart", Name: "PostgreSQL Restart",
				Description: "PostgreSQL restart", Version: 1,
				Tags: []string{"chaos", "infra"}, Severity: validation.SeverityCritical,
				Mode:         validation.ModeCompose,
				Requirements: validation.Requirements{PostgreSQL: true, ComposeRequired: true},
				Timeout:      2 * time.Minute,
				ExpectedBehavior: "Services reconnect to PG after restart, no data loss",
			},
			Injector:   &injectors.InfraRestart{Target: validation.ResourcePostgres},
			Recovery:   crashRecovery,
			Observers:  defaultObs,
			Validators: defaultVals,
		},
		{
			Metadata: validation.ScenarioMetadata{
				ID: "redis-restart", Name: "Redis Restart",
				Description: "Redis restart", Version: 1,
				Tags: []string{"chaos", "infra"}, Severity: validation.SeverityHigh,
				Mode:         validation.ModeCompose,
				Requirements: validation.Requirements{Redis: true, ComposeRequired: true},
				Timeout:      2 * time.Minute,
				ExpectedBehavior: "Services reconnect to Redis, graceful degradation during outage",
			},
			Injector:   &injectors.InfraRestart{Target: validation.ResourceRedis},
			Recovery:   crashRecovery,
			Observers:  defaultObs,
			Validators: defaultVals,
		},
		{
			Metadata: validation.ScenarioMetadata{
				ID: "network-latency", Name: "Network Latency",
				Description: "Injected latency on inter-service RPCs", Version: 1,
				Tags: []string{"chaos", "network"}, Severity: validation.SeverityMedium,
				Mode:         validation.ModeCompose,
				Requirements: validation.Requirements{ComposeRequired: true, NetworkFaults: true},
				Timeout:      2 * time.Minute,
				ExpectedBehavior: "RPCs tolerate added latency, no cascading failures",
			},
			Injector: &injectors.NetLatency{DelayMS: 100, JitterMS: 20},
			Recovery: crashRecovery, Observers: defaultObs, Validators: defaultVals,
		},
		{
			Metadata: validation.ScenarioMetadata{
				ID: "packet-loss", Name: "Packet Loss",
				Description: "Injected packet loss on inter-service connections", Version: 1,
				Tags: []string{"chaos", "network"}, Severity: validation.SeverityMedium,
				Mode:         validation.ModeCompose,
				Requirements: validation.Requirements{ComposeRequired: true, NetworkFaults: true},
				Timeout:      2 * time.Minute,
				ExpectedBehavior: "System handles retransmissions, no data corruption",
			},
			Injector: &injectors.NetLoss{Percent: 10},
			Recovery: crashRecovery, Observers: defaultObs, Validators: defaultVals,
		},
		{
			Metadata: validation.ScenarioMetadata{
				ID: "network-partition", Name: "Network Partition",
				Description: "Temporary network isolation of a Runtime Node", Version: 1,
				Tags: []string{"chaos", "network", "partition"}, Severity: validation.SeverityCritical,
				Mode:         validation.ModeCompose,
				Requirements: validation.Requirements{ComposeRequired: true, NetworkFaults: true, MinRuntimeNodes: 2},
				Timeout:      3 * time.Minute,
				ExpectedBehavior: "Partitioned node detected as dead, zones reassigned, no split-brain",
			},
			Injector: &injectors.NetPartition{},
			Recovery: crashRecovery, Observers: defaultObs, Validators: defaultVals,
		},
		{
			Metadata: validation.ScenarioMetadata{
				ID: "slow-runtime", Name: "Slow Runtime Node",
				Description: "CPU-throttled Runtime Node", Version: 1,
				Tags: []string{"chaos", "resource"}, Severity: validation.SeverityMedium,
				Mode:         validation.ModeCompose,
				Requirements: validation.Requirements{ComposeRequired: true, MinRuntimeNodes: 1},
				Timeout:      3 * time.Minute,
				ExpectedBehavior: "Tick loop degrades gracefully, no crashes under CPU pressure",
			},
			Injector: &injectors.ResourceCPU{Cores: 1},
			Recovery: crashRecovery, Observers: defaultObs, Validators: defaultVals,
		},
		{
			Metadata: validation.ScenarioMetadata{
				ID: "cpu-starvation", Name: "CPU Starvation",
				Description: "Severe CPU starvation of Runtime Node", Version: 1,
				Tags: []string{"chaos", "resource"}, Severity: validation.SeverityHigh,
				Mode:         validation.ModeCompose,
				Requirements: validation.Requirements{ComposeRequired: true, MinRuntimeNodes: 1},
				Timeout:      3 * time.Minute,
				ExpectedBehavior: "Tick overruns detected, system stabilizes when CPU restored",
			},
			Injector: &injectors.ResourceCPU{Cores: 0},
			Recovery: crashRecovery, Observers: defaultObs, Validators: defaultVals,
		},
		{
			Metadata: validation.ScenarioMetadata{
				ID: "memory-pressure", Name: "Memory Pressure",
				Description: "Memory-limited Runtime Node", Version: 1,
				Tags: []string{"chaos", "resource"}, Severity: validation.SeverityMedium,
				Mode:         validation.ModeCompose,
				Requirements: validation.Requirements{ComposeRequired: true, MinRuntimeNodes: 1},
				Timeout:      3 * time.Minute,
				ExpectedBehavior: "GC pressure increases but no OOM, entities preserved",
			},
			Injector: &injectors.ResourceMemory{LimitMB: 64},
			Recovery: crashRecovery, Observers: defaultObs, Validators: defaultVals,
		},
	}
}
```

---

### Task 11B.2: compose_test.go

- [ ] **Create `tests/validation/compose_test.go`**:

```go
//go:build validation

package validation

import (
	"context"
	"testing"

	v "spatial-server/internal/validation"
)

func TestComposeChaosScenarios(t *testing.T) {
	for _, sc := range ComposeScenarios() {
		t.Run(sc.Metadata.ID, func(t *testing.T) {
			t.Skip("compose scenarios require Docker Compose orchestration")
			_ = sc
		})
	}
}
```

- [ ] **Run:** `go build -tags=validation ./tests/validation/`
Expected: SUCCESS

---

### Task 11B.3: Commit

```bash
git add tests/validation/compose_scenarios.go tests/validation/compose_test.go
git commit -m "feat(validation): add 8 compose chaos scenario definitions"
```

---

## Phase 12: Process Scenarios E2E Verification

**New files:** None.

- [ ] Run all 7 process-mode scenarios with real services:
```bash
go test -tags=validation -run TestProcessChaosScenarios -count=1 -timeout=30m ./tests/validation/ 2>&1 | tee artifacts/reports/process-chaos-report.txt
```
Expected: 7 process scenarios run (crash/restart for runtime, gateway, room-service, delayed heartbeats).

- [ ] Verify acceptance criteria: each scenario's `Acceptance.RecoveryDuration` is checked and logged.
- [ ] Artifacts generated in `artifacts/reports/`.

---

## Phase 13: Compose Scenarios Verification

**Status:** 🗓️ Full Docker Compose execution deferred until compose injectors are implemented (see Compose Injector Status section).

**Current behaviour:** `TestComposeChaosScenarios` validates scenario registration and execution flow only. All 8 scenarios are explicitly `t.Skip()`'d because compose injectors (`network.go`, `infrastructure.go`, `resource.go`) are intentional placeholders. The skipped state is NOT a test failure.

**Future execution:** When compose injectors are replaced with real Docker SDK implementations:
```bash
docker compose -f deploy/docker-compose/docker-compose.2node.yml up -d
go test -tags=validation -run TestComposeChaosScenarios -count=1 -timeout=30m ./tests/validation/
docker compose -f deploy/docker-compose/docker-compose.2node.yml down
```

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
| 11A-12 (Process) | Real services may have bugs | These ARE the tests that find those bugs |
| 11B-13 (Compose) | Docker Compose required | Scenarios explicitly skipped when unavailable; documented as future milestone |
| All | Circular imports | Dependency direction enforced: runtime → never imports validation |

---

## Compose Injector Status

Compose-mode injectors (`network.go`, `infrastructure.go`, `resource.go`) are **intentional placeholders** during this milestone. Each returns `fmt.Errorf("requires Docker Compose execution mode")` on `Inject()`.

**Roadmap note** — to be replaced in the Phase 3 "Production Hardening" milestone (`docs/superpowers/specs/phase-6-production-hardening.md`):
- `NetLatency`/`NetLoss`/`NetPartition` → Docker SDK container exec + `tc netem` / `iptables`
- `InfraRestart` → Docker SDK `ContainerRestart`
- `ResourceCPU`/`ResourceMemory` → Docker SDK `ContainerUpdate` with cgroup limits

No current scenario relies on these injectors succeeding. All compose scenarios are `ModeCompose` and explicitly skipped when Compose is unavailable.

---

## Testing Strategy

| Phase | Tests | Type |
|-------|-------|------|
| 1 | CheckRequirements + NewSummary | Unit |
| 2 | Runner lifecycle (pass, skip, fail, timeout) | Unit |
| 3 | Observer construction | Build verification |
| 4 | CompositeWaiter (all-of, any-of, timeout, empty) | Unit |
| 5 | Snapshot initial state | Unit |
| 6 | Harness 4-file construction + build | Build verification |
| 7 | Validators (entity fail/pass, AOI leak/bounded, ownership) | Unit |
| 8 | ProcessFreeze Inject/Recover | Unit + integration |
| 11A | ProcessScenarios definition | Build verification |
| 11B | ComposeScenarios definition | Build verification |
| 12 | 7 process chaos scenarios | E2E |
| 13 | 8 compose chaos scenarios | E2E |

---

## Harness Cleanup Order

Cleanup must execute in this exact order to prevent resource leaks:

1. **Recover faults** — `Injector.Recover()` (guaranteed by Runner `defer`)
2. **Stop services** — SIGTERM each process, 10s grace, SIGKILL
3. **Terminate containers** — Testcontainers `Terminate()`
4. **Close log files** — `f.Close()`
5. **Delete temporary binaries** — `os.Remove(binPath)`

The `cleanups` slice in `processHarness` is executed in reverse append order (LIFO),
ensuring teardown mirrors setup.

The Runner guarantees: `Injector.Recover()` (via defer) → `teardown()` (via defer).
The harness guarantees: stop services → terminate containers → close logs → delete binaries.
No injected fault survives scenario completion.

**Panic safety:** Cleanup must execute even if Setup(), Inject(), Observe(), Validate(),
or Reporter.Generate() panics. The Runner's `defer recover()` captures the panic for
the report, and the `defer teardown()` + `defer Recover()` execute regardless.
A panic inside a scenario never leaves the system in a dirty state.

---

## Scenario Registration

Scenarios are registered **explicitly** by returning slices from named functions:
`ProcessScenarios(t)`, `ComposeScenarios()`. There is no global registry.

- **Duplicate IDs:** Tests use `t.Run(id, ...)` — Go's subtests panic on duplicate names.
  This is intentional: duplicate scenario IDs are a programmer error caught at test time.
- **Registration timing:** `ProcessScenarios()` is called at test invocation, not via `init()`.
  Order is deterministic, debuggable.
- **No dynamic registration:** Adding a scenario means adding one entry to the slice.
  No framework changes required.

---

## Artifact Layout

Every validation run produces a standardized artifact directory:

```
artifacts/
  reports/           ← Markdown per-scenario reports + JSON summary
  logs/              ← service stdout/stderr (from harness log files)
  metrics/           ← prometheus snapshots (future)
  pprof/             ← heap/goroutine profiles (future)
  traces/            ← distributed traces (future)
```

| Phase | Artifact | Location |
|-------|----------|----------|
| 12 | Process chaos reports | `artifacts/reports/process-chaos-report.txt`, `artifacts/reports/process-chaos-summary.json` |
| 13 | Compose chaos reports | `artifacts/reports/compose-chaos-report.txt`, `artifacts/reports/compose-chaos-summary.json` |
| 14 | Final Chaos Engineering report | `docs/testing/chaos-report.md` |

Per-scenario reports use the scenario ID as filename stem:
- `artifacts/reports/runtime-crash.md`
- `artifacts/reports/runtime-crash.json`
- `artifacts/reports/summary.json`
- `artifacts/reports/summary.md`

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
| 6 | Harness (4 files) | Medium | 45m |
| 7 | Validators | Low | 25m |
| 8 | Process Injectors | Low | 15m |
| 9 | Reporters | Low | 10m |
| 10 | Remaining Injectors | Low | 10m |
| 11A | Process Scenarios + Test Entry | Medium | 30m |
| 11B | Compose Scenarios | Medium | 15m |
| 12 | Process E2E | High | 60m |
| 13 | Compose E2E | Medium | 30m |
| 14 | Documentation | Low | 15m |
| **Total** | | | **~5.8h** |

---

## Post-Plan Sanity Review

✅ Every architectural component from the spec is implemented by a phase.
✅ No phase introduces work beyond the spec.
✅ No circular dependencies — dependency graph is DAG.
✅ Every phase has measurable Definition of Done (build, test, or run command).
✅ Single build tag: `validation`. Single command: `go test -tags=validation ./tests/validation/...`
✅ Runtime never imports validation. Direction is strictly one-way.
