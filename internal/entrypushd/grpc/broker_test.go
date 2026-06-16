package grpc_test

import (
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	streamv1 "nbox/gen/stream/v1"
	grpcint "nbox/internal/entrypushd/grpc"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(&discardWriter{}, nil))
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

func TestBroker_EmptyFilter_ReceivesAll(t *testing.T) {
	b := grpcint.NewBroker(discardLogger())
	_, ch := b.Subscribe(grpcint.Filter{})

	b.Publish(&streamv1.Event{Id: "1", Type: "nbox.entry.upserted", Subject: "a/b"})

	select {
	case got := <-ch:
		require.Equal(t, "1", got.Id)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected event, got timeout")
	}
}

func TestBroker_PrefixFilter_Matches(t *testing.T) {
	b := grpcint.NewBroker(discardLogger())
	_, ch := b.Subscribe(grpcint.Filter{Prefixes: []string{"development/"}})

	b.Publish(&streamv1.Event{Id: "1", Subject: "development/banking/api_key"})
	b.Publish(&streamv1.Event{Id: "2", Subject: "passbox/banking/api_key"})

	select {
	case got := <-ch:
		require.Equal(t, "1", got.Id, "should only receive the development/ event")
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected event 1, got timeout")
	}

	// 2 should NOT arrive
	select {
	case got := <-ch:
		t.Fatalf("unexpected event %s: passbox/ should not match", got.Id)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestBroker_TypeFilter_ExactMatch(t *testing.T) {
	b := grpcint.NewBroker(discardLogger())
	_, ch := b.Subscribe(grpcint.Filter{Types: []string{"nbox.entry.deleted"}})

	b.Publish(&streamv1.Event{Id: "1", Type: "nbox.entry.upserted", Subject: "a"})
	b.Publish(&streamv1.Event{Id: "2", Type: "nbox.entry.deleted", Subject: "a"})

	select {
	case got := <-ch:
		require.Equal(t, "2", got.Id, "only deleted should pass")
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected event 2")
	}
}

func TestBroker_TypeFilter_WildcardSuffix(t *testing.T) {
	b := grpcint.NewBroker(discardLogger())
	_, ch := b.Subscribe(grpcint.Filter{Types: []string{"nbox.entry.*"}})

	b.Publish(&streamv1.Event{Id: "1", Type: "nbox.entry.upserted"})
	b.Publish(&streamv1.Event{Id: "2", Type: "nbox.template.created"})
	b.Publish(&streamv1.Event{Id: "3", Type: "nbox.entry.secret.read"})

	received := map[string]bool{}
	timeout := time.After(150 * time.Millisecond)
	for len(received) < 2 {
		select {
		case got := <-ch:
			received[got.Id] = true
		case <-timeout:
			t.Fatalf("expected 2 events, got %v", received)
		}
	}
	require.Contains(t, received, "1")
	require.Contains(t, received, "3")
	require.NotContains(t, received, "2")
}

func TestBroker_PrefixAndType_AND(t *testing.T) {
	b := grpcint.NewBroker(discardLogger())
	_, ch := b.Subscribe(grpcint.Filter{
		Prefixes: []string{"passbox/"},
		Types:    []string{"nbox.entry.*"},
	})

	// passes both
	b.Publish(&streamv1.Event{Id: "1", Subject: "passbox/foo", Type: "nbox.entry.upserted"})
	// fails prefix
	b.Publish(&streamv1.Event{Id: "2", Subject: "development/foo", Type: "nbox.entry.upserted"})
	// fails type
	b.Publish(&streamv1.Event{Id: "3", Subject: "passbox/foo", Type: "nbox.template.created"})

	select {
	case got := <-ch:
		require.Equal(t, "1", got.Id, "only the AND-match event passes")
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected event 1")
	}

	select {
	case got := <-ch:
		t.Fatalf("unexpected event %s", got.Id)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestBroker_Unsubscribe_ClosesChannel(t *testing.T) {
	b := grpcint.NewBroker(discardLogger())
	id, ch := b.Subscribe(grpcint.Filter{})
	b.Unsubscribe(id)

	_, ok := <-ch
	require.False(t, ok, "channel must be closed after Unsubscribe")
}

func TestBroker_Unsubscribe_Idempotent(t *testing.T) {
	b := grpcint.NewBroker(discardLogger())
	id, _ := b.Subscribe(grpcint.Filter{})
	b.Unsubscribe(id)
	b.Unsubscribe(id) // must not panic
}

func TestBroker_DropIfFull_DoesNotBlock(t *testing.T) {
	b := grpcint.NewBroker(discardLogger())
	_, ch := b.Subscribe(grpcint.Filter{})

	// Fill the buffer (cap 20) + push one extra. Publisher must not block.
	done := make(chan struct{})
	go func() {
		for range 50 {
			b.Publish(&streamv1.Event{Id: "x"})
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("publish blocked on full channel")
	}

	// Drain
	count := 0
	for {
		select {
		case <-ch:
			count++
		case <-time.After(20 * time.Millisecond):
			require.GreaterOrEqual(t, count, 1, "should receive at least 1 event")
			require.LessOrEqual(t, count, 20, "should not exceed buffer size")
			return
		}
	}
}

func TestBroker_ConcurrentPublish(t *testing.T) {
	b := grpcint.NewBroker(discardLogger())
	_, ch := b.Subscribe(grpcint.Filter{})

	const N = 100
	var wg sync.WaitGroup
	for i := range N {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			b.Publish(&streamv1.Event{Id: "x"})
		}(i)
	}
	wg.Wait()

	// At least some events should be received (drops are allowed but not all)
	received := 0
	timeout := time.After(100 * time.Millisecond)
loop:
	for {
		select {
		case <-ch:
			received++
		case <-timeout:
			break loop
		}
	}
	require.Positive(t, received)
}
