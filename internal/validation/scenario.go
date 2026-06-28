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

type SetupFunc func() (Infrastructure, func(), error)

type AcceptancePolicy string

const (
	AcceptWarn AcceptancePolicy = "warn"
	AcceptFail AcceptancePolicy = "fail"
)

type AcceptanceCriterion struct {
	Threshold time.Duration
	Policy    AcceptancePolicy
}

type AcceptanceCriteria struct {
	RecoveryDuration     AcceptanceCriterion
	OwnershipConvergence AcceptanceCriterion
	TickP95              AcceptanceCriterion
	ReconnectTime        AcceptanceCriterion
	QueueDrainTime       AcceptanceCriterion
}

type ScenarioDefinition struct {
	Metadata   ScenarioMetadata
	Setup      SetupFunc
	Injector   Injector
	Observers  []Observer
	Validators []Validator
	Recovery   RecoveryWaiter
	Reporter   Reporter
	Acceptance AcceptanceCriteria
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

const (
	EvKeyEntityCount      = "entity-count"
	EvKeyGhostCount       = "ghost-count"
	EvKeyCmdDrops         = "cmd-drops"
	EvKeyZoneOwnerCount   = "zone-owner-count"
	EvKeyDisconnectedCount = "disconnected-count"
	EvKeyRuntimeNodes     = "runtime-nodes"
	EvKeyRoomServiceAddr  = "room-service-endpoint"
	EvKeyGatewayAddr      = "gateway-endpoint"
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

func EvidenceMap(evidence []Evidence) map[string]Evidence {
	m := make(map[string]Evidence, len(evidence))
	for _, e := range evidence {
		m[e.Key] = e
	}
	return m
}

type ObserverPolicy string

const (
	PolicyRequired ObserverPolicy = "required"
	PolicyOptional ObserverPolicy = "optional"
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
