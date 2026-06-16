package resiliency

import (
	"context"
	"errors"
	"testing"
	"time"
)

// newNoRetryGuard creates a Guard with MaxRetries=0 (no partial-batch retries)
// and sufficient semaphore/circuit-breaker headroom for unit tests.
func newNoRetryGuard() *Guard {
	return NewGuard(GuardConfig{
		MaxConcurrency: 10,
		MaxRetries:     0,
		BaseDelay:      1 * time.Millisecond,
		Name:           "test-no-retry",
	})
}

// TestGuard_NoRetry_SuccessCallsOpOnce verifies that with MaxRetries==0 a
// successful operation is called exactly once.
func TestGuard_NoRetry_SuccessCallsOpOnce(t *testing.T) {
	g := newNoRetryGuard()
	calls := 0

	err := g.Execute(context.Background(), func() error {
		calls++
		return nil
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected op called 1 time, got %d", calls)
	}
}

// TestGuard_NoRetry_ErrorReturnedOnce verifies that with MaxRetries==0 an error
// from the operation is propagated and the operation is called exactly once.
func TestGuard_NoRetry_ErrorReturnedOnce(t *testing.T) {
	g := newNoRetryGuard()
	boom := errors.New("boom")
	calls := 0

	err := g.Execute(context.Background(), func() error {
		calls++
		return boom
	})

	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if calls != 1 {
		t.Fatalf("expected op called 1 time, got %d", calls)
	}
}

// TestGuard_NoRetry_PartialBatchNotRetried verifies that with MaxRetries==0
// even a PartialBatchError is NOT retried — it is returned as-is.
func TestGuard_NoRetry_PartialBatchNotRetried(t *testing.T) {
	g := newNoRetryGuard()
	calls := 0

	err := g.Execute(context.Background(), func() error {
		calls++
		return &PartialBatchError{Remaining: 3}
	})

	if err == nil {
		t.Fatal("expected a PartialBatchError, got nil")
	}
	var pbe *PartialBatchError
	if !errors.As(err, &pbe) {
		t.Fatalf("expected PartialBatchError, got: %T: %v", err, err)
	}
	if calls != 1 {
		t.Fatalf("expected op called 1 time (no retry), got %d", calls)
	}
}

// TestGuard_WithRetries_RetriesPartialBatch verifies that MaxRetries>0 still
// retries PartialBatchErrors (regression guard for the retry path).
func TestGuard_WithRetries_RetriesPartialBatch(t *testing.T) {
	g := NewGuard(GuardConfig{
		MaxConcurrency: 10,
		MaxRetries:     3,
		BaseDelay:      1 * time.Millisecond,
		Name:           "test-with-retry",
	})

	calls := 0
	err := g.Execute(context.Background(), func() error {
		calls++
		if calls < 3 {
			return &PartialBatchError{Remaining: 1}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("expected success after retries, got: %v", err)
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
}
