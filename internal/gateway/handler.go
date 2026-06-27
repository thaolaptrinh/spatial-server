package gateway

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync/atomic"

	"github.com/thaolaptrinh/spatial-server/internal/auth"
	"github.com/thaolaptrinh/spatial-server/internal/session"
	transportws "github.com/thaolaptrinh/spatial-server/internal/transport/websocket"
	"github.com/thaolaptrinh/spatial-server/internal/types"
)

type ZoneLookuper interface {
	LookupZone(ctx context.Context, zoneID string) (host string, port int32, err error)
}

type Handler struct {
	mux        *http.ServeMux
	cache      *RouterCache
	lookuper   ZoneLookuper
	jwtSecret  []byte
	pool       *session.Pool
	wsAccepter transportws.Accepter
	draining   atomic.Bool
	conns      atomic.Int64
	softLimit  int
	readyFn    func() bool
}

func NewHandler(cache *RouterCache, lookuper ZoneLookuper, jwtSecret []byte, wsAccepter transportws.Accepter) *Handler {
	h := &Handler{
		cache:      cache,
		lookuper:   lookuper,
		jwtSecret:  jwtSecret,
		pool:       session.NewPool(),
		wsAccepter: wsAccepter,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", h.handleHealth)
	mux.HandleFunc("/live", h.handleLive)
	mux.HandleFunc("/ready", h.handleReady)
	mux.HandleFunc("/ws", h.handleWS)
	h.mux = mux
	h.softLimit = 9000
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (h *Handler) SetDraining(v bool) {
	h.draining.Store(v)
}

func (h *Handler) SetReadyFn(fn func() bool) {
	h.readyFn = fn
}

func (h *Handler) ConnCount() int64 {
	return h.conns.Load()
}

func (h *Handler) handleLive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "alive"})
}

func (h *Handler) handleReady(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.draining.Load() {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	if h.readyFn != nil && !h.readyFn() {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	if h.conns.Load() >= int64(h.softLimit) {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) handleWS(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "missing token", http.StatusBadRequest)
		return
	}

	claims, err := auth.ValidateToken(token, h.jwtSecret)
	if err != nil {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}

	h.conns.Add(1)
	defer h.conns.Add(-1)

	host, port, err := h.lookuper.LookupZone(r.Context(), claims.ZoneID)
	if err != nil {
		http.Error(w, "zone not available", http.StatusServiceUnavailable)
		return
	}

	clientID := claims.PlayerID
	serverID := types.ServerID("")
	_ = serverID

	c, err := h.wsAccepter.Accept(w, r)
	if err != nil {
		slog.Warn("websocket accept", slog.String("error", err.Error()))
		return
	}
	c.SetReadLimit(64 * 1024)

	sess := session.NewSession(clientID, claims.PlayerID, types.ZoneID(claims.ZoneID), serverID)
	h.pool.Add(sess)

	go h.relayWS(c, clientID, host, int(port), claims)
}
