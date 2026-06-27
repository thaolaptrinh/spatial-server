# ADR 007: Autoscaling

## Status

Approved

## Context

The platform must scale Game Server and Gateway instances based on load, without manual intervention.

## Problem

Manual capacity management is error-prone and wasteful. The platform needs automated scaling to handle variable player load without over-provisioning or under-provisioning Game Server and Gateway instances.

## Decision

### Metrics

| Metric | Threshold | Action |
|--------|-----------|--------|
| CPU (Game Server) | >70% for 30s | Spawn new Game Server |
| Memory (Game Server) | >80% for 30s | Spawn new Game Server |
| Connections per Gateway | >10K for 30s | Spawn new Gateway |
| Zone ownership imbalance | stddev > threshold | Rebalance zones (no new server) |
| Room count per Game Server | >N (configurable) | Reassign zones |

### Scale-Up Flow

1. Room Service detects metric threshold breached.
2. Room Service signals orchestrator (Docker Compose scale, or HPA in K3s).
3. New Game Server starts and registers (JOINING state).
4. Room Service selects most loaded ACTIVE servers.
5. Room Service picks zones to transfer (least loaded zones first).
6. Zone migration executes (see [ADR-002](002-zone-migration.md)).
7. New server reaches ACTIVE state.

### Scale-Down Flow

1. Room Service detects sustained low load.
2. Room Service selects a DRAINING candidate (least zones).
3. All zones transferred to other servers.
4. Candidate transitions to SHUTDOWN and exits.

### Production Path

- Dev/staging: Room Service manages scale manually (CLI or API call).
- Production (K3s): HPA with custom metrics (Prometheus adapter) triggers pod scaling.
- Room Service provides `/metrics` endpoint for Prometheus scraping.

## Alternatives

1. **Predictive scaling**: Use historical load data to pre-scale before demand arrives. More efficient but complex to model and requires training data.
2. **Reactive scaling (HPA)**: Scale based on current real-time metrics. Simpler to implement but slower to respond to sudden spikes.
3. **Schedule-based scaling**: Scale on a fixed schedule based on known player patterns. Ineffective for unpredictable or viral load events.

## Tradeoffs

- Reactive scaling is simplest but may lag behind sudden load spikes, causing brief performance degradation.
- Zone rebalancing adds latency to scale-up (1-2s per zone transferred).
- Metric thresholds need careful tuning to avoid thrashing (rapid scale-up/down cycles).

## Consequences

- Scale-up latency = zone transfer time (~1-2s per zone).
- Scale-down must be graceful — never kill a Game Server with active zones.
- Need a way to trigger scaling in Docker Compose (manual for now).

## Future Considerations

- Predictive scaling using ML models trained on historical load patterns.
- Over-provision buffer for rapid spike absorption.
- Autoscaling alerts and dry-run mode for safe threshold tuning.
- Custom Prometheus metrics for spatial-server-specific load signals.

## Replaces

None.
