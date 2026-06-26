package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
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
	"github.com/thaolaptrinh/spatial-server/pkg/logging"
	spatialserverv1 "github.com/thaolaptrinh/spatial-server/proto/gen/spatialserver/v1"
)

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

	tickCtx, tickCancel := context.WithCancel(context.Background())
	go func() {
		logger.Info("ticker starting", slog.String("rate", tickRateStr))
		t := time.NewTicker(tickRate)
		defer t.Stop()
		for {
			select {
			case <-tickCtx.Done():
				logger.Info("ticker stopped")
				return
			case <-t.C:
			}
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
	tickCancel()
	heartbeatCancel()
	srv.GracefulStop()
	logger.Info("game-server stopped")
}
