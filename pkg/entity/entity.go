package entity

import (
	"github.com/thaolaptrinh/spatial-server/internal/types"
)

type Attributes map[string][]byte

type Lifecycle interface {
	Spawn()
	Despawn()
	OnEnterZone(zoneID types.ZoneID)
	OnLeaveZone(zoneID types.ZoneID)
}

type Entity struct {
	ID        types.EntityID
	Type      string
	Position  types.Vector3
	Rotation  types.Rotation
	Attrs     Attributes
	ZoneID    types.ZoneID
	OwnerID   types.ServerID
	RuntimeID types.RuntimeID
}

func New(id types.EntityID, typ string, runtimeID types.RuntimeID) *Entity {
	return &Entity{
		ID:        id,
		Type:      typ,
		Attrs:     make(Attributes),
		RuntimeID: runtimeID,
	}
}
