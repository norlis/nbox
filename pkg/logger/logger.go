package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/norlis/httpgate/logging"
	"nbox/pkg/env"
)

// Config holds logger configuration. Loaded once at startup via LoadConfig().
type Config struct {
	Level       string `env:"LOG_LEVEL"              envDefault:"info"`
	Environment string `env:"DEPLOYMENT_ENVIRONMENT" envDefault:"development"`
}

// LoadConfig reads logger configuration from environment variables.
func LoadConfig() (Config, error) {
	cfg := Config{}
	if err := env.Parse(&cfg); err != nil {
		return Config{}, fmt.Errorf("logger: load config: %w", err)
	}
	return cfg, nil
}

type LogLevel string

// Slog maps the config level to slog. Unknown values default to Info.
func (l LogLevel) Slog() slog.Level {
	switch strings.ToLower(string(l)) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// New returns the platform-standard logger (NDJSON, OTel fields, W3C trace
// injection) writing to stderr. service is the binary's logical name.
func New(cfg Config, service, version string) *slog.Logger {
	return NewWithWriter(os.Stderr, cfg, service, version)
}

// NewWithWriter is New with an explicit sink, for tests. Since httpgate
// v1.2.0 the standard handler already mirrors event.duration as
// event.duration_human — no local decoration needed.
func NewWithWriter(w io.Writer, cfg Config, service, version string) *slog.Logger {
	return logging.New(w,
		logging.WithService(service, version),
		logging.WithEnvironment(cfg.Environment),
		logging.WithLevel(LogLevel(cfg.Level).Slog()),
	)
}
