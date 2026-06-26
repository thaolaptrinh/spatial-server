# Pitaya Reference Analysis

> **Last Updated:** 2026-06-26

## Purpose

Analyze the Pitaya distributed game server framework as a reference for architectural ideas applicable to Spatial Server. Pitaya is a production-grade Go framework used by Wildlife Studios, with a component-based architecture, pluggable RPC transports, and etcd-based service discovery.

---

## Overall Architecture

### Runtime Model

Pitaya is a **framework, not an engine** -- it provides networking, clustering, and component lifecycle but has no built-in entity model, spatial indexing, or game loop. Applications register components (handlers for client messages, remotes for server RPCs) and Pitaya wires them together.

### Service Topology

Two server modes:

| Mode | Description |
|------|-------------|
| **Standalone** | Single process, no service discovery, no RPC. Suitable for development or single-server deployments. |
| **Cluster** | Multi-process with etcd service discovery, NATS/gRPC RPC, frontend/backend split. |

Two server types within cluster mode:

| Type | Acceptors | Role |
|------|-----------|------|
| **Frontend** | Yes (TCP/WS) | Client connection termination, handshake, message forwarding |
| **Backend** | No | Pure RPC processing, never directly connected to clients |

Servers are identified by a `Type` string (e.g., `"connector"`, `"room"`, `"metagame"`, `"worker"`). Routing is type-based: `serverType.serviceName.methodName`.

### Deployment Model

Docker Compose for local/CI. Documentation references Read the Docs. No K8s-native deployment tooling.

### Process Model

`App` struct is the central runtime. Startup:
1. `NewDefaultBuilder()` constructs builder with default dependencies
2. `AddAcceptor()` adds network listeners (frontend only)
3. `Build()` wires everything into an `App`
4. `app.Start()` begins processing

The `HandlerService` spawns `config.Concurrency.Handler.Dispatch` goroutines for message processing. Each acceptor spawns a goroutine per connection.

---

## Networking

### Connection Handling

- TCP and WebSocket acceptors via `Acceptor` interface
- Raw TCP uses Pomelo-length-prefixed protocol
- WebSocket uses Gorilla WebSocket library
- Both wrapped into `PlayerConn` interface

### Connection Lifecycle

1. Acceptor accepts connection, pushes to `GetConnChan()`
2. `handlerService.Handle(conn)` spawned in goroutine
3. Handshake packet processed (validators, dictionary setup)
4. Client enters working state after `HandshakeAck`
5. Heartbeat goroutine monitors liveness (configurable interval)
6. On disconnect: session closed, cleanup callbacks fired

### Session Lifecycle

Session is a **first-class concept** with explicit lifecycle:

| Phase | Action |
|-------|--------|
| Creation | `sessionPool.NewSession()` creates and optionally registers |
| Binding | `session.Bind(uid)` links user ID, triggers callbacks |
| Working | Push/Kick/ResponseMID operations available |
| Close | Callbacks fired, resources released |

Sessions exist in a `SessionPool` with `sessionsByUID` and `sessionsByID` concurrent maps.

### Packet Routing

Route format: `serverType.serviceName.methodName`. If target type matches local server, processed locally; otherwise forwarded via RPC. The router selects a target server instance (random by default, custom functions supported).

### Gateway Design

Frontend servers are the Gateway. They:
- Accept client connections
- Handle handshake and protocol negotiation
- Forward messages to backend servers via Sys RPC
- Route responses back to the correct client

Backend servers create ephemeral `Remote` agents for the duration of Sys RPC requests. Session modifications on backends require explicit `PushToFront` calls.

---

## Runtime

### Entity Management

Pitaya has **no built-in entity model**. Applications define their own entity representation. The closest concept is `Groups` -- named collections of UIDs (e.g., for rooms or channels).

### Room/Space Representation

No built-in room or space abstraction. Groups provide membership management with TTL-based cleanup, but no spatial concepts.

### Ownership

No built-in ownership model. The first server to bind a session UID claims it (with duplicate session closing). All other ownership must be application-defined.

### Lifecycle

Component lifecycle: `Init -> AfterInit -> [running] -> BeforeShutdown -> Shutdown`. Module lifecycle: `BeforeStart -> Start -> [running] -> AfterStart -> BeforeShutdown -> Shutdown -> AfterShutdown`. Pitaya provides hooks but no entity lifecycle.

---

## Distributed Architecture

### Server Communication

Two RPC transports:

| Transport | Pros | Cons |
|-----------|------|------|
| **NATS** | Lightweight, pub/sub semantics, request-reply built-in | External dependency, no built-in TLS, message size limits |
| **gRPC** | Streaming, TLS, protobuf-native | Heavier, connection management complexity |

Both implement `RPCClient`/`RPCServer` interfaces for interchangeability.

### RPC Strategy

Three RPC types:

| Type | Description | Session |
|------|-------------|---------|
| **Sys** | Forwarded client messages | Full session protobuf included |
| **User** | Application-level RPC | No session |
| **Reliable** | Redis-backed async RPC with retry | No reply (fire-and-forget) |

### Registration

Servers register in etcd under `{prefix}/servers/{type}/{id}` with JSON metadata (ID, Type, Metadata map, Frontend flag, Hostname). etcd leases provide heartbeat (default 60s TTL).

### Discovery

`EtcdServiceDiscovery` with:
- Initial sync on startup
- Periodic refresh (default 120s)
- Watch-based change notification
- Server type blacklist support
- Parallel fetching (configurable workers)

`SDListener` interface enables RPC clients to react to server joins/leaves.

### Scaling

- Horizontal: add more instances of any server type
- Routing: custom `RoutingFunc` per server type
- Cross-region: gRPC transport with metadata-based host/port resolution
- No built-in load balancing metrics (random routing by default)

### Failure Recovery

- Server crash: etcd lease expires (60s TTL), other servers notified via watch
- Session recovery: not built-in (application responsibility)
- RPC retry: `ReliableRPC` via Redis-backed worker queue (async only)
- No automatic session migration or state transfer

---

## Data Structures

### Important Runtime Structures

| Structure | Description |
|-----------|-------------|
| `App` | Central runtime: acceptors, server identity, RPC endpoints, modules |
| `Session` | Per-client state: ID, UID, data store, binding, close callbacks |
| `SessionPool` | Concurrent session registry (by UID and by ID) |
| `Agent` | Per-connection handler: packet decode/encode, heartbeat, write buffering |
| `Server` | Server identity: ID, Type, Metadata, Frontend, Hostname |
| `GroupService` | UID group management with TTL cleanup |
| `Handler`/`Remote` | Reflective method metadata for dispatch |

### Memory Ownership

No shared entity state. Sessions are pooled and referenced by pointer with concurrent-safe access. Group membership is in-memory with mutex protection.

### Synchronization Model

Handler dispatch goroutines process messages concurrently. Session operations use mutex-protected access within `session.go`. Groups use mutex for membership operations.

### Tick Loop

No game loop. Pitaya uses a timer system with configurable precision (default 1s). Timers fire inside handler dispatch goroutines.

### AOI Implementation

**No built-in AOI.** Pitaya does not include spatial indexing. AOI must be implemented by the application using Pitaya's networking and RPC infrastructure.

---

## Infrastructure

### Deployment

Docker Compose for e2e tests (`docker-compose.yml` with etcd + NATS). No production deployment tooling included.

### Configuration

Viper-based YAML config with comprehensive defaults struct (`PitayaConfig`). Config precedence: env vars > config file > defaults.

### Service Startup

Builder chain:
1. `NewDefaultBuilder()` (etcd SD, NATS RPC, memory groups)
2. `AddAcceptor()` (TCP/WS listeners)
3. `Register()` / `RegisterRemote()` (application components)
4. `AddRoute()` (custom routing)
5. `Build()` -> `Start()`

### Dependency Management

Go modules. Major deps: gorilla/websocket, nats-io/nats.go, etcd client v3, prometheus, logrus, viper, cobra, protobuf, gRPC, OpenTelemetry.

### Logging

Logrus with interface wrapper for extensibility. Context-aware logging extracts trace/request/session IDs.

### Monitoring

- Prometheus metrics on configurable port (default 9090)
- Statsd reporter (host: localhost:9125)
- OpenTelemetry tracing integration

---

## Engineering Practices

### Repository Organization

```
pkg/             -- Core library (app, builder, cluster, session, etc.)
cmd/             -- CLI (REPL)
examples/        -- Demo applications
e2e/             -- End-to-end tests
benchmark/       -- Performance benchmarks
docs/            -- Documentation (readthedocs)
```

### Package Boundaries

Well-separated packages with clear interfaces. `pkg/cluster/` defines RPC/SD interfaces, implementations in sub-packages. `pkg/session/` is self-contained. `pkg/service/` holds handler/remote dispatch logic.

### Interfaces

Consumer-package interfaces defined in `pkg/cluster/`, `pkg/acceptor/`, `pkg/session/`. Small, focused interfaces (4-6 methods). Strategy pattern via `RoutingFunc` type.

### Dependency Inversion

Strong. `Builder` explicitly wires dependencies. `App` constructor takes all dependencies (no service locator). Default implementations can be replaced (NATS <-> gRPC, etcd <-> custom SD).

### Testing

GoMock for interface mocking (auto-generated via `make mocks`). Table-driven unit tests. End-to-end tests with Docker Compose. Benchmark tests. Readthedocs documentation.

### Configuration

Viper-based with explicit `PitayaConfig` struct. Complete defaults documented in config package (641 lines). YAML files for per-environment overrides.

### Build System

Makefile with 199 lines: build, test, lint, mock generation, benchmark, proto generation.

---

## Adopt

| Idea | Rationale |
|------|-----------|
| **Builder pattern for DI** | Clean explicit assembly of dependencies. No global state or service locator. Matches our preference for explicit wiring. |
| **Consumer-package interfaces** | Defining interfaces where they are consumed (not implemented). Already in our standards but validated by this reference. |
| **Pipeline/middleware system** | Before/after handler hooks for validation, logging, metrics. Clean separation of cross-cutting concerns from business logic. |
| **Context propagation** | Trace ID, request ID, session ID propagated through context. Matches our metadata propagation design. |

---

## Adapt

| Idea | Adaptation |
|------|------------|
| **Frontend/Backend split** | Maps to our Gateway/Game Server split. We use gRPC for inter-service communication instead of NATS. |
| **Session binding pattern** | Our reconnection token system serves the same purpose. Adapt the session lookup/bind/kick semantics to our Gateway model. |
| **Groups with TTL** | Useful for lightweight player grouping. Adapt for runtime-scoped player sets without the complexity of full zone management. |
| **Timer system** | Adjustable-precision timer with periodic/count/after/cond variants. Adapt for our game loop scheduling but with Hz-based intervals. |

---

## Reject

| Idea | Rationale |
|------|-----------|
| **etcd service discovery** | ADR-005 specifies DNS + gRPC registration. etcd adds operational complexity (3-node cluster minimum) for marginal benefit at our scale. |
| **NATS RPC** | We use direct gRPC P2P. NATS adds a message broker dependency that our architecture explicitly avoids for realtime data. |
| **Logrus logging** | We use standard library `log/slog`. Adopting Logrus would add a dependency for functionality the stdlib now provides. |
| **Viper configuration** | We use koanf. Both are viable; switching would contradict our configuration standard. |
| **Reflection-based handler detection** | We use compiled protobuf gRPC with explicit service definitions. Reflection provides flexibility at the cost of type safety and discoverability. |
| **Component lifecycle (Init/AfterInit/BeforeShutdown/Shutdown)** | Over-engineered for our service model. Our services have simpler lifecycle needs (startup, running, shutdown). |
| **Reliable RPC via Redis workers** | Async fire-and-forget RPC has limited usefulness for realtime systems. We prefer synchronous RPC with appropriate timeouts and retries. |
| **Session data store (Set/Get/Remove)** | In-memory session state creates consistency challenges. Our architecture keeps session state in the Gateway connection cache and Game Server entity state. |
| **Standalone mode** | Our architecture is distributed by design. A standalone mode would create a parallel code path with no architectural benefit. |
| **Cobra CLI** | We don't need a REPL or complex CLI for our services. Simple flag parsing suffices. |
| **OpenTelemetry tracing integration** | Not yet in our standards. Valuable for future but adds complexity for MVP. |

---

## References

- [Repository Structure](../architecture/repository-structure.md)
- [ADR-005: Game Server registration via DNS + gRPC](../adr/005-gs-registration.md)
- [ADR-021: Gateway internal architecture](../adr/021-gateway-architecture.md)
- [Standards: Configuration](../standards/configuration.md)
- [Standards: Logging](../standards/logging.md)
- [GoWorld Analysis](goworld-analysis.md)
- [Architecture Comparison](architecture-comparison.md)
