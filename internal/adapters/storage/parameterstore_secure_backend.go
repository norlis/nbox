package storage

import (
	"context"

	"go.uber.org/zap"
	"nbox/internal/domain"
	"nbox/internal/domain/backend"
	"nbox/internal/domain/models"
	"nbox/internal/domain/models/operations"
)

type ParameterStoreSecureBackend struct {
	logger *zap.Logger
	base   *ParameterStoreBackend
}

func NewParameterStoreSecureBackend(
	backend *ParameterStoreBackend,
	logger *zap.Logger,
) *ParameterStoreSecureBackend {
	return &ParameterStoreSecureBackend{
		base:   backend,
		logger: logger,
	}
}

func (p *ParameterStoreSecureBackend) BackendType() backend.StorageBackendType {
	return backend.BackendParameterStoreSecure
}

func (p *ParameterStoreSecureBackend) Upsert(ctx context.Context, entries []models.Entry) operations.Results {
	secureEntries := make([]models.Entry, len(entries))
	for i, entry := range entries {
		entry.Secure = true
		secureEntries[i] = entry
	}
	return p.base.Upsert(ctx, secureEntries)
}

func (p *ParameterStoreSecureBackend) Resolve(ctx context.Context, key string) (*models.Entry, error) {
	entry, err := p.base.Retrieve(ctx, key, domain.WithDecryption(true))
	if err != nil || entry == nil {
		return nil, err
	}
	return entry, nil
}

func (p *ParameterStoreSecureBackend) Retrieve(ctx context.Context, key string, opts ...domain.RetrieveOption) (*models.Entry, error) {
	opts = append(opts, domain.WithDecryption(false))
	return p.base.Retrieve(ctx, key, opts...)
}

func (p *ParameterStoreSecureBackend) Delete(ctx context.Context, key string) error {
	return p.base.Delete(ctx, key)
}
