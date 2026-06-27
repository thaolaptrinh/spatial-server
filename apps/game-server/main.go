package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	"github.com/thaolaptrinh/spatial-server/internal/migration"
	"github.com/thaolaptrinh/spatial-server/internal/types"
	"github.com/thaolaptrinh/spatial-server/pkg/config"
	"github.com/thaolaptrinh/spatial-server/pkg/entity"
	"github.com/thaolaptrinh/spatial-server/pkg/game"
	grpcinterceptor "github.com/thaolaptrinh/spatial-server/pkg/grpc"
	"github.com/thaolaptrinh/spatial-server/pkg/logging"
	"github.com/thaolaptrinh/spatial-server/pkg/metrics"
	"github.com/thaolaptrinh/spatial-server/pkg/storage"
	"github.com/thaolaptrinh/spatial-server/pkg/zone"
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

func (s *gameServerServer) AssignZone(ctx context.Context, req *spatialserverv1.AssignZoneRequest) (*spatialserverv1.AssignZoneResponse, error) {
	z := zone.New(
		types.ZoneID(req.GetZoneId()),
		types.RuntimeID(req.GetRuntimeId()),
		int(req.GetGridX()),
		int(req.GetGridY()),
		req.GetZoneSize(),
	)
	if err := s.game.AssignZone(z); err != nil {
		return &spatialserverv1.AssignZoneResponse{Success: false}, nil
	}
	return &spatialserverv1.AssignZoneResponse{Success: true}, nil
}

func (s *gameServerServer) ReleaseZone(ctx context.Context, req *spatialserverv1.ReleaseZoneRequest) (*spatialserverv1.ReleaseZoneResponse, error) {
	if err := s.game.ReleaseZone(types.ZoneID(req.GetZoneId())); err != nil {
		return &spatialserverv1.ReleaseZoneResponse{Success: false}, nil
	}
	return &spatialserverv1.ReleaseZoneResponse{Success: true}, nil
}

func main() {
	cfg, err := config.Load("configs/defaults.yml", "configs/game-server.yml")
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	logger := logging.NewDefault(cfg.Service.Name)

	gRPCPort := cfg.GRPC.Port
	host := cfg.GRPC.Host
	tickRate := cfg.Game.TickRate

	serverID := types.NewServerID()

	conn, err := grpc.NewClient(cfg.RoomService.Addr,
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

	reg := metrics.NewRegistry()

	go func() {
		mux := http.NewServeMux()
		mux.Handle("/metrics", reg.Handler())
		metricsAddr := fmt.Sprintf(":%d", cfg.Metrics.Port)
		logger.Info("metrics HTTP server starting", slog.String("addr", metricsAddr))
		if err := (&http.Server{Addr: metricsAddr, Handler: mux}).ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("metrics http serve error", slog.String("error", err.Error()))
		}
	}()

	healthSrv := health.NewServer()
	healthSrv.SetServingStatus("spatialserver.v1.GameServer", grpc_health_v1.HealthCheckResponse_SERVING)

	srv := grpc.NewServer(grpcinterceptor.ServerOptions("game-server", reg)...)
	grpc_health_v1.RegisterHealthServer(srv, healthSrv)
	reflection.Register(srv)

	gameCtx, gameCancel := context.WithCancel(context.Background())

	pgPool, err := storage.NewPostgresPool(context.Background(), cfg.Postgres.DSN)
	if err != nil {
		logger.Error("connect to postgres failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer pgPool.Close()

	if err := migration.Run(pgPool, "pkg/storage/migrations"); err != nil {
		logger.Error("run migrations failed", slog.String("error", err.Error()))
		os.Exit(1)
	}

	snapRepo := storage.NewSnapshotStore(pgPool)
	g := game.New(serverID, game.WithTickRate(tickRate), game.WithSnapshotter(snapshotAdapter{repo: snapRepo, runtime: string(serverID)}, cfg.Game.SnapshotInterval))

	// Crash recovery: restore from latest snapshot
	zoneID := types.ZoneID(string(serverID) + "-z1")
	if snap, _, err := snapRepo.Latest(context.Background(), zoneID); err == nil {
		hydrateFromSnapshot(g, snap)
		logger.Info("recovered entities from snapshot", slog.String("zone", string(zoneID)))
	} else {
		// Seed NPCs from config
		for _, spec := range cfg.Game.NPCs {
			npc := entity.New(types.NewEntityID(), spec.Type, types.RuntimeID(""))
			npc.Position = spec.Position
			npc.Behavior = spec.Behavior
			npc.Lifecycle = &game.NPCLifecycle{Behavior: newBehaviorFor(spec)}
			g.AddEntity(npc)
		}
	}

	gs := newGameServerServer(g)
	spatialserverv1.RegisterGameServerServer(srv, gs)

	go func() {
		logger.Info("game loop starting", slog.String("tick", tickRate.String()))
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

type snapshotAdapter struct {
	repo    *storage.SnapshotStore
	runtime string
}

func (s snapshotAdapter) Save(zoneID types.ZoneID, snap []byte, tick int64) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := s.repo.Save(ctx, string(zoneID), s.runtime, snap, tick); err != nil {
		slog.Warn("snapshot save", slog.String("error", err.Error()))
	}
}

func hydrateFromSnapshot(g *game.Game, data []byte) {
	var rows []struct {
		ID       string  `json:"id"`
		Type     string  `json:"type"`
		Behavior string  `json:"behavior"`
		X        float64 `json:"x"`
		Y        float64 `json:"y"`
		Z        float64 `json:"z"`
	}
	if json.Unmarshal(data, &rows) != nil {
		return
	}
	for _, r := range rows {
		npc := entity.New(types.EntityID(r.ID), r.Type, types.RuntimeID(""))
		npc.Position = types.Vector3{X: r.X, Y: r.Y, Z: r.Z}
		npc.Behavior = r.Behavior
		npc.Lifecycle = &game.NPCLifecycle{Behavior: newBehaviorFor(config.NPCSpec{Behavior: r.Behavior, Position: npc.Position})}
		g.AddEntity(npc)
	}
}

func newBehaviorFor(spec config.NPCSpec) game.Behavior {
	switch spec.Behavior {
	case "patrol":
		return &game.PatrolBehavior{Speed: 10, Waypoints: spec.Waypoints}
	case "wander":
		return &game.WanderBehavior{Origin: spec.Position, Radius: spec.Radius, Speed: 10}
	default:
		return &game.IdleBehavior{BobAmplitude: 0.5, BobFreq: 2}
	}
}
