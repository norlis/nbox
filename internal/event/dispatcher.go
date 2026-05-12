package event

import (
	"context"
	"encoding/json"

	"go.uber.org/zap"
)

type dispatcher struct {
	logger    *zap.Logger
	publisher Publisher
}

// NewDispatcher creates a Dispatcher that fans out events via the given Publisher.
func NewDispatcher(logger *zap.Logger, publisher Publisher) Dispatcher {
	return &dispatcher{
		logger:    logger,
		publisher: publisher,
	}
}

func (d *dispatcher) Dispatch(ctx context.Context, e Event[json.RawMessage]) {
	go func() {
		d.logger.Info("DispatchEvent",
			zap.String("type", string(e.Type)),
			zap.String("transactionId", e.TransactionId),
			zap.String("username", e.Username),
		)

		if err := d.publisher.Publish(context.Background(), e); err != nil {
			d.logger.Error("ErrPublishEvent",
				zap.String("eventType", string(e.Type)),
				zap.Error(err),
			)
		}
	}()
}
