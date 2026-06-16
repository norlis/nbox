package publisher_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"nbox/internal/event/publisher"
)

func TestNew_DisabledViaFlag_ReturnsNoop(t *testing.T) {
	pub, err := publisher.New(
		publisher.Config{Enabled: false, Topic: "irrelevant"},
		nil, nil, zap.NewNop(),
	)
	require.NoError(t, err)
	_, isNoop := pub.(*publisher.NoopPublisher)
	require.True(t, isNoop, "Enabled=false must return NoopPublisher regardless of Topic")
}

func TestNew_EnabledWithoutTopic_FailsFast(t *testing.T) {
	_, err := publisher.New(
		publisher.Config{Enabled: true, Topic: ""},
		nil, nil, zap.NewNop(),
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "NBOX_EVENT_TOPIC")
}

func TestLoadConfig_DefaultsAreApplied(t *testing.T) {
	t.Setenv("NBOX_EVENT_PUBLISH", "")
	t.Setenv("NBOX_EVENT_TOPIC", "")
	t.Setenv("NBOX_EVENT_SOURCE", "")
	t.Setenv("NBOX_EVENT_MAX_ATTEMPTS", "")
	t.Setenv("NBOX_EVENT_INITIAL_BACKOFF", "")
	t.Setenv("NBOX_EVENT_MAX_BACKOFF", "")

	cfg, err := publisher.LoadConfig()
	require.NoError(t, err)
	require.True(t, cfg.Enabled, "default Enabled=true")
	require.Equal(t, "nbox", cfg.Source)
	require.Equal(t, 3, cfg.MaxAttempts)
	require.Equal(t, 100*time.Millisecond, cfg.InitialBackoff)
	require.Equal(t, time.Second, cfg.MaxBackoff)
}

func TestLoadConfig_BoolParsing(t *testing.T) {
	cases := []struct {
		set  string
		want bool
	}{
		{"true", true},
		{"false", false},
		{"1", true},
		{"0", false},
		{"TRUE", true},
		{"False", false},
	}
	for _, tc := range cases {
		t.Run(tc.set, func(t *testing.T) {
			t.Setenv("NBOX_EVENT_PUBLISH", tc.set)
			cfg, err := publisher.LoadConfig()
			require.NoError(t, err)
			require.Equal(t, tc.want, cfg.Enabled)
		})
	}
}
