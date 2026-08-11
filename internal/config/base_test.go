package config_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"nbox/internal/config"
	"nbox/pkg/env"
)

func TestBase_DefaultEnvironment(t *testing.T) {
	t.Setenv("NBOX_ENV", "")
	cfg := config.Base{}
	require.NoError(t, env.Parse(&cfg))
	require.Equal(t, "development", cfg.Environment)
}

func TestBase_EnvironmentFromEnv(t *testing.T) {
	t.Setenv("NBOX_ENV", "production")
	cfg := config.Base{}
	require.NoError(t, env.Parse(&cfg))
	require.Equal(t, "production", cfg.Environment)
}

// TestBase_EmbedsCleanlyInOuterStruct verifies that env.Parse walks into
// anonymous embedded fields. This is the property nbox.Config and
// entrypushd.Config rely on (Tasks 5 and 6).
func TestBase_EmbedsCleanlyInOuterStruct(t *testing.T) {
	type Outer struct {
		config.Base
		Extra string `env:"EXTRA_VAR" envDefault:"hello"`
	}

	t.Setenv("NBOX_ENV", "staging")
	t.Setenv("EXTRA_VAR", "")

	cfg := Outer{}
	require.NoError(t, env.Parse(&cfg))
	require.Equal(t, "staging", cfg.Environment) // promoted access
	require.Equal(t, "hello", cfg.Extra)
}
