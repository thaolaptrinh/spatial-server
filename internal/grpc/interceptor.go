package grpcinterceptor

import (
	"context"
	"log/slog"
	"runtime/debug"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/thaolaptrinh/spatial-server/internal/metrics"
)

func RecoveryInterceptor(_ *metrics.Registry) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("grpc panic", slog.Any("panic", r), slog.String("method", info.FullMethod), slog.String("stack", string(debug.Stack())))
				err = status.Error(codes.Internal, "internal error")
			}
		}()
		return handler(ctx, req)
	}
}

func RecoveryStreamInterceptor(_ *metrics.Registry) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) (err error) {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("grpc stream panic", slog.Any("panic", r), slog.String("stack", string(debug.Stack())))
				err = status.Error(codes.Internal, "internal error")
			}
		}()
		return handler(srv, ss)
	}
}

func LoggingUnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		if err != nil {
			slog.Warn("grpc request", slog.String("method", info.FullMethod), slog.String("error", err.Error()), slog.Duration("duration", time.Since(start)))
		} else {
			slog.Info("grpc request", slog.String("method", info.FullMethod), slog.Duration("duration", time.Since(start)))
		}
		return resp, err
	}
}

func MetricsUnaryInterceptor(reg *metrics.Registry, service string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		reg.GRPCRequestDuration.WithLabelValues(service, method(info.FullMethod)).Observe(time.Since(start).Seconds())
		return resp, err
	}
}

func MetricsStreamInterceptor(reg *metrics.Registry, service string) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		start := time.Now()
		err := handler(srv, ss)
		reg.GRPCRequestDuration.WithLabelValues(service, method(info.FullMethod)).Observe(time.Since(start).Seconds())
		return err
	}
}

func ServerOptions(service string, reg *metrics.Registry) []grpc.ServerOption {
	return []grpc.ServerOption{
		grpc.ChainUnaryInterceptor(RecoveryInterceptor(reg), LoggingUnaryInterceptor(), MetricsUnaryInterceptor(reg, service)),
		grpc.ChainStreamInterceptor(RecoveryStreamInterceptor(reg), MetricsStreamInterceptor(reg, service)),
	}
}

func method(full string) string {
	if i := strings.LastIndex(full, "/"); i >= 0 {
		return full[i+1:]
	}
	return full
}
