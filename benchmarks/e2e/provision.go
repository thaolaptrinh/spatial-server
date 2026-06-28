// Package e2e drives the real distributed topology (Client → WebSocket →
// Gateway → gRPC → Runtime Node → AOI → events → gRPC → Gateway → WebSocket →
// Client) for end-to-end benchmarking. It talks to a running Docker Compose
// stack (make dev-up-full) over the host-published ports.
package e2e

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	spatialserverv1 "github.com/thaolaptrinh/spatial-server/proto/gen/spatialserver/v1"
)

// StackAddrs holds the host-published endpoints of the running stack.
type StackAddrs struct {
	Gateway      string // e.g. "127.0.0.1:8080"
	RoomService  string // e.g. "127.0.0.1:9001"
	PostgresDSN  string // e.g. "postgres://spatial:spatial@127.0.0.1:5432/spatial?sslmode=disable"
	JWTSecret    string // HS256 signing secret configured on the gateway
}

// Provision creates a runtime with one zone, seeds the ownership rows the
// gateway needs, and waits until the room-service has assigned the zone to a
// registered runtime node. Returns the zone id clients should target.
func Provision(ctx context.Context, addrs StackAddrs, runtimeID string, zoneCount int) (string, error) {
	conn, err := grpc.NewClient(addrs.RoomService, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return "", fmt.Errorf("dial room-service: %w", err)
	}
	defer conn.Close()

	api := spatialserverv1.NewSpatialServerAPIClient(conn)
	resp, err := api.CreateRuntime(ctx, &spatialserverv1.CreateRuntimeRequest{
		RuntimeId: runtimeID, ZoneCount: int32(zoneCount),
	})
	if err != nil {
		return "", fmt.Errorf("create runtime: %w", err)
	}
	if len(resp.GetZones()) == 0 {
		return "", fmt.Errorf("create runtime returned no zones")
	}
	zoneID := resp.GetZones()[0].GetZoneId()

	// Seed the runtime + zone rows the lookup path reads. ON CONFLICT keeps
	// provisioning idempotent across repeated benchmark runs.
	pool, err := pgxpool.New(ctx, addrs.PostgresDSN)
	if err != nil {
		return "", fmt.Errorf("connect postgres: %w", err)
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx,
		`INSERT INTO runtimes (id, status, zone_count) VALUES ($1, 'active', $2) ON CONFLICT (id) DO NOTHING`,
		runtimeID, zoneCount); err != nil {
		return "", fmt.Errorf("seed runtime: %w", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO zones (id, runtime_id, grid_x, grid_y, status) VALUES ($1, $2, 0, 0, 'unowned') ON CONFLICT (id) DO NOTHING`,
		zoneID, runtimeID); err != nil {
		return "", fmt.Errorf("seed zone: %w", err)
	}

	// Wait for the room-service to assign the zone to a registered node.
	roomCli := spatialserverv1.NewRoomServiceClient(conn)
	lookupCtx, lookupCancel := context.WithTimeout(ctx, 30*time.Second)
	defer lookupCancel()
	if err := waitForAssignment(lookupCtx, roomCli, zoneID); err != nil {
		return "", err
	}
	return zoneID, nil
}

func waitForAssignment(ctx context.Context, roomCli spatialserverv1.RoomServiceClient, zoneID string) error {
	for {
		resp, err := roomCli.LookupZone(ctx, &spatialserverv1.LookupZoneRequest{ZoneId: zoneID})
		if err == nil && (resp.GetAdvertiseAddr() != "" || resp.GetHost() != "") {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("zone %s never assigned: %w", zoneID, ctx.Err())
		case <-time.After(500 * time.Millisecond):
		}
	}
}
