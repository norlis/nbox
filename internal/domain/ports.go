package domain

import (
	"context"
	"nbox/internal/domain/backend"
	"nbox/internal/domain/models"
	"nbox/internal/domain/models/operations"
	"time"
)

type RetrieveConfig struct {
	Decrypt bool
}

type RetrieveOption func(*RetrieveConfig)

func WithDecryption(decrypt bool) RetrieveOption {
	return func(c *RetrieveConfig) {
		c.Decrypt = decrypt
	}
}

type EntryPartialStore interface {
	// Upsert inserts or updates entries (handles encryption internally if Secure=true).
	Upsert(ctx context.Context, entries []models.Entry) operations.Results

	// Retrieve fetches an entry. Backend decides whether to decrypt automatically.
	Retrieve(ctx context.Context, key string, opts ...RetrieveOption) (*models.Entry, error)

	// Delete removes an entry and its history if applicable.
	Delete(ctx context.Context, key string) error

	// Resolve future show secrets or data on parameter store
	Resolve(ctx context.Context, key string) (*models.Entry, error)

	BackendType() backend.StorageBackendType
}

type EntryFullStore interface {
	EntryPartialStore

	// List retrieves entries by prefix (e.g., to build configuration trees).
	List(ctx context.Context, prefix string) ([]models.Entry, error)

	RetrieveMany(ctx context.Context, keys []string) (map[string]*models.Entry, error)
}

type EntryManager interface {
	Upsert(ctx context.Context, entries []models.Entry) operations.Results
	Retrieve(ctx context.Context, key string, opts ...RetrieveOption) (*models.Entry, error)
	Resolve(ctx context.Context, key string) (*models.Entry, error)
	Delete(ctx context.Context, key string) error
	List(ctx context.Context, prefix string) ([]models.Entry, error)

	RetrieveMany(ctx context.Context, keys []string) (map[string]*models.Entry, error)
	RegisterBackend(adapter EntryPartialStore)
}

type HistoryConfig struct {
	Since time.Time
	To    time.Time
	Limit int32
}

type HistoryOption func(*HistoryConfig)

func WithHistorySince(t time.Time) HistoryOption {
	return func(c *HistoryConfig) {
		c.Since = t
	}
}

func WithHistoryTo(t time.Time) HistoryOption {
	return func(c *HistoryConfig) {
		c.To = t
	}
}

func WithHistoryLimit(limit int32) HistoryOption {
	return func(c *HistoryConfig) {
		c.Limit = limit
	}
}

type TrackingRepository interface {
	CreateBatch(ctx context.Context, tracks []models.Tracking) error
	History(ctx context.Context, key string, opts ...HistoryOption) ([]models.Tracking, error)
}
