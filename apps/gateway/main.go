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

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"

	"github.com/thaolaptrinh/spatial-server/pkg/gateway"
	"github.com/thaolaptrinh/spatial-server/pkg/logging"
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

	wsPort := k.Int("gateway.ws_port")
	cache := gateway.NewRouterCache(5 * time.Second)
	handler := gateway.NewHandler(cache, nil, nil)

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
