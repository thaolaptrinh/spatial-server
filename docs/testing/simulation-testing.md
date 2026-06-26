# Simulation Testing

> **Last Updated:** 2026-06-26

## Purpose

Define the simulation framework for testing Spatial Server with thousands of virtual clients performing realistic behavior patterns.

## Overview

The simulation framework is a standalone Go application (separate from production code) that emulates player connections at scale. It validates correctness (AOI visibility, state sync) while measuring performance (latency histograms per RPC type).

## Architecture

```
┌──────────────────────────────────────────────┐
│              Simulation Framework             │
│                                              │
│  ┌─────────┐  ┌──────────┐  ┌────────────┐  │
│  │ Scenario│─▶│ Scheduler│─▶│ VClient    │  │
│  │ Engine  │  │          │  │ Pool (N)   │  │
│  └─────────┘  └──────────┘  └─────┬──────┘  │
│                                    │         │
│  ┌──────────────────┐              │         │
│  │ Metrics Collector │◀────────────┘         │
│  └──────────────────┘                        │
│        │                                     │
│  ┌─────▼──────┐  ┌──────────────────┐        │
│  │ Latency     │  │ AOI Verifier    │        │
│  │ Histograms  │  │                 │        │
│  └────────────┘  └─────────────────┘        │
└──────────────────────────────────────────────┘
```

## Virtual Client Behaviors

Each virtual client follows a configurable behavior pattern:

| Behavior | Description | Default Ratio |
|----------|-------------|---------------|
| Idle | Connect and stay still | 10% |
| Walk | Move randomly within zone at walk speed | 40% |
| Run | Move at run speed with direction changes | 30% |
| Teleport | Occasional large position jumps | 5% |
| Patrol | Follow waypoint path | 15% |

Behaviors are assigned at connection time via configurable ratio weights.

## Movement Patterns

```go
type MovementConfig struct {
    WalkSpeed     float64 // units per tick
    RunSpeed      float64
    DirectionChangeInterval time.Duration // seconds between random turns
    Bounds        Rectangle   // zone boundaries
    TeleportChance float64   // 0.0–1.0 probability per tick
}
```

Clients validate that their position updates are echoed correctly by checking `EntityMove` packets from the server.

## AOI Verification

The framework verifies AOI correctness by maintaining a local oracle:

1. Each virtual client tracks its own position locally.
2. The oracle computes expected visible entities based on AOI radius.
3. When `EntitySpawn`/`EntityDespawn` packets arrive, the framework asserts they match oracle expectations.
4. Discrepancies are logged with position snapshots for debugging.

```go
type AOIOracle struct {
    players map[PlayerID]Position
    radius  float64
}

func (o *AOIOracle) ExpectVisible(self PlayerID) []PlayerID {
    var visible []PlayerID
    for id, pos := range o.players {
        if id == self { continue }
        if distance(o.players[self], pos) <= o.radius {
            visible = append(visible, id)
        }
    }
    return visible
}
```

## Per-RPC Latency Histograms

Every RPC type gets its own latency histogram:

| RPC | Histogram Buckets (ms) |
|-----|------------------------|
| AuthRequest | 1, 5, 10, 25, 50, 100, 250, 500, 1000 |
| PositionUpdate | 1, 5, 10, 25, 50, 100, 250 |
| EntityAction | 1, 5, 10, 25, 50, 100, 250, 500 |
| ChatMessage | 1, 5, 10, 25, 50, 100, 250, 500 |

Histograms use `github.com/HdrHistogram/hdrhistogram-go` for high-dynamic-range recording.

## Running Simulations

```bash
# Basic simulation: 1,000 clients, 5 minutes
go run ./test/simulation/... \
    --clients 1000 \
    --duration 5m \
    --gateway ws://gateway:8080/ws

# With specific behavior mix
go run ./test/simulation/... \
    --clients 5000 \
    --duration 10m \
    --behavior idle=0.1,walk=0.4,run=0.3,teleport=0.05,patrol=0.15 \
    --aoi-verify \
    --metrics-file results.json

# Profile collection
go run ./test/simulation/... \
    --clients 2000 \
    --duration 30m \
    --pprof-cpu \
    --pprof-heap
```

## Output Format

```json
{
    "scenario": "medium-load",
    "duration": "10m",
    "clients": 1000,
    "latency_ms": {
        "PositionUpdate": { "p50": 12, "p95": 45, "p99": 78, "p99_9": 150 },
        "AuthRequest":    { "p50": 8,  "p95": 32, "p99": 55, "p99_9": 110 }
    },
    "aoi_errors": 0,
    "dropped_connections": 2,
    "throughput_msg_per_sec": 18500,
    "timestamp": "2026-06-26T12:00:00Z"
}
```

## References

- [Testing Strategy](strategy.md)
- [Load Testing](load-testing.md)
- [Benchmark Scenarios](benchmark-scenarios.md)
- [ADR-020](../adr/020-benchmark-strategy.md) — Benchmark Strategy
