//go:build integration

package integration

import (
	"context"
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/thaolaptrinh/spatial-server/pkg/protocol"
	spatialserverv1 "github.com/thaolaptrinh/spatial-server/proto/gen/spatialserver/v1"
)

func TestEndToEnd_SpawnMoveDespawn(t *testing.T) {
	s := startStack(t)
	defer s.cleanup()

	pgDSN := s.pgDSN
	redisAddr := s.redisAddr

	roomBin := buildService(t, "room-service")
	defer startService(t, "room-service", roomBin,
		"SPATIAL_POSTGRES__DSN="+pgDSN,
		"SPATIAL_REDIS__ADDR="+redisAddr,
		"SPATIAL_GRPC__HOST=127.0.0.1",
		"SPATIAL_GRPC__PORT=19000",
	)()
	waitForGRPC(t, "127.0.0.1:19000", 30*time.Second)

	gameBin := buildService(t, "game-server")
	defer startService(t, "game-server", gameBin,
		"SPATIAL_GRPC__HOST=127.0.0.1",
		"SPATIAL_GRPC__PORT=19001",
		"SPATIAL_ROOM_SERVICE__ADDR=127.0.0.1:19000",
	)()
	waitForGRPC(t, "127.0.0.1:19001", 30*time.Second)

	waitForActiveServer(t, pgDSN, 30*time.Second)

	gwBin := buildService(t, "gateway")
	defer startService(t, "gateway", gwBin,
		"SPATIAL_GATEWAY__WS_PORT=18080",
		"SPATIAL_ROOM_SERVICE__ADDR=127.0.0.1:19000",
	)()
	waitForHTTP(t, "http://127.0.0.1:18080/health", 30*time.Second)

	conn, err := grpc.NewClient("127.0.0.1:19000",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	defer conn.Close()

	apiClient := spatialserverv1.NewSpatialServerAPIClient(conn)
	createResp, err := apiClient.CreateRuntime(context.Background(), &spatialserverv1.CreateRuntimeRequest{
		RuntimeId: "r1",
		ZoneCount: 1,
	})
	require.NoError(t, err)
	require.Len(t, createResp.Zones, 1)
	zoneID := createResp.Zones[0].ZoneId
	t.Logf("created runtime r1, zone=%s", zoneID)

	seedPool, err := pgxpool.New(context.Background(), pgDSN)
	require.NoError(t, err)
	_, err = seedPool.Exec(context.Background(),
		`INSERT INTO runtimes (id, status, zone_count) VALUES ('r1', 'active', 1) ON CONFLICT DO NOTHING`)
	require.NoError(t, err)
	_, err = seedPool.Exec(context.Background(),
		`INSERT INTO zones (id, runtime_id, grid_x, grid_y, status) VALUES ($1, 'r1', 0, 0, 'unowned')`,
		zoneID)
	require.NoError(t, err, "seed zone row")
	seedPool.Close()

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"runtime_id": "r1",
		"player_id":  "p1",
		"zone_id":    zoneID,
	})
	tokenStr, err := token.SignedString([]byte("dev-secret-key-change-in-production"))
	require.NoError(t, err)

	u := url.URL{Scheme: "ws", Host: "127.0.0.1:18080", Path: "/ws", RawQuery: fmt.Sprintf("token=%s", tokenStr)}
	t.Logf("dialing %s", u.String())

	wsConn, _, err := websocket.Dial(context.Background(), u.String(), nil)
	require.NoError(t, err)
	defer wsConn.CloseNow()

	readCtx, readCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer readCancel()
	_, frame, err := wsConn.Read(readCtx)
	require.NoError(t, err, "should read a WS frame from the relay")
	assert.GreaterOrEqual(t, len(frame), 8, "frame should have 8-byte header")

	version, id, payload, compressed, seq, err := protocol.Decode(frame)
	require.NoError(t, err)
	assert.Equal(t, byte(protocol.ProtocolVersionV1), version)
	assert.Equal(t, protocol.PacketIDEntitySpawn, id, "first frame should be SPAWN")
	assert.False(t, compressed)
	assert.NotEmpty(t, payload, "spawn payload should not be empty")
	t.Logf("received SPAWN: id=%d, seq=%d, payload_len=%d", id, seq, len(payload))
}
