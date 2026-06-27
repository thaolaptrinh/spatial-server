package game

import (
	"github.com/thaolaptrinh/spatial-server/internal/types"
	"github.com/thaolaptrinh/spatial-server/internal/game/entity"
	"github.com/thaolaptrinh/spatial-server/pkg/protocol"
	v1 "github.com/thaolaptrinh/spatial-server/proto/gen/spatialserver/v1"
	"google.golang.org/protobuf/proto"
)

func encodeSpawnFrame(e *entity.Entity) []byte {
	b, _ := proto.Marshal(&v1.EntitySnapshot{
		EntityId: string(e.ID), Type: e.Type,
		Position: &v1.Vector3{X: e.Position.X, Y: e.Position.Y, Z: e.Position.Z},
	})
	return protocol.Encode(protocol.PacketIDEntitySpawn, b, false, 0)
}

func encodeDespawnFrame(id types.EntityID) []byte {
	b, _ := proto.Marshal(&v1.EntityID{Id: string(id)})
	return protocol.Encode(protocol.PacketIDEntityDespawn, b, false, 0)
}

func encodeMoveFrame(id types.EntityID, pos types.Vector3) []byte {
	b, _ := proto.Marshal(&v1.EntityUpdate{
		EntityId: string(id), Position: &v1.Vector3{X: pos.X, Y: pos.Y, Z: pos.Z},
	})
	return protocol.Encode(protocol.PacketIDEntityMove, b, false, 0)
}

func encodeStateFrame(id types.EntityID, animation string, health int32) []byte {
	b, _ := proto.Marshal(&v1.EntityState{EntityId: string(id), Animation: animation, Health: health})
	return protocol.Encode(protocol.PacketIDEntityState, b, false, 0)
}
