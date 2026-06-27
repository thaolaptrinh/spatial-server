package room

import (
	"context"

	"github.com/thaolaptrinh/spatial-server/internal/types"
)

type ServerStore interface {
	Register(ctx context.Context, info *ServerInfo) error
	Heartbeat(ctx context.Context, id types.ServerID) error
	Get(ctx context.Context, id types.ServerID) (*ServerInfo, error)
	LeastLoaded(ctx context.Context) (*ServerInfo, error)
	Remove(ctx context.Context, id types.ServerID) error
}

type ZoneStore interface {
	Claim(ctx context.Context, zoneID string, runtimeID types.RuntimeID, serverID types.ServerID) error
	Release(ctx context.Context, zoneID string, serverID types.ServerID) error
	Lookup(ctx context.Context, zoneID string) (types.ServerID, error)
}
