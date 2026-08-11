// Package publisher provides event.Publisher implementations.
package publisher

import (
	"fmt"
	"time"

	"nbox/pkg/env"
)

// Config holds publisher tuning. Loaded once via LoadConfig() in main.go.
type Config struct {
	// Enabled controls whether nbox emits events at all. When false, New
	// returns a NoopPublisher regardless of other settings.
	Enabled bool `env:"NBOX_EVENT_PUBLISH" envDefault:"true"`

	// Source is the CloudEvent Source attribute. Defaults to "nbox".
	Source string `env:"NBOX_EVENT_SOURCE" envDefault:"nbox"`

	// MaxAttempts is the publish retry count including the first attempt.
	MaxAttempts int `env:"NBOX_EVENT_MAX_ATTEMPTS" envDefault:"3"`

	// InitialBackoff is the starting delay between retries.
	InitialBackoff time.Duration `env:"NBOX_EVENT_INITIAL_BACKOFF" envDefault:"100ms"`

	// MaxBackoff caps the exponential backoff growth.
	MaxBackoff time.Duration `env:"NBOX_EVENT_MAX_BACKOFF" envDefault:"1s"`
}

// LoadConfig reads the publisher config from environment variables.
// Called exactly once at app startup (cmd/nbox/main.go).
func LoadConfig() (Config, error) {
	cfg := Config{}
	if err := env.Parse(&cfg); err != nil {
		return Config{}, fmt.Errorf("publisher: load config: %w", err)
	}
	return cfg, nil
}
