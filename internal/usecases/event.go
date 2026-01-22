package usecases

import (
	"context"
	"encoding/json"

	"go.uber.org/zap"
	"nbox/internal/domain"
)

type EventUseCase struct {
	logger    *zap.Logger
	publisher domain.EventPublisher
}

func NewEventUseCase(logger *zap.Logger, publisher domain.EventPublisher) domain.EventNotifier {
	return &EventUseCase{
		logger:    logger,
		publisher: publisher,
	}
}

func (e *EventUseCase) Dispatch(ctx context.Context, event domain.Event[json.RawMessage]) {
	go func() {
		e.logger.Info("DispatchEvent",
			zap.String("type", string(event.Type)),
			zap.String("transactionId", event.TransactionId),
			zap.String("username", event.Username),
		)

		if err := e.publisher.Publish(context.Background(), event); err != nil {
			e.logger.Error("ErrPublishEvent",
				zap.String("eventType", string(event.Type)),
				zap.Error(err),
			)
		}
	}()
}
