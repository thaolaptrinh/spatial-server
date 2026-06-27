package main

import (
	"testing"
	"time"

	"github.com/thaolaptrinh/spatial-server/internal/gateway"
)

func TestGatewayWired(t *testing.T) {
	cache := gateway.NewRouterCache(time.Second)
	h := gateway.NewHandler(cache, nil, []byte("test"))
	if h == nil {
		t.Fatal("handler is nil")
	}
}
