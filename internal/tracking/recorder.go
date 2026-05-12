package tracking

import (
	"context"
	"encoding/json"
	"time"

	"github.com/norlis/httpgate/pkg/adapter/apidriven/middleware"
	"go.uber.org/zap"
	"nbox/internal/application"
	"nbox/internal/entry"
	"nbox/internal/event"
	"nbox/internal/prefix"
)

// Recorder wraps an entry.Manager, adding change tracking and event dispatching on writes.
type Recorder struct {
	base     entry.Manager
	tracker  Store
	notifier event.Dispatcher
	logger   *zap.Logger
}

// NewRecorder returns an entry.Manager decorator that records changes and dispatches events.
func NewRecorder(
	base entry.Manager,
	tracker Store,
	notifier event.Dispatcher,
	logger *zap.Logger,
) entry.Manager {
	return &Recorder{
		base:     base,
		tracker:  tracker,
		notifier: notifier,
		logger:   logger,
	}
}

func (r *Recorder) user(ctx context.Context) string {
	user, ok := application.UserFromContext(ctx)
	if ok {
		return user.Name
	}
	return "ghost"
}

func (r *Recorder) dispatchEvents(ctx context.Context, traceID, updatedBy string, results entry.Results) {
	now := time.Now().UTC()

	for key, res := range results {
		if res.Err != nil {
			continue
		}

		payloadObj := res.Output
		if payloadObj != nil && payloadObj.Secure {
			safeCopy := *payloadObj
			safeCopy.Value = "*****"
			payloadObj = &safeCopy
		}

		payloadBytes, _ := json.Marshal(payloadObj)

		r.notifier.Dispatch(ctx, event.Event[json.RawMessage]{
			Type:          event.EntryActions,
			TransactionId: traceID,
			Username:      updatedBy,
			Timestamp:     now,
			Payload:       payloadBytes,
		})

		r.logger.Debug("Event dispatched",
			zap.String("key", key),
			zap.String("event", string(event.EntryActions)),
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
	bgCtx := context.WithoutCancel(ctx)
	id := middleware.TraceIdFromContext(ctx)

	go func() {
		r.registerChanges(bgCtx, updatedBy, results, entryIndex)
		r.dispatchEvents(bgCtx, id, updatedBy, results)
	}()

	return results
}

func (r *Recorder) Retrieve(ctx context.Context, key string, opts ...entry.RetrieveOption) (*entry.Entry, error) {
	return r.base.Retrieve(ctx, key, opts...) //nolint:wrapcheck
}

func (r *Recorder) Resolve(ctx context.Context, key string) (*entry.Entry, error) {
	val, err := r.base.Resolve(ctx, key)
	if err != nil {
		return nil, err
	}

	if !val.Secure {
		return val, nil
	}

	bgCtx := context.WithoutCancel(ctx)
	updatedBy := r.user(ctx)
	id := middleware.TraceIdFromContext(ctx)

	go func() {
		auditEntry := *val
		if auditEntry.Secure {
			auditEntry.Value = "*****"
		}

		//nolint:errchkjson
		payload, _ := json.Marshal(auditEntry)
		r.notifier.Dispatch(bgCtx, event.Event[json.RawMessage]{
			TransactionId: id,
			Type:          event.EntryRetrieveSecretValue,
			Username:      updatedBy,
			Timestamp:     time.Now().UTC(),
			Payload:       payload,
		})
	}()

	return val, nil
}

func (r *Recorder) Delete(ctx context.Context, key string) error {
	return r.base.Delete(ctx, key)
}

func (r *Recorder) List(ctx context.Context, pfx string) ([]entry.Entry, error) {
	return r.base.List(ctx, pfx)
}

func (r *Recorder) RegisterBackend(adapter entry.PartialStore) {
	r.base.RegisterBackend(adapter)
}

func (r *Recorder) RetrieveMany(ctx context.Context, keys []string) (map[string]*entry.Entry, error) {
	return r.base.RetrieveMany(ctx, keys)
}
