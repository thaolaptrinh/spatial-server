package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

const (
	prefix    = "SPATIAL_"
	delimiter = "."
)

type Config struct {
	Service          ServiceConfig          `koanf:"service"`
	Logging          LoggingConfig          `koanf:"logging"`
	GRPC             GRPCConfig             `koanf:"grpc"`
	Postgres         PostgresConfig         `koanf:"postgres"`
	Redis            RedisConfig            `koanf:"redis"`
	Gateway          GatewayConfig          `koanf:"gateway"`
	RoomService      RoomServiceConfig      `koanf:"room_service"`
	Game             GameConfig             `koanf:"game"`
	SpatialServerAPI SpatialServerAPIConfig `koanf:"spatial_api"`
}

type ServiceConfig struct {
	Name     string `koanf:"name"`
	Instance string `koanf:"instance"`
	Debug    bool   `koanf:"debug"`
}

type LoggingConfig struct {
	Level string `koanf:"level"`
	JSON  bool   `koanf:"json"`
}

type GRPCConfig struct {
	Host string `koanf:"host"`
	Port int    `koanf:"port"`
}

type PostgresConfig struct {
	DSN string `koanf:"dsn"`
}

type RedisConfig struct {
	Addr string `koanf:"addr"`
}

type GatewayConfig struct {
	WSPort        int           `koanf:"ws_port"`
	JWTSecret     string        `koanf:"jwt_secret"`
	MaxPacketSize int           `koanf:"max_packet_size"`
	SoftConnLimit int           `koanf:"soft_conn_limit"`
	HardConnLimit int           `koanf:"hard_conn_limit"`
	DrainTimeout  time.Duration `koanf:"drain_timeout"`
}

type RoomServiceConfig struct {
	Addr string `koanf:"addr"`
}

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

func Load(files ...string) (*Config, error) {
	k := koanf.New(delimiter)

	for _, f := range files {
		if err := k.Load(file.Provider(f), yaml.Parser()); err != nil {
			return nil, fmt.Errorf("load config file %s: %w", f, err)
		}
	}

	if err := k.Load(env.Provider(prefix, delimiter, func(s string) string {
		s = strings.TrimPrefix(s, prefix)
		s = strings.ToLower(s)
		return strings.ReplaceAll(s, "__", ".")
	}), nil); err != nil {
		return nil, fmt.Errorf("load env config: %w", err)
	}

	var cfg Config
	if err := k.Unmarshal("", &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	return &cfg, nil
}
