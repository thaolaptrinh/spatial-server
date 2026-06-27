# Phase 1Finish — Hardened Vertical Slice

> **Last Updated:** 2026-06-27
> **Status:** Draft

## Purpose

Phase 1G delivered a demoable single-server vertical slice: `make demo` connects a client through WebSocket → Gateway → gRPC Relay → Game Server → AOI entity spawn/move/despawn. However the slice rests on several shortcuts that block production readiness:

1. **State is volatile.** Room Service tracks zone ownership and the Game Server registry in in-memory maps (`pkg/room/room.go`). A restart loses every ownership assignment and every registered server — the platform cannot survive a single process restart.
2. **The packet protocol deviates from its own ADR.** `pkg/protocol/protocol.go` encodes a 3-byte header (`flags + packetID`), but [ADR-010](../../adr/010-packet-protocol.md) mandates `version + packetID + msgType + flags + sequence`. Sequence numbers are required for reliable ordering and future delta compression.
3. **No business API.** The `SpatialServerAPI` service (runtime lifecycle: create/destroy/list) is declared in `proto/spatialserver/v1/spatial_server_api.proto` but has no implementation. Runtimes cannot be created or inspected through the API surface.
4. **No real integration tests.** `test/integration/realtime_test.go` is a `t.Skip()` placeholder; correctness is only verified by running `make demo` by hand.
5. **Config is duplicated.** All three service mains re-implement the same koanf bootstrap inline, despite `pkg/config.Config` existing for this purpose. Gateway-specific knobs (`gateway.ws_port`, `room_service.addr`) are not modeled in the shared struct.

**Phase 1Finish closes these five gaps.** After this phase the vertical slice is durable, protocol-compliant, API-driven, integration-tested, and consistently configured — a defensible foundation for the Phase 2 realtime features and Phase 3 distributed scaling.

## Scope

- Wire Room Service ownership + registry to PostgreSQL via a `pgx`-backed repository layer (replace in-memory maps; honor [ADR-001](../../adr/001-zone-ownership.md))
- Align the packet protocol header with ADR-010: add version (1B) + sequence number (4B) to the existing 3-byte header → **8-byte total**, update `Encode`/`Decode` and every caller
- Implement the `SpatialServerAPI` service (`CreateRuntime`, `DestroyRuntime`, `GetRuntimeInfo`, `ListRuntimes`, `GetRuntimeMetrics`) on the Room Service, backed by the `runtimes` table
- Replace the skipped integration test with a real Testcontainers harness that brings up Postgres + Redis + all three services and verifies spawn/move/despawn end-to-end through a WebSocket client
- Harden the Gateway: `/ready` (probes Room Service gRPC health), `/live` (process alive), 64 KiB max packet size enforcement, graceful drain on SIGTERM (stop accepting new connections, finish active streams)
- Refactor all three service mains to load `pkg/config.Config` consistently (extend the struct with `Gateway`, `RoomService`, `Game`, `SpatialServerAPI` sections); keep the koanf env provider (`SPATIAL_` prefix, `__` → `.`)

**Out of scope:**
- Multi-server coordination, zone migration, cross-zone AOI (Phase 3)
- Metrics endpoints and Prometheus (Phase 2)
- TLS / mTLS (Phase 6)
- NPC AI / entity simulation loop (Phase 2)
- Session resumption and backpressure (Phase 4)

## Architecture

```
                      ┌─────────────────────────────────────────┐
   Client (WSS) ─────▶│  Gateway :8080                          │
                      │  /ws   JWT → LookupZone → Relay stream  │
                      │  /ready  (probes room-service gRPC)     │
                      │  /live   (process alive)                │
                      │  64 KiB packet cap · SIGTERM drain      │
                      └──────────────┬──────────────────────────┘
                                     │ gRPC (Relay bidi stream)
                          ┌──────────▼──────────┐
                          │  Game Server :9000  │
                          │  Entity sim + AOI   │
                          │  Register/Heartbeat │──┐
                          └─────────────────────┘  │ gRPC
                                                   │
                          ┌───────────────────────▼──────────┐
                          │  Room Service :9000              │
                          │  RoomService  (Register/         │
                          │    Heartbeat/LookupZone)         │
                          │  SpatialServerAPI (CreateRuntime │
                          │    /DestroyRuntime/Info/List/    │
                          │    Metrics)                      │
                          │  pgx repository layer ─────┐     │
                          └────────────────────────────┼─────┘
                                                       │
                          ┌────────────────────────────▼───┐
                          │  PostgreSQL                    │
                          │  game_servers · runtimes ·     │
                          │  zones  (migration 001)        │
                          └────────────────────────────────┘
                          ┌────────────────────────────────┐
                          │  Redis  (cache / pubsub ready) │
                          └────────────────────────────────┘

  Config: every service loads pkg/config.Config{Service,Logging,GRPC,
          Postgres,Redis,Gateway,RoomService,Game,SpatialServerAPI}
          via koanf (file + SPATIAL_ env).
```

## Components

### 1. PostgreSQL-backed Repository Layer

Replace the in-memory `ServerRegistry` and `ZoneOwnership` with a repository that reads and writes the tables already defined in `pkg/storage/migrations/001_initial.up.sql` (`game_servers`, `runtimes`, `zones`).

- **New file `pkg/storage/repository.go`** defines three repositories, each accepting a `*pgxpool.Pool`:
  - `ServerRepository` — `Register`, `Heartbeat` (updates `last_heartbeat`, `status='active'`), `Get`, `LeastLoaded` (`WHERE status='active' AND … ORDER BY zone_count`), `Remove`. Backed by `game_servers`.
  - `ZoneRepository` — `Claim` (`INSERT … ON CONFLICT DO NOTHING`, checks affected rows for the unique `(runtime_id, grid_x, grid_y)` constraint per ADR-001), `Release`, `Lookup`, `AssignOnMiss` (atomic claim against the least-loaded server). Backed by `zones`.
  - `RuntimeRepository` — `Create`, `Destroy` (`status='destroyed'`, set `destroyed_at`), `Get`, `List` (paginated via cursor), `CountPlayers`. Backed by `runtimes`.
- **Modify `pkg/room/room.go`**: keep the public method shapes (`Register`, `Heartbeat`, `LookupZone`, `ResolveZone`) but back them by the repository. Introduce a `Store` interface in `pkg/room` (consumer package, per the coding standard) so `room` depends on an interface, not on `pgx` directly. `apps/room-service/main.go` injects the concrete `storage.ServerRepository`/`ZoneRepository`.
- All SQL uses `pgxpool` with `context.Context` plumbing and wrapped errors (`fmt.Errorf("claim zone %s: %w", id, err)` per `docs/standards/error-handling.md`). No `"failed to"` prefixes.
- Heartbeat-timeout sweeping (orphan reassignment per ADR-001) is **not** implemented here — it requires multi-server and is Phase 3. This phase only persists ownership; the in-process Game Server is the sole owner.

### 2. Packet Protocol Alignment (ADR-010)

Bring `pkg/protocol/protocol.go` into compliance with [ADR-010](../../adr/010-packet-protocol.md). The ADR's full header is 9 bytes (`version + packetID + msgType + flags + sequence`); this phase implements the **8-byte subset** by folding `msgType` into the packetID space (every packet body is a protobuf, so a separate message-type byte is currently redundant — `msgType` is reserved and will be split out if compression/encryption flags need disambiguation):

```
Offset  Size  Field
0       1     Protocol version (constant ProtocolVersionV1 = 0x01)
1       2     Packet ID (uint16, big-endian)
3       1     Flags (bit0 = compressed)
4       4     Sequence number (uint32, big-endian)
8       …     Payload (protobuf bytes; gzip-compressed if flag set)
```

- `const headerSize = 8`, add `ProtocolVersion` and `ProtocolVersionV1`.
- `Encode(id, payload, compress, seq uint32)` now takes a sequence number and writes the 8-byte header. A zero `seq` is legal (un-sequenced control packets).
- `Decode(packet)` returns `(version, id, payload, compressed, seq, err)`. A version mismatch returns a typed error so the Gateway can drop the frame rather than process it.
- **Callers updated:** `pkg/game/game.go` (`encodeSpawn`/`encodeDespawn`/`dispatch`), `tools/client/main.go` (read/write loops now read sequence and stamp an incrementing counter), and the new integration test client.
- Compression threshold/level constants are unchanged.

### 3. SpatialServerAPI Service

Implement the five RPCs declared in `proto/spatialserver/v1/spatial_server_api.proto`, registered on the Room Service gRPC server alongside `RoomService`.

- **New file `pkg/api/spatial_server.go`** (`type SpatialServerAPIServer struct{ … }`) depending on `pkg/room.RuntimeRepository` (interface) — consumer-defined interface, no `pgx` import in `pkg/api`.
- **`CreateRuntime`**: `INSERT INTO runtimes (id, zone_count, zone_size, status='creating')`, then provision `zone_count` rows in `zones` (`grid_x`/`grid_y` derived from `zone_size`, status `unowned`), flip runtime `status='active'`, and return `CreateRuntimeResponse{ runtime_id, zone_count, zones[] }`. The first `LookupZone` against any of its zones lazily assigns ownership to the least-loaded registered Game Server.
- **`DestroyRuntime`**: set `status='draining'` then `status='destroyed'` (cascades to `zones` via the `ON DELETE CASCADE` FK). Return `{success: true}`.
- **`GetRuntimeInfo`**: single-row lookup returning status, zone count (from `zones`), player count (from `CountPlayers` — a stub returning 0 in this phase; real counts come with Phase 2 metrics), `created_at`.
- **`ListRuntimes`**: cursor pagination over `runtimes WHERE status != 'destroyed'`; `page_token` is the last seen `id`.
- **`GetRuntimeMetrics`**: returns placeholder metrics (player count 0, latencies 0) — fully wired in Phase 2 once `/metrics` exists.
- Registered in `apps/room-service/main.go`: `spatialserverv1.RegisterSpatialServerAPIServer(srv, …)`. Health status `spatialserver.v1.SpatialServerAPI` set to `SERVING`.

### 4. Gateway Hardening

Extend `pkg/gateway/handler.go` and `pkg/gateway/gateway.go` per [ADR-021](../../adr/021-gateway-architecture.md):

- **`/live`**: always 200 OK `{ "status": "alive" }` — kubelet liveness probe, never depends on dependencies.
- **`/ready`**: 200 OK only if (a) the Room Service gRPC `Health.Check` returns `SERVING` and (b) the gateway is not draining. Otherwise 503. Uses a pooled `grpc_health_v1.HealthClient` to `room_service.addr` with a 2s timeout.
- **64 KiB max packet**: enforce in the WS-read pump via `conn.SetReadLimit(64 * 1024)` (coder/websocket) and additionally reject any decoded frame whose declared payload would exceed the cap. Oversized frames close the connection with code 1009 (Message Too Big).
- **Connection counter**: `atomic.Int64` on `Handler`; `/ready` returns 503 when count ≥ soft limit (9000) per ADR-021. Hard limit (10000) rejects new upgrades with 503.
- **Graceful drain on SIGTERM**: `apps/gateway/main.go` sets a `draining atomic.Bool` on signal; `/ready` flips to 503 immediately; `http.Server.Shutdown(ctx)` with a 30s deadline lets active Relay streams finish. New WS upgrades are refused with 503 once draining.

### 5. Config Consolidation

Extend `pkg/config.Config` so every service main calls `config.Load("configs/defaults.yml", "configs/<service>.yml")` once and reads typed fields.

- **Modify `pkg/config/config.go`**: add
  ```go
  Gateway          GatewayConfig          `koanf:"gateway"`           // WSPort, JWTSecret, MaxPacketSize, SoftConnLimit, HardConnLimit, DrainTimeout
  RoomService      RoomServiceConfig      `koanf:"room_service"`      // Addr
  Game             GameConfig             `koanf:"game"`              // TickRate, MaxEntities, ZoneCellSize, AOIRadius
  SpatialServerAPI SpatialServerAPIConfig `koanf:"spatial_api"`       // DefaultZoneCount, DefaultZoneSize
  ```
  Keep the `SPATIAL_` env prefix and `__`→`.` transform; add unit tests for the new sections (env override `SPATIAL_GATEWAY__WS_PORT=9090` etc.).
- **Modify all three mains** (`apps/gateway/main.go`, `apps/room-service/main.go`, `apps/game-server/main.go`): delete the inline `koanf.New(".")` blocks; replace with `cfg, err := config.Load(…)`; read `cfg.Gateway.WSPort`, `cfg.RoomService.Addr`, `cfg.Game.TickRate`, etc.
- **Modify configs**: `configs/gateway.yml` already exposes `gateway.*` and `room_service.addr`; `configs/game-server.yml` already exposes `game.*`. Add `gateway.max_packet_size: 65536`, `gateway.soft_conn_limit: 9000`, `gateway.hard_conn_limit: 10000`, `gateway.drain_timeout: 30s` to `configs/defaults.yml`.

### 6. Testcontainers Integration Test

Replace the `t.Skip()` in `test/integration/realtime_test.go` with a real end-to-end test.

- **New file `test/integration/harness.go`**: Testcontainers helpers — `StartPostgres(t)` (runs `migration.Run` against the container DSN), `StartRedis(t)`, `StartService(t, name, cfg)` (builds the binary with `go build` and `exec.Command`s it with `SPATIAL_*` env, returning address + a `Cleanup` that sends SIGTERM and waits). One Postgres + one Redis container shared by the three services.
- **Modify `test/integration/realtime_test.go`**: `TestEndToEnd_SpawnMoveDespawn` boots room-service → game-server → gateway in order, polls each `grpc_health_v1` / `/ready` until ready, then drives a `coder/websocket` client:
  1. Mint a JWT (HS256, `dev-secret-key-change-in-production`) for `player_id=p1`, `runtime_id=r1`, `zone_id=z1`.
  2. Call `SpatialServerAPI.CreateRuntime({runtime_id: "r1", zone_count: 1, zone_size: 100})`.
  3. Dial `ws://gateway/ws?token=…`, assert the seeded NPC `EntitySpawn` arrives (8-byte header decodes cleanly).
  4. Send three `PositionUpdate(0x03)` frames with incrementing sequence numbers; assert an `EntityMove(0x06)` echoes back.
  5. Close the WS; assert `EntityDespawn(0x05)` is observed by a second observer client (or assert the Game Server `EntityCount()` decrements via a debug RPC).
- Gate behind `//go:build integration` and `go test -tags=integration ./test/integration/...`. Add `testcontainers-go`, `testcontainers-go/postgres`, `testcontainers-go/redis` to `go.mod`.

## Data Flow

**Runtime creation → first connection (new path):**
```
1. Operator → SpatialServerAPI.CreateRuntime(r1, zone_count=4)
2. Room Service → RuntimeRepository.Create → INSERT runtimes; INSERT 4 zones (unowned)
3. Room Service → flip runtime status='active' → return zones[]
4. Client → Gateway /ws?token (runtime_id=r1, zone_id=z1)
5. Gateway → RoomService.LookupZone(z1)
6. Room Service → ZoneRepository.Lookup (miss) → ServerRepository.LeastLoaded → ZoneRepository.Claim
   (INSERT … server_id=<game-server>, status='active'; unique constraint guarantees single owner)
7. Room Service → return {host, port} of owning Game Server
8. Gateway → Relay stream → Game Server spawns avatar entity
9. Game Server → EntitySpawn(0x04) frame (8-byte header, seq=N) → Gateway → WS → Client
```

**Graceful drain (new path):**
```
1. SIGTERM received by gateway main
2. draining.Store(true) → /ready returns 503 immediately (LB stops sending)
3. http.Server.Shutdown(ctx 30s) — no new /ws upgrades accepted
4. Active Relay streams: read pump sees ctx cancel → sends KIND_DISCONNECT → stream closes
5. Game Server removes entities, emits EntityDespawn to remaining observers
6. Gateway process exits after last connection drains or deadline
```

## Files Changed

| File | Action |
|------|--------|
| `pkg/config/config.go` | Modify — add `Gateway`, `RoomService`, `Game`, `SpatialServerAPI` sections |
| `pkg/config/config_test.go` | Modify — cover new sections + env overrides |
| `pkg/protocol/protocol.go` | Modify — 8-byte header, version + sequence |
| `pkg/protocol/protocol_test.go` | Create — round-trip, version-mismatch, sequence-ordering tests |
| `pkg/storage/repository.go` | Create — `ServerRepository`, `ZoneRepository`, `RuntimeRepository` (pgx) |
| `pkg/storage/repository_test.go` | Create — repository unit tests with a Testcontainers pg |
| `pkg/room/room.go` | Modify — back by `Store` interface; drop in-memory maps |
| `pkg/room/room_test.go` | Modify — adapt to repository-backed implementation |
| `pkg/api/spatial_server.go` | Create — `SpatialServerAPI` service implementation |
| `pkg/api/spatial_server_test.go` | Create — RPC unit tests with a fake `RuntimeRepository` |
| `pkg/gateway/handler.go` | Modify — `/ready`, `/live`, read-limit, drain check |
| `pkg/gateway/gateway.go` | Modify — connection counter, drain flag, soft/hard limits |
| `pkg/gateway/handler_test.go` | Modify — cover `/ready` healthy/draining, oversized frame |
| `pkg/game/game.go` | Modify — update `Encode`/`Decode` call sites for 8-byte header |
| `apps/gateway/main.go` | Modify — `config.Load`, drain-on-SIGTERM, `/ready` probe wiring |
| `apps/room-service/main.go` | Modify — `config.Load`, inject repositories, register `SpatialServerAPI` |
| `apps/game-server/main.go` | Modify — `config.Load`, updated protocol call sites |
| `tools/client/main.go` | Modify — stamp/read sequence numbers on encode/decode |
| `test/integration/harness.go` | Create — Testcontainers boot helpers |
| `test/integration/realtime_test.go` | Modify — replace `t.Skip()` with full E2E test |
| `configs/defaults.yml` | Modify — add `gateway.max_packet_size`, conn limits, `drain_timeout` |
| `configs/gateway.yml` | Modify — align with new `GatewayConfig` fields |
| `configs/game-server.yml` | Modify — align with new `GameConfig` fields |
| `configs/room-service.yml` | Modify — add `spatial_api` defaults |
| `go.mod` / `go.sum` | Modify — add `testcontainers-go` (+ postgres, redis modules) |
| `Makefile` | Modify — `make test-integration` target (`go test -tags=integration ./test/integration/...`) |

## References

- [ADR-001 Zone Ownership](../../adr/001-zone-ownership.md)
- [ADR-010 Packet Protocol](../../adr/010-packet-protocol.md)
- [ADR-021 Gateway Architecture](../../adr/021-gateway-architecture.md)
- [ADR-023 Entity Model](../../adr/023-entity-model.md)
- [Master Phase Roadmap](./master-phase-roadmap.md)
- [Phase 1G spec](./2026-06-26-phase1g-make-it-demoable.md) — predecessor
- [Phase 2 spec](./phase-2-realtime-features.md) — successor
- [Error handling standard](../../standards/error-handling.md)
- [Configuration standard](../../standards/configuration.md)
