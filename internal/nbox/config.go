// Package nbox provides nbox-binary-scoped configuration. Loaded once at
// startup by cmd/nbox/main.go and injected into every nbox-specific
// consumer (storage backends, HTTP middleware, exporters, AWS health
// checkers).
//
// Other binaries in this repo (e.g. cmd/entrypushd) MUST NOT import this
// package; their config lives elsewhere.
package nbox

import (
	"fmt"
	"time"

	"nbox/internal/config"
	"nbox/pkg/env"
)

// Config holds the nbox application configuration loaded from environment variables.
type Config struct {
	// --- Shared base (embedded) ---
	config.Base

	// --- Storage (DynamoDB & S3) ---
	BucketName             string `env:"NBOX_BUCKET_NAME"                 envDefault:"nbox-store"                pkl:"bucketName"`
	EntryTableName         string `env:"NBOX_ENTRIES_TABLE_NAME"          envDefault:"nbox-entry-table"          pkl:"entryTableName"`
	TrackingEntryTableName string `env:"NBOX_TRACKING_ENTRIES_TABLE_NAME" envDefault:"nbox-tracking-entry-table" pkl:"trackingEntryTableName"`
	BoxTableName           string `env:"NBOX_BOX_TABLE_NAME"              envDefault:"nbox-box-table"            pkl:"boxTableName"`

	// --- Parameter Store ---
	ParameterStoreKeyId string `env:"NBOX_PARAMETER_STORE_KEY_ID"    pkl:"parameterStoreKeyId"`
	ParameterShortArn   bool   `env:"NBOX_PARAMETER_STORE_SHORT_ARN" envDefault:"true"         pkl:"parameterShortArn"`

	// --- Security ---
	HmacSecretKey []byte `env:"HMAC_SECRET_KEY" required:"true"`
	// BasicAuthCredentials is the JSON array of basic-auth users; see CLAUDE.md.
	BasicAuthCredentials []byte `env:"NBOX_BASIC_AUTH_CREDENTIALS"`

	// --- app ---
	DefaultPrefix string `env:"NBOX_DEFAULT_PREFIX" envDefault:"global" pkl:"defaultPrefix"`
	InstanceName  string `env:"INSTANCE_NAME"       envDefault:"nbox"   pkl:"instanceName"`

	// --- Stages ---
	Stages []string `env:"NBOX_STAGES" envDefault:"development,qa,beta,sandbox,production,dr" envSeparator:"," pkl:"stages"`

	// --- Security ---
	CSRFTrustedOrigins []string `env:"NBOX_CSRF_TRUSTED_ORIGINS" envSeparator:"," pkl:"csrfTrustedOrigins"`

	SpecsPath string `env:"NBOX_SPECS_PATH" envDefault:"/etc/nbox/specs"`

	// ConfigTableName is the DynamoDB table for dynamic config. Empty => env
	// only (current behavior; no dynamic reload).
	ConfigTableName string `env:"NBOX_CONFIG_TABLE_NAME"`

	// ConfigTTL is the refresh interval of the config cache.
	ConfigTTL time.Duration `env:"NBOX_CONFIG_TTL" envDefault:"45s"`
}

// LoadConfig reads the Config from environment variables.
func LoadConfig() (*Config, error) {
	cfg := &Config{}

	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("nbox: load config: %w", err)
	}

	return cfg, nil
}
