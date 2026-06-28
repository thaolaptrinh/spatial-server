package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/coder/websocket"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"

	"github.com/thaolaptrinh/spatial-server/pkg/protocol"
	spatialserverv1 "github.com/thaolaptrinh/spatial-server/proto/gen/spatialserver/v1"
)

func main() {
	addr := flag.String("addr", "localhost:8080", "Gateway WebSocket address")
	player := flag.String("player", "p1", "Player ID (used as entity ID and JWT player_id)")
	runtimeID := flag.String("runtime", "r1", "Runtime ID for JWT runtime_id claim")
	zoneID := flag.String("zone", "z1", "Zone ID for JWT zone_id claim (overridden when --provision is set)")
	secret := flag.String("secret", "dev-secret-key-change-in-production", "JWT signing secret (must match gateway config)")
	interval := flag.Duration("interval", 1*time.Second, "Position update interval")
	action := flag.String("action", "", "EntityAction to send on connect (e.g. jump)")

	provision := flag.Bool("provision", false, "Provision runtime+zone via room-service before connecting")
	rsAddr := flag.String("room-service", "localhost:9001", "Room service gRPC address (used with --provision)")
	pgDSN := flag.String("pg-dsn", "postgres://spatial:spatial@localhost:5432/spatial?sslmode=disable", "PostgreSQL DSN (used with --provision)")
	zoneCount := flag.Int("zone-count", 1, "Number of zones to create (used with --provision)")
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if *provision {
		zID, err := doProvision(ctx, *rsAddr, *pgDSN, *runtimeID, *zoneCount)
		if err != nil {
			log.Fatalf("provision: %v", err)
		}
		zoneID = &zID
		log.Printf("provisioned runtime=%s zone=%s", *runtimeID, *zoneID)
	}

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

			_, id, payload, _, _, err := protocol.Decode(msg)
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

			case protocol.PacketIDEntityAction:
				var act spatialserverv1.EntityAction
				if err := proto.Unmarshal(payload, &act); err != nil {
					log.Printf("unmarshal action: %v", err)
					continue
				}
				log.Printf("ACTION %s %s", act.GetEntityId(), act.GetAction())

			case protocol.PacketIDEntityState:
				var st spatialserverv1.EntityState
				if err := proto.Unmarshal(payload, &st); err != nil {
					log.Printf("unmarshal state: %v", err)
					continue
				}
				log.Printf("STATE %s attrs=%d", st.GetEntityId(), len(st.GetAttributes()))

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
				// Trace a 4-corner box: (100,100)→(110,100)→(110,110)→(100,110) and repeat
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
				frame := protocol.Encode(protocol.PacketIDPositionUpdate, payload, false, 0)
				if err := conn.Write(ctx, websocket.MessageBinary, frame); err != nil {
					log.Printf("write error: %v", err)
					cancel()
					return
				}
				if *action != "" && tick == 1 {
					actPayload, _ := proto.Marshal(&spatialserverv1.EntityAction{
						EntityId:  *player,
						Action:    *action,
						Timestamp: time.Now().UnixMilli(),
					})
					actFrame := protocol.Encode(protocol.PacketIDEntityAction, actPayload, false, 1)
					if err := conn.Write(ctx, websocket.MessageBinary, actFrame); err != nil {
						log.Printf("action write error: %v", err)
					} else {
						log.Printf("sent action: %s", *action)
					}
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

func doProvision(ctx context.Context, rsAddr, pgDSN, runtimeID string, zoneCount int) (string, error) {
	pool, err := pgxpool.New(ctx, pgDSN)
	if err != nil {
		return "", fmt.Errorf("connect postgres: %w", err)
	}
	defer pool.Close()

	zoneID := ""
	conn, err := grpc.NewClient(rsAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return "", fmt.Errorf("dial room-service: %w", err)
	}
	defer conn.Close()

	api := spatialserverv1.NewSpatialServerAPIClient(conn)
	resp, err := api.CreateRuntime(ctx, &spatialserverv1.CreateRuntimeRequest{
		RuntimeId: runtimeID, ZoneCount: int32(zoneCount),
	})
	if err != nil {
		// Runtime already exists (e.g. retry) — derive zone ID from pattern
		zoneID = fmt.Sprintf("%s-z1", runtimeID)
	} else {
		if len(resp.GetZones()) == 0 {
			return "", fmt.Errorf("create runtime returned no zones")
		}
		zoneID = resp.GetZones()[0].GetZoneId()
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO runtimes (id, status, zone_count) VALUES ($1, 'active', $2) ON CONFLICT (id) DO NOTHING`,
		runtimeID, zoneCount); err != nil {
		return "", fmt.Errorf("seed runtime: %w", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO zones (id, runtime_id, grid_x, grid_y, status) VALUES ($1, $2, 0, 0, 'unowned') ON CONFLICT (id) DO NOTHING`,
		zoneID, runtimeID); err != nil {
		return "", fmt.Errorf("seed zone: %w", err)
	}
	return zoneID, nil
}
