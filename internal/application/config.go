package application

import (
	"fmt"

	"nbox/pkg/env"
)

// CredentialsLoaderConfig defines how authentication credentials are loaded.
// Supports multiple sources: environment variables, AWS Secrets Manager, or file.
type CredentialsLoaderConfig struct {
	Source    string `env:"NBOX_CREDENTIALS_SOURCE"         envDefault:"env"`                         // Deprecated: Source type: "env", "secretsmanager", or "file"
	EnvVarKey string `env:"NBOX_BASIC_AUTH_CREDENTIALS_KEY" envDefault:"NBOX_BASIC_AUTH_CREDENTIALS"` // Environment variable name containing credentials
	SecretARN string `env:"NBOX_CREDENTIALS_SECRET_ARN"`                                              // Deprecated: AWS Secrets Manager secret ARN
	FilePath  string `env:"NBOX_CREDENTIALS_FILE"           envDefault:".credentials.json"`           // Deprecated: Path to credentials file
}

// Config holds the application configuration loaded from environment variables.
type Config struct {
	// --- Core AWS ---
	RegionName string `env:"AWS_REGION" envDefault:"us-east-1" pkl:"regionName"` // AWS region for all services
	AccountId  string `env:"ACCOUNT_ID" pkl:"accountId"`                         // AWS account ID (used for ARN construction)

	// --- Storage (DynamoDB & S3) ---
	BucketName             string `env:"NBOX_BUCKET_NAME"                 envDefault:"nbox-store"                pkl:"bucketName"`             // S3 bucket for templates
	EntryTableName         string `env:"NBOX_ENTRIES_TABLE_NAME"          envDefault:"nbox-entry-table"          pkl:"entryTableName"`         // DynamoDB table for entries
	TrackingEntryTableName string `env:"NBOX_TRACKING_ENTRIES_TABLE_NAME" envDefault:"nbox-tracking-entry-table" pkl:"trackingEntryTableName"` // DynamoDB table for change history
	BoxTableName           string `env:"NBOX_BOX_TABLE_NAME"              envDefault:"nbox-box-table"            pkl:"boxTableName"`           // DynamoDB table for box/templates metadata
	PrefixConfigTableName  string `env:"NBOX_PREFIX_CONFIG_TABLE_NAME"    envDefault:"nbox-prefix-config-table"  pkl:"prefixConfigTableName"`  // DynamoDB table for prefix configurations

	// --- Parameter Store ---
	ParameterStoreKeyId string `env:"NBOX_PARAMETER_STORE_KEY_ID"    pkl:"parameterStoreKeyId"`                         // KMS key ID for SecureString encryption
	ParameterShortArn   bool   `env:"NBOX_PARAMETER_STORE_SHORT_ARN" envDefault:"true"         pkl:"parameterShortArn"` // Use short ARN format (e.g., /path/key instead of full ARN)

	// --- Security ---
	HmacSecretKey     []byte                  `env:"HMAC_SECRET_KEY" required:"true"` // Secret key for HMAC token signing (openssl rand -base64 32 | openssl rand -hex 32)
	CredentialsLoader CredentialsLoaderConfig // Nested config for credentials loading

	// --- app ---
	DefaultPrefix string `env:"NBOX_DEFAULT_PREFIX" envDefault:"global" pkl:"defaultPrefix"` // default prefix for entries

	// --- Legacy (deprecated) ---
	AllowedPrefixes []string `env:"NBOX_ALLOWED_PREFIXES" envDefault:"development/,qa/,beta/,production/" envSeparator:"," pkl:"allowedPrefixes"` // Deprecated: list of allowed prefixes

	SpecsPath string `env:"NBOX_SPECS_PATH" envDefault:"/etc/nbox/specs"`
}

// NewConfigFromEnv loads configuration by reading environment variables.
// It uses the env package to parse tags, apply defaults, and convert types.
func NewConfigFromEnv() (*Config, error) {
	cfg := &Config{}

	//  Auto load: Read struct tags, lookup ENV vars, apply defaults, convert types
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Post-load enrichment: Append defaultPrefix to allowed prefixes list
	// Note: AllowedPrefixes is already populated from env/default values
	cfg.AllowedPrefixes = append(cfg.AllowedPrefixes, cfg.DefaultPrefix+"/")

	return cfg, nil
}
