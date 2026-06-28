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
	action := flag.String("action", "", "EntityAction to send on connect (e.g. jump)")
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
