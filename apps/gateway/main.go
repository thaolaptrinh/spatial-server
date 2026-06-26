package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"

	"github.com/thaolaptrinh/spatial-server/pkg/logging"
	"github.com/thaolaptrinh/spatial-server/pkg/storage"
)

func main() {
	k := koanf.New(".")
	if err := k.Load(file.Provider("configs/defaults.yml"), yaml.Parser()); err != nil {
		fmt.Fprintf(os.Stderr, "load defaults: %v\n", err)
		os.Exit(1)
	}
	if err := k.Load(file.Provider("configs/gateway.yml"), yaml.Parser()); err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	logger := logging.NewDefault(k.String("service.name"))
	ctx := logging.WithLogger(context.Background(), logger)

	redisAddr := k.String("redis.addr")
	redisClient, err := storage.NewRedisClient(redisAddr)
	if err != nil {
		logger.Error("redis connection failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer redisClient.Close()

	gRPCPort := k.Int("grpc.port")
	roomServiceAddr := fmt.Sprintf("room-service:%d", gRPCPort)
	gRPCPool, err := grpc.NewClient(roomServiceAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		logger.Error("grpc dial failed", slog.String("target", roomServiceAddr), slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer gRPCPool.Close()

	healthSrv := health.NewServer()
	healthSrv.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)

	logger.Info("gateway starting",
		slog.String("ws_port", fmt.Sprintf("%d", k.Int("gateway.ws_port"))),
		slog.String("room_service", roomServiceAddr),
	)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	logger.Info("gateway shutting down")
	logger.Info("gateway stopped")

	_ = ctx
	_ = gRPCPool
}
