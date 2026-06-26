# Compression

> **Last Updated:** 2026-06-26

## Purpose

Define the compression strategy for reducing bandwidth usage in client↔server communication.

## Strategy Overview

| Phase | Algorithm | Scope | When |
|-------|-----------|-------|------|
| MVP (Phase 1) | gzip | Per-packet | Payloads > 256 bytes |
| Phase 2 | gzip | Per-packet | Same as MVP |
| Phase 3 | gzip | Per-packet | Same as MVP |
| Phase 4 | LZ4 or Snappy | Stream-level | All packets |

## MVP: Per-Packet gzip (Phase 1–3)

For the MVP, compression uses gzip on individual packets:

- **Algorithm:** gzip at compression level 3 (balance of speed and ratio).
- **Granularity:** Per-packet (each packet is compressed independently).
- **Threshold:** Only compress payloads larger than 256 bytes. Smaller payloads have negligible benefit and waste CPU.
- **Flag bit:** `bit 0` in packet Flags header indicates the payload is gzip-compressed.
- **Decompression:** Server decompresses before protobuf deserialization; client decompresses before processing.

### When gzip Is Applied

| Packet Type | Typical Size | Compressed? |
|-------------|-------------|-------------|
| AuthRequest | ~200 bytes | No |
| PositionUpdate | ~30 bytes | No |
| EntitySpawn | ~150 bytes | No |
| EntityMove | ~30 bytes | No |
| EntityState | 300–2000 bytes | Yes (if >256) |
| ChatBroadcast | 100–500 bytes | Conditional |
| ZoneState sync | 1–50 KB | Yes |

## Phase 4: Stream-Level LZ4/Snappy

In Phase 4, compression moves to stream-level using LZ4 or Snappy:

- **Algorithm:** LZ4 or Snappy (choice deferred to benchmarking). Both prioritize speed over ratio, suitable for realtime.
- **Granularity:** Stream-level — a compression session spans multiple packets within a connection.
- **Dictionary:** A shared compression dictionary can be negotiated during handshake for better ratios on small packets.
- **Backward compatibility:** Phase 4 servers still accept gzip-compressed packets from MVP clients.
- **Fallback:** If stream compression causes latency spikes, fall back to per-packet gzip automatically.

### LZ4 vs Snappy

| Metric | LZ4 | Snappy |
|--------|-----|--------|
| Compression speed | ~500 MB/s | ~400 MB/s |
| Decompression speed | ~1.5 GB/s | ~1.8 GB/s |
| Compression ratio (game data) | ~2.5× | ~2.3× |
| Library | `github.com/pierrec/lz4` | `github.com/golang/snappy` |

Final selection will be based on benchmarks with actual game traffic patterns in Phase 4.

## Threshold Tuning

Compression thresholds are configurable per service:

```go
type CompressionConfig struct {
    Algorithm  string // "gzip", "lz4", "snappy", "none"
    Level      int    // compression level (algorithm-specific)
    MinSize    int    // minimum payload size to compress (bytes)
    Enabled    bool
}
```

Default thresholds:

| Environment | Algorithm | Level | MinSize |
|-------------|-----------|-------|---------|
| Development | none | — | — |
| Staging | gzip | 3 | 256 |
| Production (Phase 1–3) | gzip | 3 | 256 |
| Production (Phase 4) | lz4 | — | 0 (stream) |

## Metrics

The following metrics are exported to monitor compression effectiveness:

- `compression_ratio` — uncompressed_size / compressed_size (per-packet) or total_bytes_in / total_bytes_out (stream).
- `compression_cpu_per_packet` — nanoseconds spent compressing per packet.
- `decompression_cpu_per_packet` — nanoseconds spent decompressing per packet.
- `packets_compressed_total` — counter of compressed packets.
- `packets_skipped_below_threshold` — counter of packets below MinSize.

## References

- [WebSocket Protocol](websocket.md)
- [Serialization](serialization.md)
- [ADR-010](../adr/010-packet-protocol.md) — Packet Protocol
