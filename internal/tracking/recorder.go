package tracking

import (
	"context"
	"encoding/json"
	"time"

	"github.com/norlis/httpgate/middleware"
	"go.uber.org/zap"
	"nbox/internal/application"
	"nbox/internal/entry"
	"nbox/internal/event"
	"nbox/internal/prefix"
)

const asyncTaskTimeout = 5 * time.Second

// Recorder wraps an entry.Manager, adding change tracking and event publishing on writes.
type Recorder struct {
	base      entry.Manager
	tracker   Store
	publisher event.Publisher
	logger    *zap.Logger
}

// NewRecorder returns an entry.Manager decorator that records changes and publishes events.
func NewRecorder(
	base entry.Manager,
	tracker Store,
	publisher event.Publisher,
	logger *zap.Logger,
) entry.Manager {
	return &Recorder{
		base:      base,
		tracker:   tracker,
		publisher: publisher,
		logger:    logger,
	}
}

func (r *Recorder) user(ctx context.Context) string {
	return application.ActorFromContext(ctx)
}

// async runs fn detached from the request lifecycle (WithoutCancel) with a bounded
// timeout and panic recovery: audit/event work must survive request cancellation,
// but must not leak goroutines or crash the process.
func (r *Recorder) async(parent context.Context, task string, fn func(ctx context.Context)) {
	bgCtx := context.WithoutCancel(parent)
	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				r.logger.Error("async task panicked",
					zap.String("task", task),
					zap.Any("recover", rec),
				)
			}
		}()
		ctx, cancel := context.WithTimeout(bgCtx, asyncTaskTimeout)
		defer cancel()
		fn(ctx)
	}()
}

func (r *Recorder) publishEvents(ctx context.Context, traceID, updatedBy string, results entry.Results) {
	now := time.Now().UTC()
	events := make([]event.Event, 0, len(results))

	for _, res := range results {
		if res.Err != nil {
			continue
		}

		payloadObj := res.Output
		if payloadObj != nil && payloadObj.Secure {
			safeCopy := *payloadObj
			safeCopy.Value = "*****"
			payloadObj = &safeCopy
		}

		payloadBytes, err := json.Marshal(payloadObj)
		if err != nil {
			// Drop rather than publish an event with a nil payload.
			r.logger.Error("marshal event payload failed; event dropped",
				zap.String("transactionId", traceID),
				zap.Error(err))
			continue
		}

		events = append(events, event.Event{
			Type:          event.EntryUpserted,
			TransactionId: traceID,
			Username:      updatedBy,
			Timestamp:     now,
			Payload:       payloadBytes,
		})
	}

	if len(events) == 0 {
		return
	}

	if err := r.publisher.Publish(ctx, events...); err != nil {
		r.logger.Error("publish batch failed",
			zap.Int("count", len(events)),
			zap.Error(err),
		)
	}
}

func (r *Recorder) registerChanges(ctx context.Context, updatedBy string, results entry.Results, entryIndex map[string]entry.Entry) {
	var batch []Record
	now := time.Now().UTC()

	for key, res := range results {
		if res.Err != nil {
			continue
		}

		original, exists := entryIndex[key]
		if !exists {
			continue
		}

		rec := Record{
			Key:            key,
			Value:          original.Value,
			Secure:         original.Secure,
			StorageBackend: prefix.BackendDynamoDB,
			UpdatedAt:      now,
			UpdatedBy:      updatedBy,
			Action:         entry.Updated.String(),
		}

		if original.Secure {
			rec.Value = "*****"
		}

		if res.Output != nil {
			rec.Value = res.Output.Value
			rec.Secure = res.Output.Secure
			rec.StorageBackend = res.Output.Metadata.StorageBackend
			rec.Action = res.Output.Metadata.Action
			rec.Version = res.Output.Metadata.Version
			rec.Hash = res.Output.Metadata.Fingerprint
		}

		batch = append(batch, rec)
	}

	if len(batch) > 0 {
		if err := r.tracker.CreateBatch(ctx, batch); err != nil {
			r.logger.Error("Async tracking batch failed", zap.Error(err))
		}
	}
}

func (r *Recorder) Upsert(ctx context.Context, entries []entry.Entry) entry.Results {
	entryIndex := make(map[string]entry.Entry, len(entries))
	for _, e := range entries {
		entryIndex[e.Key] = e
	}

	results := r.base.Upsert(ctx, entries)
	updatedBy := r.user(ctx)
	id := middleware.TraceIDFromContext(ctx)

	r.async(ctx, "upsert.track", func(c context.Context) {
		r.registerChanges(c, updatedBy, results, entryIndex)
		r.publishEvents(c, id, updatedBy, results)
	})

	return results
}

func (r *Recorder) Retrieve(ctx context.Context, key string, opts ...entry.RetrieveOption) (*entry.Entry, error) {
	return r.base.Retrieve(ctx, key, opts...) //nolint:wrapcheck
}

// Resolve delegates to the base store. Secret reads are NOT audited/published
// (see decision: reads are not state changes and don't belong on the event
// topic; nothing consumed the secret.read event and entrypushd's delivery
// resolves would flood it).
func (r *Recorder) Resolve(ctx context.Context, key string) (*entry.Entry, error) {
	return r.base.Resolve(ctx, key) //nolint:wrapcheck
}

func (r *Recorder) Delete(ctx context.Context, key string) error {
	return r.base.Delete(ctx, key)
}

func (r *Recorder) List(ctx context.Context, pfx string, opts ...entry.ListOption) ([]entry.Entry, error) {
	return r.base.List(ctx, pfx, opts...)
}

func (r *Recorder) RegisterBackend(adapter entry.PartialStore) {
	r.base.RegisterBackend(adapter)
}

func (r *Recorder) RetrieveMany(ctx context.Context, keys []string) (map[string]*entry.Entry, error) {
	return r.base.RetrieveMany(ctx, keys)
}
