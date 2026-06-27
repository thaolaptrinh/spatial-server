package grpcinterceptor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/thaolaptrinh/spatial-server/internal/metrics"
)

func TestRecovery_RecoversPanic(t *testing.T) {
	h := RecoveryInterceptor(metrics.NewRegistry())
	info := &grpc.UnaryServerInfo{FullMethod: "/svc/Boom"}
	_, err := h(context.Background(), nil, info, func(context.Context, any) (any, error) { panic("boom") })
	st, _ := status.FromError(err)
	assert.Equal(t, codes.Internal, st.Code())
}

func TestMetrics_RecordsLatency(t *testing.T) {
	h := MetricsUnaryInterceptor(metrics.NewRegistry(), "svc")
	_, err := h(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/svc/Ping"}, func(context.Context, any) (any, error) { return nil, nil })
	assert.NoError(t, err)
}
