# ADR 020: Benchmark Strategy

## Status

Accepted

## Context

Spatial Server's performance targets (10K connections per Gateway, <100ms p95 latency, 5K entities per Game Server) must be validated before production deployment. Without a systematic benchmark strategy, performance regression can go undetected, and capacity planning is guesswork.

## Decision

### Dedicated Simulation Framework

Build a standalone simulation framework (separate from production code) capable of:

- **Virtual Clients**: simulate thousands of WebSocket connections.
- **Movement Patterns**: walk, run, teleport, idle — configurable ratios.
- **Join/Leave Patterns**: configurable arrival/departure rates.
- **AOI Verification**: validate that entities see correct neighbors.
- **RPC Latency Measurement**: per-RPC histograms (p50, p95, p99, p99.9).
- **CPU/Memory/Network Profiling**: per-service resource tracking.
- **Benchmark Reports**: automated report generation with comparison to previous runs.

### When to Benchmark

| Phase | Scope | Required for |
|-------|-------|-------------|
| Phase 1 | Basic connectivity | Not required (no realtime yet) |
| Phase 2 | AOI + position sync | Validate single Game Server performance |
| Phase 3 | Multiple Game Servers + zone transfer | Validate distributed performance |
| Phase 4 | Full production stack | Final sign-off before production |

### Benchmark Scenarios

| Scenario | Virtual Clients | Duration | Target |
|----------|---------------|----------|--------|
| Baseline | 0 | 5 min | CPU/memory idle |
| Light load | 100 | 5 min | AOI correctness |
| Medium load | 1,000 | 10 min | Latency <100ms p95 |
| Heavy load | 5,000 | 10 min | No dropped connections |
| Burst | 0 → 5,000 in 10s | 2 min | Connection rate handling |
| Zone transfer | 1,000 (moving across zones) | 10 min | Migration correctness |
| Stability | 2,000 | 60 min | Memory leak detection |

### Benchmark Reporting

Each benchmark run produces:
- Summary: pass/fail per target metric.
- Histograms: latency distribution for all RPCs.
- Flame graphs: CPU profile (pprof) of Game Server.
- Memory profile: heap allocation over time.
- Network: bandwidth per connection, total throughput.

Reports are stored in `benchmarks/reports/` with timestamped filenames.

### Regression Detection

- Benchmark reports from each phase are compared to previous phase.
- Any regression >10% in p95 latency or throughput triggers investigation.
- Automated benchmark gate in CI: run light load scenario on every PR.
- CI benchmark must pass before merging to main.

### Performance Targets (Validation Criteria)

| Metric | Target | Measurement |
|--------|--------|-------------|
| Gateway connections | 10,000 concurrent | Count from Gateway metrics |
| End-to-end latency | <100ms p95 | Client-side timestamp in simulation framework |
| Internal RPC latency | <5ms p99 | Game Server RPC metrics |
| Tick duration | <50ms p99 | Game Server tick metrics |
| Entity count per Game Server | 5,000 | Game Server entity count metrics |
| Memory per entity | ~5 KB | Game Server memory / entity count |
| Zone transfer time | <1s per zone | Room Service zone transfer metrics |

### Tools

| Tool | Purpose |
|------|---------|
| Custom Go simulation framework | Primary load generator (WebSocket clients) |
| k6 | Secondary load generator (HTTP/gRPC) |
| Go pprof | CPU and memory profiling |
| Prometheus + Grafana | Real-time metrics during benchmark runs |
| `go test -bench` | Micro-benchmarks (AOI, serialization) |

## Consequences

- No production deployment without passing benchmarks.
- Benchmark framework is built in Phase 2 (alongside AOI implementation).
- CI benchmark gate catches regressions before they reach staging.
- Performance targets are validated, not assumed.
- Simulation framework doubles as integration test framework.

## Replaces

- None. Benchmark strategy was previously undefined.
