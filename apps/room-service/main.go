package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
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
	"github.com/thaolaptrinh/spatial-server/pkg/api"
	"github.com/thaolaptrinh/spatial-server/pkg/config"
	"github.com/thaolaptrinh/spatial-server/pkg/logging"
	"github.com/thaolaptrinh/spatial-server/pkg/room"
	"github.com/thaolaptrinh/spatial-server/pkg/storage"
	spatialserverv1 "github.com/thaolaptrinh/spatial-server/proto/gen/spatialserver/v1"
)

type roomServiceServer struct {
	spatialserverv1.UnimplementedRoomServiceServer
	servers room.ServerStore
	zones   room.ZoneStore
}

func (s *roomServiceServer) Register(ctx context.Context, req *spatialserverv1.RegisterRequest) (*spatialserverv1.RegisterResponse, error) {
	err := s.servers.Register(ctx, &room.ServerInfo{
		ID:       types.ServerID(req.ServerId),
		Host:     req.Host,
		Port:     int(req.Port),
		Status:   types.ServerStatusJoining,
		MaxZones: int(req.MaxZones),
	})
	if err != nil {
		return &spatialserverv1.RegisterResponse{Success: false}, nil
	}
	return &spatialserverv1.RegisterResponse{Success: true}, nil
}

func (s *roomServiceServer) Heartbeat(ctx context.Context, req *spatialserverv1.HeartbeatRequest) (*spatialserverv1.HeartbeatResponse, error) {
	err := s.servers.Heartbeat(ctx, types.ServerID(req.ServerId))
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
				Server: &spatialserverv1.ServerID{Id: string(info.ID)},
				Host:   info.Host,
				Port:   int32(info.Port),
			}, nil
		}
	}
	server, err := s.servers.LeastLoaded(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "no servers available for zone %s", req.ZoneId)
	}
	if err := s.zones.Claim(ctx, req.ZoneId, "", server.ID); err != nil {
		return nil, status.Errorf(codes.Internal, "claim zone %s: %v", req.ZoneId, err)
	}
	return &spatialserverv1.LookupZoneResponse{
		Server: &spatialserverv1.ServerID{Id: string(server.ID)},
		Host:   server.Host,
		Port:   int32(server.Port),
	}, nil
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

	if err := migration.Run(pgPool, "pkg/storage/migrations"); err != nil {
		logger.Error("migration failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
	logger.Info("migrations completed")

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.GRPC.Port))
	if err != nil {
		logger.Error("listen failed", slog.Int("port", cfg.GRPC.Port), slog.String("error", err.Error()))
		os.Exit(1)
	}

	healthSrv := health.NewServer()
	healthSrv.SetServingStatus("spatialserver.v1.RoomService", grpc_health_v1.HealthCheckResponse_SERVING)

	srv := grpc.NewServer()
	grpc_health_v1.RegisterHealthServer(srv, healthSrv)
	reflection.Register(srv)

	service := &roomServiceServer{
		servers: storage.NewServerRepository(pgPool),
		zones:   storage.NewZoneRepository(pgPool),
	}
	spatialserverv1.RegisterRoomServiceServer(srv, service)

	apiSrv := api.NewSpatialServerAPI(api.NewMemoryRuntimeStore(), fmt.Sprintf("gateway:%d", cfg.Gateway.WSPort))
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
