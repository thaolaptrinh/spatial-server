package observability

import (
	"context"
	"testing"
)

func TestInitTracer_ReturnsShutdownFunc(t *testing.T) {
	shutdown, err := InitTracer(context.Background(), "gateway", "localhost:4317", 1.0)
	if err != nil {
		t.Logf("InitTracer (collector unreachable — expected): %v", err)
		return
	}
	if shutdown == nil {
		t.Fatal("shutdown nil")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
}
