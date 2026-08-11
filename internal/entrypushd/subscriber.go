package entrypushd

import (
	"fmt"
	"log/slog"

	natsgo "github.com/nats-io/nats.go"
	"github.com/norlis/event-driven/pkg/eventmux"
	"github.com/norlis/event-driven/pkg/transport/nats/core"
)

// NewNATSSubscriber returns an eventmux.Subscription backed by NATS core.
// QueueGroup is left EMPTY → fan-out: every entrypushd instance receives
// every event (replaces the SQS competing-consumer).
func NewNATSSubscriber(
	cfg Config,
	nc *natsgo.Conn,
	logger *slog.Logger,
) (eventmux.Subscription, error) {
	sub, err := core.NewSubscriber(nc, core.SubscriberConfig{
		Subject: cfg.EventSubject(),
	}, logger.With(slog.String("logger", "nats-subscriber")))
	if err != nil {
		return nil, fmt.Errorf("entrypushd: nats subscriber: %w", err)
	}
	return sub, nil
}
