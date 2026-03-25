package usecases

import (
	"context"
	"encoding/json"
	"time"

	"github.com/norlis/httpgate/pkg/adapter/apidriven/middleware"
	"go.uber.org/zap"
	"nbox/internal/application"
	"nbox/internal/domain"
	"nbox/internal/domain/backend"
	"nbox/internal/domain/models"
	"nbox/internal/domain/models/operations"
)

type entryManagerWithTracking struct {
	base     domain.EntryManager
	tracker  domain.TrackingRepository
	notifier domain.EventNotifier
	logger   *zap.Logger
}

func NewEntryManagerWithTracking(
	base domain.EntryManager,
	tracker domain.TrackingRepository,
	notifier domain.EventNotifier,
	logger *zap.Logger,
) domain.EntryManager {
	return &entryManagerWithTracking{
		base:     base,
		tracker:  tracker,
		notifier: notifier,
		logger:   logger,
	}
}

func (e *entryManagerWithTracking) user(ctx context.Context) string {
	user, ok := application.UserFromContext(ctx)

	if ok {
		return user.Name
	}

	return "ghost"
}

func (e *entryManagerWithTracking) dispatchEvents(
	ctx context.Context,
	TraceID string,
	updatedBy string,
	results operations.Results,
) {
	now := time.Now().UTC()

	for key, res := range results {
		if res.Err != nil {
			continue
		}

		payloadObj := res.Output
		if payloadObj != nil && payloadObj.Secure {
			safeCopy := *payloadObj
			safeCopy.Value = "*****" // Masked
			payloadObj = &safeCopy
		}

		payloadBytes, _ := json.Marshal(payloadObj)

		e.notifier.Dispatch(ctx, domain.Event[json.RawMessage]{
			Type:          domain.EventEntryActions,
			TransactionId: TraceID,
			Username:      updatedBy,
			Timestamp:     now,
			Payload:       payloadBytes,
		})

		// Log opcional para debug
		e.logger.Debug(
			"Event dispatched",
			zap.String("key", key),
			zap.String("event", string(domain.EventEntryActions)),
		)
	}
}

func (e *entryManagerWithTracking) registerChanges(
	ctx context.Context,
	updatedBy string,
	results operations.Results,
	entryIndex map[string]models.Entry,
) {
	var trackingBatch []models.Tracking
	now := time.Now().UTC()

	for key, res := range results {
		// Solo trackeamos éxitos
		if res.Err != nil {
			continue
		}

		// Recuperamos entrada original para fallbacks
		originalEntry, exists := entryIndex[key]
		if !exists {
			continue
		}

		trackItem := models.Tracking{
			Key:            key,
			Value:          originalEntry.Value,
			Secure:         originalEntry.Secure,
			StorageBackend: backend.BackendDynamoDB,
			UpdatedAt:      now,
			UpdatedBy:      updatedBy,
			Action:         operations.Updated.String(),
		}

		if originalEntry.Secure {
			trackItem.Value = "*****"
		}

		if res.Output != nil {
			trackItem.Value = res.Output.Value
			trackItem.Secure = res.Output.Secure
			trackItem.StorageBackend = res.Output.Metadata.StorageBackend
			trackItem.Action = res.Output.Metadata.Action
			trackItem.Version = res.Output.Metadata.Version
			trackItem.Hash = res.Output.Metadata.Fingerprint
		}

		trackingBatch = append(trackingBatch, trackItem)
	}

	if len(trackingBatch) > 0 {
		if err := e.tracker.CreateBatch(ctx, trackingBatch); err != nil {
			e.logger.Error("Async tracking batch failed", zap.Error(err))
		}
	}
}

func (e *entryManagerWithTracking) Upsert(ctx context.Context, entries []models.Entry) operations.Results {
	entryIndex := make(map[string]models.Entry, len(entries))
	for _, e := range entries {
		entryIndex[e.Key] = e
	}

	results := e.base.Upsert(ctx, entries)
	updatedBy := e.user(ctx)
	bgCtx := context.WithoutCancel(ctx)
	ID := middleware.TraceIdFromContext(ctx)

	go func() {
		e.registerChanges(bgCtx, updatedBy, results, entryIndex)
		e.dispatchEvents(bgCtx, ID, updatedBy, results)
	}()

	return results
}

func (e *entryManagerWithTracking) Retrieve(ctx context.Context, key string, opts ...domain.RetrieveOption) (*models.Entry, error) {
	return e.base.Retrieve(ctx, key, opts...)
}

func (e *entryManagerWithTracking) Resolve(ctx context.Context, key string) (*models.Entry, error) {
	val, err := e.base.Resolve(ctx, key)
	if err != nil {
		return nil, err
	}

	if !val.Secure {
		return val, nil
	}

	bgCtx := context.WithoutCancel(ctx)
	updatedBy := e.user(ctx)
	id := middleware.TraceIdFromContext(ctx)

	go func() {
		auditEntry := *val
		if auditEntry.Secure {
			auditEntry.Value = "*****"
		}

		//nolint:errchkjson
		payload, _ := json.Marshal(auditEntry)
		e.notifier.Dispatch(bgCtx, domain.Event[json.RawMessage]{
			TransactionId: id,
			Type:          domain.EventEntryRetrieveSecretValue,
			Username:      updatedBy,
			Timestamp:     time.Now().UTC(),
			Payload:       payload,
		})
	}()

	return val, nil
}

func (e *entryManagerWithTracking) Delete(ctx context.Context, key string) error {
	return e.base.Delete(ctx, key)
}

func (e *entryManagerWithTracking) List(ctx context.Context, prefix string) ([]models.Entry, error) {
	return e.base.List(ctx, prefix)
}

func (e *entryManagerWithTracking) RegisterBackend(adapter domain.EntryPartialStore) {
	e.base.RegisterBackend(adapter)
}

func (e *entryManagerWithTracking) RetrieveMany(ctx context.Context, keys []string) (map[string]*models.Entry, error) {
	return e.base.RetrieveMany(ctx, keys)
}
