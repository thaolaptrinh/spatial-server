# ADR 022: Session Management and Reconnection

## Status

Accepted

## Context

Clients connect to Spatial Server via WebSocket through the Gateway. Network conditions are unreliable — clients may experience temporary disconnections due to network blips, browser tab switches, mobile handoffs (Wi-Fi to cellular), or brief server interruptions. Without session resumption, every disconnect forces the client to:

1. Re-authenticate with the Business Backend (obtain a new JWT).
2. Reconnect to the Gateway with the new JWT.
3. Re-synchronize all game state from scratch.

This creates a poor user experience and adds load on the Business Backend for JWT issuance.

The platform needs to distinguish between a temporary network interruption (where the client will return within seconds) and a permanent departure (where the client has closed the game). The distinction determines whether the session state should be preserved or released.

## Problem

Temporary disconnections must not require full re-authentication and state re-synchronization. The platform must support session resumption within a reasonable window, preserving the player's in-game state (position, attributes, zone assignment) so the player can resume seamlessly upon reconnection.

## Decision

### Session Token Issuance

- After the client's JWT is validated and the WebSocket connection is established, the Gateway issues an opaque session token.
- The session token is a cryptographically random 32-byte value, base64url-encoded (44 characters).
- The session token is sent to the client in a `session_established` message immediately after the WebSocket handshake completes.
- The session token is stored in Redis by the Gateway with the following structure:

```
Key: session:{session_token}
Value: {
  player_id: "...",
  runtime_id: "...",
  zone_id: "...",
  game_server_addr: "...",
  created_at: <unix timestamp>,
  last_activity: <unix timestamp>
}
TTL: 60 seconds (sliding)
```

### Reconnection Flow

1. Client disconnects (network blip, tab switch, etc.).
2. Gateway detects disconnect, does **not** immediately clean up the session.
3. Gateway starts a 30-second reconnection window timer.
4. Gateway sends a `peer_disconnected` notification to the Game Server (so the Game Server knows the player is temporarily absent but should not despawn immediately).
5. Game Server marks the player entity as `disconnected` but **does not despawn** or release state.
6. Game Server continues to simulate the player's entity for up to 30 seconds (e.g., apply gravity, idle animation). After 30s, the Game Server despawns the entity.
7. Within the 30-second window, if the client reconnects:
   a. Client opens a new WebSocket connection to the Gateway: `wss://gateway/v1/ws?session=<session_token>`.
   b. Gateway looks up `session:{session_token}` in Redis.
   c. If found and within the 30-second window:
      - Gateway validates the session is not expired, belongs to the same runtime.
      - Gateway re-establishes the connection to the Game Server via gRPC.
      - Gateway sends a `player_reconnected` notification to the Game Server.
      - Game Server resumes the player entity from `disconnected` state.
      - Game Server replays missed state deltas (position updates, attribute changes) accumulated during the disconnect.
      - Gateway responds with `session_resumed` to the client.
      - The 30-second timer is cancelled.
   d. If not found or expired: the client must fall back to full JWT authentication.
8. After 30 seconds, if no reconnection:
   - Gateway deletes `session:{session_token}` from Redis.
   - Gateway sends `player_disconnected` (final) to the Game Server.
   - Game Server despawns the player entity, releases zone resources, broadcasts despawn to other players.

### Game Server Missed State Deltas

- While a player is in `disconnected` state, the Game Server buffers state deltas (position, attribute changes, event notifications) in a ring buffer.
- Maximum buffer size: 1000 deltas (configurable). If the buffer overflows, the oldest deltas are dropped.
- On reconnection, the Game Server replays buffered deltas in order to the reconnecting client.
- After replay, the Game Server sends the full current state of all visible entities (not just changed ones) to ensure consistency.

### Session Cleanup

- **TTL-based eviction from Redis**: the `session:{session_token}` key has a 60-second sliding TTL. Each access resets the TTL.
- On orderly disconnect (client sends close frame), the session is immediately deleted from Redis and the 30-second reconnection window is not started.
- On unexpected disconnect (network loss, timeout), the session remains for the 30-second window as described above.
- Orphaned sessions (Gateway crash without cleanup) are automatically evicted by Redis TTL within 60 seconds.

### Session Token Validation Rules

- Session tokens are single-use within the reconnection window. Once a session is resumed, a new session token is issued.
- Session tokens are bound to the source IP: if the reconnection comes from a different IP, the session is rejected (mitigates session hijacking).
- Session tokens are opaque — no embedded claims. All claim data is in Redis.
- No rate limiting on session reconnection attempts (separate from JWT auth rate limiting), but failed session lookups (invalid token) are logged and monitored.

## Alternatives

1. **Full re-authentication for every reconnect**: Simplest implementation — no session management, no Redis dependency. Forces the client to obtain a new JWT from Business Backend on every reconnect. Rejected because it creates a poor user experience and adds load on the Business Backend.

2. **Client-side session state (cookie/localStorage)**: Client stores serialized session state and presents it on reconnect. No server-side session storage needed. Rejected because the client cannot be trusted with session state — a malicious client could tamper with the state or replay an old session.

3. **No reconnection window, just JWT caching**: Gateway caches validated JWTs for a short period (e.g., 30s). On reconnect, if the JWT is in the cache, the client can reuse it. Simpler than full session management but does not preserve game state — the player would still need to re-enter the game world and re-sync state.

4. **Long-lived sessions (hours)**: Session persists indefinitely until orderly disconnect. Simpler logic but wastes resources on zombie sessions and complicates capacity planning for Game Servers (they must reserve memory for disconnected players indefinitely).

## Tradeoffs

- Redis dependency for session storage adds operational complexity but provides fast (sub-ms) lookups and automatic TTL-based cleanup.
- 30-second reconnection window is a reasonable balance: long enough for most network blips (mobile handoffs typically complete in 5-10s), short enough to avoid excessive resource reservation.
- Buffering state deltas in the Game Server adds memory overhead per disconnected player (~10 KB for 1000 deltas). Acceptable because the number of concurrently disconnected players is bounded by the reconnection window and the connection limit.
- Source IP binding for session tokens may cause issues with clients behind load balancers or proxies with changing IPs. Mitigation: if the source IP changes but the session token is valid, allow the reconnection but log a warning (this is a configurable behavior).

## Consequences

- Temporary disconnections (network blips, tab switches) result in seamless reconnection without visible state loss.
- Permanent disconnections (or orderly quits) clean up state promptly — no zombie sessions.
- Game Servers must implement the `disconnected` state and delta buffering logic.
- Clients must store the session token and present it on reconnection before the JWT would be needed.
- Redis is now a dependency for the Gateway (used for session storage). Redis must be highly available (sentinel or cluster mode) to avoid single points of failure for reconnection.

## Future Considerations

- **Cross-region reconnection**: if a client's network path changes to a different region, the session could be migrated to the nearest Gateway (requires session token to be stored in a global Redis cluster).
- **Reconnection without state loss for long disconnects**: for mobile games where the player may be disconnected for minutes (tunnel, airplane), consider storing a snapshot of entity state in Redis so the Game Server can reconstruct it.
- **Reconnection metrics**: track reconnect success rate, time-to-reconnect, and delta replay size to tune the 30-second window and delta buffer size.

## Replaces

- Previous design had no reconnection support — any disconnect required full re-authentication via Business Backend and full state re-synchronization.
