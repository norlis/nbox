package config

// Base holds env vars consumed by more than one binary in this repo.
// Add fields here only when at least two binaries depend on them.
type Base struct {
	// Environment identifies the deployment context (dev / staging / prod).
	// Used for log fields, metric tags, and CloudEvent extensions across
	// every binary in this repo. Default "development" so local runs work
	// without configuration.
	Environment string `env:"NBOX_ENV" envDefault:"development"`
}
