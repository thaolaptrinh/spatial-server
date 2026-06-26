# Testing Documentation

> **Last Updated:** 2026-06-26

## Purpose

Define the testing strategy for Spatial Server — unit, integration, load, simulation, chaos, and benchmark testing.

## Contents

| File | Description |
|------|-------------|
| [strategy.md](strategy.md) | Overall testing strategy — levels, targets, CI gates, and performance targets |
| [unit-testing.md](unit-testing.md) | Unit testing guidelines — mocks, table-driven tests, and coverage requirements |
| [integration-testing.md](integration-testing.md) | Integration testing — Docker Compose test environment, service interaction tests |
| [load-testing.md](load-testing.md) | Load testing — k6 scenarios, connection ramp-up, and throughput targets |
| [simulation-testing.md](simulation-testing.md) | Simulation testing — multi-client scenarios, zone transfers, and crash recovery |
| [chaos-testing.md](chaos-testing.md) | Chaos testing — fault injection, network partitions, and recovery validation |
| [benchmark-scenarios.md](benchmark-scenarios.md) | Benchmark scenarios — latency, throughput, and resource benchmarks |

## Reading Order

1. Start with [strategy.md](strategy.md) for the overall testing approach.
2. Read [unit-testing.md](unit-testing.md) and [integration-testing.md](integration-testing.md) for development testing.
3. Review [load-testing.md](load-testing.md) and [simulation-testing.md](simulation-testing.md) for performance validation.
4. Study [chaos-testing.md](chaos-testing.md) for resilience testing.
5. Consult [benchmark-scenarios.md](benchmark-scenarios.md) for performance benchmarking.

## Related Documents

- [Architecture](../architecture/README.md) — System architecture and performance budgets
- [Operations](../operations/README.md) — Operational procedures
- [Standards](../standards/README.md) — Coding and testing standards
