package entrypushd_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"nbox/internal/entrypushd"
)

const testHmacSecret = "test-hmac-secret-key"

func TestLoadConfig_RequiresQueue(t *testing.T) {
	t.Setenv("ENTRYPUSHD_QUEUE", "")
	t.Setenv("HMAC_SECRET_KEY", testHmacSecret)

	_, err := entrypushd.LoadConfig()
	require.Error(t, err)
	require.Contains(t, err.Error(), "ENTRYPUSHD_QUEUE")
}

func TestLoadConfig_ParsesQueueAndDefaults(t *testing.T) {
	t.Setenv("ENTRYPUSHD_QUEUE", "nbox-events")
	t.Setenv("ENTRYPUSHD_WORKERS", "")
	t.Setenv("ENTRYPUSHD_GRPC_LISTEN", "")
	t.Setenv("NBOX_ENV", "")
	t.Setenv("HMAC_SECRET_KEY", testHmacSecret)

	cfg, err := entrypushd.LoadConfig()
	require.NoError(t, err)
	require.Equal(t, "nbox-events", cfg.Queue)
	require.Equal(t, 2, cfg.Workers, "default Workers=2")
	require.Equal(t, ":9337", cfg.GRPCListen, "default GRPCListen=:9337")
	require.Equal(t, "development", cfg.Environment)
	require.Equal(t, []byte(testHmacSecret), cfg.HmacSecretKey)
}

func TestLoadConfig_WorkersFromEnv(t *testing.T) {
	t.Setenv("ENTRYPUSHD_QUEUE", "nbox-events")
	t.Setenv("ENTRYPUSHD_WORKERS", "8")
	t.Setenv("HMAC_SECRET_KEY", testHmacSecret)

	cfg, err := entrypushd.LoadConfig()
	require.NoError(t, err)
	require.Equal(t, 8, cfg.Workers)
}

func TestLoadConfig_GRPCListenFromEnv(t *testing.T) {
	t.Setenv("ENTRYPUSHD_QUEUE", "nbox-events")
	t.Setenv("ENTRYPUSHD_GRPC_LISTEN", "0.0.0.0:50051")
	t.Setenv("HMAC_SECRET_KEY", testHmacSecret)

	cfg, err := entrypushd.LoadConfig()
	require.NoError(t, err)
	require.Equal(t, "0.0.0.0:50051", cfg.GRPCListen)
}

func TestLoadConfig_ApproleRolesJSON_FromEnv(t *testing.T) {
	rolesJSON := `[{"role_id":"uuid-1","name":"watcher","secret_hashes":[],"opa_roles":["entrypushd"],"status":"active"}]`
	t.Setenv("ENTRYPUSHD_QUEUE", "nbox-events")
	t.Setenv("HMAC_SECRET_KEY", testHmacSecret)
	t.Setenv("NBOX_APPROLE_ROLES", rolesJSON)

	cfg, err := entrypushd.LoadConfig()
	require.NoError(t, err)
	require.JSONEq(t, rolesJSON, string(cfg.ApproleRolesJSON))
}

func TestLoadConfig_AWSARNMapJSON_FromEnv(t *testing.T) {
	arnJSON := `[{"arn":"arn:aws:iam::123:role/x","name":"x","roles":["entrypushd"],"status":"active"}]`
	t.Setenv("ENTRYPUSHD_QUEUE", "nbox-events")
	t.Setenv("HMAC_SECRET_KEY", testHmacSecret)
	t.Setenv("NBOX_AWS_ARN_MAP", arnJSON)

	cfg, err := entrypushd.LoadConfig()
	require.NoError(t, err)
	require.JSONEq(t, arnJSON, string(cfg.AWSARNMapJSON))
}

func TestLoadConfig_ApproleDisabled_DefaultFalse(t *testing.T) {
	t.Setenv("ENTRYPUSHD_QUEUE", "nbox-events")
	t.Setenv("HMAC_SECRET_KEY", testHmacSecret)
	t.Setenv("NBOX_APPROLE_DISABLED", "")

	cfg, err := entrypushd.LoadConfig()
	require.NoError(t, err)
	require.False(t, cfg.ApproleDisabled, "ApproleDisabled must default to false when unset")
}

func TestLoadConfig_ApproleDisabled_TrueFromEnv(t *testing.T) {
	t.Setenv("ENTRYPUSHD_QUEUE", "nbox-events")
	t.Setenv("HMAC_SECRET_KEY", testHmacSecret)
	t.Setenv("NBOX_APPROLE_DISABLED", "true")

	cfg, err := entrypushd.LoadConfig()
	require.NoError(t, err)
	require.True(t, cfg.ApproleDisabled)
}

func TestLoadConfig_NboxURL_FromEnv(t *testing.T) {
	t.Setenv("ENTRYPUSHD_QUEUE", "nbox-events")
	t.Setenv("HMAC_SECRET_KEY", testHmacSecret)
	t.Setenv("ENTRYPUSHD_NBOX_URL", "http://nbox:7337")

	cfg, err := entrypushd.LoadConfig()
	require.NoError(t, err)
	require.Equal(t, "http://nbox:7337", cfg.NboxURL)
}

func TestLoadConfig_NboxURL_DefaultsEmpty(t *testing.T) {
	t.Setenv("ENTRYPUSHD_QUEUE", "nbox-events")
	t.Setenv("HMAC_SECRET_KEY", testHmacSecret)
	t.Setenv("ENTRYPUSHD_NBOX_URL", "")

	cfg, err := entrypushd.LoadConfig()
	require.NoError(t, err)
	require.Empty(t, cfg.NboxURL, "empty NboxURL disables snapshot")
}
