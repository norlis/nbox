package logger_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"regexp"
	"testing"

	"github.com/norlis/httpgate/logging"
	"github.com/norlis/httpgate/trace"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestNewWithWriter_StandardShape(t *testing.T) {
	var buf bytes.Buffer
	log := logger.NewWithWriter(&buf, logger.Config{Level: "info", Environment: "production"}, "nbox", "abc1234")

	log.Info("payment processed", slog.String("nbox.key", "global/x"))

	var doc map[string]any
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("not NDJSON: %v", err)
	}
	ts, _ := doc["timestamp"].(string)
	if !regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z$`).MatchString(ts) {
		t.Errorf("timestamp not ISO 8601 UTC ms: %q", ts)
	}
	for k, want := range map[string]string{
		"log.level": "info", "message": "payment processed",
		"service.name": "nbox", "service.version": "abc1234",
		"deployment.environment.name": "production", "nbox.key": "global/x",
	} {
		if doc[k] != want {
			t.Errorf("%s = %v, want %q", k, doc[k], want)
		}
	}
}

func TestNewWithWriter_TraceInjection(t *testing.T) {
	var buf bytes.Buffer
	log := logger.NewWithWriter(&buf, logger.Config{Level: "info"}, "nbox", "abc1234")
	tc := trace.New()
	log.InfoContext(trace.NewContext(context.Background(), tc), "event")

	var doc map[string]any
	_ = json.Unmarshal(buf.Bytes(), &doc)
	if doc["trace_id"] != tc.TraceID || doc["span_id"] != tc.SpanID {
		t.Errorf("trace not injected: %v", doc)
	}
}

func TestNewWithWriter_DebugFilteredByDefault(t *testing.T) {
	var buf bytes.Buffer
	log := logger.NewWithWriter(&buf, logger.Config{Level: "info"}, "nbox", "x")
	log.Debug("noise")
	if buf.Len() != 0 {
		t.Errorf("debug should be filtered at info level: %s", buf.String())
	}
}

func TestNewWithWriter_DurationHumanMirror(t *testing.T) {
	var buf bytes.Buffer
	log := logger.NewWithWriter(&buf, logger.Config{Level: "info"}, "nbox", "x")

	log.Info("request completed", slog.Int64(logging.KeyEventDuration, 65484041))

	var doc map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &doc))
	assert.Equal(t, "65.484041ms", doc[logging.KeyEventDurationHuman])
	assert.EqualValues(t, 65484041, doc[logging.KeyEventDuration])
}

func TestNewWithWriter_DurationHumanAbsentWithoutDuration(t *testing.T) {
	var buf bytes.Buffer
	log := logger.NewWithWriter(&buf, logger.Config{Level: "info"}, "nbox", "x")

	log.Info("no duration here")

	var doc map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &doc))
	_, ok := doc[logging.KeyEventDurationHuman]
	assert.False(t, ok, "mirror must not appear without event.duration")
}
