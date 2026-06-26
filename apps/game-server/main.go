package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	"github.com/thaolaptrinh/spatial-server/internal/types"
	"github.com/thaolaptrinh/spatial-server/pkg/entity"
	"github.com/thaolaptrinh/spatial-server/pkg/game"
	"github.com/thaolaptrinh/spatial-server/pkg/logging"
	spatialserverv1 "github.com/thaolaptrinh/spatial-server/proto/gen/spatialserver/v1"
)

type clientEntry struct {
	ch   chan []byte
	done chan struct{}
}

type clientRegistry struct {
	mu      sync.Mutex
	clients map[string]*clientEntry
}

func newClientRegistry() *clientRegistry {
	return &clientRegistry{clients: make(map[string]*clientEntry)}
}

func (r *clientRegistry) register(id string, ch chan []byte) chan struct{} {
	r.mu.Lock()
	done := make(chan struct{})
	r.clients[id] = &clientEntry{ch: ch, done: done}
	r.mu.Unlock()
	return done
}

func (r *clientRegistry) unregister(id string) {
	r.mu.Lock()
	if e, ok := r.clients[id]; ok {
		close(e.done)
		close(e.ch)
		delete(r.clients, id)
	}
	r.mu.Unlock()
}

func (r *clientRegistry) send(id string, data []byte) {
	r.mu.Lock()
	e, ok := r.clients[id]
	r.mu.Unlock()
	if !ok {
		return
	}
	select {
	case e.ch <- data:
	case <-e.done:
	}
}

type gameServerServer struct {
	spatialserverv1.UnimplementedGameServerServer
	game    *game.Game
	clients *clientRegistry
}

func newGameServerServer(g *game.Game) *gameServerServer {
	s := &gameServerServer{
		game:    g,
		clients: newClientRegistry(),
	}
	go s.drainOutbox()
	return s
}

func (s *gameServerServer) drainOutbox() {
	for pkt := range s.game.Outbox {
		s.clients.send(pkt.ClientID, pkt.Data)
	}
}

func (s *gameServerServer) Relay(stream spatialserverv1.GameServer_RelayServer) error {
	ctx := stream.Context()
	sendMu := &sync.Mutex{}
	var owned []string
	cleanup := func() {
		for _, id := range owned {
			s.clients.unregister(id)
			s.game.EnqueueRemoveEntity(types.EntityID(id))
		}
	}
	defer cleanup()

	for {
		pkt, err := stream.Recv()
		if err != nil {
			return err
		}

		switch pkt.GetKind() {
		case spatialserverv1.Kind_KIND_CONNECT:
			id := pkt.GetClientId()
			ch := make(chan []byte, 64)
			done := s.clients.register(id, ch)
			owned = append(owned, id)

			s.game.EnqueueAddEntity(entity.New(
				types.EntityID(id),
				"avatar",
				types.RuntimeID(pkt.GetMeta().GetRuntimeId()),
			))

			go func(ch chan []byte, done chan struct{}) {
				defer s.clients.unregister(id)
				for {
					select {
					case data, ok := <-ch:
						if !ok {
							return
						}
						sendMu.Lock()
						err := stream.Send(&spatialserverv1.RelayPacket{
							ClientId: id,
							Kind:     spatialserverv1.Kind_KIND_DATA,
							Payload:  data,
						})
						sendMu.Unlock()
						if err != nil {
							return
						}
					case <-ctx.Done():
						return
					case <-done:
						return
					}
				}
			}(ch, done)

		case spatialserverv1.Kind_KIND_DATA:
			select {
			case s.game.Inbox <- game.InboundPacket{
				ClientID: pkt.GetClientId(),
				Data:     pkt.GetPayload(),
			}:
			default:
			}

		case spatialserverv1.Kind_KIND_DISCONNECT:
			s.clients.unregister(pkt.GetClientId())
			s.game.EnqueueRemoveEntity(types.EntityID(pkt.GetClientId()))
		}
	}
}

func main() {
	k := koanf.New(".")
	if err := k.Load(file.Provider("configs/defaults.yml"), yaml.Parser()); err != nil {
		fmt.Fprintf(os.Stderr, "load defaults: %v\n", err)
		os.Exit(1)
	}
	if err := k.Load(file.Provider("configs/game-server.yml"), yaml.Parser()); err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	logger := logging.NewDefault(k.String("service.name"))

	gRPCPort := k.Int("grpc.port")
	host := k.String("grpc.host")
	tickRateStr := k.String("game.tick_rate")
	tickRate, _ := time.ParseDuration(tickRateStr)

	serverID := types.NewServerID()

	roomServiceHost := "room-service"
	conn, err := grpc.NewClient(fmt.Sprintf("%s:9000", roomServiceHost),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		logger.Error("connect to room service failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer conn.Close()

	rsClient := spatialserverv1.NewRoomServiceClient(conn)
	_, err = rsClient.Register(context.Background(), &spatialserverv1.RegisterRequest{
		ServerId: string(serverID),
		Host:     host,
		Port:     int32(gRPCPort),
		MaxZones: 10,
	})
	if err != nil {
		logger.Error("register with room service failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
	logger.Info("registered with room service", slog.String("server_id", string(serverID)))

	heartbeatCtx, heartbeatCancel := context.WithCancel(context.Background())
	go func() {
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-heartbeatCtx.Done():
				return
			case <-t.C:
				_, err := rsClient.Heartbeat(context.Background(), &spatialserverv1.HeartbeatRequest{
					ServerId: string(serverID),
				})
				if err != nil {
					logger.Warn("heartbeat failed", slog.String("error", err.Error()))
				}
			}
		}
	}()

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", gRPCPort))
	if err != nil {
		logger.Error("listen failed", slog.Int("port", gRPCPort), slog.String("error", err.Error()))
		os.Exit(1)
	}

	healthSrv := health.NewServer()
	healthSrv.SetServingStatus("spatialserver.v1.GameServer", grpc_health_v1.HealthCheckResponse_SERVING)

	srv := grpc.NewServer()
	grpc_health_v1.RegisterHealthServer(srv, healthSrv)
	reflection.Register(srv)

	gameCtx, gameCancel := context.WithCancel(context.Background())
	g := game.New(serverID, game.WithTickRate(tickRate))

	// Seed NPC for demo (vertical slice)
	g.AddEntity(entity.New(types.NewEntityID(), "npc", types.NewRuntimeID()))

	gs := newGameServerServer(g)
	spatialserverv1.RegisterGameServerServer(srv, gs)

	go func() {
		logger.Info("game loop starting", slog.String("tick", tickRateStr))
		if err := g.Run(gameCtx); err != nil && err != context.Canceled {
			logger.Error("game loop exited", slog.String("error", err.Error()))
		}
	}()

	go func() {
		logger.Info("game-server starting", slog.Int("port", gRPCPort))
		if err := srv.Serve(lis); err != nil {
			logger.Error("grpc serve error", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	logger.Info("game-server shutting down")
	gameCancel()
	heartbeatCancel()
	srv.GracefulStop()
	logger.Info("game-server stopped")
}
