//go:build validation

package validation

import (
	"testing"
	"time"

	"github.com/thaolaptrinh/spatial-server/internal/validation"
	"github.com/thaolaptrinh/spatial-server/internal/validation/harness"
	"github.com/thaolaptrinh/spatial-server/internal/validation/injectors"
	"github.com/thaolaptrinh/spatial-server/internal/validation/observers"
	"github.com/thaolaptrinh/spatial-server/internal/validation/recovery"
	"github.com/thaolaptrinh/spatial-server/internal/validation/validators"
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
