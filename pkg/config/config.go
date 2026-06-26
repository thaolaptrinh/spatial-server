package config

import (
	"fmt"

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
	Service  ServiceConfig  `koanf:"service"`
	Logging  LoggingConfig  `koanf:"logging"`
	GRPC     GRPCConfig     `koanf:"grpc"`
	Postgres PostgresConfig `koanf:"postgres"`
	Redis    RedisConfig    `koanf:"redis"`
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

func Load(files ...string) (*Config, error) {
	k := koanf.New(delimiter)

	for _, f := range files {
		if err := k.Load(file.Provider(f), yaml.Parser()); err != nil {
			return nil, fmt.Errorf("load config file %s: %w", f, err)
		}
	}

	if err := k.Load(env.Provider(prefix, delimiter, func(s string) string {
		return s
	}), nil); err != nil {
		return nil, fmt.Errorf("load env config: %w", err)
	}

	var cfg Config
	if err := k.Unmarshal("", &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	return &cfg, nil
}
