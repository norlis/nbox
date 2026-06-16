package logger_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"
	"nbox/pkg/logger"
)

func TestLoadConfig_LevelFromEnv(t *testing.T) {
	t.Setenv("LOG_LEVEL", "debug")

	cfg, err := logger.LoadConfig()
	require.NoError(t, err)
	assert.Equal(t, "debug", cfg.Level)
}

func TestLoadConfig_DefaultLevel(t *testing.T) {
	t.Setenv("LOG_LEVEL", "")

	cfg, err := logger.LoadConfig()
	require.NoError(t, err)
	assert.Equal(t, "info", cfg.Level, "default log level should be info when LOG_LEVEL is unset")
}

func TestNewLogger_ReturnsNonNil(t *testing.T) {
	l, err := logger.NewLogger(logger.Config{Level: "warn"})
	require.NoError(t, err, "NewLogger must not error for a valid config")
	require.NotNil(t, l, "NewLogger must return a non-nil *zap.Logger")
}

func TestNewLogger_DoesNotPanic(t *testing.T) {
	assert.NotPanics(t, func() {
		_, _ = logger.NewLogger(logger.Config{Level: "warn"})
	})
}

func TestLogLevel_AtomicLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected zapcore.Level
	}{
		{"debug", zapcore.DebugLevel},
		{"info", zapcore.InfoLevel},
		{"warn", zapcore.WarnLevel},
		{"error", zapcore.ErrorLevel},
		{"DEBUG", zapcore.DebugLevel},
		{"INFO", zapcore.InfoLevel},
		{"WARN", zapcore.WarnLevel},
		{"ERROR", zapcore.ErrorLevel},
		{"", zapcore.InfoLevel},        // default
		{"unknown", zapcore.InfoLevel}, // default
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			level := logger.LogLevel(tt.input).AtomicLevel()
			assert.Equal(t, tt.expected, level)
		})
	}
}
