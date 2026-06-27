package coder

import (
	"context"
	"fmt"
	"net/http"

	ws "github.com/coder/websocket"

	"github.com/thaolaptrinh/spatial-server/internal/transport/websocket"
)

type Conn struct {
	c *ws.Conn
}

type Accepter struct {
	Options *ws.AcceptOptions
}

func (a Accepter) Accept(w http.ResponseWriter, r *http.Request) (websocket.Connection, error) {
	opts := a.Options
	if opts == nil {
		opts = &ws.AcceptOptions{}
	}
	c, err := ws.Accept(w, r, opts)
	if err != nil {
		return nil, fmt.Errorf("websocket accept: %w", err)
	}
	return Conn{c: c}, nil
}

func (c Conn) Read(ctx context.Context) ([]byte, error) {
	_, data, err := c.c.Read(ctx)
	if err != nil {
		return nil, fmt.Errorf("websocket read: %w", err)
	}
	return data, nil
}

func (c Conn) Write(ctx context.Context, data []byte) error {
	err := c.c.Write(ctx, ws.MessageBinary, data)
	if err != nil {
		return fmt.Errorf("websocket write: %w", err)
	}
	return nil
}

func (c Conn) Close(statusCode uint16, reason string) error {
	return c.c.Close(ws.StatusCode(statusCode), reason)
}

func (c Conn) CloseNow() error {
	return c.c.CloseNow()
}

func (c Conn) SetReadLimit(n int64) {
	c.c.SetReadLimit(n)
}
