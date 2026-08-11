package tracking

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	"nbox/internal/entry"
	"nbox/internal/event"
	"nbox/internal/prefix"
)

// --- minimal fakes -------------------------------------------------------

// fakeManager satisfies entry.Manager with no-ops.
type fakeManager struct{}

func (f *fakeManager) Upsert(_ context.Context, entries []entry.Entry) entry.Results {
	return entry.Results{}
}
func (f *fakeManager) Delete(_ context.Context, _ string) error { return nil }
func (f *fakeManager) Retrieve(_ context.Context, _ string, _ ...entry.RetrieveOption) (*entry.Entry, error) {
	return nil, nil
}
func (f *fakeManager) Resolve(_ context.Context, _ string) (*entry.Entry, error) { return nil, nil }
func (f *fakeManager) List(_ context.Context, _ string, _ ...entry.ListOption) ([]entry.Entry, error) {
	return nil, nil
}

func (f *fakeManager) RetrieveMany(_ context.Context, _ []string) (map[string]*entry.Entry, error) {
	return nil, nil
}
func (f *fakeManager) RegisterBackend(_ entry.PartialStore) {}

// fakeStore satisfies tracking.Store with no-ops.
type fakeStore struct{}

func (f *fakeStore) CreateBatch(_ context.Context, _ []Record) error { return nil }
func (f *fakeStore) History(_ context.Context, _ string, _ ...HistoryOption) ([]Record, error) {
	return nil, nil
}

// fakePublisher satisfies event.Publisher with no-ops.
type fakePublisher struct{}

func (f *fakePublisher) Publish(_ context.Context, _ ...event.Event) error { return nil }

// --- helper: build a *Recorder with an observer logger ------------------

func newTestRecorder(t *testing.T) (*Recorder, *observer.ObservedLogs) {
	t.Helper()
	core, logs := observer.New(zapcore.DebugLevel)
	logger := zap.New(core)

	r := &Recorder{
		base:      &fakeManager{},
		tracker:   &fakeStore{},
		publisher: &fakePublisher{},
		logger:    logger,
	}
	return r, logs
}

// --- tests for async helper ----------------------------------------------

// TestAsync_NormalFnRunsToCompletion verifies that a normal fn is called.
func TestAsync_NormalFnRunsToCompletion(t *testing.T) {
	r, _ := newTestRecorder(t)

	done := make(chan struct{})
	r.async(context.Background(), "test.normal", func(_ context.Context) {
		close(done)
	})

	// Block until the goroutine signals completion (no sleep).
	<-done
}

// TestAsync_PanicFnDoesNotCrashAndIsLogged verifies panic recovery and logging.
func TestAsync_PanicFnDoesNotCrashAndIsLogged(t *testing.T) {
	r, logs := newTestRecorder(t)

	r.async(context.Background(), "test.panic", func(_ context.Context) {
		panic("deliberate test panic")
	})

	// The recover+log runs in the helper's OUTER deferred func, which executes
	// after fn fully unwinds. Poll the observer until it lands rather than racing
	// on a done-signal closed from inside fn (which fires before the recover).
	require.Eventually(t, func() bool {
		return logs.FilterMessage("async task panicked").Len() == 1
	}, time.Second, time.Millisecond, "expected exactly one 'async task panicked' log entry")

	entries := logs.FilterMessage("async task panicked").All()
	assert.Equal(t, zapcore.ErrorLevel, entries[0].Level)
	assert.Equal(t, "test.panic", entries[0].ContextMap()["task"])
}

// TestAsync_CtxPassedToFnHasDeadline verifies the helper injects a deadline.
func TestAsync_CtxPassedToFnHasDeadline(t *testing.T) {
	r, _ := newTestRecorder(t)

	var (
		mu          sync.Mutex
		hasDeadline bool
		done        = make(chan struct{})
	)

	r.async(context.Background(), "test.deadline", func(c context.Context) {
		defer close(done)
		_, ok := c.Deadline()
		mu.Lock()
		hasDeadline = ok
		mu.Unlock()
	})

	<-done

	mu.Lock()
	ok := hasDeadline
	mu.Unlock()

	assert.True(t, ok, "expected the context passed to fn to have a deadline")
}

// TestAsync_MultiplePanicsAreEachLogged verifies every panicking task is
// independently recovered and logged. Asserts via polling because each
// recover+log lands in the helper's deferred func after its fn unwinds.
func TestAsync_MultiplePanicsAreEachLogged(t *testing.T) {
	r, logs := newTestRecorder(t)

	const count = 3
	for range count {
		r.async(context.Background(), "test.multi-panic", func(_ context.Context) {
			panic("boom")
		})
	}

	require.Eventually(t, func() bool {
		return logs.FilterMessage("async task panicked").Len() == count
	}, time.Second, time.Millisecond, "expected each panic to be recovered and logged")
}

// fakePartialStore for a non-index backend used in gateway split-brain reasoning.
// (Kept here as documentation for the gateway_test seam — not wired to a test yet.)
type fakePartialStore struct {
	backendType prefix.StorageBackendType
	deleteErr   error
}

func (f *fakePartialStore) BackendType() prefix.StorageBackendType { return f.backendType }
func (f *fakePartialStore) Upsert(_ context.Context, entries []entry.Entry) entry.Results {
	return entry.Results{}
}
func (f *fakePartialStore) Delete(_ context.Context, _ string) error { return f.deleteErr }
func (f *fakePartialStore) Resolve(_ context.Context, _ string) (*entry.Entry, error) {
	return nil, errors.New("not implemented")
}

func (f *fakePartialStore) Retrieve(_ context.Context, _ string, _ ...entry.RetrieveOption) (*entry.Entry, error) {
	return nil, errors.New("not implemented")
}
