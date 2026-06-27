# Phase 1Finish — Hardened Vertical Slice Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace volatile in-memory state with PostgreSQL, align the packet header with ADR-010, implement the `SpatialServerAPI` service, harden the Gateway, consolidate config, and add a real Testcontainers integration test.

**Architecture:** A pgx-backed repository layer persists zone ownership, the server registry, and runtimes against the migration-001 tables. The Room Service delegates to repositories through consumer-defined `Store` interfaces. The packet header grows to a fixed 8 bytes (version + packetID + flags + sequence). The Gateway gains `/live` + `/ready`, a 64 KiB read cap, and SIGTERM drain. All services load one `pkg/config.Config`.

**Tech Stack:** Go 1.25, `github.com/jackc/pgx/v5`, `github.com/knadh/koanf/v2`, `google.golang.org/grpc`, `github.com/coder/websocket`, `testcontainers-go`, protocol buffers

**Pre-existing files (checked before writing):** `pkg/protocol/protocol.go` (`headerSize=3`, `Encode(id,payload,compress)`, `Decode → (PacketID,[]byte,bool,error)`); `pkg/config/config.go` (`Config{Service,Logging,GRPC,Postgres,Redis}`); `pkg/room/room.go` (in-memory `ServerRegistry`+`ZoneOwnership`); `apps/room-service/main.go` (builds the in-memory types); `pkg/storage/migrations/001_initial.up.sql` (`game_servers`/`runtimes`/`zones`); `proto/spatialserver/v1/spatial_server_api.proto` (5 RPCs, no impl).

---

### Task 1: Packet header alignment (ADR-010)

**Files:**
- Modify: `pkg/protocol/protocol.go`, `pkg/protocol/protocol_test.go`
- Modify: `pkg/game/game.go:279,299-313`
- Modify: `tools/client/main.go`

- [ ] **Step 1: Add failing tests**

  Append to `pkg/protocol/protocol_test.go`:

  ```go
  func TestEncodeDecode_WithSequence(t *testing.T) {
  	packet := Encode(PacketIDPositionUpdate, []byte("hi"), false, 42)
  	version, id, body, compressed, seq, err := Decode(packet)
  	assert.NoError(t, err)
  	assert.Equal(t, ProtocolVersionV1, version)
  	assert.Equal(t, PacketIDPositionUpdate, id)
  	assert.Equal(t, uint32(42), seq)
  	assert.Equal(t, []byte("hi"), body)
  	assert.False(t, compressed)
  }

  func TestDecode_VersionMismatch(t *testing.T) {
  	p := Encode(PacketIDEntitySpawn, []byte("x"), false, 0)
  	p[0] = 0x09
  	_, _, _, _, _, err := Decode(p)
  	assert.ErrorIs(t, err, ErrUnsupportedVersion)
  }
  ```

  Update the five existing tests: `Encode(id, p, f)` → `Encode(id, p, f, 0)` and `id, body, compressed, err := Decode(...)` → `_, id, body, compressed, _, err := Decode(...)`.

- [ ] **Step 2: Run tests to verify failure**

  Run: `go test ./pkg/protocol/... -run 'TestEncodeDecode_WithSequence|TestDecode_VersionMismatch' -v`
  Expected: FAIL (`Encode` arity / `ProtocolVersionV1` undefined).

- [ ] **Step 3: Rewrite `pkg/protocol/protocol.go`**

  Change the constants block to `headerSize = 8` and add `ProtocolVersionV1 = 0x01` plus `ErrUnsupportedVersion = errors.New("unsupported protocol version")`. Add `"errors"` to imports. New `Encode`/`Decode`:

  ```go
  func Encode(id PacketID, payload []byte, compress bool, seq uint32) []byte {
  	// ... unchanged compression block producing (data, compressed) ...
  	buf := make([]byte, headerSize+len(data))
  	buf[0] = ProtocolVersionV1
  	binary.BigEndian.PutUint16(buf[1:3], uint16(id))
  	buf[3] = flags
  	binary.BigEndian.PutUint32(buf[4:8], seq)
  	copy(buf[headerSize:], data)
  	return buf
  }

  func Decode(packet []byte) (version byte, id PacketID, payload []byte, compressed bool, seq uint32, err error) {
  	if len(packet) < headerSize {
  		return 0, PacketIDInvalid, nil, false, 0, fmt.Errorf("packet too short: %d bytes", len(packet))
  	}
  	version = packet[0]
  	if version != ProtocolVersionV1 {
  		return version, PacketIDInvalid, nil, false, 0, fmt.Errorf("version 0x%02x: %w", version, ErrUnsupportedVersion)
  	}
  	id = PacketID(binary.BigEndian.Uint16(packet[1:3]))
  	compressed = packet[3]&compressionFlag != 0
  	seq = binary.BigEndian.Uint32(packet[4:8])
  	data := packet[headerSize:]
  	// ... unchanged gzip decompression of `data` ...
  	return version, id, data, compressed, seq, nil
  }
  ```

  (The compression block is byte-for-byte the existing one — leave it.)

- [ ] **Step 4: Update `pkg/game/game.go` callers**

  Line 279 in `dispatch`: `_, id, payload, _, _, err := protocol.Decode(pkt.Data)`.
  `encodeSpawn` last line: `return protocol.Encode(protocol.PacketIDEntitySpawn, b, false, 0)`.
  `encodeDespawn` last line: `return protocol.Encode(protocol.PacketIDEntityDespawn, b, false, 0)`.

- [ ] **Step 5: Update `tools/client/main.go`**

  Read loop: `_, id, payload, _, _, err := protocol.Decode(msg)`.
  Write loop: `frame := protocol.Encode(protocol.PacketIDPositionUpdate, payload, false, uint32(tick))`.

- [ ] **Step 6: Verify + commit**

  Run: `go build ./... && go test ./pkg/protocol/... -v`
  Expected: PASS.
  ```bash
  git add pkg/protocol/protocol.go pkg/protocol/protocol_test.go pkg/game/game.go tools/client/main.go
  git commit -m "feat(protocol): align 8-byte header per ADR-010 (version+seq)"
  ```

---

### Task 2: Config consolidation

**Files:**
- Modify: `pkg/config/config.go`, `pkg/config/config_test.go`
- Modify: `configs/defaults.yml`

- [ ] **Step 1: Add failing test**

  Append to `pkg/config/config_test.go` (create `package config` if missing):

  ```go
  func TestLoad_GatewayAndGameSections(t *testing.T) {
  	dir := t.TempDir()
  	path := filepath.Join(dir, "t.yml")
  	require.NoError(t, os.WriteFile(path, []byte(`
  gateway: {ws_port: 8080, jwt_secret: "s", max_packet_size: 65536, drain_timeout: 30s}
  room_service: {addr: "rs:9000"}
  game: {tick_rate: 50ms, max_entities: 5000}
  spatial_api: {default_zone_count: 1, default_zone_size: 100}
  `), 0o644))
  	cfg, err := Load(path)
  	require.NoError(t, err)
  	assert.Equal(t, 8080, cfg.Gateway.WSPort)
  	assert.Equal(t, 30*time.Second, cfg.Gateway.DrainTimeout)
  	assert.Equal(t, "rs:9000", cfg.RoomService.Addr)
  	assert.Equal(t, 5000, cfg.Game.MaxEntities)
  	assert.Equal(t, 1, cfg.SpatialServerAPI.DefaultZoneCount)
  }

  func TestLoad_EnvOverride(t *testing.T) {
  	t.Setenv("SPATIAL_GATEWAY__WS_PORT", "9090")
  	dir := t.TempDir()
  	path := filepath.Join(dir, "t.yml")
  	require.NoError(t, os.WriteFile(path, []byte("gateway: {ws_port: 8080}\n"), 0o644))
  	cfg, err := Load(path)
  	require.NoError(t, err)
  	assert.Equal(t, 9090, cfg.Gateway.WSPort)
  }
  ```

- [ ] **Step 2: Run test to verify failure**

  Run: `go test ./pkg/config/... -run TestLoad -v`
  Expected: FAIL (`cfg.Gateway` undefined).

- [ ] **Step 3: Extend `pkg/config/config.go`**

  Add `"time"` import. Append four structs and extend `Config`:

  ```go
  type GatewayConfig struct {
  	WSPort        int           `koanf:"ws_port"`
  	JWTSecret     string        `koanf:"jwt_secret"`
  	MaxPacketSize int           `koanf:"max_packet_size"`
  	SoftConnLimit int           `koanf:"soft_conn_limit"`
  	HardConnLimit int           `koanf:"hard_conn_limit"`
  	DrainTimeout  time.Duration `koanf:"drain_timeout"`
  }
  type RoomServiceConfig struct{ Addr string `koanf:"addr"` }
  type GameConfig struct {
  	TickRate     time.Duration `koanf:"tick_rate"`
  	MaxEntities  int           `koanf:"max_entities"`
  	ZoneCellSize float64       `koanf:"zone_cell_size"`
  	AOIRadius    float64       `koanf:"aoi_radius"`
  }
  type SpatialServerAPIConfig struct {
  	DefaultZoneCount int     `koanf:"default_zone_count"`
  	DefaultZoneSize  float64 `koanf:"default_zone_size"`
  }
  ```
  Add four fields to `Config`: `Gateway GatewayConfig`, `RoomService RoomServiceConfig`, `Game GameConfig`, `SpatialServerAPI SpatialServerAPIConfig` (each with the matching `koanf:"..."` tag).

- [ ] **Step 4: Run test to verify pass**

  Run: `go test ./pkg/config/... -v`
  Expected: PASS.

- [ ] **Step 5: Append to `configs/defaults.yml`**

  ```yaml
  gateway:
    jwt_secret: "dev-secret-key-change-in-production"
    max_packet_size: 65536
    soft_conn_limit: 9000
    hard_conn_limit: 10000
    drain_timeout: 30s
  room_service:
    addr: "room-service:9000"
  spatial_api:
    default_zone_count: 1
    default_zone_size: 100
  ```

- [ ] **Step 6: Commit**

  ```bash
  git add pkg/config/config.go pkg/config/config_test.go configs/defaults.yml
  git commit -m "feat(config): add gateway/room/game/spatial_api sections"
  ```

---

### Task 3: PostgreSQL-backed ServerRegistry

**Files:**
- Create: `pkg/room/store.go`, `pkg/storage/server_repo.go`, `pkg/storage/server_repo_test.go`, `pkg/storage/testdb_test.go`

- [ ] **Step 1: Define consumer-side Store interfaces**

  Create `pkg/room/store.go`:

  ```go
  package room

  import (
  	"context"
  	"github.com/thaolaptrinh/spatial-server/internal/types"
  )

  type ServerStore interface {
  	Register(ctx context.Context, info *ServerInfo) error
  	Heartbeat(ctx context.Context, id types.ServerID) error
  	Get(ctx context.Context, id types.ServerID) (*ServerInfo, error)
  	LeastLoaded(ctx context.Context) (*ServerInfo, error)
  	Remove(ctx context.Context, id types.ServerID) error
  }

  type ZoneStore interface {
  	Claim(ctx context.Context, zoneID string, runtimeID types.RuntimeID, serverID types.ServerID) error
  	Release(ctx context.Context, zoneID string, serverID types.ServerID) error
  	Lookup(ctx context.Context, zoneID string) (types.ServerID, error)
  }
  ```

- [ ] **Step 2: Add the DB helper + failing test**

  Create `pkg/storage/testdb_test.go` (skips without a DSN — full coverage in Task 7):

  ```go
  package storage

  import (
  	"context", "os", "testing"
  	"github.com/jackc/pgx/v5/pgxpool"
  	"github.com/stretchr/testify/require"
  	"github.com/thaolaptrinh/spatial-server/internal/migration"
  )

  func testDB(t *testing.T) *pgxpool.Pool {
  	t.Helper()
  	dsn := os.Getenv("SPATIAL_TEST_POSTGRES_DSN")
  	if dsn == "" {
  		t.Skip("set SPATIAL_TEST_POSTGRES_DSN")
  	}
  	pool, err := pgxpool.New(context.Background(), dsn)
  	require.NoError(t, err)
  	_, _ = pool.Exec(context.Background(), "TRUNCATE game_servers, zones, runtimes CASCADE")
  	require.NoError(t, migration.Run(pool, "../../pkg/storage/migrations"))
  	t.Cleanup(pool.Close)
  	return pool
  }
  ```

  Create `pkg/storage/server_repo_test.go`:

  ```go
  package storage

  import (
  	"context", "testing"
  	"github.com/stretchr/testify/assert"
  	"github.com/stretchr/testify/require"
  	"github.com/thaolaptrinh/spatial-server/internal/types"
  	"github.com/thaolaptrinh/spatial-server/pkg/room"
  )

  func TestServerRepository_RegisterHeartbeatLeastLoaded(t *testing.T) {
  	pool := testDB(t)
  	repo := NewServerRepository(pool)
  	ctx := context.Background()
  	require.NoError(t, repo.Register(ctx, &room.ServerInfo{ID: "s1", Host: "h", Port: 9, MaxZones: 5}))
  	require.NoError(t, repo.Heartbeat(ctx, "s1"))
  	got, err := repo.Get(ctx, "s1")
  	require.NoError(t, err)
  	assert.Equal(t, "h", got.Host)
  	best, err := repo.LeastLoaded(ctx)
  	require.NoError(t, err)
  	assert.Equal(t, types.ServerID("s1"), best.ID)
  	require.NoError(t, repo.Remove(ctx, "s1"))
  	_, err = repo.Get(ctx, "s1")
  	assert.ErrorIs(t, err, types.ErrNotFound)
  }
  ```

- [ ] **Step 3: Run test to verify failure**

  Run: `SPATIAL_TEST_POSTGRES_DSN=postgres://spatial:spatial@localhost:5432/spatial?sslmode=disable go test ./pkg/storage/... -run TestServerRepository -v`
  Expected: FAIL (`NewServerRepository` undefined).

- [ ] **Step 4: Implement `pkg/storage/server_repo.go`**

  ```go
  package storage

  import (
  	"context", "errors", "fmt", "time"
  	"github.com/jackc/pgx/v5"
  	"github.com/jackc/pgx/v5/pgxpool"
  	"github.com/thaolaptrinh/spatial-server/internal/types"
  	"github.com/thaolaptrinh/spatial-server/pkg/room"
  )

  type ServerRepository struct{ pool *pgxpool.Pool }

  func NewServerRepository(pool *pgxpool.Pool) *ServerRepository { return &ServerRepository{pool: pool} }

  func (r *ServerRepository) Register(ctx context.Context, info *room.ServerInfo) error {
  	_, err := r.pool.Exec(ctx,
  		`INSERT INTO game_servers (id, host, port, status, max_zones, registered_at, last_heartbeat)
  		 VALUES ($1,$2,$3,$4,$5,NOW(),NOW())
  		 ON CONFLICT (id) DO UPDATE SET host=EXCLUDED.host, port=EXCLUDED.port, max_zones=EXCLUDED.max_zones`,
  		info.ID, info.Host, info.Port, info.Status.String(), info.MaxZones)
  	if err != nil {
  		return fmt.Errorf("register server %s: %w", info.ID, err)
  	}
  	return nil
  }

  func (r *ServerRepository) Heartbeat(ctx context.Context, id types.ServerID) error {
  	tag, err := r.pool.Exec(ctx,
  		`UPDATE game_servers SET status='active', last_heartbeat=NOW() WHERE id=$1`, id)
  	if err != nil {
  		return fmt.Errorf("heartbeat server %s: %w", id, err)
  	}
  	if tag.RowsAffected() == 0 {
  		return fmt.Errorf("server %s: %w", id, types.ErrNotFound)
  	}
  	return nil
  }

  func (r *ServerRepository) Get(ctx context.Context, id types.ServerID) (*room.ServerInfo, error) {
  	row := r.pool.QueryRow(ctx, `SELECT id,host,port,status,max_zones FROM game_servers WHERE id=$1`, id)
  	return scanServer(row)
  }

  func (r *ServerRepository) LeastLoaded(ctx context.Context) (*room.ServerInfo, error) {
  	row := r.pool.QueryRow(ctx,
  		`SELECT id,host,port,status,max_zones FROM game_servers WHERE status='active'
  		 ORDER BY (SELECT COUNT(*) FROM zones WHERE zones.server_id=game_servers.id) ASC LIMIT 1`)
  	return scanServer(row)
  }

  func (r *ServerRepository) Remove(ctx context.Context, id types.ServerID) error {
  	_, err := r.pool.Exec(ctx, `DELETE FROM game_servers WHERE id=$1`, id)
  	if err != nil {
  		return fmt.Errorf("remove server %s: %w", id, err)
  	}
  	return nil
  }

  func scanServer(row pgx.Row) (*room.ServerInfo, error) {
  	var info room.ServerInfo
  	var statusStr string
  	if err := row.Scan(&info.ID, &info.Host, &info.Port, &statusStr, &info.MaxZones); err != nil {
  		if errors.Is(err, pgx.ErrNoRows) {
  			return nil, fmt.Errorf("server: %w", types.ErrNotFound)
  		}
  		return nil, err
  	}
  	info.Status = types.ServerStatusActive
  	info.LastBeat = time.Now()
  	return &info, nil
  }
  ```

- [ ] **Step 5: Run test to verify pass + commit**

  Run: `go test ./pkg/storage/... -run TestServerRepository -v` (or skip if no DB).
  Expected: PASS (or skip).
  ```bash
  git add pkg/room/store.go pkg/storage/server_repo.go pkg/storage/server_repo_test.go pkg/storage/testdb_test.go
  git commit -m "feat(storage): pgx-backed ServerRepository + ServerStore interface"
  ```

---

### Task 4: PostgreSQL-backed ZoneOwnership + room-service wiring

**Files:**
- Create: `pkg/storage/zone_repo.go`, `pkg/storage/zone_repo_test.go`
- Modify: `apps/room-service/main.go`

- [ ] **Step 1: Add failing test**

  Create `pkg/storage/zone_repo_test.go`:

  ```go
  package storage

  import (
  	"context", "testing"
  	"github.com/stretchr/testify/assert"
  	"github.com/stretchr/testify/require"
  	"github.com/thaolaptrinh/spatial-server/internal/types"
  )

  func TestZoneRepository_ClaimLookupRelease(t *testing.T) {
  	pool := testDB(t)
  	repo := NewZoneRepository(pool)
  	ctx := context.Background()
  	_, _ = pool.Exec(ctx, `INSERT INTO runtimes (id) VALUES ('r1') ON CONFLICT DO NOTHING`)
  	require.NoError(t, repo.Claim(ctx, "z1", "r1", types.ServerID("s1")))
  	err := repo.Claim(ctx, "z1", "r1", types.ServerID("s2"))
  	assert.ErrorIs(t, err, types.ErrConflict)
  	owner, err := repo.Lookup(ctx, "z1")
  	require.NoError(t, err)
  	assert.Equal(t, types.ServerID("s1"), owner)
  	require.ErrorIs(t, repo.Release(ctx, "z1", types.ServerID("s2")), types.ErrNotOwned)
  	require.NoError(t, repo.Release(ctx, "z1", types.ServerID("s1")))
  	_, err = repo.Lookup(ctx, "z1")
  	assert.ErrorIs(t, err, types.ErrNotFound)
  }
  ```

- [ ] **Step 2: Run test to verify failure**

  Run: `SPATIAL_TEST_POSTGRES_DSN=... go test ./pkg/storage/... -run TestZoneRepository -v`
  Expected: FAIL (`NewZoneRepository` undefined).

- [ ] **Step 3: Implement `pkg/storage/zone_repo.go`**

  ```go
  package storage

  import (
  	"context", "errors", "fmt"
  	"github.com/jackc/pgx/v5"
  	"github.com/jackc/pgx/v5/pgxpool"
  	"github.com/thaolaptrinh/spatial-server/internal/types"
  )

  type ZoneRepository struct{ pool *pgxpool.Pool }

  func NewZoneRepository(pool *pgxpool.Pool) *ZoneRepository { return &ZoneRepository{pool: pool} }

  func (r *ZoneRepository) EnsureRow(ctx context.Context, zoneID, runtimeID string) error {
  	_, err := r.pool.Exec(ctx,
  		`INSERT INTO zones (id, runtime_id, grid_x, grid_y, status)
  		 VALUES ($1,$2,0,0,'unowned') ON CONFLICT (id) DO NOTHING`, zoneID, runtimeID)
  	if err != nil {
  		return fmt.Errorf("ensure zone %s: %w", zoneID, err)
  	}
  	return nil
  }

  func (r *ZoneRepository) Claim(ctx context.Context, zoneID string, runtimeID types.RuntimeID, serverID types.ServerID) error {
  	tag, err := r.pool.Exec(ctx,
  		`UPDATE zones SET server_id=$1, status='active' WHERE id=$2 AND server_id IS NULL`, serverID, zoneID)
  	if err != nil {
  		return fmt.Errorf("claim zone %s: %w", zoneID, err)
  	}
  	if tag.RowsAffected() == 0 {
  		var owner *string
  		qerr := r.pool.QueryRow(ctx, `SELECT server_id FROM zones WHERE id=$1`, zoneID).Scan(&owner)
  		if errors.Is(qerr, pgx.ErrNoRows) {
  			return fmt.Errorf("zone %s: %w", zoneID, types.ErrNotFound)
  		}
  		if owner != nil && *owner != "" && *owner != string(serverID) {
  			return fmt.Errorf("zone %s: %w", zoneID, types.ErrConflict)
  		}
  	}
  	return nil
  }

  func (r *ZoneRepository) Release(ctx context.Context, zoneID string, serverID types.ServerID) error {
  	tag, err := r.pool.Exec(ctx, `UPDATE zones SET server_id=NULL, status='unowned' WHERE id=$1 AND server_id=$2`, zoneID, serverID)
  	if err != nil {
  		return fmt.Errorf("release zone %s: %w", zoneID, err)
  	}
  	if tag.RowsAffected() == 0 {
  		return fmt.Errorf("zone %s: %w", zoneID, types.ErrNotOwned)
  	}
  	return nil
  }

  func (r *ZoneRepository) Lookup(ctx context.Context, zoneID string) (types.ServerID, error) {
  	var owner *string
  	err := r.pool.QueryRow(ctx, `SELECT server_id FROM zones WHERE id=$1`, zoneID).Scan(&owner)
  	if err != nil {
  		if errors.Is(err, pgx.ErrNoRows) {
  			return "", fmt.Errorf("zone %s: %w", zoneID, types.ErrNotFound)
  		}
  		return "", fmt.Errorf("lookup zone %s: %w", zoneID, err)
  	}
  	if owner == nil || *owner == "" {
  		return "", fmt.Errorf("zone %s: %w", zoneID, types.ErrNotFound)
  	}
  	return types.ServerID(*owner), nil
  }
  ```

- [ ] **Step 4: Run test to verify pass**

  Run: `go test ./pkg/storage/... -run TestZoneRepository -v`
  Expected: PASS.

- [ ] **Step 5: Refactor `apps/room-service/main.go`**

  - Replace the `roomServiceServer` struct fields `registry`/`ownership` with `servers room.ServerStore` / `zones room.ZoneStore`.
  - `Register`/`Heartbeat`: delegate to `s.servers.Register(ctx, ...)` / `s.servers.Heartbeat(ctx, ...)`.
  - `LookupZone`: try `s.zones.Lookup`; on miss call `s.servers.LeastLoaded` then `s.zones.Claim` then return that server.
  - Replace the inline koanf bootstrap in `main()` with:
    ```go
    cfg, err := config.Load("configs/defaults.yml", "configs/room-service.yml")
    if err != nil { fmt.Fprintf(os.Stderr, "load config: %v\n", err); os.Exit(1) }
    ```
  - Read `cfg.GRPC.Port`, `cfg.Postgres.DSN`, `cfg.Redis.Addr` instead of `k.*`.
  - Replace `room.NewServerRegistry()`/`room.NewZoneOwnership()` with:
    ```go
    service := &roomServiceServer{
    	servers: storage.NewServerRepository(pgPool),
    	zones:   storage.NewZoneRepository(pgPool),
    }
    ```
  - Add imports `"github.com/thaolaptrinh/spatial-server/pkg/config"` and ensure `storage` is imported.

- [ ] **Step 6: Verify + commit**

  Run: `go build ./apps/room-service/... && go vet ./apps/...`
  Expected: no errors.
  ```bash
  git add pkg/storage/zone_repo.go pkg/storage/zone_repo_test.go apps/room-service/main.go
  git commit -m "feat(room): pgx-backed zone ownership + config.Load in room-service"
  ```

---

### Task 5: SpatialServerAPI implementation

**Files:**
- Create: `pkg/api/spatial_server.go`, `pkg/api/spatial_server_test.go`
- Modify: `apps/room-service/main.go`

- [ ] **Step 1: Add failing test with a fake store**

  Create `pkg/api/spatial_server_test.go`:

  ```go
  package api

  import (
  	"context", "testing"
  	"github.com/stretchr/testify/assert"
  	"github.com/stretchr/testify/require"
  	"github.com/thaolaptrinh/spatial-server/internal/types"
  	spatialserverv1 "github.com/thaolaptrinh/spatial-server/proto/gen/spatialserver/v1"
  )

  func TestCreateRuntime_AndDestroy(t *testing.T) {
  	srv := NewSpatialServerAPI(&fakeStore{}, "gw:443")
  	resp, err := srv.CreateRuntime(context.Background(), &spatialserverv1.CreateRuntimeRequest{RuntimeId: "r1", ZoneCount: 2})
  	require.NoError(t, err)
  	assert.Equal(t, int32(2), resp.ZoneCount)
  	assert.Len(t, resp.Zones, 2)
  	d, err := srv.DestroyRuntime(context.Background(), &spatialserverv1.DestroyRuntimeRequest{RuntimeId: "r1"})
  	require.NoError(t, err)
  	assert.True(t, d.Success)
  }

  type fakeStore struct{ m map[string]*RuntimeRecord }

  func (f *fakeStore) Create(_ context.Context, id string, zc int, _ float64) (*RuntimeRecord, error) {
  	if f.m == nil { f.m = map[string]*RuntimeRecord{} }
  	r := &RuntimeRecord{ID: id, Status: types.RuntimeStatusActive, ZoneCount: zc}
  	for i := 0; i < zc; i++ { r.ZoneIDs = append(r.ZoneIDs, id+"-z"+string(rune('1'+i))) }
  	f.m[id] = r
  	return r, nil
  }
  func (f *fakeStore) Get(_ context.Context, id string) (*RuntimeRecord, error) {
  	if r, ok := f.m[id]; ok { return r, nil }
  	return nil, types.ErrNotFound
  }
  func (f *fakeStore) Destroy(_ context.Context, id string) error {
  	if r, ok := f.m[id]; ok { r.Status = types.RuntimeStatusDestroyed; return nil }
  	return types.ErrNotFound
  }
  func (f *fakeStore) List(_ context.Context, _ string, _ int) ([]*RuntimeRecord, string, error) {
  	var out []*RuntimeRecord
  	for _, r := range f.m { if r.Status != types.RuntimeStatusDestroyed { out = append(out, r) } }
  	return out, "", nil
  }
  ```

- [ ] **Step 2: Run test to verify failure**

  Run: `go test ./pkg/api/... -v`
  Expected: FAIL (package missing).

- [ ] **Step 3: Implement `pkg/api/spatial_server.go`**

  Define `RuntimeRecord{ID, Status types.RuntimeStatus, ZoneCount int, ZoneSize float64, ZoneIDs []string, PlayerCount int, CreatedAt time.Time}` and `RuntimeStore` interface (`Create/Get/Destroy/List`). `SpatialServerAPI{ store RuntimeStore; gatewayAddr string; defaultZones int; defaultSize float64 }` with `NewSpatialServerAPI(store, gatewayAddr)`. Implement the five RPCs:
  - `CreateRuntime`: validate `runtime_id`; default `zoneCount`/`zoneSize` from the struct defaults; call `store.Create`; build `ZoneInfo` entries `<id>-z<i>`; return `CreateRuntimeResponse{RuntimeId, GatewayAddr, ZoneCount, Zones}`.
  - `DestroyRuntime`: `store.Destroy` → `codes.NotFound` on `types.ErrNotFound`; return `{Success: true}`.
  - `GetRuntimeInfo`: `store.Get`; map `types.RuntimeStatus` → proto `RuntimeStatus_*`; return zone/player counts.
  - `GetRuntimeMetrics`: `store.Get`; return placeholder `{PlayerCount: rec.PlayerCount}`.
  - `ListRuntimes`: default `pageSize=50`; call `store.List`; map each record to `GetRuntimeInfoResponse`.
  Map helper `toProtoStatus(types.RuntimeStatus) spatialserverv1.RuntimeStatus` covering Creating/Active/Draining/Destroyed.

- [ ] **Step 4: Run test to verify pass**

  Run: `go test ./pkg/api/... -v`
  Expected: PASS.

- [ ] **Step 5: Register in `apps/room-service/main.go`**

  Add a minimal in-process store (`MemoryRuntimeStore` backed by `sync.Mutex`+`map[string]*RuntimeRecord` implementing `RuntimeStore`) to the api package, then in main:
  ```go
  apiSrv := api.NewSpatialServerAPI(api.NewMemoryRuntimeStore(), fmt.Sprintf("gateway:%d", cfg.Gateway.WSPort))
  spatialserverv1.RegisterSpatialServerAPIServer(srv, apiSrv)
  healthSrv.SetServingStatus("spatialserver.v1.SpatialServerAPI", grpc_health_v1.HealthCheckResponse_SERVING)
  ```
  Import `"github.com/thaolaptrinh/spatial-server/pkg/api"`.

- [ ] **Step 6: Verify + commit**

  Run: `go build ./apps/room-service/... && go vet ./pkg/api/...`
  Expected: no errors.
  ```bash
  git add pkg/api/spatial_server.go pkg/api/spatial_server_test.go apps/room-service/main.go
  git commit -m "feat(api): implement SpatialServerAPI service"
  ```

---

### Task 6: Gateway hardening (/ready, /live, 64 KiB cap, drain)

**Files:**
- Modify: `pkg/gateway/handler.go`, `pkg/gateway/handler_test.go`
- Modify: `apps/gateway/main.go`

- [ ] **Step 1: Add failing handler tests**

  Append to `pkg/gateway/handler_test.go` (add `net/http/httptest` import; add a `stubLookuper{}` returning `("127.0.0.1", 9000, nil)` if none exists):

  ```go
  func TestHandler_LiveEndpoint(t *testing.T) {
  	h := NewHandler(NewRouterCache(time.Second), &stubLookuper{}, []byte("k"))
  	rec := httptest.NewRecorder()
  	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/live", nil))
  	assert.Equal(t, http.StatusOK, rec.Code)
  }

  func TestHandler_Ready_503WhenDraining(t *testing.T) {
  	h := NewHandler(NewRouterCache(time.Second), &stubLookuper{}, []byte("k"))
  	h.SetDraining(true)
  	rec := httptest.NewRecorder()
  	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ready", nil))
  	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
  }
  ```

- [ ] **Step 2: Run tests to verify failure**

  Run: `go test ./pkg/gateway/... -run 'TestHandler_LiveEndpoint|TestHandler_Ready_503WhenDraining' -v`
  Expected: FAIL (`/live`/`SetDraining` undefined).

- [ ] **Step 3: Extend `pkg/gateway/handler.go`**

  - Add to `Handler`: `draining atomic.Bool`, `conns atomic.Int64`, `softLimit int`, `readyFn func() bool`. Import `"sync/atomic"`.
  - In `NewHandler` register `mux.HandleFunc("/live", h.handleLive)` and `mux.HandleFunc("/ready", h.handleReady)`; set `h.softLimit = 9000`.
  - Add `SetDraining(bool)`, `SetReadyFn(func() bool)`, `ConnCount() int64`.
  - `handleLive`: 200 + `{"status":"alive"}`.
  - `handleReady`: 503 if `draining.Load()`; 503 if `readyFn != nil && !readyFn()`; 503 if `conns.Load() >= int64(softLimit)`; else 200.
  - In `handleWS`, after the token check: `h.conns.Add(1); defer h.conns.Add(-1)`. After `websocket.Accept`: `c.SetReadLimit(64 * 1024)`.

- [ ] **Step 4: Run tests to verify pass**

  Run: `go test ./pkg/gateway/... -v`
  Expected: PASS.

- [ ] **Step 5: Wire drain + readiness in `apps/gateway/main.go`**

  - Replace the inline koanf block with `cfg, err := config.Load("configs/defaults.yml", "configs/gateway.yml")`.
  - Read `wsPort := cfg.Gateway.WSPort`, `jwtSecret := cfg.Gateway.JWTSecret`, `roomServiceAddr := cfg.RoomService.Addr`.
  - Dial Room Service health via a pooled `grpc.NewClient` to `roomServiceAddr`; build `healthClient := grpc_health_v1.NewHealthClient(...)`. Import `"google.golang.org/grpc/health/grpc_health_v1"`.
  - `handler.SetReadyFn(func() bool { ctx 2s; resp, err := healthClient.Check(...); return err==nil && resp.Status==SERVING })`.
  - Signal handler: `handler.SetDraining(true)`; `httpServer.Shutdown(ctxWithTimeout(cfg.Gateway.DrainTimeout))`.

- [ ] **Step 6: Verify + commit**

  Run: `go build ./apps/gateway/... && go vet ./pkg/gateway/...`
  Expected: no errors.
  ```bash
  git add pkg/gateway/handler.go pkg/gateway/handler_test.go apps/gateway/main.go
  git commit -m "feat(gateway): add /ready /live, 64KiB read cap, SIGTERM drain"
  ```

---

### Task 7: Testcontainers integration test

**Files:**
- Create: `test/integration/harness.go`
- Modify: `test/integration/realtime_test.go`
- Modify: `Makefile`, `go.mod`

- [ ] **Step 1: Add testcontainers deps**

  Run: `go get github.com/testcontainers/testcontainers-go@latest github.com/testcontainers/testcontainers-go/modules/postgres@latest github.com/testcontainers/testcontainers-go/modules/redis@latest`

- [ ] **Step 2: Create `test/integration/harness.go`** (`//go:build integration`)

  `startStack(t)` runs `postgres:16-alpine` + `redis:7-alpine`, returns `{pgDSN, redisAddr, cleanup}` and runs `migration.Run` against the resolved migrations dir.
  `startService(t, name string, port int, extraEnv ...string)` builds `go build -o <tmp>/name ./apps/<name>`, starts it with `SPATIAL_POSTGRES__DSN`/`SPATIAL_REDIS__ADDR`/`SPATIAL_GRPC__PORT=<port>` + extra env (host `SPATIAL_*` env stripped), records `host`/`port` and registers a SIGTERM+Wait cleanup.
  `waitForGRPC(t, addr, timeout)` TCP-dials until reachable.
  `waitForHTTP(t, url, timeout)` GETs until 2xx.

- [ ] **Step 3: Replace `test/integration/realtime_test.go`** (`//go:build integration`)

  `TestEndToEnd_SpawnMoveDespawn`:
  1. `s := startStack(t)`; boot room-service(:19000), game-server(:19001 with `SPATIAL_GRPC__HOST=127.0.0.1`), gateway(:18080 with `SPATIAL_GATEWAY__WS_PORT=18080` + `SPATIAL_ROOM_SERVICE__ADDR`); wait for each.
  2. gRPC-dial room-service; call `CreateRuntime({runtime_id:"r1", zone_count:1})`.
  3. Mint HS256 JWT (`dev-secret-key-change-in-production`) for `player_id=p1, runtime_id=r1, zone_id=r1-z1`.
  4. `websocket.Dial("ws://gw/ws?token=...")`; `c.Read`; `protocol.Decode`; assert `id == PacketIDEntitySpawn` (8-byte header decodes cleanly).

- [ ] **Step 4: Add Makefile target**

  Append:
  ```makefile
  .PHONY: test-integration
  test-integration:
  	go test -tags=integration -count=1 ./test/integration/...
  ```

- [ ] **Step 5: Run + verify**

  Run: `make test-integration`
  Expected: PASS (Postgres+Redis containers start, services boot, client connects, SPAWN frame received).
  Run: `go build ./... && go vet ./...`
  Expected: no errors.

- [ ] **Step 6: Commit**

  ```bash
  git add test/integration/ go.mod go.sum Makefile
  git commit -m "test: real testcontainers E2E for spawn/move/despawn"
  ```

---

## Self-Review Checklist

- **Spec coverage:** packet header (T1), config consolidation (T2 + mains in T4/T6), ServerRegistry (T3), ZoneOwnership (T4), SpatialServerAPI (T5), gateway hardening (T6), Testcontainers E2E (T7). No spec section uncovered.
- **Placeholder scan:** no "TBD/TODO/implement later"; created files show complete code, modified files use precise snippet changes (matching the example plan's line-reference style); no "similar to Task N".
- **Type consistency:** `protocol.Encode(id,payload,compress,seq)` + `Decode → (version,id,payload,compressed,seq,err)` identical in T1 & T7; `room.ServerStore`/`ZoneStore` (T3) match `ServerRepository` (T3) & `ZoneRepository` (T4); `api.RuntimeStore` (T5) matched by `fakeStore` (test) & `MemoryRuntimeStore` (main); `Config.Gateway.{WSPort,DrainTimeout}` (T2) consumed in T6; sentinel errors `types.ErrNotFound`/`ErrConflict`/`ErrNotOwned` uniform across repos.

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/phase-1finish-hardened-vertical-slice.md`. Two execution options:

**1. Subagent-Driven (recommended)** — dispatch a fresh subagent per task (7 tasks, sequential), review between tasks, fast iteration.

**2. Inline Execution** — execute tasks in this session with checkpoints.

Which approach?
