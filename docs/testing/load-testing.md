# Load Testing

> **Last Updated:** 2026-06-26

## Purpose

Define the load testing methodology for validating Spatial Server performance under realistic concurrent load.

## Tools

| Tool | Use Case | When |
|------|----------|------|
| k6 | HTTP/gRPC endpoint load (Room Service, Admin API) | Every milestone |
| Custom WebSocket client (Go) | WebSocket connection load (Gateway) | Every milestone |
| Simulation framework | Full end-to-end load (≥1,000 virtual clients) | Phase 2+ |

## k6 Scenarios

### Room Service RPC Load

```javascript
import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
    stages: [
        { duration: '2m', target: 100 },  // ramp up
        { duration: '5m', target: 100 },  // steady
        { duration: '1m', target: 0 },    // ramp down
    ],
    thresholds: {
        http_req_duration: ['p(95)<500', 'p(99)<1000'],
        http_req_failed: ['rate<0.01'],
    },
};

export default function () {
    const res = http.post('http://room-service:8080/v1/runtime', JSON.stringify({
        runtime_id: 'test-' + __VU,
        zone_count: 4,
    }), { headers: { 'Content-Type': 'application/json' } });
    check(res, { 'status is 200': (r) => r.status === 200 });
    sleep(1);
}
```

### Admin API Load

```javascript
export default function () {
    const res = http.get('http://gateway:8080/admin/health');
    check(res, { 'health ok': (r) => r.status === 200 });
}
```

## Custom WebSocket Client Load

For scenarios k6 cannot handle (WebSocket binary protocol), use the dedicated Go load client:

```bash
# 1,000 concurrent connections, 10s ramp-up, 5m duration
go run ./test/load/websocket/... \
    --connections 1000 \
    --ramp-up 10s \
    --duration 5m \
    --gateway ws://gateway:8080/ws
```

The load client supports:
- Configurable connection count and ramp-up rate
- Realistic message patterns (position updates at 20Hz, occasional actions)
- Per-connection latency tracking
- Metrics export to Prometheus

## Scenarios

| Scenario | Connections | Ramp-Up | Steady State | Validates |
|----------|-------------|---------|--------------|-----------|
| Connection burst | 0→5,000 | 10s | — | Connection rate handling |
| Steady state | 2,000 | 30s | 10 min | Latency under sustained load |
| Mixed traffic | 1,000 | 20s | 10 min | Concurrent Auth + Position + Chat |
| Stress | 5,000 | 60s | 5 min | System limits and failure modes |

## Metrics Collection

| Metric | Source | Target |
|--------|--------|--------|
| Connections/sec | Gateway metrics | ≥500/s |
| End-to-end latency p95 | Client-side timestamps | <100ms |
| Message throughput | Gateway metrics | ≥10,000 msg/s per Gateway |
| Error rate | Gateway logs | <0.1% |
| CPU/memory per service | pprof / `top` | Within capacity plan |

## Reporting

Each load test run produces:
1. Latency histogram (p50, p95, p99, p99.9, max)
2. Connection lifecycle chart (connect rate, disconnect rate, active count)
3. Error breakdown by packet type
4. Resource utilization per service

Reports are stored in `test/load/reports/` with timestamped filenames.

## References

- [Testing Strategy](strategy.md)
- [Simulation Testing](simulation-testing.md)
- [Benchmark Scenarios](benchmark-scenarios.md)
- [ADR-020](../adr/020-benchmark-strategy.md) — Benchmark Strategy
