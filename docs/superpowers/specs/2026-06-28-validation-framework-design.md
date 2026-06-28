# Validation Framework Design — v1.0

> **Last Updated:** 2026-06-28

## Purpose

Define the architecture for a generic Validation Framework that serves as the single execution engine for all validation activities across the repository: chaos testing, integration testing, benchmarking, load testing, soak testing, and future validation suites.

The framework validates operational correctness under failure — it does not introduce new product features.

---

## Architecture

### Package Layout

```
internal/validation/               ← framework core (imports runtime, not reverse)
  runner.go                        ← ScenarioRunner — single execution engine
  scenario.go                      ← ScenarioDefinition + all interfaces
  measure.go                       ← shared Stats + RecoveryStats model
  report.go                        ← ValidationReport + SummaryReport (JSON/Markdown)

internal/validation/harness/
  harness.go                       ← //go:build validation, single infrastructure harness

internal/validation/injectors/
  process.go                       ← SIGSTOP/CONT/KILL, restart
  network.go                       ← latency, loss, partition via tc/iptables (compose)
  infrastructure.go                ← PG restart, Redis restart (compose)
  resource.go                      ← CPU/memory stress (compose)

internal/validation/validators/
  entity.go                        ← I-01, I-03, I-04
  ownership.go                     ← O-01, O-02, O-03
  aoi.go                           ← G-01, G-02, G-03
  session.go                       ← reconnect, disconnect cleanup
  scheduler.go                     ← T-01, T-02, T-03, T-04

internal/validation/observers/
  state.go                         ← service state observation
  metrics.go                       ← metric scraping observation
  routing.go                        ← routing correctness observation

internal/validation/recovery/
  waiter.go                        ← RecoveryWaiter abstraction
  conditions.go                    ← heartbeat, healthy, ownership, drained, timeout

internal/game/
  probes.go                        ← //go:build validation, Snapshot() for diagnostics

tests/validation/
  scenarios.go                     ← ChaosScenarios() returns 15 definitions
  chaos_test.go                    ← TestChaos iterates scenarios, calls Runner
```

### Dependency Direction

```
tests/validation/                  ← scenario definitions (data, no logic)
    ↓
internal/validation/*              ← framework interfaces + runner (no build tag)
    ↓
internal/game/probes.go           ← //go:build validation only
    ↓
internal/game/*                    ← runtime (never imports validation)
```

Runtime never depends on the Validation Framework. The framework may depend on the Runtime.
Direction is strictly one-way.

### Build Tags

| Package | Tag | Purpose |
|---------|-----|---------|
| `internal/validation/` | none | Interfaces, types, runner — always compiled |
| `internal/validation/harness/` | `validation` | Testcontainers + Docker SDK |
| `internal/validation/injectors/` | `validation` | Docker SDK, stress-ng |
| `internal/validation/observers/` | `validation` | gRPC client, WS client |
| `internal/validation/recovery/` | `validation` | gRPC client |
| `internal/game/probes.go` | `validation` | Diagnostic snapshot (zero-cost in production) |
| `tests/validation/` | `validation` | Chaos scenario definitions + test entry point |

Single command: `go test -tags=validation ./tests/validation/...`

---

## Core Interfaces

### Infrastructure

```go
type ResourceID string

// Resource types — NOT deployment instances.
// The harness resolves concrete instances dynamically.
const (
    ResourcePostgres    ResourceID = "postgres"
    ResourceRedis       ResourceID = "redis"
    ResourceRoomService ResourceID = "room-service"
    ResourceRuntime     ResourceID = "runtime"      // any game-server
    ResourceGateway     ResourceID = "gateway"
)

type Infrastructure interface {
    Endpoint(id ResourceID) (string, error)
    Process(id ResourceID) (*os.Process, error)
    ProcessByIndex(id ResourceID, n int) (*os.Process, error)
    Database(id ResourceID) (string, error)
    RuntimeNodes() int
    DialServices(ids ...ResourceID) error
    Close() error
}
```

Resource identifiers represent **types**, not deployment instances. `ResourceRuntime` + `ProcessByIndex(ResourceRuntime, 0)` replaces ResourceGameServer1/ResourceGameServer2. Adding a 3rd runtime node requires zero framework changes — the harness simply returns it from RuntimeNodes().

### Scenario Definition

```go
type ExecutionMode string

const (
    ModeProcess ExecutionMode = "process"  // process-level faults only
    ModeCompose ExecutionMode = "compose"  // network/resource faults via Docker Compose
    ModeAny     ExecutionMode = "any"      // runner selects available mode
)

type Severity string

const (
    SeverityLow      Severity = "low"
    SeverityMedium   Severity = "medium"
    SeverityHigh     Severity = "high"
    SeverityCritical Severity = "critical"
)

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

type Requirements struct {
    MinRuntimeNodes int
    ComposeRequired bool
    PostgreSQL      bool
    Redis           bool
    NetworkFaults   bool
}

type SetupFunc func(t *testing.T) (Infrastructure, func(), error)

type ScenarioDefinition struct {
    Metadata   ScenarioMetadata
    Setup      SetupFunc
    Injector   Injector
    Observers  []Observer
    Validators []Validator
    Recovery   RecoveryWaiter
    Reporter   Reporter
}
```

Scenarios are pure data. No logic lives in definitions. The Runner is unchanged as scenarios grow.

### Scenario Requirements & Skippable

```go
// CheckRequirements verifies the current environment satisfies a scenario's needs.
// If not, the scenario is reported as SKIPPED (not FAILED).
func CheckRequirements(infra Infrastructure, req Requirements) (met bool, reason string)
```

If minimum runtime nodes are unavailable, or Docker Compose is required but not running, the
scenario is skipped, not failed. This makes the framework portable across CI, local development,
and future deployment environments.

### Injector

```go
type Injector interface {
    Name() string
    Inject(ctx context.Context, infra Infrastructure) error
    Recover(ctx context.Context, infra Infrastructure) error
}
```

Inject applies fault. Recover rolls back. Both receive Infrastructure — the Runner never knows how faults are applied.

### RecoveryWaiter

```go
type RecoveryWaiter interface {
    Wait(ctx context.Context, infra Infrastructure) error
}
```

Encapsulates polling strategy internally. The Runner calls `Wait()` — no polling logic in the runner.
Recovery conditions are composed inside the waiter implementation (e.g., `AllOf(heartbeat, healthy)`,
`AnyOf(ownership, timeout)`).

```go
type RecoveryCondition interface {
    Name() string
    Met(ctx context.Context, infra Infrastructure) (bool, error)
}
```

Built-in conditions: `HeartbeatRestored`, `HealthyEndpoint`, `OwnershipStabilized`, `QueueDrained`, `FixedDelay`, `Timeout`.

### Observer

```go
type Evidence struct {
    Timestamp time.Time
    Source    string   // observer name
    Kind      string   // "state", "metric", "event"
    Key       string
    Value     string
}

type Observer interface {
    Name() string
    Observe(ctx context.Context, infra Infrastructure) ([]Evidence, error)
}
```

Observers collect evidence. They do not judge correctness. Evidence is timestamped and attributed.

### Validator

```go
type ValidationResult struct {
    Validator       string
    Passed          bool
    EvidenceIndices []int    // indices into report.Evidence array
    Detail          string   // human-readable explanation
}

type Validator interface {
    Name() string
    Validate(evidence []Evidence) ValidationResult
}
```

Validators evaluate evidence. Every PASS/FAIL cites supporting evidence by index. Reports answer "why?" without reading logs.

### Reporter

```go
type Reporter interface {
    Generate(report *ValidationReport) error
}
```

Built-in reporters: `JSONReporter`, `MarkdownReporter`.

---

## Runner Lifecycle

```
Setup(ctx)
    ↓
CheckRequirements(infra, req)
    ├── unmet → OutcomeSkipped (skip remaining phases)
    └── met
            ↓
        Injector.Inject(ctx)
            ↓
        RecoveryWaiter.Wait(ctx)
            ↓
        Observers.Observe(ctx)
            ↓
        Validators.Validate(evidence)
            ↓
        Reporter.Generate(report)
            ↓
        Teardown(cleanup)
```

The Runner orchestrates. It owns no business logic. Every phase delegates to the scenario's configured interfaces.

---

## Measurement

```go
type TickStats struct {
    Mean   time.Duration
    P50    time.Duration
    P95    time.Duration
    P99    time.Duration
    Max    time.Duration
    Count  int
}

type RecoveryStats struct {
    Duration           time.Duration
    OwnershipRestoredAt time.Time
    FirstHealthyAt     time.Time
}

type Measurement struct {
    TickDurations    []time.Duration
    QueueDepths      map[string][]int
    DropCounts       map[string]int
    EventCounts      map[string]int
    Recovery         RecoveryStats
}
```

Shared model. Benchmark, chaos, integration — all report through the same measurement layer.

---

## Validation Report

```go
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
    Framework     FrameworkMeta
    Scenario      ScenarioMetadata
    Outcome       Outcome
    Duration      time.Duration
    Evidence      []Evidence
    Validations   []ValidationResult
    Measurement   Measurement
    RootCause     string
}
```

Framework metadata (version, timestamp) enables future cross-run comparison and regression tracking.

---

## Report Summary

In addition to per-scenario reports, an execution summary aggregates across all scenarios:

```go
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
        Count       int
        MinDuration time.Duration
        MaxDuration time.Duration
        MeanDuration time.Duration
    }
    Invariants struct {
        Total     int
        Passed    int
        Failed    int
    }
    Scenarios []SummaryEntry
}

type SummaryEntry struct {
    Name     string
    Outcome  Outcome
    Duration time.Duration
    Reason   string // "requirements not met: ComposeRequired" for skipped
}
```

The summary is CI-optimized: compact enough for pipeline output, while detailed per-scenario reports remain available for debugging.

---

## Scenario Catalog

```go
func ChaosScenarios() []ScenarioDefinition
func IntegrationScenarios() []ScenarioDefinition
```

Scenarios are registered explicitly by returning a slice from a named function.
The test runner calls `ChaosScenarios()` and iterates. No `init()` magic — deterministic, explicit, debuggable.

```go
func ChaosScenarios() []ScenarioDefinition {
    return []ScenarioDefinition{
        {
            Metadata: ScenarioMetadata{
                Name: "gateway-crash",
                // ...
            },
            Injector:  &injectors.ProcessCrash{Resource: ResourceGateway},
            Recovery:  &recovery.CompositeWaiter{...},
            Observers: []Observer{&observers.State{}, &observers.Routing{}},
            Validators: []Validator{&validators.Entity{}, &validators.Ownership{}},
        },
        // ...
    }
}
```

Adding a new scenario adds one entry to the slice. The Runner never changes.

---

## Internal Probes

```go
//go:build validation

package game

type Snapshot struct {
    EntityCount int
    GhostCount  int
    ZoneOwners  map[types.ZoneID]types.ServerID
    QueueDepths map[string]int
}

func (g *Game) Snapshot() Snapshot {
    g.mu.RLock()
    defer g.mu.RUnlock()
    // ... capture read-only diagnostic state
}
```

Build-tag gated. Zero-cost in production. Observers call `Snapshot()` through the Game Server's gRPC interface when built with the validation tag. No permanent debug service.

---

## Scenarios (15 total)

| # | Scenario | Injector | Mode |
|---|----------|----------|------|
| 1 | Runtime Node crash | ProcessCrash | Process |
| 2 | Runtime Node restart | ProcessRestart | Process |
| 3 | Gateway crash | ProcessCrash | Process |
| 4 | Gateway restart | ProcessRestart | Process |
| 5 | Room Service crash | ProcessCrash | Process |
| 6 | Room Service restart | ProcessRestart | Process |
| 7 | PostgreSQL restart | InfraRestart | Compose |
| 8 | Redis restart | InfraRestart | Compose |
| 9 | Network latency | NetLatency | Compose |
| 10 | Packet loss | NetLoss | Compose |
| 11 | Network partition | NetPartition | Compose |
| 12 | Delayed heartbeats | ProcessFreeze[auto-recover] | Process |
| 13 | Slow Runtime Node | ResourceCPU | Compose |
| 14 | CPU starvation | ResourceCPU | Compose |
| 15 | Memory pressure | ResourceMemory | Compose |

---

## File Count

| Layer | Files | Purpose |
|-------|-------|---------|
| Core | 4 | runner, scenario, measure, report |
| Harness | 1 | single infrastructure harness |
| Injectors | 4 | process, network, infrastructure, resource |
| Validators | 5 | entity, ownership, aoi, session, scheduler |
| Observers | 3 | state, metrics, routing |
| Recovery | 2 | waiter, conditions |
| Probes | 1 | game snapshot (build-tag gated) |
| Tests | 2 | scenario definitions, test entry point |
| **Total** | **22** | |

---

## Architecture Freeze

The Validation Framework architecture is frozen as of this document.

From this point forward:
- Do not redesign package boundaries.
- Do not introduce new framework abstractions.
- Do not refactor the execution engine.

Future work may only add:
- Scenario definitions (entries in `ChaosScenarios()`, etc.)
- Injectors (new files in `internal/validation/injectors/`)
- Recovery conditions (new files in `internal/validation/recovery/`)
- Observers (new files in `internal/validation/observers/`)
- Validators (new files in `internal/validation/validators/`)
- Reporters (new files, implementing the `Reporter` interface)

The core framework — `runner.go`, `scenario.go`, `measure.go`, `report.go` — must remain stable
for the lifetime of the project.

---

## Constraints

- Do not redesign the platform.
- Do not introduce new product features.
- Do not optimize.
- Maintain a green repository.
- One build tag: `validation`.
- Single command: `go test -tags=validation ./tests/validation/...`
- Runtime never depends on the Validation Framework.

---

## References

- [Runtime Invariants](../architecture/runtime-invariants.md)
- [ADR-011: Failure Recovery](../adr/011-failure-recovery.md)
- [Production SLO](../operations/slo.md)
- [Chaos Testing Guide](../testing/chaos-testing.md)
- [Benchmark Reports](../../benchmarks/reports/)
- [Integration Test Harness](../../tests/integration/harness.go)
- [Distributed Correctness Tests](../../internal/game/distributed_test.go)
