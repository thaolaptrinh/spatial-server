# Phase 1G — Make It Demoable Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** One command (`make demo`) builds containers, starts services, runs a Go client CLI that prints spawn/move events, and tears down cleanly.

**Architecture:** Three Dockerfiles (gateway, room-service, game-server) based on `golang:1.25-alpine`; a single-file Go CLI at `tools/client/main.go` dials `ws://localhost:8080/ws?token=<jwt>`, sends position updates every 1s, and prints SPAWN/MOVE/DESPAWN events; `make demo` orchestrates compose up → health poll → client → compose down.

**Tech Stack:** Go 1.25, Docker Compose v2+, `github.com/coder/websocket`, `github.com/golang-jwt/jwt/v5`, protobuf (protowire), `make`

**Pre-existing files (checked before writing):**
- `build/docker/gateway.Dockerfile` exists (Go 1.23, needs version bump)
- `build/docker/room-service.Dockerfile` exists (Go 1.23, needs version bump + migration dir)
- `build/docker/game-server.Dockerfile` exists (Go 1.23, needs version bump)
- `deploy/docker-compose/docker-compose.yml` exists (has depends_on YAML bug)
- `configs/defaults.yml` and `configs/gateway.yml` exist
- `pkg/protocol/protocol.go` — `Decode` returns `(PacketID, []byte, bool, error)`, `Encode` takes `(PacketID, []byte, bool) []byte`
- `pkg/game/game.go` — `Inbox chan InboundPacket`, `Outbox chan OutboundPacket`, both buffered (4096)
- `apps/game-server/main.go` — seeds NPC at line 248: `g.AddEntity(entity.New(...))`

---

### Task 1: Fix Dockerfiles — Go version bump + migration dir

**Files:**
- Modify: `build/docker/gateway.Dockerfile`
- Modify: `build/docker/room-service.Dockerfile`
- Modify: `build/docker/game-server.Dockerfile`

- [ ] **Step 1: Bump gateway.Dockerfile Go version 1.23 → 1.25**

  Change the first line from `FROM golang:1.23-alpine AS builder` to `FROM golang:1.25-alpine AS builder`, and the runtime base from `alpine:3.19` to `alpine:3.21`.

  ```bash
  # No code change needed — edit directly
  ```

  Edit `build/docker/gateway.Dockerfile`:
  - Line 1: `golang:1.23` → `golang:1.25`
  - Line 8: `alpine:3.19` → `alpine:3.21`

- [ ] **Step 2: Bump room-service.Dockerfile Go version and add migrations**

  Edit `build/docker/room-service.Dockerfile`:
  - Line 1: `golang:1.23` → `golang:1.25`
  - Line 8: `alpine:3.19` → `alpine:3.21`
  - Lines 10-13 should remain: `WORKDIR /app`, `COPY --from=builder /room-service .`, `COPY configs/ configs/`, `COPY pkg/storage/migrations/ pkg/storage/migrations/`

- [ ] **Step 3: Bump game-server.Dockerfile Go version**

  Edit `build/docker/game-server.Dockerfile`:
  - Line 1: `golang:1.23` → `golang:1.25`
  - Line 8: `alpine:3.19` → `alpine:3.21`

- [ ] **Step 4: Run `make docker-build` to verify**

  Run: `make docker-build 2>&1 | tail -30`
  Expected: three successful builds, no errors.

- [ ] **Step 5: Commit**

  ```bash
  git add build/docker/gateway.Dockerfile build/docker/room-service.Dockerfile build/docker/game-server.Dockerfile
  git commit -m "build: bump dockerfiles to go 1.25 and alpine 3.21"
  ```

---

### Task 2: Fix docker-compose.yml — depends_on YAML bug

**Files:**
- Modify: `deploy/docker-compose/docker-compose.yml`

- [ ] **Step 1: Read current gateway depends_on**

  Lines 61-64 currently read:
  ```yaml
      depends_on:
        - room-service
        - redis:
            condition: service_healthy
  ```

  This mixes list-form and map-form, which is invalid YAML for docker-compose.

- [ ] **Step 2: Rewrite gateway depends_on to map form**

  Change lines 61-64 to:
  ```yaml
      depends_on:
        room-service:
          condition: service_started
        redis:
          condition: service_healthy
  ```

- [ ] **Step 3: Verify compose config parses**

  Run: `docker compose -f deploy/docker-compose/docker-compose.yml config 2>&1 | head -50`
  Expected: no errors, valid YAML output.

- [ ] **Step 4: Commit**

  ```bash
  git add deploy/docker-compose/docker-compose.yml
  git commit -m "fix: gateway depends_on yaml syntax"
  ```

---

### Task 3: Create Go client CLI

**Files:**
- Create: `tools/client/main.go`

- [ ] **Step 1: Create the `tools/client/` directory**

  ```bash
  mkdir -p tools/client
  ```

- [ ] **Step 2: Write `tools/client/main.go`**

  ```go
  package main

  import (
  	"context"
  	"crypto/rand"
  	"flag"
  	"fmt"
  	"log"
  	"math"
  	"math/big"
  	"os"
  	"os/signal"
  	"syscall"
  	"time"

  	"github.com/coder/websocket"
  	"github.com/golang-jwt/jwt/v5"
  	"google.golang.org/protobuf/proto"

  	"github.com/thaolaptrinh/spatial-server/pkg/protocol"
  	spatialserverv1 "github.com/thaolaptrinh/spatial-server/proto/gen/spatialserver/v1"
  )

  func main() {
  	addr := flag.String("addr", "localhost:8080", "Gateway WebSocket address")
  	player := flag.String("player", "p1", "Player ID (used as entity ID and JWT player_id)")
  	runtimeID := flag.String("runtime", "r1", "Runtime ID for JWT runtime_id claim")
  	zoneID := flag.String("zone", "z1", "Zone ID for JWT zone_id claim")
  	secret := flag.String("secret", "dev-secret-key-change-in-production", "JWT signing secret (must match gateway config)")
  	interval := flag.Duration("interval", 1*time.Second, "Position update interval")
  	flag.Parse()

  	ctx, cancel := context.WithCancel(context.Background())
  	defer cancel()

  	// Generate JWT
  	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
  		"player_id":  *player,
  		"runtime_id": *runtimeID,
  		"zone_id":    *zoneID,
  		"exp":        time.Now().Add(5 * time.Minute).Unix(),
  	})
  	tokenStr, err := token.SignedString([]byte(*secret))
  	if err != nil {
  		log.Fatalf("generate jwt: %v", err)
  	}

  	// Dial WebSocket
  	url := fmt.Sprintf("ws://%s/ws?token=%s", *addr, tokenStr)
  	conn, _, err := websocket.Dial(ctx, url, nil)
  	if err != nil {
  		log.Fatalf("dial: %v", err)
  	}
  	defer conn.CloseNow()

  	log.Printf("connected as %s (runtime=%s, zone=%s)", *player, *runtimeID, *zoneID)

  	// Read loop
  	go func() {
  		for {
  			_, msg, err := conn.Read(ctx)
  			if err != nil {
  				log.Printf("read error (disconnected): %v", err)
  				cancel()
  				return
  			}

  			id, payload, _, err := protocol.Decode(msg)
  			if err != nil {
  				log.Printf("decode error: %v", err)
  				continue
  			}

  			switch id {
  			case protocol.PacketIDEntitySpawn:
  				var snap spatialserverv1.EntitySnapshot
  				if err := proto.Unmarshal(payload, &snap); err != nil {
  					log.Printf("unmarshal spawn: %v", err)
  					continue
  				}
  				pos := snap.GetPosition()
  				log.Printf("SPAWN %s (%s) at (%.0f, %.0f, %.0f)", snap.GetType(), snap.GetEntityId(), pos.GetX(), pos.GetY(), pos.GetZ())

  			case protocol.PacketIDEntityMove:
  				var upd spatialserverv1.EntityUpdate
  				if err := proto.Unmarshal(payload, &upd); err != nil {
  					log.Printf("unmarshal move: %v", err)
  					continue
  				}
  				pos := upd.GetPosition()
  				log.Printf("MOVE %s → (%.0f, %.0f, %.0f)", upd.GetEntityId(), pos.GetX(), pos.GetY(), pos.GetZ())

  			case protocol.PacketIDEntityDespawn:
  				var idMsg spatialserverv1.EntityID
  				if err := proto.Unmarshal(payload, &idMsg); err != nil {
  					log.Printf("unmarshal despawn: %v", err)
  					continue
  				}
  				log.Printf("DESPAWN %s", idMsg.GetId())

  			default:
  				log.Printf("UNKNOWN packetID=%d len=%d", id, len(payload))
  			}
  		}
  	}()

  	// Write loop — oscillating square pattern
  	tick := 0
  	writeTicker := time.NewTicker(*interval)
  	defer writeTicker.Stop()

  	writeLoop := func() {
  		for {
  			select {
  			case <-ctx.Done():
  				return
  			case <-writeTicker.C:
  				// Oscillate: x: 100→110→100, z: 100→100→110→100 every 4 ticks
  				phase := tick % 4
  				var moveX, moveZ float64
  				switch phase {
  				case 0:
  					moveX, moveZ = 100, 100
  				case 1:
  					moveX, moveZ = 110, 100
  				case 2:
  					moveX, moveZ = 110, 110
  				case 3:
  					moveX, moveZ = 100, 110
  				}
  				tick++

  				upd := &spatialserverv1.EntityUpdate{
  					EntityId:  *player,
  					Position:  &spatialserverv1.Vector3{X: moveX, Y: 0, Z: moveZ},
  					Timestamp: time.Now().UnixMilli(),
  				}
  				payload, err := proto.Marshal(upd)
  				if err != nil {
  					log.Printf("marshal error: %v", err)
  					continue
  				}
  				frame := protocol.Encode(protocol.PacketIDPositionUpdate, payload, false)
  				if err := conn.Write(ctx, websocket.MessageBinary, frame); err != nil {
  					log.Printf("write error: %v", err)
  					cancel()
  					return
  				}
  			}
  		}
  	}
  	go writeLoop()

  	// Wait for signal or context cancellation
  	sig := make(chan os.Signal, 1)
  	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
  	select {
  	case <-sig:
  		log.Println("shutting down")
  	case <-ctx.Done():
  	}
  }
  ```

- [ ] **Step 3: Verify it compiles**

  Run: `go build ./tools/client/`
  Expected: no errors, produces a binary or exits 0.

  Clean up the binary if produced: `rm -f client`

- [ ] **Step 4: Commit**

  ```bash
  git add tools/client/main.go
  git commit -m "feat: add demo client cli"
  ```

---

### Task 4: Add `make demo` target

**Files:**
- Modify: `Makefile`

- [ ] **Step 1: Add `demo` target to Makefile**

  Append before the final `.PHONY: docker-build` line (modify line 67). Change:

  ```makefile
  .PHONY: docker-build
  ```

  to:

  ```makefile
  .PHONY: docker-build demo

  demo:
  	docker compose -f deploy/docker-compose/docker-compose.yml up -d --build
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
  	docker compose -f deploy/docker-compose/docker-compose.yml down
  ```

  **Important:** The two-command approach (pseudo-target) uses Make's recipe rule — each line is a separate shell invocation. The `-` prefix on `go run` tells `make` to ignore its exit code, and `|| true` ensures a clean exit before running `docker compose down`. This way Ctrl+C during the client teardowns containers.

- [ ] **Step 2: Verify `make demo` target is listed**

  Run: `make -n demo 2>&1 | head -15`
  Expected: prints what `make demo` would do (dry-run). No parse errors.

- [ ] **Step 3: Full integration test (manual — only if Docker is running)**

  ```bash
  make demo
  ```

  Expected:
  1. Docker images build (gateway, room-service, game-server)
  2. Containers start (postgres, redis, room-service, game-server, gateway)
  3. "Gateway ready!" printed within 20s
  4. Client connects, prints "connected as p1", begins seeing SPAWN/MOVE events
  5. Press Ctrl+C → client exits → containers torn down

  **Note:** If Docker is unavailable, skip this step. The `make docker-build` step in Task 1 already validates that Dockerfiles produce valid images.

- [ ] **Step 4: Run `make build` and `go vet` to ensure no regressions**

  Run: `go build ./... && go vet ./internal/... ./pkg/...`
  Expected: no errors.

- [ ] **Step 5: Commit**

  ```bash
  git add Makefile
  git commit -m "feat: add make demo target"
  ```

---

## Self-Review Checklist

### Spec coverage

| Spec section | Task |
|---|---|
| Dockerfiles (3 files) | Task 1 |
| Go client CLI (`tools/client/main.go`) | Task 3 |
| `make demo` target | Task 4 |
| Config fixes (JWT secret, game-server host) | Already committed — not in this plan |
| Compose fix (depends_on YAML) | Task 2 |

### Placeholder scan
- No "TBD", "TODO", "implement later" ✅
- No "Add appropriate error handling" without showing what ✅
- No "Write tests" without actual code ✅
- No "Similar to Task N" — each step is self-contained ✅
- All code blocks contain complete implementations ✅

### Type consistency
- `protocol.Decode` returns `(PacketID, []byte, bool, error)` — client uses all four values ✅
- `protocol.Encode` takes `(PacketID, []byte, bool)` — client uses `protocol.PacketIDPositionUpdate` ✅
- Gateway reads token from `r.URL.Query().Get("token")` — client passes `?token=<jwt>` ✅
- Gateway health at `GET /health` — `make demo` polls `localhost:8080/health` ✅
- `proto.Marshal`/`proto.Unmarshal` — matching types (`EntitySnapshot`, `EntityUpdate`, `EntityID`) ✅
- Go module path `github.com/thaolaptrinh/spatial-server` — all imports use this ✅
- PacketID constants match `pkg/protocol/protocol.go`: `PacketIDEntitySpawn=0x04`, `PacketIDEntityMove=0x06`, `PacketIDEntityDespawn=0x05`, `PacketIDPositionUpdate=0x03` ✅
- `InboundPacket{ClientID, Data}`, `OutboundPacket{ClientID, Data}` — match `pkg/game/game.go` ✅

---

## Execution Handoff

Plan complete. Two execution options:

**1. Subagent-Driven (recommended)** — dispatch a fresh subagent per task (4 tasks, sequential), review between tasks, fast iteration.

**2. Inline Execution** — execute tasks in this session with checkpoints.

Which approach?
