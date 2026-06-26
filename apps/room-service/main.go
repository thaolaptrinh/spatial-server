package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	"github.com/thaolaptrinh/spatial-server/internal/migration"
	"github.com/thaolaptrinh/spatial-server/pkg/logging"
	"github.com/thaolaptrinh/spatial-server/pkg/storage"
)

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
