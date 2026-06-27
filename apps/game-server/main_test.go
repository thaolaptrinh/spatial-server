package main

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	"github.com/thaolaptrinh/spatial-server/internal/types"
	"github.com/thaolaptrinh/spatial-server/pkg/game"
	spatialserverv1 "github.com/thaolaptrinh/spatial-server/proto/gen/spatialserver/v1"
)

const bufSize = 1024 * 1024

func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found from", dir)
		}
		dir = parent
	}
}

func buildBinary(t *testing.T, pkg string) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), filepath.Base(pkg))
	cmd := exec.Command("go", "build", "-o", bin, pkg)
	cmd.Dir = moduleRoot(t)
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build %s: %v\n%s", pkg, err, out)
	}
	return bin
}

func TestGameServerBinary_StartAndGracefulShutdown(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("SIGTERM handling differs on Windows")
	}
	bin := buildBinary(t, "./apps/game-server/")
	cmd := exec.Command(bin)
	cmd.Dir = moduleRoot(t)
	cmd.Env = append(os.Environ(), "SPATIAL_GRPC__PORT=9999")

	start := time.Now()
	if err := cmd.Start(); err != nil {
		t.Fatalf("start game-server: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	time.Sleep(200 * time.Millisecond)
	cmd.Process.Signal(os.Interrupt)

	select {
	case err := <-done:
		if err != nil {
			t.Logf("game-server exit: %v", err)
		}
	case <-time.After(5 * time.Second):
		cmd.Process.Kill()
		t.Fatal("game-server did not shut down within 5s")
	}
	t.Logf("game-server started and shut down in %v", time.Since(start))
}

func TestGameServerBinary_PortConflict(t *testing.T) {
	bin := buildBinary(t, "./apps/game-server/")

	// Start first instance on port 9997
	cmd1 := exec.Command(bin)
	cmd1.Dir = moduleRoot(t)
	cmd1.Env = append(os.Environ(), "SPATIAL_GRPC__PORT=9997")
	if err := cmd1.Start(); err != nil {
		t.Fatalf("start first instance: %v", err)
	}
	defer cmd1.Process.Kill()
	time.Sleep(100 * time.Millisecond)

	// Start second instance on same port — should fail
	cmd2 := exec.Command(bin)
	cmd2.Dir = moduleRoot(t)
	cmd2.Env = append(os.Environ(), "SPATIAL_GRPC__PORT=9997")
	out, err := cmd2.CombinedOutput()
	if err == nil {
		t.Fatal("expected port conflict error, got none")
	}
	t.Logf("port conflict error: %s", string(out))

	cmd1.Process.Signal(os.Interrupt)
	cmd1.Wait()
}

func bufDialer(lis *bufconn.Listener) func(context.Context, string) (net.Conn, error) {
	return func(ctx context.Context, s string) (net.Conn, error) {
		return lis.Dial()
	}
}

func newTestServer(t *testing.T) (*spatialserverv1.GameServer_RelayClient, *game.Game, *bufconn.Listener) {
	t.Helper()
	g := game.New(types.ServerID("test-gs"), game.WithTickRate(10*time.Millisecond))
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = g.Run(ctx) }()

	srv := grpc.NewServer()
	gs := newGameServerServer(g)
	spatialserverv1.RegisterGameServerServer(srv, gs)
	lis := bufconn.Listen(bufSize)
	go srv.Serve(lis) //nolint:errcheck
	t.Cleanup(srv.Stop)

	conn, err := grpc.DialContext(context.Background(), "bufnet",
		grpc.WithContextDialer(bufDialer(lis)),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })

	client := spatialserverv1.NewGameServerClient(conn)
	stream, err := client.Relay(context.Background())
	require.NoError(t, err)

	return &stream, g, lis
}

func TestRelay_ConnectCreatesEntity(t *testing.T) {
	streamPtr, g, _ := newTestServer(t)
	stream := *streamPtr

	meta := &spatialserverv1.ConnectMeta{PlayerId: "p1", RuntimeId: "r1", ZoneId: "z1"}
	err := stream.Send(&spatialserverv1.RelayPacket{ClientId: "p1", Kind: spatialserverv1.Kind_KIND_CONNECT, Meta: meta})
	require.NoError(t, err)

	time.Sleep(30 * time.Millisecond)

	assert.Equal(t, 1, g.EntityCount())
}

func TestRelay_DataPacketReachesInbox(t *testing.T) {
	streamPtr, g, _ := newTestServer(t)
	stream := *streamPtr

	err := stream.Send(&spatialserverv1.RelayPacket{
		ClientId: "c1",
		Kind:     spatialserverv1.Kind_KIND_DATA,
		Payload:  []byte{0x01, 0x02, 0x03},
	})
	require.NoError(t, err)

	select {
	case pkt := <-g.Inbox:
		assert.Equal(t, "c1", pkt.ClientID)
		assert.Equal(t, []byte{0x01, 0x02, 0x03}, pkt.Data)
	case <-time.After(time.Second):
		t.Fatal("inbox not populated")
	}
}

func TestRelay_DisconnectRemovesEntity(t *testing.T) {
	streamPtr, g, _ := newTestServer(t)
	stream := *streamPtr

	meta := &spatialserverv1.ConnectMeta{PlayerId: "p2", RuntimeId: "r1", ZoneId: "z1"}
	err := stream.Send(&spatialserverv1.RelayPacket{ClientId: "p2", Kind: spatialserverv1.Kind_KIND_CONNECT, Meta: meta})
	require.NoError(t, err)
	time.Sleep(30 * time.Millisecond)
	assert.Equal(t, 1, g.EntityCount())

	err = stream.Send(&spatialserverv1.RelayPacket{ClientId: "p2", Kind: spatialserverv1.Kind_KIND_DISCONNECT})
	require.NoError(t, err)
	time.Sleep(30 * time.Millisecond)
	assert.Equal(t, 0, g.EntityCount())
}

func TestAssignZoneRPC_CreatesZone(t *testing.T) {
	g := game.New(types.ServerID("gs-1"))
	srv := newGameServerServer(g)

	resp, err := srv.AssignZone(context.Background(), &spatialserverv1.AssignZoneRequest{
		ZoneId:    "z1",
		RuntimeId: "r1",
		GridX:     0,
		GridY:     0,
		ZoneSize:  100,
	})
	require.NoError(t, err)
	require.True(t, resp.GetSuccess())
	require.NotNil(t, g.AOIFor(types.ZoneID("z1")))
}

func TestReleaseZoneRPC_TeardownZone(t *testing.T) {
	g := game.New(types.ServerID("gs-1"))
	srv := newGameServerServer(g)
	_, _ = srv.AssignZone(context.Background(), &spatialserverv1.AssignZoneRequest{ZoneId: "z1", RuntimeId: "r1"})
	require.NotNil(t, g.AOIFor(types.ZoneID("z1")))

	resp, err := srv.ReleaseZone(context.Background(), &spatialserverv1.ReleaseZoneRequest{ZoneId: "z1", RuntimeId: "r1"})
	require.NoError(t, err)
	require.True(t, resp.GetSuccess())
	require.Nil(t, g.AOIFor(types.ZoneID("z1")))
}
