package publisher

import (
	"context"

	"go.uber.org/zap"
	"nbox/internal/event"
)

// NoopPublisher is used in dev/local when no SNS topic is configured.
// It accepts events and logs them at debug level.
type NoopPublisher struct {
	logger *zap.Logger
}

// NewNoop returns a NoopPublisher.
func NewNoop(logger *zap.Logger) *NoopPublisher {
	return &NoopPublisher{logger: logger}
}

// Publish implements event.Publisher.
func (p *NoopPublisher) Publish(_ context.Context, events ...event.Event) error {
	for _, e := range events {
		p.logger.Debug("noop publisher: event discarded",
			zap.String("type", string(e.Type)),
			zap.String("txid", e.TransactionId),
		)
	}
	return nil
}
