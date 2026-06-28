package main

import (
	"github.com/thaolaptrinh/spatial-server/internal/game"
	"github.com/thaolaptrinh/spatial-server/internal/types"
	"github.com/thaolaptrinh/spatial-server/pkg/protocol"
	v1 "github.com/thaolaptrinh/spatial-server/proto/gen/spatialserver/v1"
	"google.golang.org/protobuf/proto"
)

// drainEvents consumes runtime Events published by the simulation core and
// translates them into the client wire format. This adapter is the only
// component that knows about the binary packet protocol; the simulation
// core is wire-format agnostic and depends only on the internal Event model.
func (s *gameServerServer) drainEvents() {
	for evt := range s.game.Events {
		if data := encodeEvent(evt); data != nil {
			s.clients.send(string(evt.Observer), data)
		}
	}
}

func encodeEvent(evt game.Event) []byte {
	switch evt.Kind {
	case game.EventSpawn:
		return encodeSpawnEvent(evt)
	case game.EventDespawn:
		return encodeDespawnEvent(evt.EntityID)
	case game.EventMove:
		return encodeMoveEvent(evt.EntityID, evt.Position)
	case game.EventState:
		return protocol.Encode(protocol.PacketIDEntityState, evt.Payload, false, 0)
	default:
		return nil
	}
}

func encodeSpawnEvent(evt game.Event) []byte {
	b, _ := proto.Marshal(&v1.EntitySnapshot{
		EntityId: string(evt.EntityID),
		Type:     evt.Type,
		Position: &v1.Vector3{X: evt.Position.X, Y: evt.Position.Y, Z: evt.Position.Z},
	})
	return protocol.Encode(protocol.PacketIDEntitySpawn, b, false, 0)
}

func encodeDespawnEvent(id types.EntityID) []byte {
	b, _ := proto.Marshal(&v1.EntityID{Id: string(id)})
	return protocol.Encode(protocol.PacketIDEntityDespawn, b, false, 0)
}

func encodeMoveEvent(id types.EntityID, pos types.Vector3) []byte {
	b, _ := proto.Marshal(&v1.EntityUpdate{
		EntityId: string(id),
		Position: &v1.Vector3{X: pos.X, Y: pos.Y, Z: pos.Z},
	})
	return protocol.Encode(protocol.PacketIDEntityMove, b, false, 0)
}
