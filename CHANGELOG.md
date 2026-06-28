# CHANGELOG

## v0.1.0-alpha (2026-06-28)

### Architecture

- **ADR-025: Platform Terminology Model** — four‑layer model (Platform → Deployable Services → Runtime Concepts → Business Concepts). Canonical terms: Space, Runtime Node, Gateway, Room Service, Game Server.
- **ADR-026: Runtime Extension Model** — three interface‑based extension points: `Lifecycle` (per‑entity), `Event`s (outbound), opaque `Attrs` (data). No plugin framework.

### Runtime Node Identity

- **`NodeDescriptor`** (`internal/room/node.go`) — immutable `NodeID` (UUIDv7, not hostname), `AdvertiseAddr` (platform‑independent routing), `Capacity`, `Load`, `Labels`, `Version`/`Build`.
- **`Allocator` interface** — `Select(descriptors) → descriptor`. `LeastLoadedAllocator` implemented. Room service routes zone ownership through the allocator.
- **Richer heartbeat** — `HeartbeatRequest` reports scheduling‑relevant load (active entities, spaces, users, queue depth, tick duration).

### Simulation Loop (Runtime Core)

- **Event model** — simulation publishes internal `Event`s (Spawn/Despawn/Move/State) instead of encoding wire packets. Wire encoding lives in the `apps/game‑server` adapter.
- **Open/Closed simulation loop** — `simulate()` invokes generic `Lifecycle.OnSimulate(e, dt)`, never type‑switches on concrete types. Movement detected by position diff.
- **Measured wall‑clock dt** — `simulate()` receives actual elapsed time (capped 250ms), not nominal tick rate.
- **Bounded inbox drain** — at most 1024 packets per tick to prevent cascade overload.
- **Control command protection** — `EnqueueAddEntity`/`RemoveEntity` block briefly (100ms) before dropping loudly (log + count).

### Observability

All 10 Prometheus metrics wired into the hot path: `tick_duration_seconds`, `tick_overruns_total`, `queue_depth`, `dropped_total`, `runtime_events_total`, `entity_count`, `active_spaces`, `active_connections`, `packets_per_sec_total`, `grpc_request_duration_seconds`. Queue depths exported per tick; drops counted on every backpressure path.

### Entity Model

- **Generic `EntityState`** — removed business‑specific `animation`/`health` proto fields (reserved 2,3). State carried as opaque `map<string,bytes> attributes`.
- **`Behavior` tag moved** from core `Entity` struct → opaque `Attrs`. `NPCLifecycle` owns the behavior object.
- **Ownership types separated** — `ServerID` (node), `PlayerID` (participant), `OwnerID` (entity controller) are distinct types.

### Gateway

- **Drain check** — `handleWS` rejects new connections during graceful drain.
- **Context propagation** — relay uses shutdown‑scoped base context (`SetBaseContext`) rather than `context.Background()`.
- **Dial timeout** — 3s deadline on relay gRPC dial, preventing indefinite block.
- **Drop metrics** — rate‑limit and Inbox drops counted and exported.

### Distributed Correctness

7 deterministic invariants validated:
1. Entity uniqueness across nodes
2. Ownership transfer preserves count
3. Cross‑node AOI ghost synchronization
4. Ghost expiration (no leaks)
5. Space isolation across nodes
6. Randomized workload (1000 iterations) — all invariants hold
7. Long‑run (500 ticks) — no entity/ghost leaks

### Benchmark Framework

- `benchmarks/runtime/` — in‑process harness with configurable users, movement patterns, tick rate; capacity stages 50→1000.
- `benchmarks/framework/` — Histogram (p50/p95/p99), pprof helpers.
- `benchmarks/e2e/` — distributed WS benchmark against real stack (JWT minting, provisioning, paired clients, round‑trip latency).

### Breaking Changes

- **Proto:** `RegisterRequest` extended with `advertise_addr`, `version`, `build`, `labels`. `HeartbeatRequest.server_id` → `node_id` + load fields. `LookupZoneResponse` added `advertise_addr`; `host`/`port` deprecated but still populated for back‑compat.
- **Proto:** `EntityState.animation`/`health` removed (reserved 2,3). Use `attributes` map.
- **ServerStore interface:** `Register` takes `*NodeDescriptor`; `Heartbeat(id, NodeLoad)`; `LeastLoaded` replaced by `List` (consumers use `Allocator`).
- **`ZoneLookuper` interface:** `LookupZone(...) (string, error)` (single routable address) instead of `(host string, port int32, err error)`.
- **Game public API:** `Tick()` added (step‑mode). `Events` channel. `EntitiesNearGrid` now requires `space` parameter.

### Migration Notes

- **Postgres:** `game_servers` table unchanged (host/port columns kept). New `advertise_addr` column recommended in a future migration.
- **Dockerfiles:** `room‑service.Dockerfile` and `game‑server.Dockerfile` both now `COPY internal/storage/migrations/` (stale path `pkg/...` removed).
- **Compose:** `game‑server` service now requires `SPATIAL_POSTGRES__DSN` env and `postgres` health dependency. Multi‑node available via `docker‑compose.multinode.yml`.

### Known Limitations

See `docs/milestones/v0.1.0‑alpha.md`.
