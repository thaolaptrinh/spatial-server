# Architecture Comparison

> **Source:** Legacy `docs/ADR.md` Appendix A
> **Last Updated:** 2026-06-26

## Reference Comparison: GoWorld vs Pitaya vs This Design

| Aspect | GoWorld (Reference) | Pitaya (Reference) | This Design |
|--------|-------------------|-------------------|-------------|
| Language | Go | Go | Go |
| Spatial Model | Entity + Space + AOI | None (groups only) | Grid-based zone + AOI |
| Inter-service RPC | Custom TCP + MsgPack | NATS / gRPC | Direct gRPC |
| Coordinator | Dispatcher (central router) | etcd | Room Service (lightweight) |
| Client Protocol | TCP | TCP + WebSocket | WebSocket |
| Scaling | Space-per-server | Server type pools | Zone-based ownership |
| Persistence | MongoDB | None built-in | PostgreSQL + Redis |
| Service Discovery | Built-in (dispatcher) | etcd | Room Service + DNS |
| Room-per-server | Yes (space = server) | Yes (server types) | No (zones span servers) |

## References

- [ADR-004](../adr/004-coordinator.md) — Coordinator Pattern
- [ADR-001](../adr/001-zone-ownership.md) — Zone Ownership
