package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"

	spatialserverv1 "github.com/thaolaptrinh/spatial-server/proto/gen/spatialserver/v1"

	"github.com/thaolaptrinh/spatial-server/internal/config"
	"github.com/thaolaptrinh/spatial-server/internal/gateway"
	"github.com/thaolaptrinh/spatial-server/internal/logging"
	"github.com/thaolaptrinh/spatial-server/internal/metrics"
	"github.com/thaolaptrinh/spatial-server/internal/transport/websocket/coder"
)

type roomLookuper struct {
	client spatialserverv1.RoomServiceClient
}

func (r *roomLookuper) LookupZone(ctx context.Context, zoneID string) (string, error) {
	resp, err := r.client.LookupZone(ctx, &spatialserverv1.LookupZoneRequest{ZoneId: zoneID})
	if err != nil {
		return "", err
	}
	if a := resp.GetAdvertiseAddr(); a != "" {
		return a, nil
	}
	return fmt.Sprintf("%s:%d", resp.GetHost(), resp.GetPort()), nil
}

func main() {
	cfg, err := config.Load("configs/defaults.yml", "configs/gateway.yml")
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	logger := logging.NewDefault(cfg.Service.Name)

	// Dial room-service for zone lookups
	rsConn, err := grpc.NewClient(cfg.RoomService.Addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		logger.Error("connect to room service", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer rsConn.Close()
	rsClient := spatialserverv1.NewRoomServiceClient(rsConn)

	// Wrap room-service as a ZoneLookuper
	lookuper := &roomLookuper{client: rsClient}

	// Health check client for readiness
	healthClient := grpc_health_v1.NewHealthClient(rsConn)

	cache := gateway.NewRouterCache(5 * time.Second)
	baseCtx, baseCancel := context.WithCancel(context.Background())
	defer baseCancel()
	handler := gateway.NewHandler(cache, lookuper, []byte(cfg.Gateway.JWTSecret), coder.Accepter{})
	handler.SetBaseContext(baseCtx)
	handler.SetReadyFn(func() bool {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		resp, err := healthClient.Check(ctx, &grpc_health_v1.HealthCheckRequest{})
		return err == nil && resp.GetStatus() == grpc_health_v1.HealthCheckResponse_SERVING
	})

	reg := metrics.NewRegistry()
	handler.SetMetrics(reg)

	go func() {
		mux := http.NewServeMux()
		mux.Handle("/metrics", reg.Handler())
		metricsAddr := fmt.Sprintf(":%d", cfg.Metrics.Port)
		logger.Info("metrics HTTP server starting", slog.String("addr", metricsAddr))
		if err := (&http.Server{Addr: metricsAddr, Handler: mux}).ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("metrics http serve error", slog.String("error", err.Error()))
		}
	}()

	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Gateway.WSPort),
		Handler: handler,
	}

	go func() {
		logger.Info("gateway HTTP server starting", slog.Int("port", cfg.Gateway.WSPort))
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("http serve error", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}()

	logger.Info("gateway starting",
		slog.Int("ws_port", cfg.Gateway.WSPort),
	)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	logger.Info("gateway shutting down")

	handler.SetDraining(true)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Gateway.DrainTimeout)
	defer cancel()
	httpServer.Shutdown(shutdownCtx)
	logger.Info("gateway stopped")
}
