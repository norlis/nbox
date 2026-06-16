package publisher

import (
	"errors"
	"fmt"
	"log/slog"

	awssns "github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/norlis/event-driven/pkg/transport/aws"
	"github.com/norlis/event-driven/pkg/transport/aws/sns"
	"go.uber.org/zap"
	"go.uber.org/zap/exp/zapslog"
	"nbox/internal/event"
)

// New returns the event.Publisher to use based on Config:
//   - cfg.Enabled=false              -> NoopPublisher (publish disabled on purpose)
//   - cfg.Enabled=true, cfg.Topic="" -> error (misconfig)
//   - cfg.Enabled=true, cfg.Topic=X  -> SNSPublisher backed by event-driven's sns.Publisher
func New(
	cfg Config,
	snsClient *awssns.Client,
	identity *aws.Identity,
	logger *zap.Logger,
) (event.Publisher, error) {
	if !cfg.Enabled {
		logger.Info("event publish disabled, using noop publisher")
		return NewNoop(logger), nil
	}
	if cfg.Topic == "" {
		return nil, errors.New("publisher: NBOX_EVENT_PUBLISH=true requires NBOX_EVENT_TOPIC")
	}

	slogLogger := slog.New(zapslog.NewHandler(logger.Core()))

	inner, err := sns.NewPublisher(snsClient, sns.PublisherConfig{
		Topic:    cfg.Topic,
		Identity: identity,
	}, slogLogger)
	if err != nil {
		return nil, fmt.Errorf("publisher: build sns: %w", err)
	}

	return &SNSPublisher{
		inner:          inner,
		source:         cfg.Source,
		maxAttempts:    cfg.MaxAttempts,
		initialBackoff: cfg.InitialBackoff,
		maxBackoff:     cfg.MaxBackoff,
		logger:         logger,
	}, nil
}
