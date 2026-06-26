# Phase 1G — Make It Demoable

> **Last Updated:** 2026-06-26
> **Status:** Draft

## Purpose

Phase 1F wired the end-to-end realtime data path (WebSocket → Gateway → bidi gRPC Relay → Game Server), but there is no way to run or demo it. Three gaps remain:

1. **No Dockerfiles**: `deploy/docker-compose/docker-compose.yml` references `build/docker/*.Dockerfile` paths that do not exist.
2. **No client**: No tool can connect to the Gateway WebSocket, send packets, or display received packets.
3. **No single command**: Running the vertical slice requires manually building images, starting containers, and running a client — there is no `make demo`.

**Phase 1G fills these gaps.** After this phase, one command (`make demo`) starts all services and connects a client that prints spawn/move/despawn events to the terminal.

## Scope

- Dockerfiles for all 3 service binaries (gateway, room-service, game-server)
- Go client CLI (`tools/client/main.go`)
- `make demo` target
- Config fixes in Docker Compose (JWT secret alignment, game-server host override)

**Out of scope:**
- Real integration tests (kept as scaffold for Phase 2)
- Production Dockerfile concerns (non-root user, distroless base, healthcheck)
- TLS/mTLS
- Metrics or observability
- Multi-client support

## Architecture

```
┌──────────────────────────────────────────────────────┐
│  Developer Machine                                    │
│                                                       │
│  ┌─────────────┐      ┌──────────────────────────┐   │
│  │ tools/client │──────▶  gateway (localhost:8080) │   │
│  │  (Go CLI)    │ WS   │  WS → Relay stream       │   │
│  └─────────────┘      └──────┬───────────────────┘   │
│                              │ gRPC                   │
│                    ┌─────────▼──────────┐             │
│                    │  game-server:9000   │             │
│                    │  Relay handler      │             │
│                    │  Game loop + AOI    │             │
│                    └────────────────────┘             │
│                              │                        │
│                    ┌─────────▼──────────┐             │
│                    │  room-service:9000  │             │
│                    │  Register/Heartbeat │             │
│                    │  LookupZone         │             │
│                    └────────────────────┘             │
│                                                       │
│  ┌──────────┐  ┌──────────┐                           │
│  │ postgres │  │  redis   │                           │
│  │ :5432    │  │ :6379    │                           │
│  └──────────┘  └──────────┘                           │
└──────────────────────────────────────────────────────┘
```

## Components

### 1. Dockerfiles (`build/docker/`)

Three Dockerfiles, one per service. All follow the same pattern:

```
FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /bin/service ./apps/<service>/

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=builder /bin/service /bin/service
COPY configs/ /configs/
EXPOSE <port>
ENTRYPOINT ["/bin/service"]
```

| Dockerfile | Port | Notes |
|------------|------|-------|
| `build/docker/gateway.Dockerfile` | 8080 (WS) | Configs baked in; env vars override via koanf |
| `build/docker/room-service.Dockerfile` | 9000 (gRPC) | Needs postgres + redis healthy |
| `build/docker/game-server.Dockerfile` | 9000 (gRPC) | Registers with room-service on startup |

Configs are at `/configs/` in the container; the binary loads `configs/defaults.yml` etc. relative to CWD `/` which resolves to `/configs/defaults.yml`.

### 2. Client CLI (`tools/client/main.go`)

A single Go file in package `main`, runnable via `go run ./tools/client/`.

**Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `-addr` | `localhost:8080` | Gateway WebSocket address |
| `-player` | `p1` | Player ID (used as entity ID and JWT `player_id`) |
| `-runtime` | `r1` | Runtime ID for JWT `runtime_id` claim |
| `-zone` | `z1` | Zone ID for JWT `zone_id` claim |
| `-secret` | `dev-secret-key-change-in-production` | JWT signing secret (must match gateway config) |
| `-interval` | `1s` | Position update interval |

**Flow:**

1. Generate JWT using `github.com/golang-jwt/jwt/v5` with HS256 and claims:
   ```go
   jwt.MapClaims{
       "player_id":  flagPlayer,
       "runtime_id": flagRuntime,
       "zone_id":    flagZone,
       "exp":        time.Now().Add(5 * time.Minute).Unix(),
   }
   ```
2. Dial WebSocket: `ws://{addr}/ws?token={jwt}`
3. Start read-loop goroutine:
   - Read binary frame from WebSocket
   - `protocol.Decode(frame)` → `(packetID, payload, _, err)`
   - Switch on `packetID`:
     - `PacketIDEntitySpawn`: `proto.Unmarshal(payload, &EntitySnapshot)` → print `"SPAWN {type} at ({x}, {y}, {z})"`
     - `PacketIDPositionUpdate`: `proto.Unmarshal(payload, &EntityUpdate)` → print `"MOVE {id} → ({x}, {y}, {z})"`
     - `PacketIDEntityDespawn`: `proto.Unmarshal(payload, &EntityID)` → print `"DESPAWN {id}"`
     - Default: print `"UNKNOWN packetID={id} len={n}"`
4. Start write-loop goroutine:
   - Every `interval`, build and send a PositionUpdate:
     ```go
     pos := &v1.Vector3{X: moveX, Y: 0, Z: moveZ}
     upd := &v1.EntityUpdate{EntityId: flagPlayer, Position: pos, Timestamp: time.Now().UnixMilli()}
     payload, _ := proto.Marshal(upd)
     frame := protocol.Encode(protocol.PacketIDPositionUpdate, payload, false)
     conn.Write(ctx, websocket.MessageBinary, frame)
     ```
   - Position oscillates in a small square pattern (e.g., x: 100→110→100, z: 100→100→110→100 every 4 ticks)
5. On SIGINT/SIGTERM: close WebSocket, exit
6. On WebSocket close (server disconnect): print reason and exit

**Imports required:**

```
github.com/coder/websocket
github.com/golang-jwt/jwt/v5
github.com/thaolaptrinh/spatial-server/pkg/protocol
github.com/thaolaptrinh/spatial-server/proto/gen/spatialserver/v1
google.golang.org/protobuf/proto
```

All dependencies already exist in `go.mod` (added by Phase 1F).

### 3. `make demo`

```makefile
DOCKER_COMPOSE := deploy/docker-compose/docker-compose.yml

.PHONY: demo
demo:
	docker compose -f $(DOCKER_COMPOSE) up -d --build
	@echo "Waiting for gateway..."
	@for i in 1 2 3 4 5 6 7 8 9 10; do \
		if curl -sf http://localhost:8080/health > /dev/null 2>&1; then \
			echo "Gateway ready!"; \
			break; \
		fi; \
		echo "  attempt $$i/10..."; \
		sleep 2; \
	done
	-go run ./tools/client/ -addr localhost:8080 -player p1 -runtime r1 -zone z1 || true
	docker compose -f $(DOCKER_COMPOSE) down
```

Polling `/health` (up to 20s) rather than a static sleep. On client exit (Ctrl+C), containers are torn down.

### 4. Config & Compose fixes

Already applied:

| Fix | File | Detail |
|-----|------|--------|
| JWT secret alignment | `deploy/docker-compose/docker-compose.yml` | `SPATIAL_GATEWAY__JWT_SECRET: "dev-secret-key-change-in-production"` (matches `configs/gateway.yml`) |
| Game-server host override | `deploy/docker-compose/docker-compose.yml` | `SPATIAL_GRPC__HOST: "game-server"` (prevents registration with `0.0.0.0`) |

## Data Flow (Demo Scenario)

```
1. make demo builds images, starts containers
2. Gateway starts, loads config, listens on :8080
3. Game-server starts, registers with room-service (host=game-server, port=9000)
4. Client runs, generates JWT, dials ws://localhost:8080/ws?token=...
5. Gateway validates JWT, calls LookupZone → gets game-server:9000
6. Gateway opens Relay stream, sends KIND_CONNECT{player_id: "p1", ...}
7. Game-server creates player entity (p1) at (0,0,0)
8. Game-server ticks: NPC (seeded at (0,0,0)) is within AOI range of p1
9. Game-server sends KIND_DATA{payload: EntitySpawn(NPC)} via Relay → Gateway → WS
10. Client prints: "SPAWN npc at (0, 0, 0)"
11. Client sends PositionUpdate(p1, 100, 0, 100) every second
12. Game-server dispatches → moves entity → AOI detects movement
13. Game-server sends KIND_DATA{payload: PositionUpdate(p1, 100, 0, 100)} to client
14. Client prints: "MOVE p1 → (100, 0, 100)"
15. Ctrl+C → client disconnects → gateway sends KIND_DISCONNECT
16. Game-server removes entity p1, sends despawn if needed
17. make demo → docker compose down
```

## Files Changed

| File | Action |
|------|--------|
| `build/docker/gateway.Dockerfile` | Create |
| `build/docker/room-service.Dockerfile` | Create |
| `build/docker/game-server.Dockerfile` | Create |
| `tools/client/main.go` | Create |
| `Makefile` | Modify (add `demo` target) |
| `deploy/docker-compose/docker-compose.yml` | Already modified (2 fixes committed) |

## References

- [Phase 1F spec](./2026-06-26-phase1f-realtime-data-path.md)
- [Phase 1F implementation plan](../plans/2026-06-26-phase1f-realtime-data-path.md)
- [ADR-012 Networking](../../adr/012-networking.md)
