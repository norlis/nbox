package publisher

import (
	"context"
	"log/slog"

	"nbox/internal/event"
	"nbox/internal/logfields"
)

// NoopPublisher is used when event publishing is disabled (NBOX_EVENT_PUBLISH=false).
// It accepts events and logs them at debug level.
type NoopPublisher struct {
	logger *slog.Logger
}

// NewNoop returns a NoopPublisher.
func NewNoop(logger *slog.Logger) *NoopPublisher {
	return &NoopPublisher{logger: logger}
}

// Publish implements event.Publisher.
func (p *NoopPublisher) Publish(ctx context.Context, events ...event.Event) error {
	for _, e := range events {
		p.logger.DebugContext(ctx, "noop publisher: event discarded",
			slog.String(logfields.KeyEventType, string(e.Type)),
		)
	}
	return nil
}
