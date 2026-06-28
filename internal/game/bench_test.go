package game

import (
	"fmt"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/thaolaptrinh/spatial-server/internal/game/entity"
	"github.com/thaolaptrinh/spatial-server/internal/game/zone"
	"github.com/thaolaptrinh/spatial-server/internal/types"
	"github.com/thaolaptrinh/spatial-server/pkg/protocol"
	v1 "github.com/thaolaptrinh/spatial-server/proto/gen/spatialserver/v1"
)

// BenchmarkGame_DispatchPositionUpdate measures the inbound hot path: decode a
// position packet, update entity + AOI. This is the per-packet runtime cost.
func BenchmarkGame_DispatchPositionUpdate(b *testing.B) {
	g := New(types.ServerID("gs-1"))
	_ = g.AssignZone(zone.New(types.ZoneID("z1"), types.RuntimeID("s1"), 0, 0, 100))
	e := entity.New(types.EntityID("p1"), "avatar", types.RuntimeID("s1"))
	e.ZoneID = types.ZoneID("z1")
	g.AddEntity(e)

	payload, _ := proto.Marshal(&v1.EntityUpdate{
		EntityId: "p1",
		Position: &v1.Vector3{X: 50, Y: 0, Z: 50},
	})
	frame := protocol.Encode(protocol.PacketIDPositionUpdate, payload, false, 0)
	pkt := InboundPacket{ClientID: "p1", Data: frame}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		g.dispatch(pkt)
	}
}

// BenchmarkGame_Publish measures the outbound event publish path (non-blocking
// channel send + metric). Called per observer per event each tick.
func BenchmarkGame_Publish(b *testing.B) {
	g := New(types.ServerID("gs-1"))
	evt := Event{Kind: EventMove, Space: "s1", Observer: "o1", EntityID: "e1", Position: types.Vector3{X: 1, Z: 2}}
	// drain to keep the channel from filling
	go func() {
		for range g.Events {
		}
	}()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		g.publish(evt)
	}
}

// BenchmarkGame_Tick measures a full single tick at a given entity count: the
// aggregate per-tick runtime cost (dispatch already queued + simulate + AOI).
func BenchmarkGame_Tick(b *testing.B) {
	for _, n := range []int{100, 500, 1000} {
		b.Run(fmt.Sprintf("entities=%d", n), func(b *testing.B) {
			g := New(types.ServerID("gs-1"), WithTickRate(50*time.Millisecond))
			_ = g.AssignZone(zone.New(types.ZoneID("z1"), types.RuntimeID("s1"), 0, 0, 1000))
			for i := 0; i < n; i++ {
				e := entity.New(types.EntityID(fmt.Sprintf("e%d", i)), "avatar", types.RuntimeID("s1"))
				e.ZoneID = types.ZoneID("z1")
				e.Position = types.Vector3{X: float64(i%100) * 5, Z: float64(i/100) * 5}
				g.AddEntity(e)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				g.Tick()
			}
		})
	}
}
