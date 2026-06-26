package gateway

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/coder/websocket"

	"github.com/thaolaptrinh/spatial-server/internal/types"
	"github.com/thaolaptrinh/spatial-server/pkg/auth"
	"github.com/thaolaptrinh/spatial-server/pkg/session"
)

type ZoneLookuper interface {
	LookupZone(ctx context.Context, zoneID string) (host string, port int32, err error)
}

type Handler struct {
	mux       *http.ServeMux
	cache     *RouterCache
	lookuper  ZoneLookuper
	jwtSecret []byte
	pool      *session.Pool
}

func NewHandler(cache *RouterCache, lookuper ZoneLookuper, jwtSecret []byte) *Handler {
	h := &Handler{
		cache:     cache,
		lookuper:  lookuper,
		jwtSecret: jwtSecret,
		pool:      session.NewPool(),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", h.handleHealth)
	mux.HandleFunc("/ws", h.handleWS)
	h.mux = mux
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

	host, port, err := h.lookuper.LookupZone(r.Context(), claims.ZoneID)
	if err != nil {
		http.Error(w, "zone not available", http.StatusServiceUnavailable)
		return
	}

	clientID := claims.PlayerID
	serverID := types.ServerID("")
	_ = serverID

	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		slog.Warn("websocket accept", slog.String("error", err.Error()))
		return
	}

	sess := session.NewSession(clientID, claims.PlayerID, types.ZoneID(claims.ZoneID), serverID)
	h.pool.Add(sess)

	go h.relayWS(c, clientID, host, int(port), claims)
}

func (h *Handler) relayWS(conn *websocket.Conn, clientID, host string, port int, claims *auth.Claims) {
	defer func() {
		h.pool.Remove(clientID)
		conn.CloseNow()
	}()

	ctx := context.Background()
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		_ = data
	}
}
