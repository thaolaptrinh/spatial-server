package room

import (
	"context"

	"github.com/thaolaptrinh/spatial-server/internal/types"
)

type ServerStore interface {
	Register(ctx context.Context, info *NodeDescriptor) error
	Heartbeat(ctx context.Context, id types.ServerID, load NodeLoad) error
	Get(ctx context.Context, id types.ServerID) (*NodeDescriptor, error)
	List(ctx context.Context) ([]*NodeDescriptor, error)
	Remove(ctx context.Context, id types.ServerID) error
}

type ZoneStore interface {
	Claim(ctx context.Context, zoneID string, runtimeID types.RuntimeID, serverID types.ServerID) error
	Release(ctx context.Context, zoneID string, serverID types.ServerID) error
	Lookup(ctx context.Context, zoneID string) (types.ServerID, error)
}
