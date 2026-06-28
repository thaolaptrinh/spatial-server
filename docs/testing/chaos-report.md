# Chaos Engineering Report

> **Date:** 2026-06-28  |  **Framework:** v1.0.0

## Summary

| Scenario | Outcome | Duration | Recovery |
|----------|---------|----------|----------|
| runtime-crash | passed | ~2min | ~30s |
| runtime-restart | passed | ~2min | ~30s |
| gateway-crash | timed_out | 2min | N/A (no restart) |
| gateway-restart | timed_out | 2min | N/A (no restart) |
| room-service-crash | timed_out | 2min | N/A (no restart) |
| room-service-restart | timed_out | 2min | N/A (no restart) |
| delayed-heartbeats | passed | ~20s | ~20s (thaw) |

**Result: 3/7 passed, 4 timed out. All compose scenarios skipped (placeholders).**

## Findings

1. **Runtime Node resilience**: Runtime crash and restart scenarios PASS. The Room Service sweeper detects orphan zones within the heartbeat timeout window. `ProcessFreeze` (SIGSTOP → SIGCONT) also passes — the Runtime resumes normal operation after thaw.

2. **Gateway & Room Service lack restart**: Gateway and Room Service SIGKILL scenarios TIMED OUT. The ProcessCrash injector kills the process, but nothing restarts it. The recovery waiter polls `HealthyEndpoint` forever until the 2-minute scenario timeout. The framework correctly identifies this as a resilience gap — no supervisor or process manager restarts these services in the test harness.

3. **Compose injectors are placeholders**: All 8 compose scenarios (PG/Redis restart, network latency/loss/partition, CPU/memory pressure) skip because their injectors return `"requires Docker Compose execution mode"`. These will be implemented in the Production Hardening milestone.

## Invariants Validated

- **Entity** (I-01, I-03, I-04): Entity count preserved after recovery (3/3 passed)
- **Ownership** (O-01, O-02, O-03): Zone ownership preserved (3/3 passed)
- **AOI** (G-01, G-02, G-03): Ghost count bounded (3/3 passed)
- **Session**: Disconnected sessions stable (3/3 passed)
- **Scheduler** (T-01..T-04): No command drops observed (3/3 passed)

## Remaining Risks

1. Gateway and Room Service have no automatic restart mechanism in the test harness. This is the harness design — services are started once per scenario. A full restart-after-crash requires either Docker Compose (with `restart: always`) or an external process supervisor. This is documented and deferred.

2. Compose-mode injectors for network faults and resource pressure are stubs. Real Docker SDK implementations are deferred to the Production Hardening milestone.

## Next Steps

- Implement Docker Compose-backed restart for Gateway/Room Service crash scenarios
- Replace compose injector stubs with real Docker SDK implementations
- Add harness instrumentation for OwnershipConvergence, ReconnectTime, QueueDrainTime acceptance criteria
