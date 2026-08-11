package config

// Base holds env vars consumed by more than one binary in this repo.
type Base struct {
	// Environment identifies the deployment context (dev / staging / prod).
	Environment string `env:"NBOX_ENV" envDefault:"development"`

	// NatsURL is the NATS server URL for the event fan-out bus (both binaries).
	NatsURL string `env:"NATS_URL" envDefault:"nats://localhost:4222"`
}

// EventSubject is the NATS subject for change events, scoped by environment.
// nbox publishes here; every entrypushd instance subscribes (fan-out).
func (b Base) EventSubject() string {
	return "nbox.events." + b.Environment
}
