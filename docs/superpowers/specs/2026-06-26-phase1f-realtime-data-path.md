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
| 1F.1 | Gateway: real WebSocket upgrade (`github.com/coder/websocket`) + JWT validation + `session.Pool` | Client completes WSS handshake; invalid token rejected; conn closes cleanly |
| 1F.2 | Game Server: implement `GameServer.Relay` — stream ↔ Inbox/Outbox bridge, drain goroutine, client→stream registry, CONNECT/DISCONNECT entity lifecycle | gRPC test client sends CONNECT → entity created; DATA → observed in Inbox; Outbox packet → received on stream; DISCONNECT → entity removed |
| 1F.3 | Gateway ↔ Game Server wiring: LookupZone → dial → open Relay stream, dual pump WS↔stream | Integration test across gateway + room-service + game-server |
| 1F.4 | `pkg/game` real protocol encode/decode + fix `dispatch` position parsing + drop-on-full send | Unit test: PositionUpdate packet moves entity + `aoi.Move`; spawn/move packets are valid protocol frames |

### 1F.1 — WebSocket layer (gateway)

- `/ws` handler performs `websocket.Accept` (`github.com/coder/websocket`); reject when the `token` query param is missing or invalid.
- `auth.ValidateToken(token, jwtSecret)` → `Claims{RuntimeID, PlayerID, ZoneID}`. `jwtSecret` is read from config key `gateway.jwt_secret`.
- `clientID = Claims.PlayerID` (1 player = 1 entity in this slice; this value becomes `RelayPacket.client_id` and the `EntityID` on the game server).
- Create `session.NewSession(clientID, Claims.PlayerID, types.ZoneID(Claims.ZoneID), serverID)` — `serverID` comes from the `LookupZone` response — and add to `session.Pool`.
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
  string      client_id = 1;
  Kind        kind      = 2;  // DATA = opaque payload; CONNECT/DISCONNECT = control
  bytes       payload   = 3;  // protocol.Encode output (DATA only); empty otherwise
  ConnectMeta meta      = 4;  // set on CONNECT only
}
message ConnectMeta {
  string player_id  = 1;
  string runtime_id = 2;
  string zone_id    = 3;
}
enum Kind {
  KIND_UNSPECIFIED = 0;
  KIND_DATA        = 1;
  KIND_CONNECT     = 2;
  KIND_DISCONNECT  = 3;
}
```

Why the control plane: the gateway holds the JWT `Claims` (player_id/runtime_id) but the game server does not. A `CONNECT` (carrying `ConnectMeta`) lets the game server create a fully-formed entity; `DISCONNECT` prevents entity leaks. `DATA` payloads stay opaque end-to-end.

Service implementation mirrors the existing `roomServiceServer` pattern (`apps/room-service/main.go`): a thin gRPC adapter struct (`gameServerServer`) in `apps/game-server/main.go` wrapping the core `pkg/game` logic — not a new package.

- Per `client_id` seen on the stream: register a send channel in `clientStreams map[string]chan []byte`.
- Entity lifecycle: on `KIND_CONNECT` → `game.EnqueueAddEntity(entity.New(EntityID(client_id), "avatar", RuntimeID(meta.runtime_id)))`; on `KIND_DISCONNECT` (or stream close / context cancel) → `game.EnqueueRemoveEntity(EntityID(client_id))` + delete send channel.
- Inbound pump: on `KIND_DATA` → `game.Inbox <- InboundPacket{ClientID, payload}`; ignore payload for CONNECT/DISCONNECT.
- Outbound drain goroutine: `for pkt := range game.Outbox` → lookup `clientStreams[pkt.ClientID]` → non-blocking send to that client's send queue; queue full → drop + log.
- Each client's send queue is drained by a goroutine writing `RelayPacket{client_id, KIND_DATA, payload}` back on the shared stream (guarded by a write mutex — gRPC streams are not safe for concurrent writers).

### 1F.3 — Gateway ↔ Game Server wiring

- On WS connect: `room.LookupZone(Claims.ZoneID)` → game-server `host:port`.
- Dial the game-server gRPC (pool/cache connections), open the `Relay` stream.
- Emit `RelayPacket{client_id, KIND_CONNECT, meta: ConnectMeta{PlayerID, RuntimeID, ZoneID}}` first.
- Dual pump:
  - WS read → `RelayPacket{client_id, KIND_DATA, payload}` → `stream.Send`.
  - `stream.Recv` → WS write.
- On WS close → `RelayPacket{client_id, KIND_DISCONNECT}` → close stream.
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

### Concurrency model (critical)

`pkg/game` currently mutates `Entities`/`aoi` only from the tick goroutine. Phase 1F introduces a second writer: the Relay handler calls `AddEntity`/`RemoveEntity` from its own goroutine → **data race** with `tick()` (would fail `go test -race`, which the repo runs on every test invocation).

Fix — keep all simulation-state mutation on the tick goroutine (actor pattern):

- `Game` gains a command channel `cmds chan func()`.
- A new enqueue API (e.g. `EnqueueAddEntity` / `EnqueueRemoveEntity`) is used by the Relay handler: it pushes a command closure onto `cmds` rather than mutating directly.
- The existing synchronous `AddEntity`/`RemoveEntity` stay as-is for setup/tests (called pre-`Run`, single-threaded) so no current caller breaks.
- `tick()` drains `cmds` and executes each closure before running simulation, so entity-map and AOI mutation from the Relay path stay single-threaded.
- `Inbox`/`Outbox` remain channels (already safe across goroutines).

This avoids broad locking and preserves the existing (already inconsistent) `g.mu` usage as a smaller follow-up rather than a blocker.

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
- The WebSocket library must be added as a dependency: `github.com/coder/websocket` (the maintained successor of `nhooyr.io/websocket`, which the standards permit). 1F.1 begins with `go get`.
- The `GameServer` gRPC service (including `Relay`) is registered on the existing game-server gRPC server, alongside health + reflection (which are the only services registered today).
- The client's zone is already assigned to a game-server (Room Service `LookupZone` returns an owner). Pre-assignment happens at startup; dynamic assignment is out of scope.
- The end-to-end integration test brings up room-service dependencies (PostgreSQL + Redis) via Testcontainers, per the repo's integration-test convention, or substitutes an in-process fake `RoomServiceClient`.

## Definition of Done

- Client completes WSS handshake with a valid JWT; an invalid token is rejected.
- `GameServer.Relay` gRPC service is implemented and bridges the stream ↔ Inbox/Outbox.
- Client connect creates an Entity; disconnect removes it.
- Entity lifecycle mutations are race-free: `go test ./pkg/game/... -race` passes with concurrent Relay-driven `AddEntity`/`RemoveEntity`.
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
