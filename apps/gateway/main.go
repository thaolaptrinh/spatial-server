package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	spatialserverv1 "github.com/thaolaptrinh/spatial-server/proto/gen/spatialserver/v1"

	"github.com/thaolaptrinh/spatial-server/pkg/gateway"
	"github.com/thaolaptrinh/spatial-server/pkg/logging"
)

type roomLookuper struct {
	client spatialserverv1.RoomServiceClient
}

func (r *roomLookuper) LookupZone(ctx context.Context, zoneID string) (string, int32, error) {
	resp, err := r.client.LookupZone(ctx, &spatialserverv1.LookupZoneRequest{ZoneId: zoneID})
	if err != nil {
		return "", 0, err
	}
	return resp.GetHost(), resp.GetPort(), nil
}

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
	if err := k.Load(env.Provider("SPATIAL_", ".", func(s string) string {
		return strings.Replace(strings.ToLower(strings.TrimPrefix(s, "SPATIAL_")), "__", ".", -1)
	}), nil); err != nil {
		fmt.Fprintf(os.Stderr, "load env: %v\n", err)
		os.Exit(1)
	}

	logger := logging.NewDefault(k.String("service.name"))

	wsPort := k.Int("gateway.ws_port")

	// Dial room-service for zone lookups
	roomServiceAddr := k.String("room_service.addr")
	rsConn, err := grpc.NewClient(roomServiceAddr,
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

	jwtSecret := k.String("gateway.jwt_secret")
	cache := gateway.NewRouterCache(5 * time.Second)
	handler := gateway.NewHandler(cache, lookuper, []byte(jwtSecret))

	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", wsPort),
		Handler: handler,
	}

	go func() {
		logger.Info("gateway HTTP server starting", slog.Int("port", wsPort))
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("http serve error", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}()

	logger.Info("gateway starting",
		slog.Int("ws_port", wsPort),
	)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	logger.Info("gateway shutting down")
	httpServer.Shutdown(context.Background())
	logger.Info("gateway stopped")
}
