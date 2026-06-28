# Production SLO — Service Level Objectives

> **Last Updated:** 2026-06-28  |  **v0.1.0-alpha**

## Purpose

Measurable objectives the Spatial Runtime must meet in production. Every value
is derived from actual benchmark data or derived from the architecture spec
(not speculated). Thresholds are **Target** (acceptable steady-state),
**Warning** (requires investigation), **Critical** (service degradation —
alarm or autoscaler must react).

## Scheduler

| Metric | Target | Warning | Critical |
|---|---|---|---|
| Tick duration (p95) | < 25 ms | > 25 ms | > 50 ms (tick overrun) |
| Tick overruns (per min) | 0 | > 0 in any 1-min window | sustained >0 for 5 min |
| Measured delta correctness | ≡ wall-clock elapsed (capped 250ms) | N/A | N/A (guaranteed by code) |

## Round-trip latency

| Metric | Target | Warning | Critical |
|---|---|---|---|
| Client→Runtime→Client p95 | < 100 ms | > 200 ms | > 500 ms |
| Gateway connect p95 | < 100 ms | > 200 ms | > 500 ms |

Measured values from the `benchmarks/reports/e2e-distributed.md` validation
run: 50 concurrent clients → round-trip p95 = 33ms (within target).

## AOI

| Metric | Target | Warning | Critical |
|---|---|---|---|
| `EntitiesInRange` p95 (per zone, ≤100 entities) | < 200 µs | > 1 ms | > 5 ms |
| Ghost count per node | < entity count | growing monotonically | > entity count (definite leak) |
| Ghost expiry | ≤ 30 s (6× TTL) | N/A | N/A (guaranteed by TTL sweep) |

## Backpressure

| Metric | Target | Warning | Critical |
|---|---|---|---|
| Inbox depth | < 25% (1024 of 4096) | > 50% for 1 min | > 90% sustained |
| Events depth | < 25% (1024 of 4096) | > 50% for 1 min | > 90% sustained |
| Cmd queue depth | < 25% (64 of 256) | > 50% (tick loop sluggish) | > 90% (stalled) |
| Data-plane drops (5-min window) | 0 | > 0 | sustained >0 for 1 min |
| Control-plane drops (any) | 0 | 1 (log+count, never silent) | > 0 sustained |

## Runtime Node registration & heartbeat

| Metric | Target | Warning | Critical |
|---|---|---|---|
| Heartbeat interval | 5 s | missed > 1 consecutive | missed > 3 (node declared dead, zones reassigned) |
| Registration latency | < 2 s | > 5 s | timeout (configurable) |

## Resource

Values at 1000 entities (single runtime node, measured via `benchmarks/reports/capacity.md`):

| Metric | Target | Warning | Critical |
|---|---|---|---|
| Memory (heap) per 1000 entities | < 10 MB | growth > 1 MB/min at steady state | > 100 MB without additional load |
| Goroutines | < 50 | growth > 10/min | > 500 (runaway leak) |
| GC runs per tick | < 1.0 / tick | > 2.0 / tick | > 5.0 / tick (allocation explosion) |

## References

- [Runtime invariants](../architecture/runtime-invariants.md)
- [Benchmark reports](../../benchmarks/reports/capacity.md)
- [E2E benchmark report](../../benchmarks/reports/e2e-distributed.md)
- [ADR-019: Observability](../adr/019-observability.md)
- [Deployment topology](../architecture/deployment-topology.md)
