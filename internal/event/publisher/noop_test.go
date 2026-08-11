package publisher_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"nbox/internal/event"
	"nbox/internal/event/publisher"
)

func TestNoopPublisher_AcceptsZeroEvents(t *testing.T) {
	p := publisher.NewNoop(zap.NewNop())
	require.NoError(t, p.Publish(context.Background()))
}

func TestNoopPublisher_AcceptsManyEvents(t *testing.T) {
	p := publisher.NewNoop(zap.NewNop())
	events := []event.Event{
		{Type: event.EntryUpserted, TransactionId: "t1", Timestamp: time.Now(), Payload: json.RawMessage(`{"k":"v"}`)},
		{Type: event.EntryDeleted, TransactionId: "t2", Timestamp: time.Now(), Payload: json.RawMessage(`{"k":"v"}`)},
	}
	require.NoError(t, p.Publish(context.Background(), events...))
}
