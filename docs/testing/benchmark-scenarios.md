# Benchmark Scenarios

> **Last Updated:** 2026-06-26

## Scenarios

| Scenario | VClients | Duration | Validates |
|----------|----------|----------|-----------|
| Baseline | 0 | 5 min | Idle CPU/memory |
| Light | 100 | 5 min | AOI correctness |
| Medium | 1,000 | 10 min | Latency <100ms p95 |
| Heavy | 5,000 | 10 min | No dropped connections |
| Burst | 0→5,000 in 10s | 2 min | Connection rate |
| Stability | 2,000 | 60 min | Memory leak detection |

## CI Gate

- Light load scenario runs on every PR
- Any latency regression >10% blocks merge
- Full benchmark suite per milestone before production deployment

## Simulation Framework (Phase 2)

Build a dedicated framework capable of:
- Thousands of virtual WebSocket clients
- Configurable movement patterns (walk/run/teleport)
- Join/Leave at scale
- AOI correctness verification
- Per-RPC latency histograms (p50, p95, p99, p99.9)
- CPU / Memory / Network profiling via pprof

## References

- [Testing Strategy](strategy.md)
- [ADR-020](../adr/020-benchmark-strategy.md) — Benchmark Strategy
