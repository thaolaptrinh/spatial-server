# Architecture Comparison: GoWorld vs Pitaya vs Spatial Server

> **Last Updated:** 2026-06-26

## Purpose

Compare GoWorld and Pitaya with Spatial Server's architecture to identify lessons, validate decisions, and document rationale for adoption, adaptation, or rejection of each reference's ideas.

---

## Comparison: GoWorld vs Pitaya

### Strengths

| Dimension | GoWorld | Pitaya |
|-----------|---------|--------|
| **Entity model** | Complete Space-Entity framework with AOI, migration, persistence | No entity model (application defines it all) |
| **Concurrency model** | Single-threaded game loop eliminates race conditions | Concurrent goroutines with mutex protection |
| **Hot-swap** | Built-in freeze/restore for zero-downtime updates | No equivalent |
| **RPC flexibility** | N/A (single custom protocol) | Multiple transports (NATS, gRPC), interchangeable |
| **Service discovery** | Manual connection to dispatchers | etcd-based automatic discovery with watch |
| **Observability** | Basic logging + CPU metrics | Prometheus, Statsd, OpenTelemetry |
| **Architecture quality** | Tight coupling, reflection-heavy, large packages | Clean interfaces, DI via Builder, well-separated packages |
| **Maintenance** | Unmaintained (last update ~2020) | Active maintenance, Wildlife Studios production use |
| **Documentation** | Minimal (Chinese + English README) | Comprehensive (Read the Docs, godoc, examples) |

### Weaknesses

| Dimension | GoWorld | Pitaya |
|-----------|--------|--------|
| **Central bottleneck** | Dispatcher is mandatory single-point-of-failure for routing | No central bottleneck for game data |
| **Transport coupling** | Single custom TCP protocol | Pluggable RPC (NATS/gRPC) |
| **Persistence coupling** | MongoDB-specific storage | No storage layer (application responsibility) |
| **Config coupling** | INI format | Viper/YAML (flexible) |
| **Package cohesion** | `entity/Entity.go` is 1313 lines, tightly coupled internals | Well-separated packages, single responsibility |
| **Testing** | Minimal unit tests, no integration tests | Comprehensive: unit + e2e + benchmark + mocks |
| **AOI** | Built-in grid-based AOI | No built-in AOI |
| **Session management** | Implicit session via ClientProxy | First-class Session with binding, push, kick |

### Tradeoffs

| Decision | GoWorld | Pitaya | Analysis |
|----------|---------|--------|----------|
| **Entity model in framework** | Yes (complex, coupled) | No (flexible, more app work) | Pitaya's approach is cleaner for frameworks; GoWorld's is better for engines. We are building an engine. |
| **Central router** | Yes (bottleneck but simple) | No (P2P but complex routing) | P2P is more scalable. ADR-004 confirms this choice. |
| **Concurrency** | Single-threaded (safe, limited) | Concurrent (fast, complex) | Single-threaded works for GoWorld's model; our architecture needs concurrent processing for scale targets. |
| **Serialization** | msgpack + custom protocol | Protobuf (NATS/gRPC) | Protobuf wins on tooling, schema evolution, and cross-language support. |
| **Discovery** | Manual config | etcd-based | DNS + gRPC (our choice) is simpler than both: no external dependency, no manual config. |

### Scalability

| Aspect | GoWorld | Pitaya |
|--------|---------|--------|
| **Connection scaling** | Add Gates (behind L4 LB) | Add Frontend servers (behind L4 LB) |
| **Compute scaling** | Add Games (CPU-based load balancing) | Add Backend servers (type-based routing) |
| **Routing bottleneck** | Dispatcher (mitigated by multi-dispatcher) | No bottleneck (P2P RPC) |
| **State scaling** | No state sharing (entities per game) | No state sharing (application-managed) |
| **Discovery scaling** | O(n) connections (every process to every dispatcher) | O(n) watches via etcd |

### Maintainability

| Aspect | GoWorld | Pitaya |
|--------|---------|--------|
| **Code organization** | Tightly coupled engine packages | Well-separated with clear interfaces |
| **Dependency management** | Direct imports throughout | DI via Builder |
| **Config** | INI with struct mapping | Viper with full defaults |
| **Testing** | Minimal | Comprehensive |
| **Documentation** | Minimal | Full Read the Docs |
| **Active maintenance** | No | Yes |

### Complexity

| Aspect | GoWorld | Pitaya |
|--------|---------|--------|
| **Learning curve** | Medium (entity/space concepts, custom protocol) | Medium-low (component model, standard RPC) |
| **Operational complexity** | Low (3 process types, single config file) | Medium (etcd + NATS + application processes) |
| **Codebase size** | ~5K lines Go + external libs | ~8K lines Go + external deps |
| **External dependencies** | MongoDB, zap, msgpack, pktconn | etcd, NATS, Viper, Logrus, Prometheus, OTel, gRPC |

### Operational Cost

| Aspect | GoWorld | Pitaya |
|--------|---------|--------|
| **Infrastructure** | MongoDB cluster + processes | etcd cluster + NATS cluster + processes |
| **Monitoring** | Minimal (CPU metrics only) | Full (Prometheus + Grafana + OTel) |
| **Operational tooling** | Supervisor scripts | Docker Compose (dev), manual prod |
| **Recovery** | Manual restart, no state recovery | etcd lease expiry + manual app recovery |

---

## Alignment with Spatial Server Architecture

### Service Topology

| Decision | GoWorld | Pitaya | Spatial Server | Alignment |
|----------|---------|--------|----------------|-----------|
| **Process types** | 3 (dispatcher, gate, game) | 2 (frontend, backend) | 3 (gateway, room service, game server) | Neither matches exactly. Room Service is unique to our architecture. |
| **Central router** | Dispatcher (yes) | None (P2P) | None (P2P gRPC) | **Pitaya aligned.** ADR-004 mandates no central game data router. |
| **Gateway role** | Transparent TCP proxy | Frontend (handles handshake, routes) | WebSocket termination + gRPC proxy | Closer to Pitaya but with protobuf instead of custom protocol. |
| **Service types** | Fixed (gate, game, dispatcher) | User-defined strings | Fixed (gateway, room-service, game-server) | GoWorld model, but our types are fixed. |

### Runtime Model

| Decision | GoWorld | Pitaya | Spatial Server | Alignment |
|----------|---------|--------|----------------|-----------|
| **Entity model** | Built-in Space-Entity | No built-in model | Zone-AOI-Entity (designed, not implemented) | Neither matches. Our model is distinct. |
| **Entity lifecycle** | Create -> EnterSpace -> Simulate -> Destroy | Application-defined | Spawn -> Simulate -> Despawn | Similar to GoWorld but simpler. |
| **Zone/Space** | Space embeds Entity (IS-A) | No zone concept | Zone is separate from Entity | **Reject GoWorld embedding.** Zones and entities are separate concepts in our model. |
| **AOI** | Grid-based via external lib | No AOI | Grid-based (100x100 cells, 3x3 query) | Similar to GoWorld in concept but different grid parameters. Our implementation will be native. |
| **Entity migration** | Freeze/serialize/send/destroy | N/A | Zone transfer via gRPC streaming | GoWorld's protocol informs our zone transfer design but adapted for protobuf + gRPC. |
| **Single-threaded game loop** | Yes (5ms tick) | No game loop | 20Hz tick budget with phases | **Reject GoWorld model.** Our architecture requires concurrent processing. |

### Networking

| Decision | GoWorld | Pitaya | Spatial Server | Alignment |
|----------|---------|--------|----------------|-----------|
| **Client transport** | TCP/KCP/WebSocket | TCP/WebSocket | WebSocket (WSS) | Both support WebSocket. Pitaya's approach is closest. |
| **Protocol** | Custom binary (pktconn) | Pomelo + protobuf | Protobuf over WSS | **Reject both.** ADR-010 specifies binary length-prefixed protobuf over WebSocket. |
| **Gateway stateless** | No (maintains client proxy state) | Yes (forwards to backend) | Yes (ADR-021) | **Pitaya aligned.** Gateway is stateless with per-connection cache only. |
| **Direct client->game path** | No (always via dispatcher) | No (always via frontend -> RPC) | Yes (via Gateway -> GS gRPC) | Neither supports this. Our architecture is unique in having Gateway proxy directly to Game Server. |

### Communication Patterns

| Decision | GoWorld | Pitaya | Spatial Server | Alignment |
|----------|---------|--------|----------------|-----------|
| **Inter-service RPC** | Custom packet protocol | NATS or gRPC | gRPC | **Pitaya gRPC aligned.** We use gRPC as specified in ADR-009. |
| **P2P game server** | No (via dispatcher) | No (via frontend/backend type routing) | Yes (direct GS-to-GS) | **Neither aligned.** Our P2P model is unique. |
| **Service discovery** | Manual dispatcher connections | etcd | DNS + gRPC | **Neither aligned.** ADR-005 specifies our approach. |
| **RPC contract** | Implicit message types | Protobuf + struct tags | Protobuf (compiled) | **Pitaya aligned** (both use protobuf). We use compiled protobuf, not runtime struct tags. |

### Data Storage

| Decision | GoWorld | Pitaya | Spatial Server | Alignment |
|----------|---------|--------|----------------|-----------|
| **Persistence** | MongoDB | None (application-managed) | PostgreSQL + Redis | Neither. PostgreSQL is per ADR-001. |
| **AOI storage** | In-memory on Game | N/A | In-memory on Game Server | **GoWorld aligned.** AOI index is in-memory, not in Redis. ADR-003. |
| **Session storage** | Implicit (GameClient) | In-memory SessionPool | Redis (reconnection tokens) | Different approaches. Redis for reconnection resilience (ADR-022). |

### Infrastructure

| Decision | GoWorld | Pitaya | Spatial Server | Alignment |
|----------|---------|--------|----------------|-----------|
| **Orchestration** | Supervisor | Docker Compose | K3s (ADR-014) | Neither. K3s-native. |
| **Configuration** | INI | Viper/YAML | Koanf/YAML | Pitaya is closer (both use YAML), but we use koanf. |
| **Logging** | zap | Logrus | slog | Neither. We use stdlib slog. |
| **Monitoring** | Basic CPU | Prometheus + Statsd + OTel | Prometheus + Grafana + Loki (ADR-019) | Pitaya's Prometheus integration is the closest match. |
| **Service startup** | Manual config parsing | Builder pattern | Koanf + explicit init | Pitaya's Builder pattern is informative but we don't adopt it wholesale. |

---

## Synthesized Lessons

### Architectural Principles Validated

1. **No central router for game data** (ADR-004). Both references show the tradeoff: GoWorld's Dispatcher is a bottleneck, Pitaya's P2P is more scalable. Our P2P gRPC model is validated.

2. **AOI in-memory on Game Server** (ADR-003). GoWorld demonstrates this works at scale. Pitaya lacks AOI entirely, confirming this is the right choice for a game server engine.

3. **Protobuf for all serialization** (ADR-010). Pitaya's use of protobuf validates our choice. GoWorld's custom protocol + msgpack shows the maintenance burden of non-standard serialization.

4. **Stateless Gateway** (ADR-021). Pitaya demonstrates that frontend servers can be stateless. GoWorld's Gate maintains client proxy state, which complicates horizontal scaling.

5. **PostgreSQL for metadata** (ADR-001). GoWorld's MongoDB coupling is a warning. Pitaya's no-storage approach pushes complexity to applications. Our PostgreSQL choice is the right middle ground.

### Patterns to Incorporate

1. **Pitaya's Builder pattern** for clean dependency injection. Avoid global state (GoWorld's `gwvar` package is an anti-pattern).

2. **Pitaya's context propagation** (`trace_id`, `request_id`, `session_id`). Already designed into our metadata model.

3. **GoWorld's Post mechanism** for marshaling work into processing loops. Useful for Gateway connection handling and Game Server entity operations.

4. **GoWorld's panic recovery** in entity dispatch. Isolates failures without process crash.

5. **Pitaya's middleware/pipeline** for cross-cutting concerns. Cleaner than inline logging/metrics/validation.

### Anti-Patterns to Avoid

1. **Reflection-heavy dispatch** (both references). GoWorld uses reflection for entity method calls and RPC dispatch. Pitaya uses reflection for handler/remote registration. Our compiled protobuf avoids this entirely.

2. **Central message router** (GoWorld). Already rejected in our architecture.

3. **Single giant package** (GoWorld's `engine/entity/Entity.go` at 1313 lines). Our package structure (`pkg/entity/`, `pkg/zone/`, `pkg/aoi/`, `pkg/space/`) enforces separation.

4. **MongoDB coupling** (GoWorld). Our repository pattern abstracts storage behind interfaces.

5. **INI configuration** (GoWorld). Already using koanf/YAML.

6. **Process manager dependency** (GoWorld's Supervisor). We use K3s.

7. **External service discovery dependency** (Pitaya's etcd). We use DNS + gRPC.

### Risks to Monitor

1. **P2P complexity.** Both references avoid full P2P meshes (GoWorld uses central dispatcher, Pitaya uses frontend/backend routing with RPC). Our P2P Game Server model increases connection management complexity. Need to validate with benchmarks.

2. **No existing reference for zone transfer.** Neither reference implements zone-based ownership transfer. GoWorld's entity migration is close but simpler (whole-entity, not zone-based). Our zone transfer protocol is new design territory.

3. **gRPC streaming for realtime.** Pitaya uses NATS (pub/sub) or gRPC (request/reply). Neither uses gRPC bidirectional streaming for entity sync. Need to validate streaming performance at scale.

4. **Concurrent entity processing.** GoWorld avoids this entirely (single-threaded). Pitaya has concurrent handlers but no entity model. Our concurrent entity processing with appropriate synchronization is unvalidated by references.

---

## Summary Classification

### Adopted (from either reference)

- GoWorld: Post mechanism, panic recovery, packet batching, freeze/restore pattern
- Pitaya: Builder pattern for DI, consumer-package interfaces, pipeline/middleware, context propagation

### Adapted (modified for our architecture)

- GoWorld: AOI callbacks, interest sets, entity-client binding, filtered client broadcast
- Pitaya: Frontend/backend split, session binding, groups with TTL, timer system

### Rejected

- Central router (GoWorld Dispatcher)
- Custom packet protocol (GoWorld pktconn)
- Reflection-based RPC (both)
- MongoDB storage (GoWorld)
- etcd service discovery (Pitaya)
- NATS RPC (Pitaya)
- INI/Viper configuration (both)
- Logrus/zap logging (both)
- Entity embeds Space IS-A (GoWorld)
- Single-threaded game loop (GoWorld)
- Component lifecycle model (Pitaya)

---

## References

- [GoWorld Analysis](goworld-analysis.md)
- [Pitaya Analysis](pitaya-analysis.md)
- [ADR-001: Zone ownership in PostgreSQL](../adr/001-zone-ownership.md)
- [ADR-003: AOI index is in-memory on Game Server](../adr/003-aoi-in-memory.md)
- [ADR-004: Room Service is lightweight metadata coordinator](../adr/004-room-service-coordinator.md)
- [ADR-005: Game Server registration via DNS + gRPC](../adr/005-gs-registration.md)
- [ADR-009: All inter-service RPCs defined in protobuf](../adr/009-rpc-contract.md)
- [ADR-010: Binary length-prefixed packet protocol over WebSocket](../adr/010-packet-protocol.md)
- [ADR-014: K3s + Terraform + Helm + cloud-init stack](../adr/014-deployment-stack.md)
- [ADR-019: Prometheus + Grafana + Loki + OpenTelemetry](../adr/019-observability-stack.md)
- [ADR-021: Gateway internal architecture](../adr/021-gateway-architecture.md)
- [ADR-022: Reconnection window with session tokens](../adr/022-reconnection.md)
- [Repository Structure](../architecture/repository-structure.md)
- [Standards: Dependency Rules](../standards/dependency-rules.md)
