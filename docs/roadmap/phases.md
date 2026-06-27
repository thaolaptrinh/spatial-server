# Development Roadmap

> **Last Updated:** 2026-06-26

## Phase Overview

| Phase | Duration | Focus | Status |
|-------|----------|-------|--------|
| Phase 0 | 3–5 days | Architecture & specs (ADRs) | ✅ Complete |
| Phase 1A | 1 week | Foundation packages | ✅ Complete |
| Phase 1B | 1 week | Service skeletons | 📋 Planned |
| Phase 1C | 1 week | Communication | 📋 Planned |
| Phase 1D | 1 week | Runtime | 📋 Planned |
| Phase 1E | 1 week | Runtime features | 📋 Planned |
| Phase 2 | 2 weeks | Distributed scaling | 📋 Planned |
| Phase 3 | 2 weeks | Production hardening | 📋 Planned |

## Phase 0: Architecture (Complete)

```
- [x] Reference research (GoWorld, Pitaya)
- [x] Architecture design document
- [x] Team review and approval
- [x] 23 ADRs created (001–023)
- [x] Implementation plan created
```

## Phase 1A: Foundation Packages (Complete)

**Purpose:** Build all reusable library packages that services will depend on. No service binaries, no runtime, no networking.

**Deliverables:**

| Package | Responsibility | Status |
|---------|---------------|--------|
| `internal/types/` | EntityID, ZoneID, Vector3, status constants, error sentinels, UUIDv7 helpers | ✅ |
| `pkg/logging/` | slog wrapper, context propagation, standard fields | ✅ |
| `pkg/config/` | koanf loader, shared config structs | ✅ |
| `pkg/entity/` | Entity model, lifecycle interface, factory | ✅ |
| `pkg/zone/` | Zone struct, status machine, grid operations | ✅ |
| `pkg/aoi/` | Grid-based AOI index (Enter, Leave, Move, Query) | ✅ |
| `proto/` | 5 protobuf definitions, buf toolchain, generated Go code | ✅ |
| `configs/defaults.yml` | Shared configuration defaults | ✅ |
| Makefile, .golangci.yml | Build, test, lint tooling | ✅ |
| Docker Compose (PG + Redis) | Local dev infrastructure | ✅ |
| CI pipeline | Format, lint, test, build | ✅ |

**Non-goals:** Service binaries, migrations, networking, authentication, packet protocol, game loop.

**Definition of Done:**
- All packages compile: `go build ./...`
- All tests pass with race detection: `go test -race ./internal/... ./pkg/...`
- Lint passes: `golangci-lint run ./internal/... ./pkg/...`
- Committed to main

---

## Phase 1B: Service Skeletons (Planned)

**Purpose:** Create the three service binaries as shells that wire dependencies, start, and shut down gracefully. No business logic, no simulation, no networking beyond gRPC server bootstrap.

**Deliverables:**

| Component | Responsibility |
|-----------|---------------|
| `pkg/storage/` | PostgreSQL pool (pgx), Redis client (go-redis). No migration runner. |
| `pkg/auth/` | **Validate only.** Decode and verify runtime token signature. No JWT generation. |
| `pkg/protocol/` | Binary packet codec: header flags, packet IDs, gzip compression. |
| `pkg/session/` | Session struct, thread-safe session pool, ID→metadata mapping. |
| `internal/migration/` | Migration runner using golang-migrate. CLI or package. |
| `configs/gateway.yml` | WS port, JWT secret, rate limit config. |
| `configs/room-service.yml` | gRPC port, PG pool size, heartbeat params. |
| `configs/game-server.yml` | gRPC port, tick rate, max entities. |
| `apps/gateway/main.go` | Wire deps, start gRPC client, accept signals, graceful shutdown. No WebSocket acceptor yet. |
| `apps/room-service/main.go` | Wire deps, start gRPC server with health endpoint, run migrations, accept signals. |
| `apps/game-server/main.go` | Wire deps, start gRPC server with health endpoint, run ticker goroutine (stub), accept signals. |
| Dockerfiles | One per service in `build/docker/`. |
| Docker Compose | Wire 3 services + PG + Redis. |

**Non-goals:** WebSocket acceptor, packet routing, game loop logic, entity simulation, AOI integration, rate limiting.

**Definition of Done:**
- All three services start in Docker Compose
- Room Service connects to PostgreSQL and runs migrations
- Game Server registers with Room Service via gRPC
- Health endpoints return 200
- SIGTERM triggers graceful shutdown with no errors
- All unit tests pass

---

## Phase 1C: Communication (Planned)

**Purpose:** Implement client connectivity end-to-end. Gateway accepts WebSocket connections, authenticates via JWT, routes packets to the correct Game Server via Room Service lookup, and forwards packets over a shared gRPC stream.

**Deliverables:**

| Component | Responsibility |
|-----------|---------------|
| `pkg/gateway/` | WebSocket acceptor (nhooyr.io), JWT validation from Phase 1B, router cache (TTL 5s), per-connection session lifecycle. Rate limiting deferred. |
| `pkg/room/` | Game Server registry, zone ownership (in-memory), heartbeat handling, `LookupZone` gRPC handler. |
| Gateway↔Room Service | `LookupZone` gRPC unary call, cache result. |
| Gateway↔Game Server | Single shared bidirectional gRPC stream per Game Server. Client packets multiplexed with `client_id` header. |
| Room Service↔Game Server | `Register`, `Heartbeat` gRPC unary calls. |
| Health endpoint | Gateway: WebSocket health check. Room Service: gRPC health. Game Server: gRPC health. |

**Non-goals:** Game loop logic, entity simulation, AOI queries, zone boundary detection, rate limiting.

**Definition of Done:**
- A WebSocket client can connect to Gateway
- Gateway validates JWT token
- Gateway calls Room Service `LookupZone` and caches the result
- Gateway opens shared gRPC stream to Game Server
- Client packets arrive at Game Server inbox
- All unit tests pass

---

## Phase 1D: Runtime (Planned)

**Purpose:** Implement the Game Server simulation loop with entity and zone management. Process inbound packets from the Gateway stream. No AOI integration yet.

**Deliverables:**

| Component | Responsibility |
|-----------|---------------|
| `pkg/game/` | Game loop with 20Hz ticker, `context.Context` lifecycle, inbound channel drain, entity map, zone map. |
| Entity lifecycle | `Spawn`, `Despawn`, position update from inbound packets. |
| Zone lifecycle | `AssignZone`, `ReleaseZone`, zone status transitions. |
| Packet dispatch | Route inbound packets by `PacketID` to handler functions. Stub handlers for position update, entity action. |
| Outbound stub | Build outbound packet queue (no AOI filtering yet — broadcast to all connected clients). |

**Non-goals:** AOI integration, entity visibility filtering, zone boundary detection, ghost entities.

**Definition of Done:**
- Game loop runs at 20Hz
- Inbound packets from Gateway are drained each tick
- Entity spawn/despawn commands create and destroy entities
- Zone assign/release updates zone state
- Stub outbound writes data to gRPC stream
- All unit tests pass

---

## Phase 1E: Runtime Features (Planned)

**Purpose:** Integrate AOI into the Game Server so entities only receive updates about visible entities. Add zone boundary detection for ghost entity support (same-server only; cross-server in Phase 2).

**Deliverables:**

| Component | Responsibility |
|-----------|---------------|
| AOI integration | On entity spawn → `aoi.Enter()`. On position update → `aoi.Move()`. On despawn → `aoi.Leave()`. |
| Entity visibility | Per-entity AOI query each tick. Build `EntitySpawn`/`EntityDespawn`/`EntityMove` packets per client. |
| Snapshots | Full state sync on AOI enter. Delta updates on position change. |
| Zone boundary detection | Detect when entity crosses zone grid boundary (same Game Server). Spawn ghost entity in old zone with TTL. |
| Outbound filter | Only send packets to clients whose entity is in the sender's AOI range. |

**Non-goals:** Cross-GameServer zone transfer, ghost entity migration, persistent entity state.

**Definition of Done:**
- Two entities in the same zone see each other (AOI within range)
- Entity leaving AOI range receives `EntityDespawn`
- Entity crossing zone boundary creates ghost entity in old zone
- Ghost entity despawns after TTL expiry
- All unit tests pass
- Integration test: 3+ entities synchronize correctly within one Game Server

---

## Phase 2: Distributed Scaling (Planned)

**Deliverables:**
- Multiple Game Server support
- Room Service zone ownership table persisted in PostgreSQL
- Zone transfer between Game Servers
- Gateway routing by zone (lookup → cache → forward)
- Game Server heartbeat + crash recovery (orphan zone detection)
- Room Service leader election (K3s Lease API)
- Load-based zone rebalancing
- Cross-server ghost entity migration
- Prometheus metrics + Grafana dashboards
- Simulation framework (v1 for basic load testing)

**Non-goals:** Production K3s deployment, production hardening, autoscaling.

---

## Phase 3: Production Hardening (Planned)

**Deliverables:**
- K3s manifests (reference, not full implementation)
- HPA configuration (custom metrics)
- Load testing + optimization (via simulation framework)
- Chaos testing (partition, crash, latency injection)
- LZ4/Snappy packet compression
- TLS for WebSocket
- mTLS for internal gRPC (optional)
- Production-ready K3s manifests
- Monitoring alerts (PagerDuty / Slack)
- Documentation for API consumers
- Benchmarking report

---

## References

- [Architecture Overview](../architecture/overview.md)
- [ADR Index](../adr/README.md)
- [Phase 1B Spec](../superpowers/specs/2026-06-26-phase1b-service-skeletons.md)
