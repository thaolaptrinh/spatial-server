package gateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/thaolaptrinh/spatial-server/internal/types"
	"github.com/thaolaptrinh/spatial-server/pkg/session"
)

type fakeLookuper struct {
	host string
	port int32
	err  error
}

func (f *fakeLookuper) LookupZone(ctx context.Context, zoneID string) (string, int32, error) {
	return f.host, f.port, f.err
}

func TestHealthHandler(t *testing.T) {
	cache := NewRouterCache(time.Second)
	h := NewHandler(cache, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "ok")
}

func TestHealthHandler_Method(t *testing.T) {
	cache := NewRouterCache(time.Second)
	h := NewHandler(cache, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/health", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestWSHandler_NoToken(t *testing.T) {
	cache := NewRouterCache(time.Second)
	h := NewHandler(cache, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestNotFound(t *testing.T) {
	cache := NewRouterCache(time.Second)
	h := NewHandler(cache, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandleWS_Health(t *testing.T) {
	cache := NewRouterCache(time.Second)
	h := NewHandler(cache, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestSessionPoolLifecycle(t *testing.T) {
	pool := session.NewPool()
	pool.Add(session.NewSession("c1", "p1", types.ZoneID("z1"), types.ServerID("gs1")))
	assert.Equal(t, 1, pool.Count())

	s, ok := pool.Get("c1")
	assert.True(t, ok)
	assert.Equal(t, "p1", s.PlayerID)

	pool.Remove("c1")
	assert.Equal(t, 0, pool.Count())
}
