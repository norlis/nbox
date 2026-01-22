package resiliency

import (
	"context"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/sony/gobreaker"
	"golang.org/x/sync/semaphore"
)

type GuardConfig struct {
	MaxConcurrency           int64
	MaxRetries               uint64
	BaseDelay                time.Duration // InitialInterval for exponential backoff
	Name                     string
	CBMaxConsecutiveFailures uint32        // Circuit breaker: consecutive failures to open (default: 5)
	CBTimeout                time.Duration // Circuit breaker: timeout to try half-open (default: 30s)
	CBInterval               time.Duration // Circuit breaker: interval to reset counters (default: 60s)
}

type Guard struct {
	sem        *semaphore.Weighted
	cb         *gobreaker.CircuitBreaker
	baseDelay  time.Duration
	maxRetries uint64
}

func NewGuard(cfg GuardConfig) *Guard {
	// Default values for circuit breaker
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

func (g *Guard) Execute(ctx context.Context, fn func() error) error {
	if err := g.sem.Acquire(ctx, 1); err != nil {
		return err
	}
	defer g.sem.Release(1)

	_, err := g.cb.Execute(func() (interface{}, error) {

		exp := backoff.NewExponentialBackOff()
		exp.InitialInterval = g.baseDelay
		exp.RandomizationFactor = 0.5 // Jitter
		exp.Multiplier = 1.5
		exp.MaxInterval = 5 * time.Second
		exp.MaxElapsedTime = 15 * time.Second

		b := backoff.WithContext(backoff.WithMaxRetries(exp, g.maxRetries), ctx)

		return nil, backoff.Retry(func() error {
			err := fn()

			if err == nil {
				return nil
			}

			if !isRetryable(err) {
				return backoff.Permanent(err)
			}

			return err
		}, b)
	})

	return err
}

func isRetryable(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "ThrottlingException") ||
		strings.Contains(msg, "ProvisionedThroughputExceededException") || // Dynamo
		strings.Contains(msg, "RateExceeded") ||
		strings.Contains(msg, "RequestLimitExceeded") ||
		strings.Contains(msg, "TooManyRequests") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "partial batch")
}
