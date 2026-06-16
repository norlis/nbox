// Package grpc contains the entrypushd-side gRPC server: an in-memory
// broker that fans out events to subscribed Watch streams, and the
// streamv1.KVStreamServer implementation.
package grpc

import (
	"log/slog"
	"strings"
	"sync"

	"github.com/google/uuid"
	streamv1 "nbox/gen/stream/v1"
)

const subscriberBuffer = 20

// Filter scopes which events a subscriber receives.
// Both empty => receive all events.
// When both non-empty, AND semantics: event must match at least one
// prefix AND at least one type pattern.
type Filter struct {
	Prefixes []string
	Types    []string
}

type subscription struct {
	filter Filter
	ch     chan *streamv1.Event
}

// Broker fans out events to all matching active subscribers in memory.
// Drop-if-full per subscriber: a slow consumer drops events, never blocks
// the publisher.
type Broker struct {
	mu     sync.RWMutex
	subs   map[string]*subscription
	logger *slog.Logger
}

// NewBroker returns an empty Broker.
func NewBroker(logger *slog.Logger) *Broker {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Broker{
		subs:   make(map[string]*subscription),
		logger: logger.With(slog.String("component", "broker")),
	}
}

// Subscribe registers a new subscriber. The returned channel is buffered
// (capacity 20). Caller MUST call Unsubscribe(id) when done.
func (b *Broker) Subscribe(f Filter) (id string, events <-chan *streamv1.Event) {
	id = uuid.NewString()
	ch := make(chan *streamv1.Event, subscriberBuffer)

	b.mu.Lock()
	b.subs[id] = &subscription{filter: f, ch: ch}
	b.mu.Unlock()

	return id, ch
}

// Unsubscribe drops the subscriber and closes its channel. Idempotent.
func (b *Broker) Unsubscribe(id string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	sub, ok := b.subs[id]
	if !ok {
		return
	}
	delete(b.subs, id)
	close(sub.ch)
}

// Publish fans out e to every subscriber whose Filter matches.
// Slow subscribers (buffer full) drop the event with a warn log.
func (b *Broker) Publish(e *streamv1.Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for id, sub := range b.subs {
		if !matchesFilter(e, sub.filter) {
			continue
		}
		select {
		case sub.ch <- e:
		default:
			b.logger.Warn("subscriber slow, dropping event",
				slog.String("subscriber_id", id),
				slog.String("event_id", e.Id),
				slog.String("type", e.Type),
				slog.String("subject", e.Subject),
			)
		}
	}
}

func matchesFilter(e *streamv1.Event, f Filter) bool {
	return matchesPrefix(e.Subject, f.Prefixes) && matchesType(e.Type, f.Types)
}

// Empty list = match all. Otherwise OR: any prefix that matches = true.
func matchesPrefix(subject string, prefixes []string) bool {
	if len(prefixes) == 0 {
		return true
	}
	for _, p := range prefixes {
		if strings.HasPrefix(subject, p) {
			return true
		}
	}
	return false
}

// Empty list = match all. Otherwise OR: exact match, or wildcard-suffix
// prefix match if the pattern ends with "*".
func matchesType(eventType string, patterns []string) bool {
	if len(patterns) == 0 {
		return true
	}
	for _, p := range patterns {
		if before, ok := strings.CutSuffix(p, "*"); ok {
			if strings.HasPrefix(eventType, before) {
				return true
			}
		} else if p == eventType {
			return true
		}
	}
	return false
}
