package observability

import (
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/stats"
)

func ServerStatsHandler() stats.Handler {
	return otelgrpc.NewServerHandler()
}

func ClientStatsHandler() stats.Handler {
	return otelgrpc.NewClientHandler()
}

func WithTraceContext() grpc.DialOption {
	return grpc.WithStatsHandler(ClientStatsHandler())
}
