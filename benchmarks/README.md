# Benchmarks

> **Last Updated:** 2026-06-28

## Purpose

Reproducible, evidence-based performance and capacity measurement for the
Spatial Runtime core. Every optimization must reference numbers produced here;
no speculative optimization.

The benchmark suite has two layers:

1. **Runtime harness** (`benchmarks/runtime/`) — drives a real `game.Game`
   in-process with a configurable number of simulated entities and movement
   patterns. Measures tick latency, allocations, GC, queue drops, goroutines.
   Reproducible and CI-friendly (no network, no Docker).
2. **Micro-benchmarks** (`internal/game/aoi` + `internal/game`) — tight
   `testing.B` benchmarks for AOI query/range/move, packet dispatch, and event
   publish. Source of truth for clean `ns/op` and `allocs/op` numbers.

The end-to-end WebSocket framework (`benchmarks/framework`) drives a deployed
Gateway for cross-node, full-stack load testing (requires the stack running).

## Running

### Capacity stages (50 → 1000 users, progressive)

```bash
go test ./benchmarks/runtime/ -run=TestCapacityReport -count=1 -timeout=300s
```

Writes `benchmarks/reports/capacity.md` plus `heap/goroutine/mutex/block` pprof
snapshots. Skipped under `-short`.

### Stage benchmarks (per-stage `go test -bench`)

```bash
go test ./benchmarks/runtime/ -bench=. -benchmem -run='^$'
```

Each stage reports `tick_p50_ms`, `tick_p95_ms`, `tick_p99_ms`, `overruns`.

### Micro-benchmarks

```bash
go test ./internal/game/aoi/ -bench=. -benchmem -run='^$'      # AOI evidence
go test ./internal/game/      -bench=. -benchmem -run='^$'     # dispatch/publish/tick
```

### Profiling during a run

```bash
go test ./benchmarks/runtime/ -bench=BenchmarkStage_1000 -benchmem \
  -cpuprofile=cpu.out -memprofile=mem.out -run='^$'
go tool pprof cpu.out        # interactive flamegraph: web command needs graphviz
```

`TestCapacityReport` additionally writes mutex/block/goroutine/heap profiles to
`benchmarks/reports/`.

## Harness configuration

`runtime.Config` is tunable: `Users`, `Pattern` (idle|walk|random|cluster|
hotspot|boundary|mixed), `MovingFrac`, `CellSize`, `Radius`, `WorldSize`,
`TickRate`, `UpdateEvery`, `Seed`. Patterns are deterministic given a fixed
seed, enabling reproducible runs.

## Latest measured result (summary)

See `reports/capacity.md` for the full, reproducible table. Headline (Ryzen
4600H, single runtime node, mixed 60% moving, 300 ticks/stage):

- All stages 50–1000 users sustained the 50ms (20Hz) tick budget.
- At 1000 users: tick mean ~10.5ms, p95 ~16.8ms, p99 ~18.6ms.
- Allocation scales ~linearly with entity count (2.86M allocs/tick at 1000);
  this is the data-derived leading indicator of the next bottleneck under
  denser clustering, and the first candidate to optimize **after** a profile
  confirms the hotspot.

## Optimization policy

1. Benchmark → 2. Profile → 3. Identify bottleneck → 4. Optimize → 5. Re-benchmark.
No step may be skipped. Regression checks: re-run the same stage benchmarks and
compare `tick_p95_ms`.
