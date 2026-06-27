package gateway

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	spatialserverv1 "github.com/thaolaptrinh/spatial-server/proto/gen/spatialserver/v1"

	"github.com/thaolaptrinh/spatial-server/internal/auth"
	transportws "github.com/thaolaptrinh/spatial-server/internal/transport/websocket"
)

func (h *Handler) relayWS(conn transportws.Connection, clientID, host string, port int, claims *auth.Claims) {
	defer func() {
		h.pool.Remove(clientID)
		conn.CloseNow()
	}()

	ctx := context.Background()

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

	ip := clientID
	connLimiter := newConnectionLimiter(100, 100, time.Now)
	ipLimiter := h.ipLimiter

	go func() {
		for {
			data, err := conn.Read(ctx)
			if err != nil {
				errCh <- err
				return
			}
			if !connLimiter.allow() {
				continue
			}
			if h.ipLimiter != nil && !ipLimiter.allow(ip) {
				continue
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

	go func() {
		for {
			pkt, err := stream.Recv()
			if err != nil {
				errCh <- err
				return
			}
			writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err = conn.Write(writeCtx, pkt.GetPayload())
			cancel()
			if err != nil {
				errCh <- err
				return
			}
		}
	}()

	<-errCh

	_ = stream.Send(&spatialserverv1.RelayPacket{
		ClientId: clientID,
		Kind:     spatialserverv1.Kind_KIND_PEER_DISCONNECTED,
	})
}
