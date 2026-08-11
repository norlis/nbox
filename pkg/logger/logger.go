package logger

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/exp/zapslog"
	"go.uber.org/zap/zapcore"
	"nbox/pkg/env"
)

// Config holds logger configuration. Loaded once at startup via LoadConfig().
type Config struct {
	Level string `env:"LOG_LEVEL" envDefault:"info"`
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

func (l LogLevel) AtomicLevel() zapcore.Level {
	switch strings.ToLower(string(l)) {
	case "debug":
		return zapcore.DebugLevel
	case "info":
		return zapcore.InfoLevel
	case "warn":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	default:
		return zapcore.InfoLevel
	}
}

func NewLogger(cfg Config) (*zap.Logger, error) {
	logLevel := LogLevel(cfg.Level)

	atomicLevel := zap.NewAtomicLevelAt(logLevel.AtomicLevel())
	// var a fxevent.Logger
	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.TimeKey = "timestamp"
	encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderCfg.EncodeDuration = zapcore.StringDurationEncoder

	config := zap.Config{
		Level:             atomicLevel,
		Development:       false,
		DisableCaller:     false,
		DisableStacktrace: false,
		Sampling:          nil,
		Encoding:          "json",
		EncoderConfig:     encoderCfg,
		OutputPaths: []string{
			"stderr",
		},
		ErrorOutputPaths: []string{
			"stderr",
		},
		InitialFields: map[string]any{
			"pid": os.Getpid(),
		},
	}

	logger, err := config.Build()
	if err != nil {
		return nil, fmt.Errorf("logger: build zap logger: %w", err)
	}
	return logger, nil
}

// NewSlog returns a *slog.Logger backed by the given *zap.Logger.
// Used at the httpgate boundary (middleware, OPA, presenter) so nbox can keep
// zap as its primary logger. Mirrors internal/entrypushd/slog_adapter.go.
func NewSlog(z *zap.Logger) *slog.Logger {
	return slog.New(zapslog.NewHandler(z.Core()))
}
