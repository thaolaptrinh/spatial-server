package logging

import (
	"context"
	"log/slog"
	"os"
)

type ctxKey string

const loggerKey ctxKey = "logger"

const (
	ServiceKey   = "service"
	InstanceKey  = "instance"
	TraceIDKey   = "trace_id"
	RequestIDKey = "request_id"
	SessionIDKey = "session_id"
	ZoneIDKey    = "zone_id"
	EntityIDKey  = "entity_id"
	RuntimeIDKey = "runtime_id"
)

func New(service string, level slog.Level, json bool) *slog.Logger {
	var h slog.Handler
	opts := &slog.HandlerOptions{Level: level}
	if json {
		h = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		h = slog.NewTextHandler(os.Stdout, opts)
	}
	return slog.New(h).With(ServiceKey, service)
}

func NewDefault(service string) *slog.Logger {
	level := slog.LevelInfo
	if os.Getenv("SPATIAL_DEBUG") == "1" {
		level = slog.LevelDebug
	}
	return New(service, level, os.Getenv("SPATIAL_JSON") == "1")
}

func WithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}

func FromContext(ctx context.Context) *slog.Logger {
	logger, ok := ctx.Value(loggerKey).(*slog.Logger)
	if !ok || logger == nil {
		return slog.Default()
	}
	return logger
}
