package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"

	"github.com/thaolaptrinh/spatial-server/internal/migration"
	"github.com/thaolaptrinh/spatial-server/internal/types"
	"github.com/thaolaptrinh/spatial-server/pkg/logging"
	"github.com/thaolaptrinh/spatial-server/pkg/room"
	"github.com/thaolaptrinh/spatial-server/pkg/storage"
	spatialserverv1 "github.com/thaolaptrinh/spatial-server/proto/gen/spatialserver/v1"
)

type roomServiceServer struct {
	spatialserverv1.UnimplementedRoomServiceServer
	registry  *room.ServerRegistry
	ownership *room.ZoneOwnership
}

func (s *roomServiceServer) Register(ctx context.Context, req *spatialserverv1.RegisterRequest) (*spatialserverv1.RegisterResponse, error) {
	err := s.registry.Register(&room.ServerInfo{
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
	err := s.registry.Heartbeat(types.ServerID(req.ServerId))
	if err != nil {
		return &spatialserverv1.HeartbeatResponse{Acknowledged: false}, nil
	}
	return &spatialserverv1.HeartbeatResponse{Acknowledged: true}, nil
}

func (s *roomServiceServer) LookupZone(ctx context.Context, req *spatialserverv1.LookupZoneRequest) (*spatialserverv1.LookupZoneResponse, error) {
	serverID, host, port, err := room.ResolveZone(s.ownership, s.registry, req.ZoneId)
	if err != nil {
		server, ok := s.registry.LeastLoaded()
		if !ok {
			return nil, status.Errorf(codes.Unavailable, "no servers available for zone %s", req.ZoneId)
		}
		if err := s.ownership.Claim(req.ZoneId, server.ID); err != nil {
			return nil, status.Errorf(codes.Internal, "claim zone %s: %v", req.ZoneId, err)
		}
		return &spatialserverv1.LookupZoneResponse{
			Server: &spatialserverv1.ServerID{Id: string(server.ID)},
			Host:   server.Host,
			Port:   int32(server.Port),
		}, nil
	}
	return &spatialserverv1.LookupZoneResponse{
		Server: &spatialserverv1.ServerID{Id: string(serverID)},
		Host:   host,
		Port:   int32(port),
	}, nil
}

func main() {
	k := koanf.New(".")
	if err := k.Load(file.Provider("configs/defaults.yml"), yaml.Parser()); err != nil {
		fmt.Fprintf(os.Stderr, "load defaults: %v\n", err)
		os.Exit(1)
	}
	if err := k.Load(file.Provider("configs/room-service.yml"), yaml.Parser()); err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}
	if err := k.Load(env.Provider("SPATIAL_", ".", func(s string) string {
		return strings.Replace(strings.ToLower(strings.TrimPrefix(s, "SPATIAL_")), "__", ".", -1)
	}), nil); err != nil {
		fmt.Fprintf(os.Stderr, "load env: %v\n", err)
		os.Exit(1)
	}

	logger := logging.NewDefault(k.String("service.name"))
	ctx := logging.WithLogger(context.Background(), logger)

	pgDSN := k.String("postgres.dsn")
	pgPool, err := storage.NewPostgresPool(ctx, pgDSN)
	if err != nil {
		logger.Error("postgres connection failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer pgPool.Close()

	redisAddr := k.String("redis.addr")
	redisClient, err := storage.NewRedisClient(redisAddr)
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

	gRPCPort := k.Int("grpc.port")
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", gRPCPort))
	if err != nil {
		logger.Error("listen failed", slog.Int("port", gRPCPort), slog.String("error", err.Error()))
		os.Exit(1)
	}

	healthSrv := health.NewServer()
	healthSrv.SetServingStatus("spatialserver.v1.RoomService", grpc_health_v1.HealthCheckResponse_SERVING)

	srv := grpc.NewServer()
	grpc_health_v1.RegisterHealthServer(srv, healthSrv)
	reflection.Register(srv)

	registry := room.NewServerRegistry()
	ownership := room.NewZoneOwnership()
	service := &roomServiceServer{registry: registry, ownership: ownership}
	spatialserverv1.RegisterRoomServiceServer(srv, service)

	go func() {
		logger.Info("room-service starting", slog.Int("port", gRPCPort))
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
