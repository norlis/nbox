package publisher

import (
	"errors"
	"fmt"
	"log/slog"

	natsgo "github.com/nats-io/nats.go"
	"github.com/norlis/event-driven/pkg/transport/nats/core"
	"nbox/internal/event"
)

// New returns the event.Publisher:
//   - cfg.Enabled=false -> NoopPublisher (publish disabled on purpose)
//   - else              -> Publisher backed by a NATS core publisher on subject
func New(cfg Config, nc *natsgo.Conn, subject string, logger *slog.Logger) (event.Publisher, error) {
	if !cfg.Enabled {
		logger.Info("event publishing disabled")
		return NewNoop(logger), nil
	}
	if subject == "" {
		return nil, errors.New("publisher: empty event subject")
	}

	inner, err := core.NewPublisher(nc, core.PublisherConfig{Subject: subject}, logger)
	if err != nil {
		return nil, fmt.Errorf("publisher: build nats core: %w", err)
	}

	return &Publisher{
		inner:          inner,
		source:         cfg.Source,
		maxAttempts:    cfg.MaxAttempts,
		initialBackoff: cfg.InitialBackoff,
		maxBackoff:     cfg.MaxBackoff,
		logger:         logger,
	}, nil
}
