package entity

import (
	"time"

	"github.com/thaolaptrinh/spatial-server/internal/types"
)

type Attributes map[string][]byte

type Lifecycle interface {
	Spawn()
	Despawn()
	OnEnterZone(zoneID types.ZoneID)
	OnLeaveZone(zoneID types.ZoneID)
	OnSimulate(dt time.Duration)
	OnAction(action string, payload []byte)
}

type BaseLifecycle struct{}

func (BaseLifecycle) Spawn()                      {}
func (BaseLifecycle) Despawn()                    {}
func (BaseLifecycle) OnEnterZone(types.ZoneID)    {}
func (BaseLifecycle) OnLeaveZone(types.ZoneID)    {}
func (BaseLifecycle) OnSimulate(time.Duration)    {}
func (BaseLifecycle) OnAction(string, []byte)     {}

type Entity struct {
	ID        types.EntityID
	Type      string
	Position  types.Vector3
	Rotation  types.Rotation
	Attrs     Attributes
	ZoneID    types.ZoneID
	OwnerID   types.ServerID
	RuntimeID types.RuntimeID
	Behavior  string
	Lifecycle Lifecycle
}

func New(id types.EntityID, typ string, runtimeID types.RuntimeID) *Entity {
	return &Entity{
		ID:        id,
		Type:      typ,
		Attrs:     make(Attributes),
		RuntimeID: runtimeID,
	}
}
