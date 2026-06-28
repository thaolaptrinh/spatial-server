# Distributed End-to-End Benchmark Report

> Generated: 2026-06-28T08:35:17+07:00

Full data path exercised: Client → WebSocket → Gateway → gRPC → Runtime Node → AOI → events → gRPC → Gateway → WebSocket → Client.
`round-trip` = client A sends a position update, a different client receives the resulting EntityMove.

| Clients | Connect p50 | Connect p95 | Round-trip p50 | Round-trip p95 | Round-trip p99 | Round-trip max | Sends/s | Frames/s | Duration |
|---|---|---|---|---|---|---|---|---|---|
| 10 | 97.19ms | 148.25ms | 42.88ms | 44.73ms | 45.71ms | 48.28ms | 100 | 1247 | 20s |
| 25 | 73.40ms | 75.52ms | 16.33ms | 51.28ms | 52.61ms | 56.92ms | 249 | 6901 | 20s |
| 50 | 88.14ms | 90.21ms | 32.41ms | 40.62ms | 44.30ms | 58.09ms | 498 | 26282 | 20s |

## Notes
- Multi-node scaling, failure injection, and long-duration (30m–3h) runs are framework-supported but executed in CI/long-running environments (see README).
