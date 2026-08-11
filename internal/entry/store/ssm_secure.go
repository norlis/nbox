package store

import (
	"context"

	"go.uber.org/zap"
	"nbox/internal/entry"
	"nbox/internal/prefix"
)

// SSMSecure es un wrapper sobre SSM que fuerza Secure=true en todas las
// operaciones. Está pensado para prefijos vault donde toda escritura debe
// ir cifrada con KMS.
type SSMSecure struct {
	logger *zap.Logger
	base   *SSM
}

func NewSSMSecure(
	base *SSM,
	logger *zap.Logger,
) *SSMSecure {
	return &SSMSecure{
		base:   base,
		logger: logger,
	}
}

func (p *SSMSecure) BackendType() prefix.StorageBackendType {
	return prefix.BackendParameterStoreSecure
}

func (p *SSMSecure) Upsert(ctx context.Context, entries []entry.Entry) entry.Results {
	secureEntries := make([]entry.Entry, len(entries))
	for i, en := range entries {
		en.Secure = true
		secureEntries[i] = en
	}
	return p.base.Upsert(ctx, secureEntries)
}

func (p *SSMSecure) Resolve(ctx context.Context, key string) (*entry.Entry, error) {
	e, err := p.base.Retrieve(ctx, key, entry.WithDecryption(true))
	if err != nil || e == nil {
		return nil, err
	}
	return e, nil
}

func (p *SSMSecure) Retrieve(ctx context.Context, key string, opts ...entry.RetrieveOption) (*entry.Entry, error) {
	opts = append(opts, entry.WithDecryption(false))
	return p.base.Retrieve(ctx, key, opts...)
}

func (p *SSMSecure) Delete(ctx context.Context, key string) error {
	return p.base.Delete(ctx, key)
}
