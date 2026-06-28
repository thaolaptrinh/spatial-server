//go:build validation

package validation

import (
	"time"

	"github.com/thaolaptrinh/spatial-server/internal/validation"
	"github.com/thaolaptrinh/spatial-server/internal/validation/injectors"
	"github.com/thaolaptrinh/spatial-server/internal/validation/observers"
	"github.com/thaolaptrinh/spatial-server/internal/validation/recovery"
	"github.com/thaolaptrinh/spatial-server/internal/validation/validators"
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
