package config

import (
	"context"
	"fmt"
	"hash/fnv"
	"log/slog"
	"sync/atomic"
	"time"
)

// Snapshot holds an immutable parsed value (T) behind an atomic pointer,
// refreshed from a Source on a TTL ticker. Reads (Current) are lock-free.
// Refresh runs in a single goroutine (Run), so there is no concurrent refresh.
type Snapshot[T any] struct {
	source Source
	key    Key
	parse  func([]byte) (T, error)
	ttl    time.Duration
	logger *slog.Logger

	current  atomic.Pointer[T]
	lastHash atomic.Uint64
}

func NewSnapshot[T any](key Key, source Source, parse func([]byte) (T, error), ttl time.Duration, logger *slog.Logger) *Snapshot[T] {
	return &Snapshot[T]{source: source, key: key, parse: parse, ttl: ttl, logger: logger}
}

// Current returns the latest parsed value. Lock-free.
//
// Precondition: Start must have succeeded first. Activate guarantees this by
// loading at provider construction (before any server OnStart), so current is
// never nil in normal operation.
func (s *Snapshot[T]) Current() T {
	return *s.current.Load()
}

// Start does the initial synchronous load. Fail-closed: returns an error so
// the caller aborts startup instead of serving without config.
func (s *Snapshot[T]) Start(ctx context.Context) error {
	if err := s.refresh(ctx); err != nil {
		return fmt.Errorf("config: initial load %q: %w", s.key.Kind, err)
	}
	return nil
}

// Run refreshes every ttl until ctx is cancelled. On error it serves stale
// (keeps the last good value) and logs.
func (s *Snapshot[T]) Run(ctx context.Context) {
	t := time.NewTicker(s.ttl)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := s.refresh(ctx); err != nil {
				s.logger.Error("config refresh failed, serving stale",
					slog.String("kind", s.key.Kind),
					slog.String("error", err.Error()))
			}
		}
	}
}

// refresh fetches, parses, and atomically swaps. Skips re-parse/log when the
// payload is unchanged (hash).
func (s *Snapshot[T]) refresh(ctx context.Context) error {
	raw, found, err := s.source.Get(ctx, s.key)
	if err != nil {
		return err
	}
	if !found {
		raw = s.key.Empty()
	}
	h := hash64(raw)
	if s.current.Load() != nil && h == s.lastHash.Load() {
		return nil // unchanged
	}
	val, err := s.parse(raw)
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}
	s.current.Store(&val)
	s.lastHash.Store(h)
	s.logger.Info("config refreshed", slog.String("kind", s.key.Kind))
	return nil
}

func hash64(b []byte) uint64 {
	h := fnv.New64a()
	_, _ = h.Write(b)
	return h.Sum64()
}
