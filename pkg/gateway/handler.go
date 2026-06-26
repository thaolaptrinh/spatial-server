package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/coder/websocket"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	spatialserverv1 "github.com/thaolaptrinh/spatial-server/proto/gen/spatialserver/v1"

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

	// Dial game-server
	target := fmt.Sprintf("%s:%d", host, port)
	gconn, err := grpc.DialContext(ctx, target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		slog.Warn("dial game-server", slog.String("error", err.Error()))
		return
	}
	defer gconn.Close()

	gc := spatialserverv1.NewGameServerClient(gconn)
	stream, err := gc.Relay(ctx)
	if err != nil {
		slog.Warn("open relay stream", slog.String("error", err.Error()))
		return
	}
	defer stream.CloseSend()

	// Send CONNECT
	connectMeta := &spatialserverv1.ConnectMeta{
		PlayerId:  claims.PlayerID,
		RuntimeId: claims.RuntimeID,
		ZoneId:    claims.ZoneID,
	}
	stream.Send(&spatialserverv1.RelayPacket{
		ClientId: clientID,
		Kind:     spatialserverv1.Kind_KIND_CONNECT,
		Meta:     connectMeta,
	})

	errCh := make(chan error, 2)

	// Pump: WS read -> Relay Send
	go func() {
		for {
			_, data, err := conn.Read(ctx)
			if err != nil {
				errCh <- err
				return
			}
			if err := stream.Send(&spatialserverv1.RelayPacket{
				ClientId: clientID,
				Kind:     spatialserverv1.Kind_KIND_DATA,
				Payload:  data,
			}); err != nil {
				errCh <- err
				return
			}
		}
	}()

	// Pump: Relay Recv -> WS write
	go func() {
		for {
			pkt, err := stream.Recv()
			if err != nil {
				errCh <- err
				return
			}
			if err := conn.Write(ctx, websocket.MessageBinary, pkt.GetPayload()); err != nil {
				errCh <- err
				return
			}
		}
	}()

	// Wait for first error or context cancel
	<-errCh

	// Send DISCONNECT
	_ = stream.Send(&spatialserverv1.RelayPacket{
		ClientId: clientID,
		Kind:     spatialserverv1.Kind_KIND_DISCONNECT,
	})
}
