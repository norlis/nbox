package handler_test

import (
	"encoding/json"
	"log/slog"
	"sync"
	"testing"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2/event"
	eventd "github.com/norlis/event-driven/pkg/event"
	"github.com/stretchr/testify/require"
	streamv1 "nbox/gen/stream/v1"
	"nbox/internal/entrypushd/handler"
)

// captureBroker records every Publish for assertion.
type captureBroker struct {
	mu   sync.Mutex
	seen []*streamv1.Event
}

func (b *captureBroker) Publish(e *streamv1.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.seen = append(b.seen, e)
}

func (b *captureBroker) Events() []*streamv1.Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]*streamv1.Event, len(b.seen))
	copy(out, b.seen)
	return out
}

func discardLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

func buildMessage(t *testing.T, ceType, subject string, payload []byte) *eventd.Message {
	t.Helper()
	ce := cloudevents.New()
	ce.SetID("tx-1")
	ce.SetType(ceType)
	ce.SetSource("nbox")
	ce.SetSubject(subject)
	ce.SetTime(time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC))
	ce.SetExtension("prefix", "development")
	ce.SetExtension("username", "alice")
	ce.SetExtension("transactionid", "tx-1")
	if payload != nil {
		require.NoError(t, ce.SetData(cloudevents.ApplicationJSON, json.RawMessage(payload)))
	}
	return eventd.NewMessageWithoutAck(ce)
}

func TestBroadcast_AcceptedType_PublishesToBroker(t *testing.T) {
	broker := &captureBroker{}
	h := handler.NewBroadcast(broker, discardLogger())

	msg := buildMessage(t, "nbox.entry.upserted", "development/banking/api_key", []byte(`{"k":"v"}`))
	h.Handle(msg)

	got := broker.Events()
	require.Len(t, got, 1)
	require.Equal(t, "tx-1", got[0].Id)
	require.Equal(t, "nbox.entry.upserted", got[0].Type)
	require.Equal(t, "nbox", got[0].Source)
	require.Equal(t, "development/banking/api_key", got[0].Subject)
	require.NotZero(t, got[0].TimeUnixMs)
	require.JSONEq(t, `{"k":"v"}`, string(got[0].Data))
	require.Equal(t, "development", got[0].Extensions["prefix"])
	require.Equal(t, "alice", got[0].Extensions["username"])
	require.Equal(t, "tx-1", got[0].Extensions["transactionid"])
}

func TestBroadcast_RejectedType_DoesNotPublish(t *testing.T) {
	broker := &captureBroker{}
	h := handler.NewBroadcast(broker, discardLogger())

	msg := buildMessage(t, "some.unknown.type", "foo", nil)
	h.Handle(msg)

	require.Empty(t, broker.Events())
}

func TestBroadcast_AcceptsAllNboxTypes(t *testing.T) {
	types := []string{
		"nbox.entry.upserted",
		"nbox.entry.deleted",
		// "nbox.template.created",
		// "nbox.template.updated",
	}
	for _, ceType := range types {
		t.Run(ceType, func(t *testing.T) {
			broker := &captureBroker{}
			h := handler.NewBroadcast(broker, discardLogger())

			msg := buildMessage(t, ceType, "foo", []byte(`{}`))
			h.Handle(msg)

			require.Len(t, broker.Events(), 1, "type %s should be accepted", ceType)
		})
	}
}
