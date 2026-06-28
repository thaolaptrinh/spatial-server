package e2e

import (
	"context"
	"fmt"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/protobuf/proto"

	"github.com/thaolaptrinh/spatial-server/benchmarks/framework"
	"github.com/thaolaptrinh/spatial-server/pkg/protocol"
	v1 "github.com/thaolaptrinh/spatial-server/proto/gen/spatialserver/v1"
)

// Config configures an e2e benchmark run against a live stack.
type Config struct {
	Clients      int
	Duration     time.Duration
	SendInterval time.Duration // per-client position-update interval
	Addrs        StackAddrs
	RuntimeID    string
	ZoneCount    int
}

// Result holds measured e2e metrics over the full distributed data path.
type Result struct {
	Clients       int
	Duration      time.Duration
	ConnectP50    time.Duration
	ConnectP95    time.Duration
	RoundTripP50  time.Duration
	RoundTripP95  time.Duration
	RoundTripP99  time.Duration
	RoundTripMax  time.Duration
	TotalFrames   int64
	TotalSends    int64
	FramesPerSec  float64
	SendsPerSec   float64
}

// Run provisions a runtime and drives N WebSocket clients through the gateway,
// measuring connect latency, downstream frame throughput, and the true
// cross-client send→receive round-trip (client A sends a position update,
// client B receives the resulting EntityMove through the entire pipeline).
func Run(ctx context.Context, cfg Config) (*Result, error) {
	zoneID, err := Provision(ctx, cfg.Addrs, cfg.RuntimeID, cfg.ZoneCount)
	if err != nil {
		return nil, err
	}

	// Shared map of entityID → last send time, used by receivers to compute
	// send→receive latency for moves authored by other clients.
	sendTimes := &sync.Map{}

	connectHist := framework.NewHistogram()
	rtHist := framework.NewHistogram()
	var framesRecv, sends atomic.Int64

	tokenFn := func(playerID string) string {
		t, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"runtime_id": cfg.RuntimeID, "player_id": playerID, "zone_id": zoneID,
		}).SignedString([]byte(cfg.Addrs.JWTSecret))
		return t
	}

	var wg sync.WaitGroup
	runCtx, cancel := context.WithTimeout(ctx, cfg.Duration)
	defer cancel()

	for i := 0; i < cfg.Clients; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			runClient(runCtx, cfg, tokenFn(fmt.Sprintf("p%d", i)), zoneID, sendTimes, connectHist, rtHist, &framesRecv, &sends)
		}()
	}
	wg.Wait()

	res := &Result{
		Clients:      cfg.Clients,
		Duration:     cfg.Duration,
		ConnectP50:   time.Duration(connectHist.Percentile(50)),
		ConnectP95:   time.Duration(connectHist.Percentile(95)),
		RoundTripP50: time.Duration(rtHist.Percentile(50)),
		RoundTripP95: time.Duration(rtHist.Percentile(95)),
		RoundTripP99: time.Duration(rtHist.Percentile(99)),
		RoundTripMax: time.Duration(rtHist.Max()),
		TotalFrames:  framesRecv.Load(),
		TotalSends:   sends.Load(),
	}
	secs := cfg.Duration.Seconds()
	if secs > 0 {
		res.FramesPerSec = float64(framesRecv.Load()) / secs
		res.SendsPerSec = float64(sends.Load()) / secs
	}
	return res, nil
}

func runClient(ctx context.Context, cfg Config, playerID, zoneID string,
	sendTimes *sync.Map, connectHist, rtHist *framework.Histogram,
	framesRecv, sends *atomic.Int64) {

	u := url.URL{Scheme: "ws", Host: cfg.Addrs.Gateway, Path: "/ws", RawQuery: "token=" + tokenStr(cfg.Addrs.JWTSecret, cfg.RuntimeID, zoneID, playerID)}

	connectStart := time.Now()
	conn, _, err := websocket.Dial(ctx, u.String(), nil)
	if err != nil {
		return
	}
	defer conn.CloseNow()
	conn.SetReadLimit(1 << 20)

	// Read loop: record connect latency on first frame, then per-move round-trip.
	first := true
	go func() {
		for {
			_, frame, err := conn.Read(ctx)
			if err != nil {
				return
			}
			if first {
				first = false
				connectHist.Observe(float64(time.Since(connectStart).Nanoseconds()))
			}
			framesRecv.Add(1)
			_, pid, payload, _, _, err := protocol.Decode(frame)
			if err != nil {
				continue
			}
			if pid == protocol.PacketIDEntityMove {
				var upd v1.EntityUpdate
				if proto.Unmarshal(payload, &upd) != nil {
					continue
				}
				if v, ok := sendTimes.Load(upd.GetEntityId()); ok {
					rtHist.Observe(float64(time.Since(v.(time.Time)).Nanoseconds()))
				}
			}
		}
	}()

	// Send loop: emit position updates, stamping sendTimes[playerID] each send.
	x := 50.0
	ticker := time.NewTicker(cfg.SendInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			x += 1
			payload, _ := proto.Marshal(&v1.EntityUpdate{
				EntityId: playerID,
				Position: &v1.Vector3{X: x, Y: 0, Z: 50},
			})
			frame := protocol.Encode(protocol.PacketIDPositionUpdate, payload, false, 0)
			sendTimes.Store(playerID, time.Now())
			wctx, wcancel := context.WithTimeout(ctx, time.Second)
			if conn.Write(wctx, websocket.MessageBinary, frame) != nil {
				wcancel()
				return
			}
			wcancel()
			sends.Add(1)
		}
	}
}

func tokenStr(secret, runtimeID, zoneID, playerID string) string {
	t, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"runtime_id": runtimeID, "player_id": playerID, "zone_id": zoneID,
	}).SignedString([]byte(secret))
	return t
}
