package websocket

import (
	"context"
	"net/http"
)

type Connection interface {
	Read(ctx context.Context) ([]byte, error)
	Write(ctx context.Context, data []byte) error
	Close(statusCode uint16, reason string) error
	CloseNow() error
	SetReadLimit(n int64)
}

type Accepter interface {
	Accept(w http.ResponseWriter, r *http.Request) (Connection, error)
}
