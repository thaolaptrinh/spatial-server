package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"

	"github.com/thaolaptrinh/spatial-server/internal/migration"
	"github.com/thaolaptrinh/spatial-server/internal/types"
	"github.com/thaolaptrinh/spatial-server/internal/room"
	"github.com/thaolaptrinh/spatial-server/internal/config"
	grpcinterceptor "github.com/thaolaptrinh/spatial-server/internal/grpc"
	"github.com/thaolaptrinh/spatial-server/internal/logging"
	"github.com/thaolaptrinh/spatial-server/internal/metrics"
	"github.com/thaolaptrinh/spatial-server/internal/storage"
	storageroom "github.com/thaolaptrinh/spatial-server/internal/storage/room"
	spatialserverv1 "github.com/thaolaptrinh/spatial-server/proto/gen/spatialserver/v1"
)

type roomServiceServer struct {
	spatialserverv1.UnimplementedRoomServiceServer
	servers   room.ServerStore
	zones     room.ZoneStore
	fanout    *room.WatcherFanout
	allocator room.Allocator
}

func (s *roomServiceServer) Register(ctx context.Context, req *spatialserverv1.RegisterRequest) (*spatialserverv1.RegisterResponse, error) {
	err := s.servers.Register(ctx, &room.NodeDescriptor{
		NodeID:    types.ServerID(req.ServerId),
		AdvertiseAddr: req.AdvertiseAddr,
		Host:      req.Host, Port: int(req.Port),
		Version:   req.Version, Build: req.Build,
		Capacity:  room.NodeCapacity{MaxZones: int(req.MaxZones)},
		Labels:    req.Labels,
	})
	if err != nil {
		return &spatialserverv1.RegisterResponse{Success: false}, nil
	}
	return &spatialserverv1.RegisterResponse{Success: true}, nil
}

func (s *roomServiceServer) Heartbeat(ctx context.Context, req *spatialserverv1.HeartbeatRequest) (*spatialserverv1.HeartbeatResponse, error) {
	err := s.servers.Heartbeat(ctx, types.ServerID(req.NodeId), room.NodeLoad{
		ActiveEntities: int(req.ActiveEntities),
		ActiveSpaces:   int(req.ActiveSpaces),
		ConnectedUsers: int(req.ConnectedUsers),
		QueueDepth:     int(req.QueueDepth),
		TickDurationMs: req.TickDurationMs,
	})
	if err != nil {
		return &spatialserverv1.HeartbeatResponse{Acknowledged: false}, nil
	}
	return &spatialserverv1.HeartbeatResponse{Acknowledged: true}, nil
}

func (s *roomServiceServer) LookupZone(ctx context.Context, req *spatialserverv1.LookupZoneRequest) (*spatialserverv1.LookupZoneResponse, error) {
	serverID, err := s.zones.Lookup(ctx, req.ZoneId)
	if err == nil {
		info, err := s.servers.Get(ctx, serverID)
		if err == nil {
			return &spatialserverv1.LookupZoneResponse{
				Server:        &spatialserverv1.ServerID{Id: string(info.NodeID)},
				AdvertiseAddr: info.Address(),
			}, nil
		}
	}
	nodes, err := s.servers.List(ctx)
	if err != nil || len(nodes) == 0 {
		return nil, status.Errorf(codes.Unavailable, "no servers available for zone %s", req.ZoneId)
	}
	server, err := s.allocator.Select(nodes)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "no servers available for zone %s", req.ZoneId)
	}
	if err := s.zones.Claim(ctx, req.ZoneId, "", server.NodeID); err != nil {
		return nil, status.Errorf(codes.Internal, "claim zone %s: %v", req.ZoneId, err)
	}
	return &spatialserverv1.LookupZoneResponse{
		Server:        &spatialserverv1.ServerID{Id: string(server.NodeID)},
		AdvertiseAddr: server.Address(),
	}, nil
}

func (s *roomServiceServer) WatchOwnership(req *spatialserverv1.WatchRequest, stream spatialserverv1.RoomService_WatchOwnershipServer) error {
	id, ch := s.fanout.Subscribe()
	defer s.fanout.Unsubscribe(id)
	ctx := stream.Context()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case change, ok := <-ch:
			if !ok {
				return nil
			}
			if err := stream.Send(change); err != nil {
				return err
			}
		}
	}
}

func main() {
	cfg, err := config.Load("configs/defaults.yml", "configs/room-service.yml")
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	logger := logging.NewDefault(cfg.Service.Name)
	ctx := logging.WithLogger(context.Background(), logger)

	pgPool, err := storage.NewPostgresPool(ctx, cfg.Postgres.DSN)
	if err != nil {
		logger.Error("postgres connection failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer pgPool.Close()

	redisClient, err := storage.NewRedisClient(cfg.Redis.Addr)
	if err != nil {
		logger.Error("redis connection failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer redisClient.Close()

	if err := migration.Run(pgPool, "internal/storage/migrations"); err != nil {
		logger.Error("migration failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
	logger.Info("migrations completed")

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

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.GRPC.Port))
	if err != nil {
		logger.Error("listen failed", slog.Int("port", cfg.GRPC.Port), slog.String("error", err.Error()))
		os.Exit(1)
	}

	healthSrv := health.NewServer()
	healthSrv.SetServingStatus("spatialserver.v1.RoomService", grpc_health_v1.HealthCheckResponse_SERVING)

	srv := grpc.NewServer(grpcinterceptor.ServerOptions("room-service", reg)...)
	grpc_health_v1.RegisterHealthServer(srv, healthSrv)
	reflection.Register(srv)

	service := &roomServiceServer{
		servers:   storageroom.NewServerRepository(pgPool),
		zones:     storageroom.NewZoneRepository(pgPool),
		fanout:    room.NewWatcherFanout(),
		allocator: room.LeastLoadedAllocator{},
	}
	spatialserverv1.RegisterRoomServiceServer(srv, service)

	apiSrv := room.NewSpatialServerAPI(room.NewMemoryRuntimeStore(), fmt.Sprintf("gateway:%d", cfg.Gateway.WSPort))
	spatialserverv1.RegisterSpatialServerAPIServer(srv, apiSrv)
	healthSrv.SetServingStatus("spatialserver.v1.SpatialServerAPI", grpc_health_v1.HealthCheckResponse_SERVING)

	go func() {
		logger.Info("room-service starting", slog.Int("port", cfg.GRPC.Port))
		if err := srv.Serve(lis); err != nil {
			logger.Error("grpc serve error", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	logger.Info("room-service shutting down")
	srv.GracefulStop()
	logger.Info("room-service stopped")
}
