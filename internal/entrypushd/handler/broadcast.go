// Package handler hosts the per-message handlers used by the entrypushd
// event consumer.
package handler

import (
	"fmt"
	"log/slog"
	"time"

	streamv1 "nbox/gen/stream/v1"
	"nbox/internal/event"

	eventd "github.com/norlis/event-driven/pkg/event"
)

// Publisher is the subset of the gRPC broker the handler depends on.
// Decouples handler tests from the concrete grpcint.Broker.
type Publisher interface {
	Publish(e *streamv1.Event)
}

// Broadcast receives an *event.Message from the event consumer, filters by
// CloudEvent type, maps the message to a streamv1.Event, and forwards it
// to the broker. The Ack/Nack lifecycle is the caller's responsibility.
type Broadcast struct {
	broker Publisher
	logger *slog.Logger
}

// NewBroadcast returns a Broadcast handler bound to the given broker.
func NewBroadcast(broker Publisher, logger *slog.Logger) *Broadcast {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Broadcast{
		broker: broker,
		logger: logger.With(slog.String("handler", "broadcast")),
	}
}

// Handle is the per-message callback. Caller (the consumer) is
// responsible for Ack/Nack; this handler just decides whether to
// broadcast.
func (h *Broadcast) Handle(msg *eventd.Message) {
	if !isAcceptedType(msg.Type()) {
		h.logger.Debug("event type rejected, skipping",
			slog.String("type", msg.Type()),
			slog.String("id", msg.ID()),
		)
		return
	}

	evt := toProtoEvent(msg)
	h.broker.Publish(evt)

	h.logger.Info("event broadcast",
		slog.String("id", evt.Id),
		slog.String("type", evt.Type),
		slog.String("subject", evt.Subject),
	)
}

var acceptedTypes = map[string]struct{}{
	string(event.EntryUpserted): {},
	string(event.EntryDeleted):  {},
	// string(event.EntrySecretRead): {},
	// string(event.TemplateCreated): {},
	// string(event.TemplateUpdated): {},
}

func isAcceptedType(t string) bool {
	_, ok := acceptedTypes[t]
	return ok
}

func toProtoEvent(msg *eventd.Message) *streamv1.Event {
	exts := map[string]string{}
	for k, v := range msg.Extensions() {
		// Extension values are any (string/int/bool/binary); stringify all.
		exts[k] = fmt.Sprint(v)
	}
	return &streamv1.Event{
		Id:         msg.ID(),
		Type:       msg.Type(),
		Source:     msg.Source(),
		Subject:    msg.Subject(),
		TimeUnixMs: time.Now().UnixMilli(), // TimeUnixMs: msg.Time().UnixMilli()
		Data:       msg.Data(),
		Extensions: exts,
	}
}
