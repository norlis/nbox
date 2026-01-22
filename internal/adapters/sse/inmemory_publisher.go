package sse

import (
	"context"
	"encoding/json"
	"fmt"

	"go.uber.org/zap"
	"nbox/internal/domain"
)

// InMemoryEventPublisher envía eventos directamente al broker en memoria.
type InMemoryEventPublisher struct {
	broker *EventBroker
	logger *zap.Logger
}

func NewInMemoryEventPublisher(broker *EventBroker, logger *zap.Logger) domain.EventPublisher {
	return &InMemoryEventPublisher{
		broker: broker,
		logger: logger,
	}
}

func (p *InMemoryEventPublisher) Publish(ctx context.Context, event domain.Event[json.RawMessage]) error {
	payloadBytes, err := json.Marshal(event)
	if err != nil {
		p.logger.Error("ErrBrokerEventEncode", zap.Error(err))
		return fmt.Errorf("failed to marshal event payload: %w", err)
	}

	message := Message{
		Name:    string(event.Type),
		Payload: payloadBytes,
	}
	p.broker.events <- message
	return nil
}
