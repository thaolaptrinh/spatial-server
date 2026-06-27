# Phase 4 — Session Resumption + Backpressure

> **Last Updated:** 2026-06-27
> **Status:** Draft

## Purpose

Phase 3 makes the platform multi-server, but every client disconnect is still **final**: the in-memory `session.Pool` (`pkg/session/session.go`) drops the session immediately, and the Game Server despawns the entity on `KIND_DISCONNECT` (`apps/game-server/main.go:164`). A one-second network blip forces full re-authentication and state re-sync. There is also no flow control: the Gateway relays unbounded, the per-client send channel is a fixed 64-slot buffer with silent drops (`apps/game-server/main.go:119`), and a slow client or a flooding client can starve the relay goroutines.

Phase 4 adds production-grade **durability** and **flow control**:

- **Session resumption (ADR-022):** Redis-backed opaque session tokens, a 30s reconnection window, and a Game Server delta ring-buffer so reconnecting clients catch up without full state re-sync.
- **Backpressure (ADR-021):** WebSocket write deadlines, bounded-buffer policies with explicit drop counters, per-connection and per-IP token-bucket rate limiting, and graceful Gateway drain.

After this phase, temporary disconnects are seamless and the platform degrades safely under load instead of livelocking.

## Scope

- Redis-backed session tokens (ADR-022): on WebSocket connect, issue an opaque 32-byte session token. Store `session:{player_id}` in Redis with a 60s sliding TTL.
- 30s reconnection window: on disconnect, the entity enters a `disconnected` state (not despawned). The client may reconnect with its session token within 30s.
- Delta ring-buffer: Game Server buffers the last N packets per session. On reconnect, replay buffered updates, then send full visible-state snapshot.
- Backpressure: 5s write deadline on every WebSocket write. If a write exceeds the deadline, close the connection.
- Bounded-buffer policies: per-client send queue (64 packets). On overflow, drop the oldest packet and increment a drop counter (metric: `gateway_dropped_packets_total`).
- Per-connection rate limiting: token bucket, 100 msg/s default. Excess packets dropped + counter incremented.
- Per-IP rate limiting: 500 msg/s aggregate across all connections from the same source IP.
- Graceful drain: on SIGTERM, the Gateway stops accepting new connections and finishes active sessions (with a configurable timeout, default 30s).

**Out of scope:**

- TLS / WSS cert management (Phase 6)
- Autoscaling-driven drain (Phase 6)
- Chaos testing of disconnect paths (Phase 6)
- Cross-region session migration (ADR-022 future consideration)

## Architecture

```
                        ┌────────────────────────────────────────────┐
                        │                   Gateway                    │
                        │                                             │
   Client ──WSS────────►│  ┌─────────────┐   ┌──────────────────┐    │
   ?session=<token>     │  │ RateLimiter  │   │  SessionStore    │    │
   OR ?token=<jwt>      │  │  per-conn    │   │  (Redis-backed)  │    │
                        │  │  per-IP      │   │  session:{token} │    │
                        │  └──────┬──────┘   └────────┬─────────┘    │
                        │         │                   │ resume?      │
                        │  ┌──────▼───────────────────▼──────────┐   │
                        │  │ relayWS                             │   │
                        │  │  - 5s write deadline                 │   │
                        │  │  - 64-slot bounded send queue        │   │
                        │  │    (drop oldest + counter on full)   │   │
                        │  └──────────────────┬──────────────────┘   │
                        │                     │ gRPC Relay stream     │
                        └─────────────────────┼──────────────────────┘
                                              │
                        ┌─────────────────────▼──────────────────────┐
                        │                 Game Server                 │
                        │  ┌──────────────────────────────────────┐  │
                        │  │ Session state machine per entity:     │  │
                        │  │   ACTIVE → DISCONNECTED → (ACTIVE |   │  │
                        │  │                DESPAWNED)              │  │
                        │  └──────────────────────────────────────┘  │
                        │  ┌──────────────────────────────────────┐  │
                        │  │ DeltaRingBuffer (per session)        │  │
                        │  │  - capacity 1000                     │  │
                        │  │  - replay on player_reconnected      │  │
                        │  └──────────────────────────────────────┘  │
                        └─────────────────────┬──────────────────────┘
                                              │
                        ┌─────────────────────▼──────────────────────┐
                        │  Redis: session:{token}  (60s sliding TTL) │
                        │  { player_id, runtime_id, zone_id,         │
                        │    game_server_addr, created_at,           │
                        │    last_activity, source_ip }              │
                        └────────────────────────────────────────────┘
```

## Components

### 1. Redis-Backed Session Store (`pkg/session/store.go`)

New file. The current `session.Pool` (`pkg/session/session.go`) is purely in-memory with a `byID map[string]*Session`. Phase 4 keeps `Pool` for the per-connection live state but adds a **Redis-backed token store** for resumption.

**`SessionStore` interface (defined in the consumer — `pkg/gateway`):**

```go
type SessionStore interface {
    Issue(ctx context.Context, s SessionRecord) (token string, err error)
    Lookup(ctx context.Context, token string) (SessionRecord, bool, error)
    Touch(ctx context.Context, token string) error
    Revoke(ctx context.Context, token string) error
}
```

**`redisSessionStore` implementation (in `pkg/session/`):**

- `Issue`: generate 32 crypto-random bytes, base64url-encode (44 chars, per ADR-022), `SET session:{token} <json> EX 60`.
- `Lookup`: `GET session:{token}`; on hit, `Touch` resets TTL (sliding window).
- `Revoke`: `DEL session:{token}`.
- Uses the existing `redis.Client` from `pkg/storage` (`storage.NewRedisClient`, `pkg/storage/storage.go:30`).

**Redis key value (ADR-022):**

```
Key:   session:{token}
Value: { player_id, runtime_id, zone_id, game_server_addr,
         created_at, last_activity, source_ip }
TTL:   60s (sliding — each access resets)
```

### 2. Session Token Issuance + Resume (`pkg/gateway/handler.go`)

The current `handleWS` (`pkg/gateway/handler.go:61`) validates a JWT and immediately upgrades. Phase 4 adds two connection paths:

**Path A — New connection (JWT):**

1. Validate JWT (unchanged, `pkg/gateway/handler.go:68`).
2. Upgrade to WebSocket.
3. `sessionStore.Issue(...)` → token.
4. Send `session_established` control message `{session_token}` to client.
5. Proceed to relay.

**Path B — Reconnection (session token):**

1. Read `?session=<token>` query param (in addition to existing `?token=<jwt>`).
2. `sessionStore.Lookup(token)`:
   - **Found + valid:** validate runtime matches, source IP matches (ADR-022 binding, configurable warn-only). Re-establish relay to the **same Game Server** recorded in the session record. Send `player_reconnected` relay kind. Emit `session_resumed` to client.
   - **Not found / expired:** return 401, force client down the JWT path.

**New relay kinds** (add to `game_server.proto` `Kind` enum): `KIND_RECONNECT = 4`, `KIND_PEER_DISCONNECTED = 5`, `KIND_PLAYER_DISCONNECTED = 6`.

### 3. Entity Session State Machine (`pkg/game/lifecycle.go`)

New file. The current Game Server removes the entity immediately on `KIND_DISCONNECT` (`apps/game-server/main.go:164`). Phase 4 introduces a per-entity state machine:

```
ACTIVE ──(peer_disconnected)──► DISCONNECTED ──(30s timeout)──► DESPAWNED
   ▲                                 │
   └───(player_reconnected)──────────┘
```

- On `KIND_PEER_DISCONNECTED`: mark entity `DISCONNECTED`. Do **not** despawn. Start (or extend) a 30s timer. Continue simulating (idle/gravity per ADR-022).
- On `KIND_RECONNECT`: cancel timer, mark `ACTIVE`, replay delta ring-buffer (Component 4), send full visible-state snapshot.
- On timer expiry: despawn entity, broadcast despawn to AOI peers, release resources.

Tracked in a new `sessionStates map[types.EntityID]*sessionState` on `Game`, where `sessionState{status, disconnectedAt, timer}`.

### 4. Delta Ring-Buffer (`pkg/game/deltabuffer.go`)

New file. One per active/disconnected session. While ADR-022 specifies buffering during the `DISCONNECTED` window, this implementation buffers continuously (simpler, bounded memory) so a reconnect always has recent context.

```go
type DeltaRingBuffer struct {
    mu    sync.Mutex
    buf   []*v1.EntityUpdate
    cap   int          // default 1000 (ADR-022)
    head  int
    count int
    drops uint64
}
```

- `Push(update)`: append, overwrite oldest on full, increment `drops`.
- `Drain() []*v1.EntityUpdate`: return all buffered in order, reset.
- On `KIND_RECONNECT`: `Drain()` → relay each as `KIND_DATA`, then send a full snapshot of all visible entities (spawn packets) to guarantee consistency (ADR-022: "After replay, send full current state of all visible entities").

Memory budget: ~10 KB per disconnected player for 1000 deltas (ADR-022 tradeoff).

### 5. Backpressure: Write Deadline + Bounded Queue (`pkg/gateway/handler.go`)

The current `relayWS` (`pkg/gateway/handler.go:96`) writes to the WebSocket with no deadline and drops silently when the Game Server send channel is full. Phase 4 enforces flow control:

**5a. WebSocket write deadline:**

- Every `conn.Write(...)` is preceded by `conn.SetWriteDeadline(ctx, now.Add(5*time.Second))` (ADR-021: "5s write deadline ... if exceeded, the connection is terminated").
- On deadline exceeded: close the connection, trigger the disconnect path (session window starts).

**5b. Bounded send queue + drop policy:**

- Replace the bare `ch chan []byte` (buffered 64 at `apps/game-server/main.go:119`) with a struct that tracks drops: `{ch chan []byte; drops atomic.Uint64}`.
- On send when channel full: drop the **oldest** packet (drain one), push the new, increment `drops`. Emit Prometheus counter `gateway_dropped_packets_total{reason="send_queue_full"}`.
- Rationale (ADR-021): a slow client must not block the Game Server relay goroutine.

### 6. Rate Limiting (`pkg/gateway/ratelimit.go`)

New file. Implements two token buckets per ADR-021:

**Per-connection bucket:** 100 msg/s default. Checked in the WS-read → Relay-Send pump (`pkg/gateway/handler.go:139`) before each `stream.Send`. On violation: drop packet, increment `gateway_rate_limit_drops_total{scope="connection"}`. After 3 violations (ADR-018): warn → disconnect.

**Per-IP bucket:** 500 msg/s aggregate. A shared `sync.Map[string]*tokenBucket` keyed by `r.RemoteAddr` IP. Checked alongside the per-connection bucket. On violation: drop + counter `gateway_rate_limit_drops_total{scope="ip"}`.

**Token bucket implementation:**

```go
type tokenBucket struct {
    mu       sync.Mutex
    tokens   float64
    rate     float64       // tokens/sec
    burst    float64
    lastTime time.Time
}
func (b *tokenBucket) Allow() bool { /* refill, deduct, return */ }
```

Parameters read from dynamic config on every refill (ADR-021: "no restart required to change limits"). Config keys: `gateway.rate_limit.per_connection_msg_per_sec`, `gateway.rate_limit.per_ip_msg_per_sec`.

### 7. Graceful Drain (`pkg/gateway/drain.go`)

New file. The current Gateway main exits on the first signal with no drain. Phase 4 adds:

- On SIGTERM/SIGINT:
  1. Stop accepting new connections: set `/ready` to 503, close the HTTP listener.
  2. Stop the `WatchOwnership` push subscriber (Phase 3).
  3. For each active session: allow up to `gateway.drain.timeout` (default 30s) to finish in-flight relay. Send WebSocket Close(1001, "server shutting down") to clients.
  4. After timeout: force-close remaining connections.
  5. Exit.

This satisfies ADR-021 readiness semantics and ADR-011 client-impact ("clients reconnect via session token").

## Data Flow

### Scenario A — Temporary disconnect + reconnect (ADR-022 happy path)

```
1. Client connects: JWT validated → token issued → session:{token} in Redis (60s TTL)
2. Gateway sends session_established{token} to client
3. Client experiences 8s network blip; WS read fails
4. Gateway: SetWriteDeadline/read error → close conn
5. Gateway: send KIND_PEER_DISCONNECTED to Game Server
6. Game Server: entity → DISCONNECTED, start 30s timer, continue buffering deltas
7. Gateway: leave session:{token} in Redis (do NOT revoke — unexpected disconnect)
8. Client reconnects: wss://gateway/v1/ws?session=<token> (within 30s)
9. Gateway: sessionStore.Lookup(token) → found, IP matches, runtime matches
10. Gateway re-establishes Relay to same Game Server, sends KIND_RECONNECT
11. Game Server: entity → ACTIVE, cancel timer
12. Game Server: DeltaRingBuffer.Drain() → relay buffered packets (KIND_DATA)
13. Game Server: send full visible-state snapshot (spawn for each visible entity)
14. Gateway: session_resumed{new_token} to client (single-use token rotated, ADR-022)
```

### Scenario B — Permanent disconnect (30s window expires)

```
1. Client disconnects, no reconnect within 30s
2. Game Server timer fires: entity → DESPAWNED
3. Game Server: despawn entity, broadcast despawn to AOI peers
4. Game Server: send KIND_PLAYER_DISCONNECTED to Gateway (final)
5. Redis: session:{token} evicted by 60s TTL (or Gateway revokes)
6. Resources released; client must full-JWT-auth on return
```

### Scenario C — Backpressure (slow client)

```
1. Client's WS read loop stalls (e.g., tab backgrounded)
2. Game Server produces updates faster than client drains
3. Per-client send queue (64) fills
4. Gateway: drop oldest packet, drops++, metric gateway_dropped_packets_total++
5. If a WS Write exceeds 5s deadline: close connection → Scenario A disconnect path
```

### Scenario D — Flooding client (rate limit)

```
1. Client sends 200 packets/s (>100 msg/s per-connection limit)
2. Gateway rate limiter: ~100 packets pass, ~100 dropped
3. Dropped packets: counter gateway_rate_limit_drops_total{scope="connection"}++
4. On 3rd violation window: warn → disconnect client
5. Same source IP across 6 connections (3000 msg/s > 500/s per-IP): per-IP bucket drops
```

## Files Changed

| File | Action | Detail |
|------|--------|--------|
| `pkg/session/store.go` | Create | `SessionStore` interface + `redisSessionStore` (Issue/Lookup/Touch/Revoke) |
| `pkg/session/store_test.go` | Create | Redis-backed tests (miniredis or Testcontainer) |
| `pkg/session/session.go` | Modify | Add `SessionRecord` struct, `SourceIP`, `GameServerAddr` fields |
| `pkg/session/token.go` | Create | Crypto-random 32-byte token generation (base64url) |
| `pkg/gateway/handler.go` | Modify | Session-token issuance + resume paths, write deadline, bounded send queue |
| `pkg/gateway/handler_test.go` | Modify | Resume-path + drop-policy unit tests |
| `pkg/gateway/ratelimit.go` | Create | Token bucket: per-connection + per-IP, dynamic config |
| `pkg/gateway/ratelimit_test.go` | Create | Token bucket math + overflow tests |
| `pkg/gateway/drain.go` | Create | Graceful drain on SIGTERM: stop accept, finish sessions, timeout |
| `pkg/gateway/gateway.go` | Modify | Track active connections for drain, expose `Drain(ctx)` |
| `pkg/gateway/gateway_test.go` | Modify | Drain behavior tests |
| `apps/gateway/main.go` | Modify | Wire SessionStore (Redis), rate limiter, drain on signal |
| `pkg/game/lifecycle.go` | Create | Entity session state machine (ACTIVE/DISCONNECTED/DESPAWNED) + 30s timer |
| `pkg/game/lifecycle_test.go` | Create | State-transition + timer tests |
| `pkg/game/deltabuffer.go` | Create | `DeltaRingBuffer` (cap 1000, Push/Drain, drop counter) |
| `pkg/game/deltabuffer_test.go` | Create | Ring-buffer overflow + ordering tests |
| `pkg/game/game.go` | Modify | Add `sessionStates` map, wire buffer into updateVisibility/tick |
| `pkg/game/game_test.go` | Modify | Disconnect/reconnect simulation tests |
| `apps/game-server/main.go` | Modify | Handle KIND_RECONNECT/KIND_PEER_DISCONNECTED/KIND_PLAYER_DISCONNECTED; wire delta buffer + lifecycle |
| `apps/game-server/main_test.go` | Modify | Reconnect replay tests |
| `proto/spatialserver/v1/game_server.proto` | Modify | Add `KIND_RECONNECT=4`, `KIND_PEER_DISCONNECTED=5`, `KIND_PLAYER_DISCONNECTED=6` to Kind enum |
| `proto/gen/spatialserver/v1/*.pb.go` | Regenerate | `make proto` |
| `configs/gateway.yml` | Modify | Add `rate_limit.*`, `drain.timeout`, `session.ttl` keys |
| `configs/game-server.yml` | Modify | Add `session.reconnect_window`, `delta_buffer.capacity` keys |
| `tests/integration/session_resume_test.go` | Create | E2E disconnect + reconnect + delta replay (Testcontainers Redis) |

## References

- [Master Phase Roadmap](./master-phase-roadmap.md)
- [Phase 1G spec](./2026-06-26-phase1g-make-it-demoable.md) — format reference
- [Phase 3 spec](./phase-3-distributed-scaling.md) — depends on multi-server routing
- [ADR-022 Session Management](../../adr/022-session-management.md)
- [ADR-021 Gateway Architecture](../../adr/021-gateway-architecture.md)
- [ADR-011 Failure Recovery](../../adr/011-failure-recovery.md)
- [Error handling](../../standards/error-handling.md)
- [Logging](../../standards/logging.md)
- [Configuration](../../standards/configuration.md)
