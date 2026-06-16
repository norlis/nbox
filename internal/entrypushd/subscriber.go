package entrypushd

import (
	"fmt"
	"log/slog"

	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/norlis/event-driven/pkg/eventmux"
	"github.com/norlis/event-driven/pkg/transport/aws"
	"github.com/norlis/event-driven/pkg/transport/aws/sqs"
)

// NewSQSSubscriber returns an eventmux.Subscription backed by event-driven's
// sqs.Subscriber.
func NewSQSSubscriber(
	cfg Config,
	sqsClient *awssqs.Client,
	identity *aws.Identity,
	logger *slog.Logger,
) (eventmux.Subscription, error) {
	sub, err := sqs.NewSubscriber(sqsClient, sqs.SubscriberConfig{
		Queue:          cfg.Queue,
		Identity:       identity,
		ConsumeWorkers: cfg.Workers,
	}, logger.With(slog.String("logger", "sqs-subscriber")))
	if err != nil {
		return nil, fmt.Errorf("entrypushd: sqs subscriber: %w", err)
	}
	return sub, nil
}
