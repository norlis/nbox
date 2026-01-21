package storage

import (
	"context"
	"nbox/internal/domain"
	"nbox/internal/domain/models"
	"nbox/internal/domain/models/operations"

	"go.uber.org/zap"
)

type ParameterStoreSecureBackend struct {
	logger *zap.Logger
	base   *ParameterStoreBackend
	//base domain.EntryPartialStore
}

func NewParameterStoreSecureBackend(
	backend *ParameterStoreBackend,
//backend domain.EntryPartialStore,
	logger *zap.Logger,
) *ParameterStoreSecureBackend {
	return &ParameterStoreSecureBackend{
		base:   backend,
		logger: logger,
	}
}

func (p *ParameterStoreSecureBackend) Upsert(ctx context.Context, entries []models.Entry) operations.Results {
	secureEntries := make([]models.Entry, len(entries))
	for i, entry := range entries {
		entry.Secure = true
		secureEntries[i] = entry
	}
	return p.base.Upsert(ctx, secureEntries)
}

func (p *ParameterStoreSecureBackend) Resolve(ctx context.Context, key string) ([]byte, error) {
	entry, err := p.base.Retrieve(ctx, key, domain.WithDecryption(true))
	if err != nil || entry == nil {
		return nil, err
	}
	return []byte(entry.Value), nil
}

func (p *ParameterStoreSecureBackend) Retrieve(ctx context.Context, key string, opts ...domain.RetrieveOption) (*models.Entry, error) {
	opts = append(opts, domain.WithDecryption(false))
	return p.base.Retrieve(ctx, key, opts...)
}

func (p *ParameterStoreSecureBackend) Delete(ctx context.Context, key string) error {
	return p.base.Delete(ctx, key)
}
