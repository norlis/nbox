package publisher

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2/event"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"nbox/internal/event"
)

// captureInner records every cloudevents.Event seen by Publish and lets the
// test simulate inner failures.
type captureInner struct {
	seen []cloudevents.Event
	err  error
}

func (c *captureInner) Publish(ce cloudevents.Event) error {
	c.seen = append(c.seen, ce)
	return c.err
}

func TestPublisher_MapsAllCEAttributes(t *testing.T) {
	inner := &captureInner{}
	p := &Publisher{
		inner:       inner,
		source:      "nbox",
		maxAttempts: 1,
		logger:      zap.NewNop(),
	}

	entryJSON := json.RawMessage(`{"Key":"development/banking/api_key","Value":"v","Secure":false}`)
	ts := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	e := event.Event{
		Type:          event.EntryUpserted,
		TransactionId: "tx-1",
		Username:      "alice",
		Timestamp:     ts,
		Payload:       entryJSON,
	}

	require.NoError(t, p.Publish(context.Background(), e))
	require.Len(t, inner.seen, 1)

	ce := inner.seen[0]
	require.Equal(t, "tx-1", ce.ID())
	require.Equal(t, "nbox", ce.Source())
	require.Equal(t, "nbox.entry.upserted", ce.Type())
	require.Equal(t, "development/banking/api_key", ce.Subject())
	require.Equal(t, ts, ce.Time())
	require.Equal(t, "alice", ce.Extensions()["username"])
	require.Equal(t, "tx-1", ce.Extensions()["transactionid"])
	require.Equal(t, "development", ce.Extensions()["prefix"])
}

func TestPublisher_PublishesMultipleEvents(t *testing.T) {
	inner := &captureInner{}
	p := &Publisher{
		inner:       inner,
		source:      "nbox",
		maxAttempts: 1,
		logger:      zap.NewNop(),
	}

	e1 := event.Event{Type: event.EntryUpserted, TransactionId: "t1", Timestamp: time.Now(), Payload: json.RawMessage(`{"Key":"a/b"}`)}
	e2 := event.Event{Type: event.EntryDeleted, TransactionId: "t2", Timestamp: time.Now(), Payload: json.RawMessage(`{"Key":"c/d"}`)}

	require.NoError(t, p.Publish(context.Background(), e1, e2))
	require.Len(t, inner.seen, 2)
	require.Equal(t, "nbox.entry.upserted", inner.seen[0].Type())
	require.Equal(t, "nbox.entry.deleted", inner.seen[1].Type())
}

func TestPublisher_ReturnsFirstError(t *testing.T) {
	boom := errors.New("boom")
	inner := &captureInner{err: boom}
	p := &Publisher{
		inner:       inner,
		source:      "nbox",
		maxAttempts: 1,
		logger:      zap.NewNop(),
	}

	e := event.Event{Type: event.EntryUpserted, TransactionId: "t1", Timestamp: time.Now(), Payload: json.RawMessage(`{"Key":"a"}`)}
	err := p.Publish(context.Background(), e)
	require.ErrorIs(t, err, boom)
}

func TestPublisher_PrefixForEmptyKey(t *testing.T) {
	inner := &captureInner{}
	p := &Publisher{
		inner:       inner,
		source:      "nbox",
		maxAttempts: 1,
		logger:      zap.NewNop(),
	}

	e := event.Event{Type: event.EntryUpserted, TransactionId: "t", Timestamp: time.Now(), Payload: json.RawMessage(`{}`)}
	require.NoError(t, p.Publish(context.Background(), e))
	require.Len(t, inner.seen, 1)
	ce := inner.seen[0]
	require.Empty(t, ce.Subject())
	_, has := ce.Extensions()["prefix"]
	require.False(t, has, "prefix extension should be absent when key is empty")
}

// flakyInner fails the first N attempts then succeeds.
type flakyInner struct {
	seen     []cloudevents.Event
	failN    int
	attempts int
}

func (f *flakyInner) Publish(ce cloudevents.Event) error {
	f.attempts++
	if f.attempts <= f.failN {
		return errors.New("transient")
	}
	f.seen = append(f.seen, ce)
	return nil
}

func TestPublisher_RetriesOnTransientError(t *testing.T) {
	inner := &flakyInner{failN: 2}
	p := newTestPublisher(inner, 3)

	e := event.Event{Type: event.EntryUpserted, TransactionId: "t", Timestamp: time.Now(), Payload: json.RawMessage(`{"Key":"a"}`)}
	require.NoError(t, p.Publish(context.Background(), e))
	require.Equal(t, 3, inner.attempts)
	require.Len(t, inner.seen, 1)
}

func TestPublisher_GivesUpAfterMaxAttempts(t *testing.T) {
	inner := &flakyInner{failN: 100} // always fails
	p := newTestPublisher(inner, 3)

	e := event.Event{Type: event.EntryUpserted, TransactionId: "t", Timestamp: time.Now(), Payload: json.RawMessage(`{"Key":"a"}`)}
	err := p.Publish(context.Background(), e)
	require.Error(t, err)
	require.Equal(t, 3, inner.attempts)
}

// countingInner counts every call to Publish and always succeeds.
type countingInner struct {
	calls int
}

func (c *countingInner) Publish(_ cloudevents.Event) error {
	c.calls++
	return nil
}

// TestPublisher_NoRetry_CallsInnerOnce verifies that with maxAttempts<=1
// the fast path is taken and the inner publisher is invoked exactly once on success.
func TestPublisher_NoRetry_CallsInnerOnce(t *testing.T) {
	inner := &countingInner{}
	p := &Publisher{
		inner:       inner,
		source:      "nbox",
		maxAttempts: 1,
		logger:      zap.NewNop(),
	}

	e := event.Event{
		Type:          event.EntryUpserted,
		TransactionId: "t",
		Timestamp:     time.Now(),
		Payload:       json.RawMessage(`{"Key":"a/b"}`),
	}
	require.NoError(t, p.Publish(context.Background(), e))
	require.Equal(t, 1, inner.calls, "inner must be called exactly once when maxAttempts=1")
}

// TestPublisher_NoRetry_ReturnsErrorOnFailure verifies that with maxAttempts<=1
// a failure from the inner publisher is propagated and inner is called exactly once.
func TestPublisher_NoRetry_ReturnsErrorOnFailure(t *testing.T) {
	boom := errors.New("boom")
	inner := &captureInner{err: boom}
	p := &Publisher{
		inner:       inner,
		source:      "nbox",
		maxAttempts: 1,
		logger:      zap.NewNop(),
	}

	e := event.Event{
		Type:          event.EntryUpserted,
		TransactionId: "t",
		Timestamp:     time.Now(),
		Payload:       json.RawMessage(`{"Key":"a"}`),
	}
	err := p.Publish(context.Background(), e)
	require.ErrorIs(t, err, boom)
	// captureInner.Publish always appends even on error; one call → one seen entry.
	require.Len(t, inner.seen, 1, "inner must be called exactly once when maxAttempts=1")
}

func newTestPublisher(inner innerPublisher, maxAttempts int) *Publisher {
	return &Publisher{
		inner:          inner,
		source:         "nbox",
		maxAttempts:    maxAttempts,
		initialBackoff: 1 * time.Millisecond,
		maxBackoff:     5 * time.Millisecond,
		logger:         zap.NewNop(),
	}
}

// TestPublisher_ExhaustedRetries_DoesNotLogPayload verifies that when all
// retry attempts are exhausted the error log does NOT contain the event
// payload — preventing secret exfiltration to log backends (e.g. CloudWatch).
func TestPublisher_ExhaustedRetries_DoesNotLogPayload(t *testing.T) {
	const secretMarker = "SUPER_SECRET_VALUE"

	// Observer captures every zap log entry written during the test.
	core, logs := observer.New(zap.ErrorLevel)
	observedLogger := zap.New(core)

	inner := &captureInner{err: errors.New("nats unavailable")}
	p := &Publisher{
		inner:          inner,
		source:         "nbox",
		maxAttempts:    1,
		initialBackoff: 1 * time.Millisecond,
		maxBackoff:     5 * time.Millisecond,
		logger:         observedLogger,
	}

	payload := json.RawMessage(`{"Key":"passbox/banking/bbva-api_key","Value":"` + secretMarker + `","Secure":true}`)
	e := event.Event{
		Type:          event.EntrySecretRead,
		TransactionId: "tx-secret-99",
		Username:      "agent-x",
		Timestamp:     time.Now(),
		Payload:       payload,
	}

	err := p.Publish(context.Background(), e)
	require.Error(t, err, "expected an error when inner publisher always fails")

	require.NotEmpty(t, logs.All(), "expected at least one log entry to be captured")

	// Assert that the secret marker does not appear in any logged field or message.
	for _, entry := range logs.All() {
		require.NotContains(t, entry.Message, secretMarker,
			"secret marker must not appear in log message")
		for _, f := range entry.Context {
			strVal := f.String
			require.NotContains(t, strVal, secretMarker,
				"secret marker must not appear in log field %q", f.Key)
		}
	}
}
