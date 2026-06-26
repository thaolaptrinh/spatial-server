# GoWorld Reference Analysis

> **Last Updated:** 2026-06-26

## Purpose

Analyze the GoWorld distributed game server engine as a reference for architectural ideas applicable to Spatial Server. GoWorld is a mature, production-oriented game server engine with a Space-Entity framework, actor-like concurrency model, and three-process topology.

---

## Overall Architecture

### Runtime Model

GoWorld uses an **actor-like model** where each game process runs game logic on a single goroutine, eliminating concurrency complexity for game code. Processes communicate via message passing through a central Dispatcher.

### Service Topology

Three mandatory process types:

| Process | Role |
|---------|------|
| Dispatcher | Central message router, entity location registry |
| Gate | Client connection termination, protocol handling |
| Game | Entity simulation, game logic execution |

The Dispatcher is a **central bottleneck** -- all inter-process messages flow through it. Multi-dispatcher mode uses entity ID hashing for horizontal scaling but adds complexity (every process connects to every dispatcher).

### Deployment Model

INI-driven deployment with desired instance counts per process type. Supervisor-based process management. Hot-swap support via SIGHUP freeze/restore.

### Process Model

Each process has a single `select` loop with ticker + packet queues. The `post.Post()` mechanism marshals work from I/O goroutines into the main loop. Async jobs (storage, KVDB) use callbacks delivered via Post.

---

## Networking

### Connection Handling

- Gate accepts TCP, KCP, and WebSocket connections simultaneously
- `ClientProxy` wraps each connection with optional Snappy compression
- Packets flow through a three-hop path: Client -> Gate -> Dispatcher -> Game
- Gate maintains `filterTrees` for efficient client broadcast

### Connection Lifecycle

1. TCP accept -> `newClientProxy()` -> register with GateService
2. Gate sends `MT_NOTIFY_CLIENT_CONNECTED` to Dispatcher
3. Client enters working state, sends/receives packets
4. On close: `onClientProxyClose()` -> `MT_NOTIFY_CLIENT_DISCONNECTED` to Dispatcher

### Session Lifecycle

Sessions are implicit -- the combination of (clientid, ownerEntityID, filterProps) on ClientProxy represents the client binding. Entity-GameClient binding is explicit via `SetClient()`.

### Packet Routing

Packets carry `MsgType (uint16)` headers. The Dispatcher maintains `entityDispatchInfo` maps to route packets to the correct Game process. Entity-to-dispatcher routing is hash-based.

### Gateway Design

Gate is a **transparent proxy** -- it terminates connections but does not interpret game logic. It performs protocol translation (TCP/KCP/WS -> internal packet format), compression, and client broadcast filtering.

---

## Runtime

### Entity Management

Entities are created via `reflect.New()`, registered in `_EntityManager.entities` (EntityMap by ID), and processed in the single-threaded game loop. The `Entity` struct is large (~30 fields) with lifecycle callbacks (`OnInit`, `OnCreated`, `OnDestroy`, `OnEnterSpace`, `OnLeaveSpace`, `OnMigrateIn`, `OnMigrateOut`).

### Space Representation

**Spaces embed Entity** (IS-A relationship). Each Game has one Nil Space for unassigned entities. Spaces optionally enable grid-based AOI. Migration between spaces on the same game is a local operation; migration between games is a remote freeze/serialize/send protocol.

### Ownership

Entity ownership is implicit -- the Game process where the entity lives owns it. Dispatcher tracks entity-to-game mapping. Ownership transfer is the entity migration protocol.

### Lifecycle

Entity lifecycle: `CreateEntity -> Init -> EnterSpace -> [simulate] -> LeaveSpace -> Destroy`. Spaces have a simpler lifecycle: created empty, entities enter/leave. Nil Space is permanent per Game.

---

## Distributed Architecture

### Server Communication

Custom packet protocol over TCP between all server types. Dispatcher is the central hub -- Gates and Games connect to it, never directly to each other.

### RPC Strategy

Method-name-based RPC with flags: `_Client` (callable from client), `_AllClients` (broadcastable), no suffix (server-only). RPC dispatch uses reflection (`reflect.Value.Call`) with msgpack argument deserialization.

### Registration

Games/Gates connect to Dispatcher, send `MT_SET_GAME_ID`/`MT_SET_GATE_ID`. Dispatcher tracks connected games/gates in maps. Service entities use KV registry for discovery.

### Discovery

KV registry (`kvreg`) stores service locations. Service entities are globally unique per shard. `checkServices()` periodically reconciles desired vs actual service instances.

### Scaling

Horizontal scaling via:
- Multiple Gates (increase connection capacity)
- Multiple Games (increase entity capacity, CPU-based load balancing)
- Multiple Dispatchers (increase routing capacity, hash-based sharding)

### Failure Recovery

- Game crash: all entities lost (no persistence recovery path in core)
- Gate crash: all client connections lost
- Dispatcher crash: remaining dispatchers handle traffic
- No built-in leader election or automatic failover

---

## Data Structures

### Important Runtime Structures

| Structure | Description |
|-----------|-------------|
| `Entity` | Central struct: ID, TypeName, Space, Position, Attrs, Client, AOI, Interest sets |
| `Space` | Embeds Entity, adds entities set, AOI manager, Kind |
| `EntityMap` | `map[EntityID]*Entity` |
| `EntitySet` | `map[*Entity]struct{}` |
| `entityDispatchInfo` | Dispatcher's per-entity routing info (gameid, blockUntilTime, pendingQueue) |
| `entityMigrateData` | Serialized entity state for migration |

### Memory Ownership

Entity structs are heap-allocated and exclusively owned by the game process. No shared memory -- all cross-process communication is via packet serialization.

### Synchronization Model

Single-threaded game logic eliminates shared-state concurrency. The `post.Post()` mechanism provides thread-safe callback queuing. Storage/KVDB operations use async callbacks.

### Tick Loop

Game loop: `select { packetQueue, ticker -> Timer.Tick() + position sync } post.Tick()`. Default tick interval: 5ms.

### AOI Implementation

External library (`github.com/xiaonanln/go-aoi`). Space-level `aoiMgr` created by `EnableAOI(distance)` using `XZListAOIManager`. Entity-level AOI nodes initialized during entity init. Callbacks `OnEnterAOI`/`OnLeaveAOI` manage interest sets.

---

## Infrastructure

### Deployment

Supervisor-managed processes. CLI tool `cmd/goworld` for build/start/stop/reload/status. Hot-swap via SIGHUP.

### Configuration

INI format via `go-ini/ini`. Sections per process type and instance. `configLock` mutex for thread safety. Defaults set explicitly in read functions.

### Service Startup

Game: parse args -> read config -> set GOMAXPROCS -> setup logging -> init storage (MongoDB) -> init KVDB -> init crontab -> setup HTTP (pprof) -> create NilSpace -> connect to dispatchers -> start LBC -> setup signals -> setup services -> main loop.

### Dependency Management

Go modules (`go.mod`). External deps: `pktconn`, `go-aoi`, `zap`, `msgpack`, `mongo-go-driver`, `go-ini/ini`, `gorilla/websocket`.

### Logging

`go.uber.org/zap` with custom wrapper (`gwlog`). Debug logging gated by compile-time consts.

### Monitoring

`opmon` package for operation monitoring. CPU-based load balancing. pprof HTTP endpoint.

---

## Engineering Practices

### Repository Organization

```
components/     -- Executable entry points (dispatcher, gate, game)
engine/         -- Core library packages
cmd/            -- CLI tooling
examples/       -- Demo applications
```

### Package Boundaries

`engine/` contains tightly coupled packages. Entity, space, and entity manager are interdependent. The `entity` package is large (Entity.go: 1313 lines, EntityManager.go: 624 lines).

### Interfaces

Consumer-package interfaces: `IEntity`, `ISpace` defined in `engine/entity/`. Delegate pattern: `IDispatcherClientDelegate`. Small interfaces (6-12 methods).

### Dependency Inversion

Limited. Packages import engine internals directly. No abstract storage interfaces -- direct MongoDB dependency in entity code.

### Testing

Unit tests for entity, netutil, post, gwlog. CI via GitHub Actions. Codecov tracking.

### Configuration

INI files with struct mapping. Thread-safe via mutex. Two-level config (common defaults + per-instance overrides).

### Build System

`make` for common tasks. Go build for binaries. No Dockerfiles in repository.

---

## Adopt

| Idea | Rationale |
|------|-----------|
| **Post mechanism** | Thread-safe callback queue marshaled into main loop. Clean pattern for cross-goroutine communication without locks. |
| **Panic recovery in entity dispatch** | `defer/recover` around every entity method call prevents single entity crash from taking down the game process. |
| **Packet batching** | `CollectEntitySyncInfos()` batches position updates per-gate. Reduces syscall overhead for high-frequency updates. |
| **Freeze/restore for hot-swap** | Clean state serialization protocol. Useful for zero-downtime Game Server updates. |

---

## Adapt

| Idea | Adaptation |
|------|------------|
| **AOI callbacks** | GoWorld uses `OnEnterAOI`/`OnLeaveAOI` for interest management. We should use a similar callback pattern but with protobuf-typed events instead of reflection. |
| **Interest sets** | GoWorld's `InterestedIn`/`InterestedBy` entity sets. We should use a similar pattern for AOI tracking but keyed by entity ID, not pointer. |
| **GameClient binding** | Entity-client binding pattern is sound. Our architecture already specifies this implicitly through the Gateway connection model. |
| **Filtered client broadcast** | Gate-side filter trees for efficient broadcast. We should adapt this to our Gateway but use protobuf-defined filter criteria. |

---

## Reject

| Idea | Rationale |
|------|-----------|
| **Central Dispatcher** | ADR-004 explicitly rejects central routers for game data. We use direct P2P gRPC between Game Servers. |
| **INI configuration** | We use koanf with YAML as specified in configuration standards. |
| **Custom packet protocol** | We use protobuf over WebSocket (ADR-010). Custom protocols add maintenance burden and tooling gaps. |
| **Reflection-based RPC** | We use compiled protobuf gRPC. Reflection-based RPC is slower, type-unsafe, and harder to document. |
| **MongoDB storage** | We use PostgreSQL as the source of truth (ADR-001). |
| **Entity embeds Space (IS-A)** | We treat Zone and Entity as separate concepts. Embedding creates unnecessary coupling. |
| **INI-based deployment config** | K8s-native deployment (ADR-014). No process manager dependency. |
| **msgpack serialization** | We use protobuf for all serialization. Single serialization format is simpler. |
| **Supervisor process management** | We use K3s orchestration (ADR-014). |
| **Reflection-based entity creation** | We will use factory functions or type registries, not reflection. Better compile-time safety. |
| **Single goroutine game loop** | Our architecture supports concurrent entity processing with appropriate synchronization boundaries. |
| **Sleep-based tick loop** | We use gRPC streaming and timer-based scheduling. Busy-wait is not appropriate for our scale model. |

---

## References

- [Repository Structure](../architecture/repository-structure.md)
- [ADR-004: Room Service is lightweight metadata coordinator](../adr/004-room-service-coordinator.md)
- [ADR-010: Binary length-prefixed packet protocol over WebSocket](../adr/010-packet-protocol.md)
- [ADR-014: K3s + Terraform + Helm + cloud-init stack](../adr/014-deployment-stack.md)
- [Pitaya Analysis](pitaya-analysis.md)
- [Architecture Comparison](architecture-comparison.md)
