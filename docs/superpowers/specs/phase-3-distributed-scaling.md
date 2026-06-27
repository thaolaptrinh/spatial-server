# Phase 3 — Distributed Scaling

> **Last Updated:** 2026-06-27
> **Status:** Draft

## Purpose

Phases 1Finish–2 deliver a production-grade single-server slice: DB-backed state, correct packet protocol, SpatialServerAPI, NPC simulation, metrics, and zone-state persistence. But the entire Runtime runs on **one Game Server** — the `GameServer` gRPC service only implements `Relay`, and Room Service is a single in-memory coordinator.

Phase 3 transforms the platform into a **multi-server cluster**. Zones are distributed across Game Servers, entities migrate across zone boundaries, the Room Service is highly available, and Gateways receive push-based routing updates. After this phase the platform scales horizontally: add Game Servers, and the coordinator rebalances zones across them.

This requires implementing the 7 remaining `GameServer` RPCs, the 2 remaining `RoomService` RPCs (`PrepareTransfer`, `TransferZone`), cross-zone AOI with ghost entities, zone migration (ADR-002), Room Service HA via K3s Lease, a heartbeat-timeout sweeper (ADR-011), and push-based routing-cache invalidation in the Gateway.

## Scope

- Implement remaining `GameServer` RPCs:
  - `AssignZone(AssignZoneRequest)` — Room Service directs a Game Server to own a zone
  - `ReleaseZone(ReleaseZoneRequest)` — Room Service directs a Game Server to release a zone
  - `ZoneStateSync(stream ZoneSnapshot)` — P2P streaming during zone transfer (source GS → target GS)
  - `MigrateEntity(MigrateEntityRequest)` — move an entity from one zone to another (cross-server)
  - `NotifyEntityEnter(EntityEnterLeave)` / `NotifyEntityLeave(EntityEnterLeave)` — cross-zone AOI boundary events
  - `SendEntityUpdate(EntityUpdate)` — forward entity position updates across zone boundaries
  - `QueryEntities(QueryEntitiesRequest)` — query entities in a radius (cross-zone AOI)
- Cross-zone AOI: when an entity nears a zone boundary, subscribe to the neighbor zone's owner. Ghost entities represent entities living in neighbor zones (ADR-003).
- Zone migration (ADR-002): Room Service initiates `PrepareTransfer` → source GS streams state via `ZoneStateSync` → target GS receives → Room Service updates ownership → clients reconnect.
- Room Service HA: K3s Lease API leader election (2 replicas, active/passive).
- Heartbeat-timeout sweeper: background goroutine marks servers `SHUTDOWN` after 3 missed heartbeats, reassigns orphaned zones (ADR-011).
- Push-based routing-cache invalidation: Gateway subscribes to Room Service ownership-change stream.

**Out of scope:**

- Session resumption / reconnection (Phase 4)
- TLS / mTLS (Phase 6)
- Autoscaling / HPA tuning (Phase 6)
- Incremental (pre-copy + delta) zone sync (future, ADR-002)

## Architecture

```
                          ┌──────────────────────────────┐
                          │       Room Service (HA)       │
                          │  Leader (K3s Lease) + Follower │
                          │  ┌────────────────────────┐   │
                          │  │ ServerRegistry (PgSQL)  │   │
                          │  │ ZoneOwnership (PgSQL)   │   │
                          │  │ Sweeper goroutine       │   │
                          │  │ OwnershipChange stream  │   │
                          │  └────────────────────────┘   │
                          └───┬──────────┬──────────┬─────┘
                  Register/HB │   Lookup │  Push    │ PrepareTransfer
              ┌───────────────┼──────────┼──────────┼────────────────┐
              │               │          │          │                │
   ┌──────────▼────┐  ┌───────▼───┐  ┌───▼────────┐          ┌──────▼─────┐
   │ Game Server A │  │  Gateway  │  │ Game Server│          │ Game Server│
   │  Zone Z1 (own)│  │  (n x)    │  │  B         │          │  C         │
   │  AOI grid Z1  │  │  RouterCache│ Zone Z2(own)│          │ Zone Z3(own)│
   │  ghost store  │  │  +push inv│  AOI grid Z2 │          │ AOI grid Z3│
   └───────┬───────┘  └─────┬─────┘  └─────┬──────┘          └─────┬──────┘
           │                 │ WSS          │                       │
           │   P2P gRPC      │             │   P2P gRPC            │
           │ NotifyEnter/    │             │  ZoneStateSync stream │
           │ Leave, Update,  │             │  (migration Z2→C)     │
           │ QueryEntities   │             │                       │
           └─────────────────┴─────────────┴───────────────────────┘

   Cross-zone AOI:  A ◄──► B   (neighbor subscription, ghost entities)
   Zone migration:  B ──state stream──► C   (Room Service orchestrates)
```

Data plane (client packets) is fully P2P: Gateway → Game Server → Game Server. Room Service handles only metadata — it is never on the hot path (ADR-004).

## Components

### 1. Multi-Zone Game Server (`internal/game/`)

The current `Game` struct holds a single global `*aoi.AOI` and flat `Entities`/`entityAOI` maps. Phase 3 makes the Game Server **zone-aware**: it may own multiple zones, each with its own AOI grid, and it tracks a peer registry for cross-server calls.

**Changes to `Game`:**

- Replace the single `aoi` field with `aoiIndex map[types.ZoneID]*aoi.AOI` (one grid per owned zone).
- Add `peerRegistry` — a mapping from `types.ZoneID` → neighbor Game Server gRPC address, populated when a zone is assigned and its neighbors resolved via Room Service `LookupZone`.
- Add `ghostStore map[types.ZoneID]map[types.EntityID]*ghostEntry` — ghost entities representing entities in neighbor zones (ADR-003, 500ms TTL already partially present in `ghostEntry`).
- `AssignZone` / `ReleaseZone` become real methods that create/destroy the per-zone AOI grid and entity sets, replacing the current stubs at `internal/game/game.go:122-128`.

**New fields on `Game`:**

| Field | Type | Purpose |
|-------|------|---------|
| `aoiIndex` | `map[types.ZoneID]*aoi.AOI` | One grid per owned zone |
| `zoneEntities` | `map[types.ZoneID]map[types.EntityID]*entity.Entity` | Entities partitioned by zone |
| `ghostStore` | `map[types.ZoneID]map[types.EntityID]*ghostEntry` | Ghosts from neighbor zones |
| `peers` | `*PeerRegistry` | Neighbor GS gRPC clients for cross-zone RPCs |
| `crossCh` | `chan crossZoneEvent` | Buffered channel for outbound cross-zone notifications |

### 2. Cross-Zone Peer Registry (`internal/game/peer.go`)

New file. A `PeerRegistry` holds live gRPC connections to neighbor Game Servers and exposes typed helpers around the `GameServer` client:

```go
type PeerRegistry struct {
    mu    sync.RWMutex
    conns map[types.ServerID]*peerConn
}
type peerConn struct {
    target types.ServerID
    addr   string
    conn   *grpc.ClientConn
    client spatialserverv1.GameServerClient
}
```

Methods: `Upsert(serverID, addr)`, `Get(zoneID)`, `NotifyEnter(...)`, `NotifyLeave(...)`, `SendEntityUpdate(...)`, `QueryEntities(...)`, `Close()`. Connections are lazy-dialed and cached. Mutual reachability between Game Servers is required (ADR-002 tradeoff).

### 3. GameServer RPC Implementations (`apps/game-server/main.go`)

The `gameServerServer` struct (currently only implements `Relay` at `apps/game-server/main.go:98`) gains the 7 new RPC handlers. Each delegates to the `Game` core via the `cmds` channel or direct method calls:

| RPC | Handler responsibility |
|-----|------------------------|
| `AssignZone` | Create zone record + AOI grid via `g.AssignZone`; subscribe to neighbors via `peers.Upsert` + `NotifyEntityEnter` fan-out |
| `ReleaseZone` | Tear down AOI grid, drop ghosts, notify neighbors via `NotifyEntityLeave`, call `g.ReleaseZone` |
| `ZoneStateSync` | Bidirectional streaming receiver: read `ZoneSnapshot` chunks from source GS, materialize entities into the per-zone AOI, then signal completion (used during migration, see Component 6) |
| `MigrateEntity` | Receive `EntitySnapshot` from neighbor, insert into target zone AOI, emit spawn to relay clients in range |
| `NotifyEntityEnter` | Register a ghost entity in `ghostStore` for the receiving zone |
| `NotifyEntityLeave` | Remove the ghost entity from `ghostStore` |
| `SendEntityUpdate` | Update ghost position + propagate to local entities whose AOI overlaps the ghost |
| `QueryEntities` | Return snapshots of entities within radius of a grid cell (used by neighbors building their ghost set) |

### 4. Cross-Zone AOI Boundary Detection (`internal/game/boundary.go`)

New file. Extends the existing `detectZoneBoundaries()` logic (`internal/game/game.go:183`) which currently only creates local ghosts. The new logic:

1. On each tick, for each entity near a zone edge (within `aoiRadius` of the zone border), determine which neighbor zones' AOI would overlap.
2. If a neighbor subscription does not yet exist for that zone, establish one (`peers.NotifyEnter`).
3. Periodically (configurable, default 1s) call `peers.QueryEntities` on the neighbor for the overlapping region, reconcile ghost set.
4. When an entity crosses fully into a neighbor zone, trigger `MigrateEntity` to the neighbor owner, remove locally, emit despawn to relay clients.

Ghost entities use the existing `ghostEntry` type with the 500ms TTL from ADR-003 (currently `ghostTTL = 5s` at `internal/game/game.go:81` — adjust to 500ms per ADR, make configurable).

### 5. Room Service: Ownership & Migration (`internal/room/` + `apps/room-service/main.go`)

The current Room Service stores state in-memory (`ServerRegistry`, `ZoneOwnership` in `internal/room/room.go`). Phase 1Finish wires these to PostgreSQL; Phase 3 adds the **coordinator logic** that sits on top.

**`PrepareTransfer(PrepareTransferRequest)`:**

1. Validate zone is `ACTIVE` and caller is the current owner (or Room Service itself).
2. Transition zone to `TRANSFERRING` (reject new entity writes — Gateway routing cache invalidated via push).
3. Return `accepted = true`.

**`TransferZone(stream ZoneSnapshot)`:** This RPC is the Room Service's record of a transfer, but the actual state bytes flow **P2P source → target** via the Game Server's `ZoneStateSync` (ADR-002). Room Service's `TransferZone` is the coordination channel: it receives completion snapshots and updates ownership atomically. The orchestration sequence:

1. Room Service calls `PrepareTransfer` internally → sets `TRANSFERRING`.
2. Room Service instructs source GS to begin streaming to target GS via direct gRPC (`ZoneStateSync`).
3. Source GS serializes zone state (entities, positions, attributes, in-memory AOI index — must be serializable per ADR-002/003).
4. Target GS loads state, begins simulation.
5. Target GS confirms → Room Service updates: `status = ACTIVE, server_id = target`.
6. Room Service pushes routing update to Gateways (Component 7).
7. Source GS releases zone resources.

### 6. Heartbeat-Timeout Sweeper (`internal/room/sweeper.go`)

New file. A background goroutine started in `apps/room-service/main.go`. Current code has no sweeper — `ServerInfo.LastBeat` (`internal/room/room.go:18`) is set but never checked.

**Behavior (ADR-011):**

- Tick every 5s (configurable).
- For each registered server, if `time.Since(LastBeat) > 15s` (3 missed 5s heartbeats):
  1. Mark server `SHUTDOWN`.
  2. Mark all its zones `ORPHAN`.
  3. For each orphan zone, pick `LeastLoaded()` ACTIVE server, call `AssignZone` on it, transition zone `ACTIVE`.
  4. New owner loads last-persisted zone state from PostgreSQL (Phase 2 persistence).
- Emit ownership-change events to the push stream (Component 7).
- Use a PostgreSQL advisory lock during reassignment to prevent split-brain across the HA pair (ADR-011, "PostgreSQL is the tiebreaker").

### 7. Push-Based Routing-Cache Invalidation (`internal/gateway/`)

The current `RouterCache` (`internal/gateway/gateway.go`) is a pure TTL cache (5s). Phase 3 adds a **subscription** to Room Service ownership changes.

**New proto RPC** (add to `room_service.proto`):

```proto
rpc WatchOwnership(WatchRequest) returns (stream OwnershipChange);
message WatchRequest {}
message OwnershipChange {
  string zone_id = 1;
  string server_id = 2;
  string host = 3;
  int32 port = 4;
}
```

**Gateway change:** `NewHandler` opens a long-lived `WatchOwnership` stream to Room Service on startup. Each `OwnershipChange` calls `RouterCache.Set(...)` (or `Invalidate(zoneID)` then re-resolve on next request), bypassing the TTL. This closes the 5s inconsistency window called out in ADR-021.

**Room Service change:** maintain an in-memory list of active watchers (Gateway stream handles). On any ownership mutation (`Claim`, `Release`, migration completion, sweeper reassignment), fan-out an `OwnershipChange` to all watchers.

### 8. Room Service HA via K3s Lease (`internal/room/leaderelection.go`)

New file. The Room Service deployment goes from 1 replica to 2 (active/passive). Only the leader serves `LookupZone`/`PrepareTransfer`/writes; the follower stands by.

- Use the Kubernetes coordination API (`coordination.k8s.io/v1` Lease) via `client-go`.
- Lease renew interval: 5s. Lease duration: 15s. Fallback acquire on leader death.
- On acquiring leadership: load full ownership table from PostgreSQL (ADR-011 — "New leader reads ownership table from PostgreSQL").
- On losing leadership: stop serving writes, drain in-flight RPCs.
- gRPC health check: leader returns `SERVING`, follower returns `NOT_SERVING` for the RoomService service so the K3s Service only routes to the leader.

**Docker Compose:** scale `room-service` to 2 replicas; the active replica is determined at runtime by lease acquisition.

## Data Flow

### Scenario A — Entity crosses zone boundary (cross-server AOI)

```
1. Entity E in Zone Z1 (Game Server A) moves toward Z1/Z2 boundary
2. Game A tick: detectZoneBoundaries() sees E within aoiRadius of Z2 edge
3. Game A peers.Lookup(Z2) → Game Server B address
4. Game A → B: NotifyEntityEnter{E, zone=Z2, pos}
5. Game B: register ghost E in ghostStore[Z2]; if local players in range, emit spawn
6. Each subsequent tick: Game A → B: SendEntityUpdate{E, pos} keeps ghost fresh
7. E fully crosses into Z2:
   a. Game A → B: MigrateEntity{snapshot(E), target_zone=Z2}
   b. Game B: materialize E into zoneEntities[Z2], AOI.Enter(E)
   c. Game A: removeEntity(E), AOI.Leave(E)
   d. Game B → A: NotifyEntityLeave{E} (remove ghost from A's perspective of B)
   e. Clients on A in range: receive despawn(E); clients on B in range: receive spawn(E)
```

### Scenario B — Zone migration (load rebalance, ADR-002)

```
1. Room Service decides Z2 (owner=B) should move to C (load rebalance)
2. Room Service: PrepareTransfer{zone=Z2, target=C} → B, C alerted
3. Room Service sets Z2 status = TRANSFERRING (Gateway cache invalidated via push)
4. Room Service instructs B to stream Z2 state to C
5. B → C: ZoneStateSync stream (chunked ZoneSnapshot: entities[], aoi_state bytes)
6. C: materializes entities, rebuilds AOI grid for Z2, confirms receipt
7. C → Room Service: ZoneStateSyncResponse{success=true}
8. Room Service: ownership = {Z2 → C}, status = ACTIVE (advisory lock guards TX)
9. Room Service: push OwnershipChange{Z2 → C} to all Gateways
10. B: ReleaseZone(Z2), tear down AOI grid, notify neighbors of ownership change
11. Clients reconnect via Gateway → routed to C (cache already updated)
```

### Scenario C — Game Server crash (ADR-011)

```
1. Game Server B stops heartbeating
2. Room Service sweeper: time.Since(B.LastBeat) > 15s
3. Mark B = SHUTDOWN, zones of B = ORPHAN
4. For each orphan zone Z2: LeastLoaded() = C
5. Room Service: AssignZone(Z2) → C (C loads last persisted state from PostgreSQL)
6. Push OwnershipChange to Gateways
7. Clients reconnect → routed to C
```

## Files Changed

| File | Action | Detail |
|------|--------|--------|
| `internal/game/game.go` | Modify | Multi-zone AOI map, per-zone entity partition, zone-aware AssignZone/ReleaseZone |
| `internal/game/peer.go` | Create | `PeerRegistry` + gRPC client cache for cross-zone RPCs |
| `internal/game/boundary.go` | Create | Cross-zone boundary detection, neighbor subscription, ghost reconciliation |
| `internal/game/boundary_test.go` | Create | Unit tests for boundary detection + ghost lifecycle |
| `internal/game/game_test.go` | Modify | Add multi-zone, cross-server AOI tests |
| `internal/game/aoi/aoi.go` | Modify | Add `Serialize()`/`Deserialize()` for zone transfer (AOI state in ZoneSnapshot) |
| `internal/game/aoi/aoi_test.go` | Modify | Serialization round-trip tests |
| `apps/game-server/main.go` | Modify | Implement AssignZone, ReleaseZone, ZoneStateSync, MigrateEntity, NotifyEntityEnter/Leave, SendEntityUpdate, QueryEntities |
| `apps/game-server/main_test.go` | Modify | gRPC handler tests for new RPCs |
| `internal/room/room.go` | Modify | Add watcher fan-out hooks on Claim/Release/migration |
| `internal/room/sweeper.go` | Create | Heartbeat-timeout sweeper goroutine + orphan reassignment |
| `internal/room/sweeper_test.go` | Create | Sweeper logic tests (fake clock) |
| `internal/room/leaderelection.go` | Create | K3s Lease leader election via client-go |
| `internal/room/migration.go` | Create | Migration orchestrator: PrepareTransfer → stream → confirm → ownership update |
| `internal/room/migration_test.go` | Create | Migration state-machine tests |
| `apps/room-service/main.go` | Modify | Wire sweeper, leader election, PrepareTransfer/TransferZone/WatchOwnership handlers |
| `internal/gateway/gateway.go` | Modify | Add push-invalidation subscriber + `Invalidate(zoneID)` on RouterCache |
| `internal/gateway/handler.go` | Modify | Start WatchOwnership stream on handler init |
| `internal/gateway/gateway_test.go` | Modify | Push-invalidation tests |
| `proto/spatialserver/v1/room_service.proto` | Modify | Add `WatchOwnership` RPC + `OwnershipChange`/`WatchRequest` messages |
| `proto/gen/spatialserver/v1/*.pb.go` | Regenerate | `make proto` |
| `deploy/docker-compose/docker-compose.yml` | Modify | Scale game-server to 2+, room-service to 2 replicas |
| `configs/game-server.yml` | Modify | Add `game.cross_zone.enabled`, `game.ghost_ttl`, neighbor config |
| `configs/room-service.yml` | Modify | Add `election.lease_name`, sweeper interval config |
| `tests/integration/migration_test.go` | Create | E2E zone migration + cross-zone AOI (Testcontainers) |

## References

- [Master Phase Roadmap](./master-phase-roadmap.md)
- [Phase 1G spec](./2026-06-26-phase1g-make-it-demoable.md) — format reference
- [ADR-002 Zone Migration](../../adr/002-zone-migration.md)
- [ADR-003 AOI Strategy](../../adr/003-aoi-strategy.md)
- [ADR-004 Coordinator](../../adr/004-coordinator.md)
- [ADR-011 Failure Recovery](../../adr/011-failure-recovery.md)
- [ADR-021 Gateway Architecture](../../adr/021-gateway-architecture.md)
- [gRPC convention](../../standards/grpc-convention.md)
- [Dependency rules](../../standards/dependency-rules.md)
