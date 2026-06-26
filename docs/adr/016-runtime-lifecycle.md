# ADR 016: Runtime Lifecycle

## Status

Accepted

## Context

Spatial Server manages realtime runtimes — not business rooms. A runtime is an instantiated realtime session (corresponding to a room/showroom/meeting in the Business Backend). The lifecycle of a runtime must be clearly defined.

## Decision

### Runtime Tokens (JWT)

- Issued by: Business Backend
- Validated by: Gateway (public key provided by Business Backend)
- Contents: `player_id`, `runtime_id`, `exp`
- Spatial Server never issues or manages JWT tokens
- Spatial Server never manages users

### Lifecycle States

| State | Description |
|-------|-------------|
| `creating` | Zones being allocated, Game Servers being assigned, not yet accepting connections |
| `active` | Runtime is running, players can connect and participate in realtime simulation |
| `draining` | Runtime being destroyed, new connections rejected, existing players being notified |
| `destroyed` | All resources released, zones freed, Game Server capacity reclaimed |

### Flow

1. **Business Backend** creates business entity (room/showroom/meeting) in its own database.
2. **Business Backend** calls `SpatialServerAPI.CreateRuntime(runtime_id, zone_count)`.
3. **Room Service** validates runtime_id is unique, sets status to `creating`.
4. **Room Service** creates zones (grid cells) for the runtime.
5. **Room Service** assigns each zone to a Game Server (lowest load first).
6. **Room Service** sets status to `active`.
7. **Room Service** returns `gateway_addr + zone list` to Business Backend.
8. **Business Backend** returns connection info to client.
9. **Client** connects to Gateway with runtime token.
10. **Gateway** validates token → looks up runtime zone → proxies to Game Server.
11. When business entity ends, **Business Backend** calls `DestroyRuntime(runtime_id)`.
12. **Room Service** sets status to `draining`, notifies Game Servers.
13. **Game Servers** disconnect players, release zone state.
14. **Room Service** deletes zone ownership, sets status to `destroyed`.

### Player Join Flow

1. Client authenticates with **Business Backend** (not Spatial Server).
2. Business Backend verifies permissions, generates runtime token (JWT).
3. Client connects to Gateway: `wss://gateway/v1/ws?token=<jwt>`.
4. Gateway validates JWT signature (Business Backend public key).
5. Gateway extracts `runtime_id` from JWT, looks up zone assignment via Room Service.
6. Gateway proxies WebSocket to the Game Server owning the player's zone.
7. Game Server creates entity for the player, adds to AOI index.
8. Game Server broadcasts entity spawn to other players in AOI range.

### Player Leave Flow

1. Client disconnects (intentional or timeout).
2. Gateway notifies Game Server.
3. Game Server removes player entity from AOI index.
4. Game Server broadcasts entity despawn to other players.
5. Gateway closes WebSocket connection.

## Consequences

- Business Backend controls runtime lifecycle (create/destroy).
- Spatial Server only manages realtime state within a runtime.
- Runtime creation is synchronous (zones allocated before returning).
- Player join is validated at two levels: JWT (Business Backend) + zone lookup (Room Service).
- Runtime destruction is graceful: players are notified, state is cleaned up.

## Replaces

- Previous design had rooms managed inside Spatial Server with business metadata.
