package resiliency

import (
	"context"
	"errors"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/sony/gobreaker"
	"golang.org/x/sync/semaphore"
)

// GuardConfig configures three resilience mechanisms the AWS SDK does NOT provide:
//
//  1. Concurrency semaphore (MaxConcurrency).
//  2. Per-client circuit breaker (gobreaker).
//  3. Retry for "partial batch" (DynamoDB UnprocessedKeys/UnprocessedItems),
//     which is a business-level condition, not an HTTP failure.
//
// Retry/backoff for ThrottlingException, ProvisionedThroughputExceededException,
// network errors, 5xx, etc., is delegated to the adaptive retryer configured
// in amazonaws.NewAwsConfig.
type GuardConfig struct {
	MaxConcurrency           int64
	MaxRetries               uint64        // Retries only for partial-batch
	BaseDelay                time.Duration // Initial backoff interval for partial-batch
	Name                     string
	CBMaxConsecutiveFailures uint32        // Circuit breaker: consecutive failures to open (default: 5)
	CBTimeout                time.Duration // Circuit breaker: timeout to transition to half-open (default: 30s)
	CBInterval               time.Duration // Circuit breaker: interval to reset counters (default: 60s)
}

type Guard struct {
	sem        *semaphore.Weighted
	cb         *gobreaker.CircuitBreaker
	baseDelay  time.Duration
	maxRetries uint64
}

func NewGuard(cfg GuardConfig) *Guard {
	maxFailures := cfg.CBMaxConsecutiveFailures
	if maxFailures == 0 {
		maxFailures = 5
	}

	timeout := cfg.CBTimeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	interval := cfg.CBInterval
	if interval == 0 {
		interval = 60 * time.Second
	}

	st := gobreaker.Settings{
		Name:        cfg.Name,
		MaxRequests: 0,
		Interval:    interval,
		Timeout:     timeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures > maxFailures
		},
	}

	return &Guard{
		sem:        semaphore.NewWeighted(cfg.MaxConcurrency),
		cb:         gobreaker.NewCircuitBreaker(st),
		baseDelay:  cfg.BaseDelay,
		maxRetries: cfg.MaxRetries,
	}
}

// Execute runs fn under the semaphore and circuit breaker.
// If fn returns a PartialBatchError it is retried with backoff
// (up to MaxRetries). Any other error is propagated as-is: the SDK
// already retried internally and Guard must not duplicate that.
func (g *Guard) Execute(ctx context.Context, fn func() error) error {
	if err := g.sem.Acquire(ctx, 1); err != nil {
		return err
	}
	defer g.sem.Release(1)

	_, err := g.cb.Execute(func() (any, error) {
		// Fast path: no partial-batch retries configured — skip building backoff.
		if g.maxRetries == 0 {
			err := fn()
			if err != nil && !isPartialBatch(err) {
				return nil, err
			}
			return nil, err
		}

		exp := backoff.NewExponentialBackOff()
		exp.InitialInterval = g.baseDelay
		exp.RandomizationFactor = 0.5
		exp.Multiplier = 1.5
		exp.MaxInterval = 5 * time.Second
		exp.MaxElapsedTime = 15 * time.Second

		b := backoff.WithContext(backoff.WithMaxRetries(exp, g.maxRetries), ctx)

		return nil, backoff.Retry(func() error {
			err := fn()
			if err == nil {
				return nil
			}
			if !isPartialBatch(err) {
				return backoff.Permanent(err)
			}
			return err
		}, b)
	})

	return err
}

func isPartialBatch(err error) bool {
	_, ok := errors.AsType[*PartialBatchError](err)
	return ok
}
