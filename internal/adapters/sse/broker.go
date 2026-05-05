package sse

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

const (
	subscriberBuffer = 20
	keepAliveEvery   = 15 * time.Second
	clientReconnect  = "retry: 5000\n\n"
)

// Message is the unit fanned out to every subscribed SSE client.
type Message struct {
	ID      string
	Name    string
	Payload []byte
}

// EventBroker is an in-memory pub/sub for SSE clients.
//
// Subscribers are tracked in a map keyed by client ID and protected by an
// RWMutex so that fan-out (Publish) does not serialize against itself, only
// against subscribe/unsubscribe.
type EventBroker struct {
	logger      *zap.Logger
	mu          sync.RWMutex
	subscribers map[string]chan Message
	closed      bool
}

func NewEventBroker(lc fx.Lifecycle, logger *zap.Logger) *EventBroker {
	b := &EventBroker{
		logger:      logger,
		subscribers: make(map[string]chan Message),
	}

	lc.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			logger.Info("Event broker started")
			return nil
		},
		OnStop: func(_ context.Context) error {
			b.shutdown()
			logger.Info("Event broker stopped")
			return nil
		},
	})

	return b
}

// Subscribe registers a client. If id is empty a UUID v7 is generated. If id
// is already taken, the previous channel is closed (the old handler will exit
// cleanly when it sees the closed channel) and a fresh one is installed.
func (b *EventBroker) Subscribe(id string) (string, <-chan Message) {
	if id == "" {
		if u, err := uuid.NewV7(); err == nil {
			id = u.String()
		} else {
			id = uuid.NewString()
		}
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		ch := make(chan Message)
		close(ch)
		return id, ch
	}

	if old, ok := b.subscribers[id]; ok {
		close(old)
		b.logger.Debug("evicted previous subscriber with same id", zap.String("clientId", id))
	}

	ch := make(chan Message, subscriberBuffer)
	b.subscribers[id] = ch
	return id, ch
}

// Unsubscribe removes the client and closes its channel. Idempotent.
func (b *EventBroker) Unsubscribe(id string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if ch, ok := b.subscribers[id]; ok {
		delete(b.subscribers, id)
		close(ch)
	}
}

// Publish fans the message out to every active subscriber. Slow subscribers
// (full buffer) drop the event rather than blocking the publisher.
func (b *EventBroker) Publish(msg Message) {
	if msg.ID == "" {
		if u, err := uuid.NewV7(); err == nil {
			msg.ID = u.String()
		}
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	for id, ch := range b.subscribers {
		select {
		case ch <- msg:
		default:
			b.logger.Warn("subscriber buffer full, dropping event",
				zap.String("clientId", id),
				zap.String("event", msg.Name),
			)
		}
	}
}

func (b *EventBroker) shutdown() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.closed = true
	for id, ch := range b.subscribers {
		close(ch)
		delete(b.subscribers, id)
	}
}

// ServeHTTP streams events to a single SSE client.
func (b *EventBroker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rc := http.NewResponseController(w)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Long-lived stream: clear any server-wide WriteTimeout for this request.
	_ = rc.SetWriteDeadline(time.Time{})

	if err := rc.Flush(); err != nil {
		b.logger.Error("streaming unsupported by ResponseWriter", zap.Error(err))
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	id, ch := b.Subscribe(r.URL.Query().Get("clientId"))
	defer b.Unsubscribe(id)

	b.logger.Debug("SSE client connected",
		zap.String("clientId", id),
		zap.String("remoteAddr", r.RemoteAddr),
	)

	if _, err := fmt.Fprint(w, clientReconnect); err != nil {
		return
	}
	if err := rc.Flush(); err != nil {
		return
	}

	ticker := time.NewTicker(keepAliveEvery)
	defer ticker.Stop()

	ctx := r.Context()

	for {
		select {
		case <-ctx.Done():
			b.logger.Debug("SSE client disconnected", zap.String("clientId", id))
			return

		case msg, ok := <-ch:
			if !ok {
				b.logger.Debug("subscriber channel closed, terminating SSE stream",
					zap.String("clientId", id),
				)
				return
			}
			if _, err := fmt.Fprintf(w, "id: %s\nevent: %s\ndata: %s\n\n", msg.ID, msg.Name, msg.Payload); err != nil {
				b.logger.Debug("failed to write event payload, dropping client",
					zap.String("clientId", id),
					zap.Error(err),
				)
				return
			}
			if err := rc.Flush(); err != nil {
				b.logger.Debug("flush failed after event, dropping client",
					zap.String("clientId", id),
					zap.Error(err),
				)
				return
			}

		case <-ticker.C:
			if _, err := fmt.Fprint(w, ":ping\n\n"); err != nil {
				b.logger.Debug("failed to write keep-alive ping, dropping client",
					zap.String("clientId", id),
					zap.Error(err),
				)
				return
			}
			if err := rc.Flush(); err != nil {
				b.logger.Debug("flush failed after ping, dropping client",
					zap.String("clientId", id),
					zap.Error(err),
				)
				return
			}
		}
	}
}