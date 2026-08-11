// Package entrypushd contains the entrypushd consumer wiring.
package entrypushd

import (
	"fmt"
	"time"

	"nbox/internal/config"
	"nbox/pkg/env"
)

// Config holds entrypushd tuning. Loaded once via LoadConfig() in main.go.
// No other package reads these env vars.
type Config struct {
	// --- Shared base (embedded) ---
	config.Base

	// GRPCListen is the listen address for the gRPC server (KVStream/Watch).
	// Default ":9337".
	GRPCListen string `env:"ENTRYPUSHD_GRPC_LISTEN" envDefault:":9337"`

	// NboxURL is the base HTTP URL of nbox used to fetch the snapshot
	// (current state) on Watch connect, e.g. "http://nbox:7337".
	// Empty/unset => snapshot disabled; Watch streams deltas only.
	NboxURL string `env:"ENTRYPUSHD_NBOX_URL"`

	// HmacSecretKey is the HMAC signing key for M2M JWTs. Must match
	// the value used by nbox (so JWTs minted here verify there). The
	// gRPC auth interceptor uses it via the HMACKey named type.
	HmacSecretKey []byte `env:"HMAC_SECRET_KEY" required:"true"`

	// M2M auth seed — read here so they are validated/parsed via the typed pipeline.

	// ApproleRolesJSON is the JSON array of AppRole definitions seeding the
	// in-memory store. Empty/unset means no roles are registered and all
	// AppRole auth attempts will be rejected.
	ApproleRolesJSON []byte `env:"NBOX_APPROLE_ROLES"`

	// AWSARNMapJSON is the JSON array of ARN mappings for the AWS STS
	// authenticator (Fase 2.1.b). Empty/unset disables the AWS-STS scheme.
	AWSARNMapJSON []byte `env:"NBOX_AWS_ARN_MAP"`

	// ApproleDisabled is the kill switch for all M2M auth. When true,
	// all auth attempts return codes.Unavailable until the process restarts.
	ApproleDisabled bool `env:"NBOX_APPROLE_DISABLED" envDefault:"false"`

	// ConfigTableName is the DynamoDB table for dynamic config. Empty => env
	// only (current behavior; no dynamic reload).
	ConfigTableName string `env:"NBOX_CONFIG_TABLE_NAME"`

	// ConfigTTL is the refresh interval of the config cache.
	ConfigTTL time.Duration `env:"NBOX_CONFIG_TTL" envDefault:"45s"`

	// KeepaliveInterval is the period between nbox.keepalive events on
	// each Watch stream. Must stay below the ALB idle timeout. 0 disables
	// the heartbeat (deltas only, pre-heartbeat behavior).
	KeepaliveInterval time.Duration `env:"ENTRYPUSHD_KEEPALIVE_INTERVAL" envDefault:"60s"`
}

// LoadConfig returns Config from env. Errors if required values are missing.
func LoadConfig() (Config, error) {
	cfg := Config{}
	if err := env.Parse(&cfg); err != nil {
		return Config{}, fmt.Errorf("entrypushd: load config: %w", err)
	}
	return cfg, nil
}
