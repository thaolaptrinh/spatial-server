package framework

import (
	"context"
	"fmt"
	"time"

	"github.com/coder/websocket"
	"google.golang.org/protobuf/proto"

	"github.com/thaolaptrinh/spatial-server/pkg/protocol"
	v1 "github.com/thaolaptrinh/spatial-server/proto/gen/spatialserver/v1"
)

type Client struct {
	conn      *websocket.Conn
	latencies *Histogram
	PlayerID  string
}

func NewClient(ctx context.Context, addr, token string, latencies *Histogram) (*Client, error) {
	conn, _, err := websocket.Dial(ctx, fmt.Sprintf("ws://%s/ws?token=%s", addr, token), nil)
	if err != nil {
		return nil, fmt.Errorf("dial: %w", err)
	}
	return &Client{conn: conn, latencies: latencies, PlayerID: "bench"}, nil
}

func (c *Client) Run(ctx context.Context) error {
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				payload, _ := proto.Marshal(&v1.EntityUpdate{
					EntityId: c.PlayerID,
					Position: &v1.Vector3{X: 100, Y: 0, Z: 100},
					Timestamp: time.Now().UnixMilli(),
				})
				c.conn.Write(ctx, websocket.MessageBinary, protocol.Encode(protocol.PacketIDPositionUpdate, payload, false, 0))
			}
		}
	}()

	for {
		_, msg, err := c.conn.Read(ctx)
		if err != nil {
			return fmt.Errorf("read: %w", err)
		}
		t0 := time.Now()
		_, _, _, _, _, err = protocol.Decode(msg)
		if err != nil {
			continue
		}
		c.latencies.Observe(float64(time.Since(t0).Microseconds()))
	}
}
