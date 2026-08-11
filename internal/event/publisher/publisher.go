package publisher

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	cloudevents "github.com/cloudevents/sdk-go/v2/event"
	"go.uber.org/zap"
	"nbox/internal/event"
)

// innerPublisher is the contract the Publisher delegates to. Decoupled from
// the concrete transport so we can test the mapping in isolation.
type innerPublisher interface {
	Publish(cloudevents.Event) error
}

// Publisher converts event.Event values into cloudevents.Event and delegates
// publication to inner (a NATS core publisher in production). Transport-neutral.
type Publisher struct {
	inner          innerPublisher
	source         string
	maxAttempts    int
	initialBackoff time.Duration
	maxBackoff     time.Duration
	logger         *zap.Logger
}

// Publish implements event.Publisher.
func (p *Publisher) Publish(ctx context.Context, events ...event.Event) error {
	var firstErr error
	for _, e := range events {
		ce, err := p.toCloudEvent(e)
		if err != nil {
			p.logger.Error("encode failed, dropping event",
				zap.String("type", string(e.Type)),
				zap.String("txid", e.TransactionId),
				zap.Error(err),
			)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if err := p.publishOnce(ctx, ce, e); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		p.logger.Info("event published",
			zap.String("type", string(e.Type)),
			zap.String("subject", ce.Subject()),
			zap.String("txid", e.TransactionId),
		)
	}
	return firstErr
}

func (p *Publisher) publishOnce(ctx context.Context, ce cloudevents.Event, orig event.Event) error {
	// Fast path: no retries configured — skip building the backoff entirely.
	if p.maxAttempts <= 1 {
		if err := p.inner.Publish(ce); err != nil {
			p.logger.Error("publish exhausted retries, dropping event",
				zap.String("type", string(orig.Type)),
				zap.String("subject", ce.Subject()),
				zap.String("txid", orig.TransactionId),
				zap.Error(err),
			)
			return fmt.Errorf("publish: %w", err)
		}
		return nil
	}

	bo := backoff.NewExponentialBackOff()
	bo.InitialInterval = p.initialBackoff
	bo.MaxInterval = p.maxBackoff

	attempts := uint64(p.maxAttempts - 1)
	boRetry := backoff.WithContext(backoff.WithMaxRetries(bo, attempts), ctx)

	op := func() error { return p.inner.Publish(ce) }

	if err := backoff.Retry(op, boRetry); err != nil {
		p.logger.Error("publish exhausted retries, dropping event",
			zap.String("type", string(orig.Type)),
			zap.String("subject", ce.Subject()),
			zap.String("txid", orig.TransactionId),
			zap.Error(err),
		)
		return fmt.Errorf("publish: %w", err)
	}
	return nil
}

func (p *Publisher) toCloudEvent(e event.Event) (cloudevents.Event, error) {
	key := extractKey(e.Payload)
	ce := cloudevents.New()
	ce.SetID(e.TransactionId)
	ce.SetSource(p.source)
	ce.SetType(string(e.Type))
	if key != "" {
		ce.SetSubject(key)
		ce.SetExtension("prefix", topSegment(key))
	}
	ce.SetTime(e.Timestamp)
	ce.SetExtension("username", e.Username)
	ce.SetExtension("transactionid", e.TransactionId)
	if err := ce.SetData(cloudevents.ApplicationJSON, e.Payload); err != nil {
		return ce, fmt.Errorf("set data: %w", err)
	}
	return ce, nil
}

// extractKey parses the JSON payload (an entry.Entry) and pulls the Key
// field. Returns "" on any failure — the CE is published without subject.
func extractKey(payload json.RawMessage) string {
	var head struct {
		Key string `json:"Key"`
	}
	if err := json.Unmarshal(payload, &head); err != nil {
		return ""
	}
	return head.Key
}

// topSegment returns the first path segment of key (e.g. "development" for
// "development/banking/api_key").
func topSegment(key string) string {
	key = strings.Trim(key, "/")
	if before, _, ok := strings.Cut(key, "/"); ok {
		return before
	}
	return key
}
