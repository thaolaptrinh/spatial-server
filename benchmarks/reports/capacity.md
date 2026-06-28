# Capacity Benchmark Report

> Generated: 2026-06-28T07:46:06+07:00  |  ticks/stage: see table  |  pattern: mixed (60% moving)

Tick budget per stage = the configured tick rate (default 50ms / 20Hz). `tick p95` over budget means the runtime can no longer sustain the rate.

| Users | Ticks | Tick mean | Tick p50 | Tick p95 | Tick p99 | Tick max | Over budget? | Allocs/tick | Heap (MB) | GCs | GC pause | Goroutines | Drops |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| 50 | 300 | 119µs | 102µs | 205µs | 325µs | 981µs | no | 29.4k | 2.5 | 3 | 170µs | 3 | 0 |
| 100 | 300 | 428µs | 451µs | 783µs | 893µs | 1.46ms | no | 59.6k | 1.6 | 8 | 536µs | 3 | 0 |
| 250 | 300 | 1.22ms | 1.27ms | 2.04ms | 2.38ms | 3.08ms | no | 202.2k | 2.5 | 28 | 2.45ms | 3 | 0 |
| 500 | 300 | 2.94ms | 2.85ms | 4.86ms | 5.40ms | 6.92ms | no | 679.5k | 2.3 | 95 | 6.19ms | 3 | 0 |
| 1000 | 300 | 8.77ms | 7.94ms | 14.26ms | 15.95ms | 17.22ms | no | 2.86M | 4.8 | 252 | 17.08ms | 3 | 0 |

## Runtime events (last stage)

| Kind | Count |
|---|---|
| despawn | 36 |
| spawn | 9938 |

## Bottleneck analysis

- All measured stages sustained the tick budget (max p95 = 14.26ms).
- No latency cliff reached within the tested range.
- **Allocation pressure (heaviest stage):** 2.86M allocs/tick, 0.84 GC runs/tick, 17.08ms total GC pause over 300 ticks.
- Allocation scales ~linearly with entity count (see AOI micro-benchmarks). This is the leading indicator of the next bottleneck under denser clustering; it is the first candidate to optimize *only after* a CPU/heap profile confirms the hotspot.

## Profiling

Collect during runs with standard `go test` flags:
`go test -bench=. -benchmem -cpuprofile=cpu.out -memprofile=mem.out ./benchmarks/runtime/`
and `go tool pprof cpu.out`. Mutex/block/goroutine profiles are written next to this report by `TestCapacityReport`.
