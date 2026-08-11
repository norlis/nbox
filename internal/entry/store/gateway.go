package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
	"nbox/internal/entry"
	"nbox/internal/prefix"
)

// Gateway implementa entry.Manager orquestando múltiples backends según
// el prefix.Config asociado a cada key.
type Gateway struct {
	backends   map[prefix.StorageBackendType]entry.PartialStore
	index      entry.Store
	prefixRepo prefix.Store
	logger     *zap.Logger
}

func NewGateway(
	index entry.Store,
	prefixRepo prefix.Store,
	logger *zap.Logger,
) entry.Manager {
	return &Gateway{
		backends:   make(map[prefix.StorageBackendType]entry.PartialStore),
		index:      index,
		logger:     logger,
		prefixRepo: prefixRepo,
	}
}

func (g *Gateway) calculateFingerprint(e *entry.Entry) {
	if e.Metadata == nil {
		e.Metadata = &entry.Metadata{}
	}
	hash := sha256.Sum256([]byte(e.Value))
	e.Metadata.Fingerprint = hex.EncodeToString(hash[:])
}

func (g *Gateway) RegisterBackend(adapter entry.PartialStore) {
	g.backends[adapter.BackendType()] = adapter
}

func (g *Gateway) resolveBackend(ctx context.Context, key string, secure bool) (entry.PartialStore, error) {
	prefixConfig, err := g.prefixRepo.ByPrefix(ctx, key)
	if err != nil {
		// No prefix config for this key: designed fallback to the index.
		// Any other error must fail closed — silently falling back could
		// route a secure entry to the default backend.
		if errors.Is(err, entry.ErrEntryNotFound) {
			return g.index, nil
		}
		return nil, fmt.Errorf("resolve prefix config for %s: %w", key, err)
	}

	targetType := prefixConfig.TypeDefault

	if secure {
		targetType = prefixConfig.TypeSecure
	}

	adapter, ok := g.backends[targetType]
	if !ok {
		return nil, fmt.Errorf("backend %s not registered for prefix %s", targetType, key)
	}

	return adapter, nil
}

func (g *Gateway) RetrieveMany(ctx context.Context, keys []string) (map[string]*entry.Entry, error) {
	return g.index.RetrieveMany(ctx, keys)
}

// Retrieve siempre va al índice global (DynamoDB).
func (g *Gateway) Retrieve(ctx context.Context, key string, _ ...entry.RetrieveOption) (*entry.Entry, error) {
	return g.index.Retrieve(ctx, key)
}

// Resolve obtiene metadata del índice y el valor real del backend correspondiente.
func (g *Gateway) Resolve(ctx context.Context, key string) (*entry.Entry, error) {
	indexEntry, err := g.index.Retrieve(ctx, key)
	if err != nil {
		return nil, err
	}
	if indexEntry == nil {
		return nil, entry.ErrEntryNotFound
	}

	backendType := indexEntry.Metadata.StorageBackend

	// If the backend is local (Dynamo) or Legacy, we already have the final value.
	if backendType == "" || backendType == prefix.BackendDynamoDB {
		return indexEntry, nil
	}

	store, ok := g.backends[backendType]
	if !ok {
		g.logger.Error("Critical: Index points to unknown backend",
			zap.String("key", key),
			zap.String("backend", string(backendType)))
		return nil, fmt.Errorf("backend not registered: %s", backendType)
	}

	resolved, err := store.Resolve(ctx, key)
	if err != nil {
		return nil, err
	}

	indexEntry.Value = resolved.Value
	indexEntry.Secure = resolved.Secure

	return indexEntry, nil
}

func (g *Gateway) Delete(ctx context.Context, key string) error {
	store, err := g.resolveBackend(ctx, key, false)
	if err != nil {
		return err
	}

	if err := store.Delete(ctx, key); err != nil {
		return err
	}

	if store != g.index {
		if err := g.index.Delete(ctx, key); err != nil {
			g.logger.Error("index delete failed after backend delete; possible split-brain, manual reconcile required",
				zap.String("key", key),
				zap.Error(err),
			)
		}
	}

	return nil
}

// List ALWAYS uses the Global Index (DynamoDB).
func (g *Gateway) List(ctx context.Context, pfx string, opts ...entry.ListOption) ([]entry.Entry, error) {
	return g.index.List(ctx, pfx, opts...)
}

func (g *Gateway) Upsert(ctx context.Context, entries []entry.Entry) entry.Results {
	results := make(entry.Results)

	externalWrites := make(map[entry.PartialStore][]entry.Entry)
	toIndex := make([]entry.Entry, 0, len(entries))

	for _, e := range entries {
		e.Metadata = &entry.Metadata{
			UpdatedAt: time.Now().UTC(),
		}
		g.calculateFingerprint(&e)

		store, err := g.resolveBackend(ctx, e.Key, e.Secure)
		if err != nil {
			results.Add(e.Key, entry.Failed, err)
			continue
		}

		if store == g.index {
			if e.Metadata == nil {
				e.Metadata = &entry.Metadata{}
			}
			e.Metadata.StorageBackend = store.BackendType()
			toIndex = append(toIndex, e)
		} else {
			externalWrites[store] = append(externalWrites[store], e)
		}
	}

	var wg sync.WaitGroup
	var mu sync.Mutex

	for store, batch := range externalWrites {
		wg.Add(1)
		go func(s entry.PartialStore, ents []entry.Entry) {
			defer wg.Done()

			opResults := s.Upsert(ctx, ents)

			mu.Lock()
			defer mu.Unlock()

			for key, res := range opResults {
				results[key] = res

				if res.Err == nil {
					if res.Output != nil {
						toIndex = append(toIndex, *res.Output)
					} else {
						toIndex = append(toIndex, ents[findEntryIndex(ents, key)])
					}
				}
			}
		}(store, batch)
	}

	wg.Wait()

	if len(toIndex) > 0 {
		idxResults := g.index.Upsert(ctx, toIndex)

		for key, res := range idxResults {
			if res.Err != nil {
				if existingRes, exists := results[key]; exists && existingRes.Err == nil {
					g.logger.Warn("Data saved in backend but failed to index", zap.String("key", key), zap.Error(res.Err))
				} else {
					results[key] = res
				}
			} else {
				if _, exists := results[key]; !exists {
					results[key] = res
				}
			}
		}
	}

	return results
}

// findEntryIndex localiza el índice de una entry por su key.
func findEntryIndex(entries []entry.Entry, key string) int {
	for i, e := range entries {
		if e.Key == key {
			return i
		}
	}
	return 0
}
