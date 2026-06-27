# Data Model

> **Last Updated:** 2026-06-26

## Purpose

Document all data entities in the Spatial Server system, their fields, storage location, and lifecycle. This is the authoritative reference for what data exists, where it lives, and how it flows between components.

## Data Flow Diagram

```mermaid
graph TB
  subgraph PostgreSQL["PostgreSQL (Source of Truth)"]
    R[("runtimes")]
    Z[("zones")]
    GS_DB[("game_servers")]
    ZS[("zone_state (planned)<br/> serialized snapshots")]
  end

  subgraph Redis["Redis (Cache / Event Bus)"]
    SC[("session:{player_id}")]
    MC[("cache:*")]
    PB[("pub/sub channels")]
  end

  subgraph GameServerMem["Game Server (In-Memory)"]
    ENT["entities<br/>(EntityID → Entity)"]
    AOI["AOI index<br/>(grid cells → entities)"]
    SUB["subscriptions<br/>(player → interest set)"]
  end

  subgraph GatewayMem["Gateway (In-Memory)"]
    RT["routing cache<br/>(zone → GameServer, TTL 5s)"]
    WS["WebSocket connections<br/>(per-client state)"]
  end

  subgraph RoomServiceMem["Room Service (In-Memory)"]
    OWN["ownership cache<br/>(zone → server_id)"]
    REG["Game Server registry<br/>(server_id → address)"]
  end

  R --> OWN
  Z --> OWN
  GS_DB --> REG
  OWN --> RT

  ENT --> ZS
  ENT --> AOI
  ENT --> SUB
```

## Runtime

A runtime is an instantiated realtime session corresponding to a business entity (room/showroom/meeting). It is the top-level lifecycle unit.

| Field | Type | Storage | Description |
|-------|------|---------|-------------|
| `runtime_id` | UUIDv7 | PostgreSQL (`runtimes`) | Unique identifier |
| `status` | enum | PostgreSQL (`runtimes`) | `creating`, `active`, `draining`, `destroyed` |
| `zone_count` | integer | PostgreSQL (`runtimes`) | Number of zones (grid cells) allocated |
| `player_count` | integer | PostgreSQL (`runtimes`) | Current connected player count |
| `created_at` | timestamp | PostgreSQL (`runtimes`) | Creation time |
| `updated_at` | timestamp | PostgreSQL (`runtimes`) | Last status change |
| `destroyed_at` | timestamp (nullable) | PostgreSQL (`runtimes`) | Destruction time (set when status = `destroyed`) |
| `metadata` | JSONB (nullable) | PostgreSQL (`runtimes`) | Optional Business Backend metadata (opaque to Spatial Server) |

**Lifecycle:** Created by `CreateRuntime` gRPC → transitions through states (creating → active → draining → destroyed). Destroyed records may be retained briefly for audit, then pruned by TTL.

**Storage:** PostgreSQL is the source of truth. Redis may cache runtime metadata for lookups (TTL-bounded). In-memory on Room Service (leader cache).

## Zone

A zone is a grid cell within a runtime. Each zone is owned by exactly one Game Server at any time.

| Field | Type | Storage | Description |
|-------|------|---------|-------------|
| `zone_id` | UUIDv7 | PostgreSQL (`zones`) | Unique identifier |
| `runtime_id` | UUIDv7 | PostgreSQL (`zones`) | Parent runtime |
| `server_id` | UUIDv7 (nullable) | PostgreSQL (`zones`) | Owning Game Server ID |
| `grid_x` | integer | PostgreSQL (`zones`) | Grid column index |
| `grid_y` | integer | PostgreSQL (`zones`) | Grid row index |
| `status` | enum | PostgreSQL (`zones`) | `unowned`, `active`, `transferring`, `orphan` |
| `size` | double precision | PostgreSQL (`zones`) | Zone edge length (world units) |
| `heartbeat_expires_at` | timestamp (nullable) | PostgreSQL (`zones`) | Ownership lease expiration (planned) |
| `entity_count` | integer | PostgreSQL (`zones`) | Current entity count (approximate, planned) |
| `created_at` | timestamp | PostgreSQL (`zones`) | Creation time (planned) |
| `last_persisted_at` | timestamp (nullable) | PostgreSQL (`zone_state`) | Last snapshot time |

**Lifecycle:** Created with runtime → assigned to a Game Server (`active`) → may transfer between servers (`transferring` → `active`) → on Game Server crash becomes `orphan` → reassigned → on runtime destroy, zone records deleted.

**Storage:** PostgreSQL is the source of truth for ownership (`zones` table). Zone state snapshots (serialized entity data) are planned for the `zone_state` table (not yet implemented — entities are in-memory only). AOI index is purely in-memory on the owning Game Server and does NOT exist in PostgreSQL or Redis.

## Entity

An entity is a dynamic object simulated within a runtime — a player, NPC, item, or any interactive object. *(Currently only `player` entities are implemented; `npc` exists as a static demo seed only.)*

The wire model is `EntitySnapshot` (`proto/spatialserver/v1/common.proto`):

| Field | Type | Storage | Description |
|-------|------|---------|-------------|
| `entity_id` | string (UUIDv7) | In-memory (+ serialized to `zone_state`, planned) | Unique identifier |
| `type` | string (free-form) | In-memory (+ serialized to `zone_state`, planned) | Entity type tag (e.g. `"player"`, `"npc"`) — not an enum |
| `position` | Vector3 (`double x, y, z`) | In-memory (+ serialized to `zone_state`, planned) | World-space position |
| `attributes` | `map<string, bytes>` | In-memory (+ serialized to `zone_state`, planned) | Opaque, type-specific key→bytes state |

> Fields modeled in ADR-023 but **not** in the current `EntitySnapshot` proto — `zone_id`, `owner_id`, `rotation` (quaternion), `velocity`, `created_at`, `last_updated_at` — are planned for a future proto revision and are tracked in ADR-023.

**Entity ID:** UUIDv7 — provides time-ordered unique IDs with no central counter. The v7 format encodes a Unix millisecond timestamp in the most significant bits, enabling rough time-based sorting without a separate timestamp index.

**Lifecycle:** Created when a player joins (or NPC/item is spawned). Lives in-memory on the owning Game Server. Updated every tick (position, attributes). Serialized to PostgreSQL at configurable intervals (default 5s) *(not yet implemented — entities are in-memory only)*. Destroyed when player leaves or entity despawns.

**Storage:** Primary storage is **in-memory** on the Game Server. Serialized to the `zone_state` PostgreSQL table for crash recovery *(planned)*. Redis does NOT store entity state. Entity data never passes through Redis.

### EntityUpdate (per-tick delta)

`EntityUpdate` (`proto/spatialserver/v1/common.proto`) carries a single per-tick position delta:

| Field | Type | Description |
|-------|------|-------------|
| `entity_id` | string | Target entity |
| `position` | Vector3 (`double x, y, z`) | New world-space position |
| `sequence` | int32 | Monotonic update sequence number |
| `timestamp` | int64 | Origin timestamp |

## Player

A player is a human participant connected to a runtime. A player has a session on the Gateway and an entity on the Game Server — these are separate but linked concepts.

| Field | Type | Storage | Description |
|-------|------|---------|-------------|
| `player_id` | string | JWT token (set by Business Backend) | Unique player identifier |
| `runtime_id` | UUIDv7 | JWT token | Runtime the player joins |
| `entity_id` | UUIDv7 | In-memory (+ session cache) | Linked entity on Game Server |
| `gateway_id` | string | Gateway (in-memory) | Which Gateway instance handles the connection |
| `connected_at` | timestamp | Gateway (in-memory) | Connection time |
| `session_ttl` | duration | Redis (`session:*`, TTL 5min) | Session cache expiry |
| `rate_limit_budget` | counter | Redis (`rate:limit:*`, rolling window) | Message rate-limit counter |

**Lifecycle:** Player authenticates with Business Backend → receives JWT → connects to Gateway → Gateway validates JWT → proxies to Game Server → Game Server creates entity → entity registered in AOI. On disconnect: entity removed from AOI, session cache invalidated.

**Storage:** Session state is on Gateway (in-memory, per-connection). Redis caches session metadata for reconnect scenarios (5-minute TTL). Rate-limit state is in Redis (rolling window counters). Entity counterpart is in-memory on Game Server.

### Player <-> Entity Relationship

```
Player (auth concept, Business Backend owns)
  │
  ├── JWT token (player_id, runtime_id, exp)
  │
  └── Gateway session (WebSocket connection, in-memory)
        │
        └── gRPC proxy
              │
              └── Game Server entity (in-memory)
                    │
                    ├── AOI index (position-based)
                    └── Interest set (entities this player can see)
```

One player = one Gateway connection = one Game Server entity. There is a 1:1:1 relationship.

## Game Server

A Game Server is a process (container/pod) that owns zones and runs the game loop.

| Field | Type | Storage | Description |
|-------|------|---------|-------------|
| `id` | string (UUIDv7) | PostgreSQL (`game_servers`) | Unique server identifier |
| `host` | string | PostgreSQL (`game_servers`) | gRPC host |
| `port` | integer | PostgreSQL (`game_servers`) | gRPC port (internal `:9000`) |
| `status` | enum | PostgreSQL (`game_servers`) | `joining`, `active`, `draining`, `shutdown` |
| `max_zones` | integer | PostgreSQL (`game_servers`) | Maximum zones this server can own |
| `metadata` | JSONB | PostgreSQL (`game_servers`) | Optional server metadata (opaque) |
| `registered_at` | timestamptz | PostgreSQL (`game_servers`) | Registration time |
| `last_heartbeat` | timestamptz | PostgreSQL (`game_servers`) | Last heartbeat time |

> Load tracking is not persisted. `Heartbeat` carries no load parameter; load reporting will be via `ReportMetrics` RPC (planned).

**Lifecycle:** Process starts → resolves Room Service via DNS → sends `Register` gRPC → status = `joining` → assigned first zone → status = `active` → calls `PrepareShutdown` on Room Service → status = `draining` → all zones transferred → status = `shutdown` → process exits. On crash: Room Service detects heartbeat timeout (15s) → marks as `shutdown` → reassigns orphan zones.

**Storage:** Registry in PostgreSQL (source of truth). Room Service caches entire registry in-memory. Heartbeat state is in-memory on Room Service leader (loss acceptable — rebuild from DB on failover).

## Data Serialization

| Entity | Serialization Format | Serialized Where | Frequency |
|--------|---------------------|------------------|-----------|
| Entity state (for persistence) | Protobuf → binary | Game Server → PostgreSQL | Every 5s |
| Entity state (for zone transfer) | Protobuf → gRPC stream | Game Server A → Game Server B | On zone transfer |
| Zone ownership | Protobuf → PostgreSQL row | Room Service → PostgreSQL | On change |
| Game Server registry | Protobuf → PostgreSQL row | Room Service → PostgreSQL | On register/heartbeat |
| Session cache | Protobuf → binary (Redis value) | Gateway → Redis | On connect/reconnect |
| Client packets | Protobuf (length-prefixed) | Client ↔ Gateway | 20Hz |

## Storage Matrix

| Entity | PostgreSQL | Redis | In-Memory (GS) | In-Memory (GW) | In-Memory (RS) |
|--------|------------|-------|----------------|----------------|----------------|
| Runtime | Source of truth | Cache (TTL) | — | — | Leader cache |
| Zone ownership | Source of truth | Cache (TTL) | — | Cache (TTL 5s) | Leader cache |
| Entity state | Persisted snapshot (5s, planned) | — | Primary storage | — | — |
| AOI index | — | — | Primary storage | — | — |
| Player session | — | Cache (TTL 5min) | Entity only | Primary (per-conn) | — |
| Game Server reg | Source of truth | — | — | — | Leader cache |

## References

- [ADR-001](../adr/001-zone-ownership.md) — Zone Ownership
- [ADR-003](../adr/003-aoi-strategy.md) — AOI Strategy
- [ADR-005](../adr/005-game-server-registration.md) — Game Server Registration
- [ADR-016](../adr/016-runtime-lifecycle.md) — Runtime Lifecycle
- [Architecture Overview](overview.md)
- [Component Responsibilities](component-responsibilities.md)
