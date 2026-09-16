package config

import (
	"time"

	"github.com/RCM7/stashito/server/internal/app/server/domain/entity"
)

type Config struct {
	Port        int           `env:"PORT,required"`
	StoragePath string        `env:"STORAGE_PATH,required"`
	LogLevel    string        `env:"LOG_LEVEL,required"`
	LogFormat   string        `env:"LOG_FORMAT,required"`
	TagTTL      time.Duration `env:"TAG_TTL,required"`

	MetricsEnabled bool `env:"METRICS_ENABLED,default=false"`
	MetricsPort    int  `env:"METRICS_PORT,default=0"`

	// Upstreams is parsed from UPSTREAM_<ALIAS>_* variables via ParseUpstreams,
	// not by envconfig.
	Upstreams entity.Registries
}
