# ADR 023: Entity Model Design

## Status

Accepted

## Context

Entities are the core simulated objects within a runtime. Every player avatar, NPC, interactive object, projectile, and environmental element is represented as an entity. The entity model must support these diverse types today while allowing future entity types (vehicles, items, dynamic obstacles) to be added without schema changes or recompilation.

The entity model lives entirely within the Game Server. It is not exposed to clients directly — clients receive serialized snapshots of entities within their Area of Interest (AOI). The Gateway never inspects or interprets entity data.

Existing ADRs define zone ownership (ADR-001), AOI strategy (ADR-003), and packet protocol (ADR-010), but do not define the entity data model itself — what fields every entity has, how entity types are distinguished, how attributes are stored, and how entity lifecycle works.

## Problem

Without a defined entity model, each Game Server implementation may choose different entity structures, leading to interoperability issues between Game Servers, inconsistent serialization formats, and difficulty adding new entity types. The entity model must be flexible enough for MVP entity types (players, NPCs, interactive objects) without requiring protobuf recompilation for each new type.

## Decision

### Entity Structure

Every entity has the following fields:

| Field | Type | Description |
|-------|------|-------------|
| `id` | UUIDv7 | Globally unique entity identifier. UUIDv7 provides time-ordered IDs for efficient indexing. |
| `type` | string | Entity type tag (e.g., `"player"`, `"npc"`, `"interactive"`). Free-form string, not a protobuf oneof. |
| `position` | Vector3 (float32, float32, float32) | World-space position (x, y, z). |
| `rotation` | Quaternion (float32, float32, float32, float32) | World-space rotation. |
| `attributes` | map<string, bytes> | Type-specific attributes as opaque byte slices. Keys are attribute names, values are protobuf-encoded or plain serialized data. |
| `zone_id` | string | The zone this entity currently belongs to. Set on spawn, changes on zone migration. |
| `owner_id` | string | The player ID that owns/controls this entity (empty for NPCs and environment objects). |

### Entity Type Is a String Tag

- Entity `type` is a free-form string, not a protobuf enum or oneof.
- This allows adding new entity types (e.g., `"vehicle"`, `"projectile"`, `"item"`) without recompiling any protobuf definitions or redeploying the Gateway.
- The Game Server maintains a registry of supported entity types and their corresponding logic handlers.
- Unknown entity types (received during zone migration from a newer Game Server) are serialized and forwarded as-is — the receiving Game Server stores the opaque data and re-serializes it for clients.

### Attributes Are Opaque Byte Slices

- The `attributes` map stores type-specific data as `map<string, bytes>`.
- The Game Server interprets the byte content based on the entity's `type` string.
- For example, a `"player"` entity might have attributes like `"health"` → `{uint32: 100}`, `"name"` → `{string: "Alice"}`, while an `"interactive"` entity might have `"state"` → `{enum: OPEN}`.
- Attributes are serialized using protobuf within the Game Server (not exposed to Gateway or clients in raw form).
- Clients receive attributes in the packet serialization format defined by ADR-010 (not raw protobuf, but optimized for realtime sync).

### Entity Lifecycle

```
spawn → simulate → despawn
```

1. **Spawn**: An entity is created in a zone.
   - Entity ID is generated (UUIDv7).
   - Entity is assigned to a zone based on its initial position.
   - Entity is registered in the zone's entity index and the AOI index (ADR-003).
   - A spawn event is broadcast to all players within AOI range.

2. **Simulate**: The entity exists and is updated each tick.
   - Player-controlled entities: updates come from client packets (position, rotation, actions).
   - NPC/autonomous entities: updates come from the Game Server's simulation loop.
   - Each tick, the Game Server checks if the entity crossed a zone boundary (zone migration per ADR-002).
   - Changed attributes are batched and sent to interested clients as state delta packets.
   - AOI index is updated if position changed.

3. **Despawn**: The entity is removed from the zone.
   - Entity is unregistered from the zone's entity index and AOI index.
   - A despawn event is broadcast to all players within AOI range.
   - Entity resources are released.

### Entity Ownership

- **Zone ownership**: An entity belongs to the zone it occupies. The Game Server that owns the zone is authoritative for that entity.
- **Player ownership**: If `owner_id` is set, the entity is player-controlled. The Game Server accepts movement and action packets only from the owning player (validated via `player_id` from the WebSocket session).
- **NPC/environment objects**: `owner_id` is empty. The Game Server is authoritative for all simulation.

### Cross-Zone Entities Are Not Supported in MVP

- An entity cannot exist in multiple zones simultaneously in the MVP.
- When an entity crosses a zone boundary (based on position):
  1. The source Game Server initiates entity migration (per ADR-002).
  2. The entity is serialized (ID, type, position, rotation, attributes) and sent to the destination Game Server.
  3. The destination Game Server spawns the entity.
  4. The source Game Server despawns the entity.
  5. Clients are notified of the zone change (the entity leaves the old zone's AOI and appears in the new zone's AOI).

- This migration applies to all entity types, including players. The transition should be seamless from the client's perspective.

### Entity ID Format (UUIDv7)

- UUIDv7 provides time-ordered, K-sortable unique identifiers.
- Benefits over UUIDv4: better database index locality, natural ordering by creation time.
- Benefits over auto-increment integers: globally unique across all Game Servers without coordination, no centralized ID generator needed.
- 128-bit (16 bytes) per ID, transmitted as 36-byte ASCII string in JSON contexts or 16-byte binary in protobuf.

## Alternatives

1. **Protobuf oneof for entity types**: Define each entity type as a protobuf oneof variant. Provides compile-time type safety and schema enforcement. Rejected because adding a new entity type requires protobuf recompilation and redeployment of all services. This slows iteration velocity for game developers.

2. **Single flat struct with all possible fields**: Every entity has every field that any entity type might need (union of all fields). Simple to implement but wastes memory on unused fields and becomes unwieldy as entity types grow.

3. **Entity Component System (ECS)**: Entities are composed of components (position, health, renderable, etc.) attached at runtime. Highly flexible and performant for games with many entity types. Rejected for MVP because the added complexity of an ECS framework is not justified when we have few entity types. May be adopted post-MVP if entity variety grows significantly.

4. **Dynamic language runtime (Lua, Python)**: Entity behavior defined in scripting languages, attributes are dynamic dictionaries. Maximum flexibility. Rejected because runtime performance for realtime simulation would suffer, and the operational complexity of embedding a scripting runtime in Go is high.

## Tradeoffs

- String-typed entity types are flexible but sacrifice compile-time safety — a typo in an entity type string will not be caught until runtime. Mitigation: Game Server logs a warning on unknown entity types and unit tests validate the entity type registry.
- Opaque byte slices for attributes are flexible but require the Game Server to know how to interpret each entity type's attributes. If a Game Server receives an entity with an unknown type during zone migration, it stores the opaque data but cannot simulate it.
- UUIDv7 IDs are larger than integer IDs (16 bytes vs 4-8 bytes) but eliminate the need for a centralized ID generator and avoid collision risks.
- Cross-zone entity migration is complex but necessary for a runtime divided into zones. The complexity is isolated to the migration protocol (ADR-002), not the entity model itself.

## Consequences

- New entity types can be added by registering a new handler in the Game Server — no protobuf changes, no Gateway changes, no client SDK changes (client receives type-specific data via the existing packet protocol).
- Entity data is self-describing: the `type` string tells the Game Server how to interpret `attributes`.
- Zone migration is scoped to single-zone entities. Cross-zone entities (e.g., a player standing on a zone boundary) are not supported and must be handled at the application level.
- Game Servers must implement the entity registry, spawn/simulate/despawn lifecycle, and migration hooks for each entity type.
- Client SDKs must understand entity type strings and their corresponding attribute schemas (documented in the game-specific protocol, not in Spatial Server).

## Future Considerations

- **ECS post-MVP**: if entity variety grows to 20+ types with complex interactions, evaluate introducing an ECS framework for better performance and composability.
- **Spatial partitioning for broadphase**: the current model assumes entities are in one zone. For entities near zone boundaries, consider a "border zone" area where entities are replicated to adjacent zones for AOI consistency.
- **Persistent entities**: some entity types (environment objects, persistent NPCs) may need to survive Game Server restarts. Their state would need to be persisted to a database and loaded on spawn.
- **Entity templates**: define common entity configurations (archetypes) as templates to avoid repeating the same attribute values for every entity of the same type.

## Replaces

- Previous design used a protobuf oneof for entity types, requiring recompilation for every new entity type.
