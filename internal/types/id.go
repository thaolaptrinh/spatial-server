package types

import (
	"fmt"

	"github.com/google/uuid"
)

func NewEntityID() EntityID {
	return EntityID(uuid.Must(uuid.NewV7()).String())
}

func NewRuntimeID() RuntimeID {
	return RuntimeID(uuid.Must(uuid.NewV7()).String())
}

func NewZoneID(runtimeID RuntimeID, gridX, gridY int) ZoneID {
	return ZoneID(fmt.Sprintf("%s:%d:%d", runtimeID, gridX, gridY))
}

func NewServerID() ServerID {
	return ServerID(uuid.Must(uuid.NewV7()).String())
}
