package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_Success(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "test.yml")
	content := []byte(`
service:
  name: test-service
  instance: "2"
  debug: true

logging:
  level: debug
  json: false

grpc:
  host: "127.0.0.1"
  port: 9999

postgres:
  dsn: "postgres://test:test@localhost:5432/test?sslmode=disable"

redis:
  addr: "localhost:6380"
`)
	require.NoError(t, os.WriteFile(cfgPath, content, 0644))

	cfg, err := Load(cfgPath)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, "test-service", cfg.Service.Name)
	assert.Equal(t, "2", cfg.Service.Instance)
	assert.True(t, cfg.Service.Debug)

	assert.Equal(t, "debug", cfg.Logging.Level)
	assert.False(t, cfg.Logging.JSON)

	assert.Equal(t, "127.0.0.1", cfg.GRPC.Host)
	assert.Equal(t, 9999, cfg.GRPC.Port)

	assert.Equal(t, "postgres://test:test@localhost:5432/test?sslmode=disable", cfg.Postgres.DSN)

	assert.Equal(t, "localhost:6380", cfg.Redis.Addr)
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/path/config.yml")
	assert.Error(t, err)
}

func TestLoad_EnvOverride(t *testing.T) {
	os.Setenv("SPATIAL_SERVICE__NAME", "env-override-service")
	os.Setenv("SPATIAL_GRPC__PORT", "7777")
	defer os.Unsetenv("SPATIAL_SERVICE__NAME")
	defer os.Unsetenv("SPATIAL_GRPC__PORT")

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "test.yml")
	content := []byte(`
service:
  name: original
  instance: "1"

grpc:
  host: "0.0.0.0"
  port: 9000
`)
	require.NoError(t, os.WriteFile(cfgPath, content, 0644))

	cfg, err := Load(cfgPath)
	require.NoError(t, err)

	assert.Equal(t, "env-override-service", cfg.Service.Name)
	assert.Equal(t, 7777, cfg.GRPC.Port)
	assert.Equal(t, "0.0.0.0", cfg.GRPC.Host)
}

func TestLoad_MultipleFiles(t *testing.T) {
	dir := t.TempDir()
	basePath := filepath.Join(dir, "base.yml")
	overridePath := filepath.Join(dir, "override.yml")

	baseContent := []byte(`
service:
  name: base-service
  instance: "1"

grpc:
  port: 9000
`)
	overrideContent := []byte(`
grpc:
  port: 8000
`)
	require.NoError(t, os.WriteFile(basePath, baseContent, 0644))
	require.NoError(t, os.WriteFile(overridePath, overrideContent, 0644))

	cfg, err := Load(basePath, overridePath)
	require.NoError(t, err)

	assert.Equal(t, "base-service", cfg.Service.Name)
	assert.Equal(t, 8000, cfg.GRPC.Port)
}

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

func TestLoad_GatewayEnvOverride(t *testing.T) {
	t.Setenv("SPATIAL_GATEWAY__WS_PORT", "9090")
	dir := t.TempDir()
	path := filepath.Join(dir, "t.yml")
	require.NoError(t, os.WriteFile(path, []byte("gateway: {ws_port: 8080}\n"), 0o644))
	cfg, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, 9090, cfg.Gateway.WSPort)
}

func TestLoad_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "invalid.yml")
	content := []byte{0xFF, 0xFE, 0x00, 0x01}
	require.NoError(t, os.WriteFile(cfgPath, content, 0644))

	_, err := Load(cfgPath)
	assert.Error(t, err)
}
