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
	"sync/atomic"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	"github.com/thaolaptrinh/spatial-server/internal/migration"
	"github.com/thaolaptrinh/spatial-server/internal/types"
	"github.com/thaolaptrinh/spatial-server/internal/config"
	"github.com/thaolaptrinh/spatial-server/internal/game/entity"
	"github.com/thaolaptrinh/spatial-server/internal/game"
	grpcinterceptor "github.com/thaolaptrinh/spatial-server/internal/grpc"
	"github.com/thaolaptrinh/spatial-server/internal/logging"
	"github.com/thaolaptrinh/spatial-server/internal/metrics"
	"github.com/thaolaptrinh/spatial-server/internal/storage"
	storagegame "github.com/thaolaptrinh/spatial-server/internal/storage/game"
	"github.com/thaolaptrinh/spatial-server/internal/game/zone"
	spatialserverv1 "github.com/thaolaptrinh/spatial-server/proto/gen/spatialserver/v1"
)

type boundedSendQueue struct {
	ch    chan []byte
	drops atomic.Uint64
}

func newBoundedSendQueue(capacity int) *boundedSendQueue {
	return &boundedSendQueue{ch: make(chan []byte, capacity)}
}

func (q *boundedSendQueue) push(data []byte) {
	select {
	case q.ch <- data:
	default:
		select {
		case <-q.ch:
			q.drops.Add(1)
		default:
		}
		select {
		case q.ch <- data:
		default:
			q.drops.Add(1)
		}
	}
}

type clientEntry struct {
	q    *boundedSendQueue
	done chan struct{}
}

type clientRegistry struct {
	mu      sync.Mutex
	clients map[string]*clientEntry
}

func newClientRegistry() *clientRegistry {
	return &clientRegistry{clients: make(map[string]*clientEntry)}
}

func (r *clientRegistry) register(id string) *clientEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	entry := &clientEntry{q: newBoundedSendQueue(64), done: make(chan struct{})}
	r.clients[id] = entry
	return entry
}

func (r *clientRegistry) unregister(id string) {
	r.mu.Lock()
	if e, ok := r.clients[id]; ok {
		close(e.done)
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
	e.q.push(data)
}

type gameServerServer struct {
	spatialserverv1.UnimplementedGameServerServer
	game    *game.Game
	clients *clientRegistry
	reg     *metrics.Registry
}

func newGameServerServer(g *game.Game, reg *metrics.Registry) *gameServerServer {
	s := &gameServerServer{
		game:    g,
		clients: newClientRegistry(),
		reg:     reg,
	}
	go s.drainEvents()
	return s
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
			entry := s.clients.register(id)
			owned = append(owned, id)

			s.game.EnqueueAddEntity(entity.New(
				types.EntityID(id),
				"avatar",
				types.RuntimeID(pkt.GetMeta().GetRuntimeId()),
			))

			go func(q *boundedSendQueue, done chan struct{}) {
				defer s.clients.unregister(id)
				for {
					select {
					case data, ok := <-q.ch:
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
			}(entry.q, entry.done)

		case spatialserverv1.Kind_KIND_DATA:
			select {
			case s.game.Inbox <- game.InboundPacket{
				ClientID: pkt.GetClientId(),
				Data:     pkt.GetPayload(),
			}:
			default:
				if s.reg != nil {
					s.reg.DroppedTotal.WithLabelValues("inbox").Inc()
				}
			}

		case spatialserverv1.Kind_KIND_RECONNECT:
			s.game.MarkReconnected(types.EntityID(pkt.GetClientId()))

		case spatialserverv1.Kind_KIND_PEER_DISCONNECTED:
			s.game.MarkDisconnected(types.EntityID(pkt.GetClientId()))

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

func (s *gameServerServer) QueryEntities(ctx context.Context, req *spatialserverv1.QueryEntitiesRequest) (*spatialserverv1.QueryEntitiesResponse, error) {
	var snaps []*spatialserverv1.EntitySnapshot
	for zid := range s.game.Zones {
		snaps = s.game.QueryLocal(types.ZoneID(zid), int(req.GetGridX()), int(req.GetGridY()), req.GetRadius())
		break
	}
	return &spatialserverv1.QueryEntitiesResponse{Entities: snaps}, nil
}

func (s *gameServerServer) NotifyEntityEnter(ctx context.Context, req *spatialserverv1.EntityEnterLeave) (*spatialserverv1.NotifyResponse, error) {
	pos := req.GetPosition()
	s.game.ApplyEntityEnter(types.ZoneID(req.GetZoneId()), types.EntityID(req.GetEntityId()), "", types.Vector3{X: pos.GetX(), Y: pos.GetY(), Z: pos.GetZ()})
	return &spatialserverv1.NotifyResponse{Acknowledged: true}, nil
}

func (s *gameServerServer) NotifyEntityLeave(ctx context.Context, req *spatialserverv1.EntityEnterLeave) (*spatialserverv1.NotifyResponse, error) {
	s.game.ApplyEntityLeave(types.ZoneID(req.GetZoneId()), types.EntityID(req.GetEntityId()))
	return &spatialserverv1.NotifyResponse{Acknowledged: true}, nil
}

func (s *gameServerServer) SendEntityUpdate(ctx context.Context, req *spatialserverv1.EntityUpdate) (*spatialserverv1.Ack, error) {
	pos := req.GetPosition()
	if pos == nil {
		return &spatialserverv1.Ack{Success: false}, nil
	}
	zoneID := s.game.ZoneOf(types.EntityID(req.GetEntityId()))
	s.game.ApplyEntityUpdate(zoneID, types.EntityID(req.GetEntityId()), types.Vector3{X: pos.GetX(), Y: pos.GetY(), Z: pos.GetZ()})
	return &spatialserverv1.Ack{Success: true}, nil
}

func (s *gameServerServer) MigrateEntity(ctx context.Context, req *spatialserverv1.MigrateEntityRequest) (*spatialserverv1.MigrateEntityResponse, error) {
	s.game.MigrateEntityIn(req.GetEntity(), types.ZoneID(req.GetTargetZoneId()))
	return &spatialserverv1.MigrateEntityResponse{Success: true}, nil
}

func (s *gameServerServer) ZoneStateSync(stream spatialserverv1.GameServer_ZoneStateSyncServer) error {
	ctx := stream.Context()
	for {
		snap, err := stream.Recv()
		if err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		s.game.LoadZoneSnapshot(snap)
	}
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
		NodeId: string(serverID),
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

	if err := migration.Run(pgPool, "internal/storage/migrations"); err != nil {
		logger.Error("run migrations failed", slog.String("error", err.Error()))
		os.Exit(1)
	}

	snapRepo := storagegame.NewSnapshotStore(pgPool)
	g := game.New(serverID, game.WithTickRate(tickRate), game.WithSnapshotter(snapshotAdapter{repo: snapRepo, runtime: string(serverID)}, cfg.Game.SnapshotInterval), game.WithMetrics(gameMetricsAdapter{reg: reg}))

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
			npc.Attrs["behavior"] = []byte(spec.Behavior)
			npc.Lifecycle = &game.NPCLifecycle{Behavior: newBehaviorFor(spec)}
			g.AddEntity(npc)
		}
	}

	gs := newGameServerServer(g, reg)
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
	repo    *storagegame.SnapshotStore
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
		npc.Attrs["behavior"] = []byte(r.Behavior)
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
