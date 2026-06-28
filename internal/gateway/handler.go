package gateway

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"

	"github.com/thaolaptrinh/spatial-server/internal/auth"
	"github.com/thaolaptrinh/spatial-server/internal/metrics"
	"github.com/thaolaptrinh/spatial-server/internal/session"
	transportws "github.com/thaolaptrinh/spatial-server/internal/transport/websocket"
	"github.com/thaolaptrinh/spatial-server/internal/types"
	spatialserverv1 "github.com/thaolaptrinh/spatial-server/proto/gen/spatialserver/v1"
)

type ZoneLookuper interface {
	LookupZone(ctx context.Context, zoneID string) (addr string, err error)
}

type Handler struct {
	mux           *http.ServeMux
	cache         *RouterCache
	lookuper      ZoneLookuper
	jwtSecret     []byte
	pool          *session.Pool
	wsAccepter    transportws.Accepter
	draining      atomic.Bool
	conns         atomic.Int64
	softLimit     int
	readyFn       func() bool
	connLimitRate float64
	ipLimiter     *ipLimiter
	reg           *metrics.Registry
	baseCtx       context.Context
}

func NewHandler(cache *RouterCache, lookuper ZoneLookuper, jwtSecret []byte, wsAccepter transportws.Accepter) *Handler {
	h := &Handler{
		cache:         cache,
		lookuper:      lookuper,
		jwtSecret:     jwtSecret,
		pool:          session.NewPool(),
		wsAccepter:    wsAccepter,
		connLimitRate: 100,
		ipLimiter:     newIPLimiter(500, 500, time.Now),
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

// SetMetrics wires the Prometheus registry so the gateway can report active
// connections and dropped packets.
func (h *Handler) SetMetrics(reg *metrics.Registry) {
	h.reg = reg
}

// SetBaseContext sets the base context used as the parent for relay goroutines
// so they are cancelled on shutdown instead of leaking.
func (h *Handler) SetBaseContext(ctx context.Context) {
	h.baseCtx = ctx
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

// Sets a marker, fixing handler below
func (h *Handler) handleWS(w http.ResponseWriter, r *http.Request) {
	if h.draining.Load() {
		http.Error(w, "draining", http.StatusServiceUnavailable)
		return
	}
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
	if h.reg != nil {
		h.reg.ActiveConnections.Inc()
		defer h.reg.ActiveConnections.Dec()
	}

	addr, err := h.lookuper.LookupZone(r.Context(), claims.ZoneID)
	if err != nil {
		http.Error(w, "zone not available", http.StatusServiceUnavailable)
		return
	}

	clientID := claims.PlayerID

	c, err := h.wsAccepter.Accept(w, r)
	if err != nil {
		slog.Warn("websocket accept", slog.String("error", err.Error()))
		return
	}
	c.SetReadLimit(64 * 1024)

	sess := session.NewSession(clientID, claims.PlayerID, types.ZoneID(claims.ZoneID), types.ServerID(""))
	h.pool.Add(sess)

	go h.relayWS(c, clientID, addr, claims)
}

type ZoneWatcher interface {
	WatchOwnership(ctx context.Context, in *spatialserverv1.WatchRequest, opts ...grpc.CallOption) (spatialserverv1.RoomService_WatchOwnershipClient, error)
}

func (h *Handler) StartOwnershipWatch(ctx context.Context, w ZoneWatcher) {
	go func() {
		for {
			stream, err := w.WatchOwnership(ctx, &spatialserverv1.WatchRequest{})
			if err != nil {
				select {
				case <-ctx.Done():
					return
				case <-time.After(2 * time.Second):
				}
				continue
			}
			for {
				change, err := stream.Recv()
				if err != nil {
					select {
					case <-ctx.Done():
						return
					case <-time.After(2 * time.Second):
					}
					break
				}
				h.cache.ApplyChange(change)
			}
		}
	}()
}
