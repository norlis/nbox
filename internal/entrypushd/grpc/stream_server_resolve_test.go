package grpc

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"nbox/internal/entrypushd/nboxclient"
)

// countingSnap counts Resolve calls. Resolve blocks on release so concurrent
// singleflight callers overlap (widening the coalescing window).
type countingSnap struct {
	resolveN atomic.Int64
	release  chan struct{}
}

func (c *countingSnap) Snapshot(_ context.Context, _, _ string) ([]nboxclient.Entry, error) {
	return nil, nil
}

func (c *countingSnap) Resolve(_ context.Context, _, key string) (string, error) {
	c.resolveN.Add(1)
	if c.release != nil {
		<-c.release
	}
	return "pt_" + key, nil
}

func TestResolveOnce_DedupesConcurrentSameEvent(t *testing.T) {
	snap := &countingSnap{release: make(chan struct{})}
	s := &StreamServer{snapshot: snap}

	const subs = 20
	var wg sync.WaitGroup
	for i := 0; i < subs; i++ {
		wg.Go(func() {
			pt, err := s.resolveOnce(context.Background(), "evt-1", "tok", "passbox/av2/test6")
			if err != nil || pt != "pt_passbox/av2/test6" {
				t.Errorf("resolveOnce = %q, %v", pt, err)
			}
		})
	}

	// Let the goroutines pile onto the in-flight singleflight call, then
	// release the (deduped) underlying Resolve(s).
	time.Sleep(50 * time.Millisecond)
	close(snap.release)
	wg.Wait()

	if got := snap.resolveN.Load(); got > subs/2 {
		t.Fatalf("expected resolve-once to dedupe well below %d, got %d", subs, got)
	}
}
