# Phase 1F — Wire Realtime Data Path

> **Last Updated:** 2026-06-26
> **Status:** Draft

## Purpose

Close the end-to-end realtime loop so a single client can connect over WebSocket, spawn into a zone, send position updates, and receive spawn/move packets back. Today the well-tested components (gateway, room service, game loop with AOI) are isolated: nothing feeds `game.Inbox`, nothing drains `game.Outbox`, the `/ws` handler returns `501 Not Implemented`, and `game_server.proto`'s `GameServer` service is unimplemented.

This phase produces one demoable vertical slice (1 client + 1 static NPC) and de-risks scaling and hardening work.

## Architecture

Bidirectional gRPC stream, multiplexed per (gateway ↔ game-server) pair. The browser speaks the binary packet protocol; the gateway is a dumb opaque relay; the game server encodes/decodes packets and runs the simulation.

```
Browser ──WSS──▶ Gateway ──bidi gRPC Relay (multiplexed)──▶ Game Server
                  │                                            │
                  │ ValidateToken (pkg/auth)                   │ RelayPacket ↔ game.Inbox/Outbox
                  │ LookupZone (Room Service)                  │ client→stream registry + fan-out
                  │                                            │ per-client send queue (drop-on-full)
                  └─ opaque bytes: WS frame == RelayPacket.payload
```

Design principles:

- **Opaque relay** — `RelayPacket.payload` is the exact output of `protocol.Encode`. The gateway never inspects packet semantics.
- **Multiplexed stream** — one bidi stream per (gateway instance, game-server) pair; `RelayPacket.client_id` routes packets. A client→stream registry in the game server fans out the global `game.Outbox`.
- **Protocol at the edges** — `pkg/protocol` Encode/Decode is used only by the game server (server edge) and the browser (client edge).

### Milestones

| Milestone | Scope | Independently testable |
|-----------|-------|------------------------|
| 1F.1 | Gateway: real WebSocket upgrade (nhooyr.io) + JWT validation + `session.Pool` | Client completes WSS handshake; invalid token rejected; conn closes cleanly |
| 1F.2 | Game Server: implement `GameServer.Relay` — stream ↔ Inbox/Outbox bridge, drain goroutine, client→stream registry, entity lifecycle on connect/disconnect | gRPC test client sends RelayPacket → observed in Inbox; Outbox packet → received on stream |
| 1F.3 | Gateway ↔ Game Server wiring: LookupZone → dial → open Relay stream, dual pump WS↔stream | Integration test across gateway + room-service + game-server |
| 1F.4 | `pkg/game` real protocol encode/decode + fix `dispatch` position parsing + drop-on-full send | Unit test: PositionUpdate packet moves entity + `aoi.Move`; spawn/move packets are valid protocol frames |

### 1F.1 — WebSocket layer (gateway)

- `/ws` handler performs `websocket.Accept` (nhooyr.io); reject when the `token` query param is missing or invalid.
- `auth.ValidateToken(token, jwtSecret)` → `Claims{RuntimeID, PlayerID, ZoneID}`. `jwtSecret` is read from config key `gateway.jwt_secret`.
- Create `session.NewSession(clientID, PlayerID, ZoneID, ...)`, add to `session.Pool`.
- On disconnect: `pool.Remove`, cleanup.
- Assumption: the client already holds a valid JWT minted out-of-band; minting is out of scope.

### 1F.2 — GameServer.Relay service

New RPC + message in `game_server.proto`:

```proto
service GameServer {
  // ...existing RPCs...
  rpc Relay(stream RelayPacket) returns (stream RelayPacket);
}
message RelayPacket {
  string client_id = 1;
  bytes  payload   = 2;  // protocol.Encode output — opaque
}
```

Service implementation (consumer-defined interface, e.g. `pkg/gameserver`):

- Per `client_id` seen on the stream: register a send channel in `clientStreams map[string]chan []byte`.
- Entity lifecycle: on first packet for a `client_id` → `game.AddEntity(entity.New(...))` derived from the client identity; on stream close / context cancel → `game.RemoveEntity`.
- Inbound pump: read `RelayPacket` → `game.Inbox <- InboundPacket{ClientID, payload}`.
- Outbound drain goroutine: `for pkt := range game.Outbox` → lookup `clientStreams[pkt.ClientID]` → non-blocking send to that client's send queue; queue full → drop + log.
- Each client's send queue is drained by a goroutine writing `RelayPacket`s back on the shared stream (guarded by a write mutex — gRPC streams are not safe for concurrent writers).

### 1F.3 — Gateway ↔ Game Server wiring

- On WS connect: `room.LookupZone(Claims.ZoneID)` → game-server `host:port`.
- Dial the game-server gRPC (pool/cache connections), open the `Relay` stream.
- Dual pump:
  - WS read → `RelayPacket{client_id, payload}` → `stream.Send`.
  - `stream.Recv` → WS write.
- If `LookupZone` returns no owner → `503` to the client (zone not yet assigned).

### 1F.4 — pkg/game real protocol + dispatch fix

- Replace ad-hoc `fmt.Sprintf` packet builders with `protocol.Encode(PacketIDEntitySpawn, payload, true)`, etc.
- Payload schema — define and document (little-endian):
  - `PositionUpdate` payload = 3×float64 (x, y, z), 24 bytes.
  - `EntitySpawn` payload = `entityID` (uint16 length-prefix + bytes) + `type` (uint16 length-prefix + bytes) + `Vector3` (3×float64).
  - `EntityDespawn` payload = `entityID` (uint16 length-prefix + bytes).
  - `EntityMove` payload = `entityID` (uint16 length-prefix + bytes) + `Vector3` (3×float64).
- Fix `dispatch`: decode `PositionUpdate` → update `e.Position` → `g.aoi.Move(e.ID, e.Position)`. (Current bug calls `Move` with the pre-update position.)
- Drop-on-full send: `select { case Outbox <- pkt: default: log drop }` so a stalled drain can never freeze the tick loop.

## Demo scenario

1 client + 1 static NPC pre-spawned (`game.AddEntity` at startup). The client connects, immediately receives an `EntitySpawn` for the NPC (AOI visibility), sends `PositionUpdate`s, and the NPC entering/leaving AOI produces spawn/despawn. This exercises the full loop with a single browser.

## Non-goals

- Multi-game-server zone transfer (Phase 3).
- Real backpressure / congestion control — drop-on-full is the vertical-slice strategy.
- JWT minting / issuance — out of scope; a valid token is assumed.
- Attribute delta sync, persistence, recovery (Phase 4).
- TLS termination beyond config wiring — certificates are a later hardening step.
- Cross-server ghost migration.

## Assumptions

- `gateway.jwt_secret` is added to `configs/gateway.yml` and `configs/defaults.yml`.
- The client's zone is already assigned to a game-server (Room Service `LookupZone` returns an owner). Pre-assignment happens at startup; dynamic assignment is out of scope.

## Definition of Done

- Client completes WSS handshake with a valid JWT; an invalid token is rejected.
- `GameServer.Relay` gRPC service is implemented and bridges the stream ↔ Inbox/Outbox.
- Client connect creates an Entity; disconnect removes it.
- `game.Outbox` is drained (the game loop never blocks on send).
- A `PositionUpdate` packet moves the entity and updates AOI.
- Spawn/move/despawn outbound packets are valid `protocol.Encode` frames.
- End-to-end integration test (gateway + room-service + game-server) passes.
- All existing unit tests still pass; lint clean.

## References

- [Phase 1E Spec](2026-06-26-phase1e-runtime-features.md)
- [ADR-009](../../adr/009-rpc-contract.md) — RPC Contract
- [ADR-012](../../adr/012-networking.md) — Networking
- [ADR-021](../../adr/021-gateway-architecture.md) — Gateway Architecture
- [ADR-022](../../adr/022-session-management.md) — Session Management
