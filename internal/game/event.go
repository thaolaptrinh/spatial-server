package game

import "github.com/thaolaptrinh/spatial-server/internal/types"

// EventKind enumerates runtime events emitted by the simulation core.
type EventKind int

const (
	EventSpawn EventKind = iota
	EventDespawn
	EventMove
	EventState
)

// Event is a runtime-domain event published by the simulation.
//
// The simulation core produces Events and never encodes wire packets itself;
// a downstream adapter translates Events into the client wire format. This
// keeps pkg/protocol and the protobuf wire types out of the simulation core.
//
// Observer is the entity the event is addressed to (the AOI fan-out result).
// EntityID is the subject of the event. Space scopes the event to a single
// Space so downstream routing can enforce isolation.
type Event struct {
	Kind     EventKind
	Space    types.RuntimeID
	Observer types.EntityID
	EntityID types.EntityID
	Type     string
	Position types.Vector3
	// Payload is an opaque, already-encoded synchronization blob for state
	// changes. The simulation does not interpret it; it forwards it verbatim.
	Payload []byte
}

// publish emits an event to the Events channel without blocking the
// simulation tick. If the channel is full the event is dropped, matching
// the non-blocking backpressure strategy used elsewhere in the runtime.
func (g *Game) publish(evt Event) {
	select {
	case g.Events <- evt:
		g.metrics.RuntimeEvent(eventKindLabel(evt.Kind))
	default:
		g.metrics.Dropped("events", 1)
	}
}
