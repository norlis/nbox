package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"nbox/internal/domain"
	"nbox/internal/domain/backend"
	"nbox/internal/domain/models"
	"nbox/internal/domain/models/operations"

	"go.uber.org/zap"
)

type Gateway struct {
	backends   map[backend.StorageBackendType]domain.EntryPartialStore
	index      domain.EntryFullStore
	prefixRepo domain.PrefixConfigRepository
	logger     *zap.Logger
}

func NewStorageGateway(
	index domain.EntryFullStore,
	prefixRepo domain.PrefixConfigRepository,
	logger *zap.Logger,
) domain.EntryManager {
	return &Gateway{
		backends:   make(map[backend.StorageBackendType]domain.EntryPartialStore),
		index:      index,
		logger:     logger,
		prefixRepo: prefixRepo,
	}
}

func (g *Gateway) calculateFingerprint(entry *models.Entry) {
	if entry.Metadata == nil {
		entry.Metadata = &models.Metadata{}
	}
	hash := sha256.Sum256([]byte(entry.Value))
	entry.Metadata.Fingerprint = hex.EncodeToString(hash[:])
}

func (g *Gateway) RegisterBackend(adapter domain.EntryPartialStore) {
	g.backends[adapter.BackendType()] = adapter
}

func (g *Gateway) resolveBackend(ctx context.Context, key string, secure bool) (domain.EntryPartialStore, error) {
	prefixConfig, err := g.prefixRepo.GetByPrefix(ctx, key)
	if err != nil {
		return g.index, nil
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

func (g *Gateway) RetrieveMany(ctx context.Context, keys []string) (map[string]*models.Entry, error) {
	return g.index.RetrieveMany(ctx, keys)
}

// Retrieve Index First.
func (g *Gateway) Retrieve(ctx context.Context, key string, _ ...domain.RetrieveOption) (*models.Entry, error) {
	return g.index.Retrieve(ctx, key) //nolint:wrapchecks
}

// Resolve optimized
// Resolve gets metadata from the index and the actual value from the backend.
func (g *Gateway) Resolve(ctx context.Context, key string) (*models.Entry, error) {
	// 1. Retrieve the "shell" and metadata from DynamoDB (Index)
	indexEntry, err := g.index.Retrieve(ctx, key)
	if err != nil {
		return nil, err
	}
	if indexEntry == nil {
		return nil, domain.ErrEntryNotFound
	}

	backendType := indexEntry.Metadata.StorageBackend

	// If the backend is local (Dynamo) or Legacy, we already have the final value
	if backendType == "" || backendType == backend.BackendDynamoDB {
		// Optional: If you want to ensure Secure is consistent
		// indexEntry.Secure = false
		return indexEntry, nil
	}

	// Find the correct adapter
	store, ok := g.backends[backendType]
	if !ok {
		g.logger.Error("Critical: Index points to unknown backend",
			zap.String("key", key),
			zap.String("backend", string(backendType)))
		return nil, fmt.Errorf("backend not registered: %s", backendType)
	}

	// 4. Get ONLY the secret value (bytes) from the source (SSM)
	// We don't use store.Retrieve() to avoid an extra call and to trust the index metadata.
	entry, err := store.Resolve(ctx, key)
	if err != nil {
		return nil, err
	}

	// "Hydrate" the index entry with the external entry
	indexEntry.Value = entry.Value
	indexEntry.Secure = entry.Secure

	return indexEntry, nil
}

func (g *Gateway) Delete(ctx context.Context, key string) error {
	store, err := g.resolveBackend(ctx, key, false) // TODO: improve this, at this step we don't know if it's a secret
	if err != nil {
		return err
	}

	// Delete from the source (SSM)
	if err := store.Delete(ctx, key); err != nil {
		return err
	}

	// Delete from the index (DynamoDB) if they are different
	if store != g.index {
		// Ignore index error to avoid blocking, or handle according to policy
		_ = g.index.Delete(ctx, key)
	}

	return nil
}

// List ALWAYS uses the Global Index (DynamoDB).
func (g *Gateway) List(ctx context.Context, prefix string) ([]models.Entry, error) {
	// It doesn't matter where the actual data is stored, DynamoDB has the indexed copy.
	return g.index.List(ctx, prefix)
}

func (g *Gateway) Upsert(ctx context.Context, entries []models.Entry) operations.Results {
	results := make(operations.Results)

	// Lists to separate work
	externalWrites := make(map[domain.EntryPartialStore][]models.Entry)
	toIndex := make([]models.Entry, 0, len(entries))

	// CLASSIFICATION (Routing)
	for _, entry := range entries {
		entry.Metadata = &models.Metadata{
			UpdatedAt: time.Now().UTC(),
		}
		g.calculateFingerprint(&entry) // calculator checksum

		store, err := g.resolveBackend(ctx, entry.Key, entry.Secure)
		if err != nil {
			results.Add(entry.Key, operations.Failed, err)
			continue
		}

		// If the destination IS the index (DynamoDB), it goes directly to the indexing queue.
		if store == g.index {
			// Ensure it has base metadata if it doesn't
			if entry.Metadata == nil {
				entry.Metadata = &models.Metadata{}
			}
			entry.Metadata.StorageBackend = store.BackendType()
			toIndex = append(toIndex, entry)
		} else {
			// If external (SSM), queue it for remote execution
			externalWrites[store] = append(externalWrites[store], entry)
		}
	}

	// EXTERNAL EXECUTION (Parallel Fan-Out)
	var wg sync.WaitGroup
	var mu sync.Mutex // To protect 'toIndex' and 'results'

	for store, batch := range externalWrites {
		wg.Add(1)
		go func(s domain.EntryPartialStore, entries []models.Entry) {
			defer wg.Done()

			// Write to SSM
			opResults := s.Upsert(ctx, entries)

			mu.Lock()
			defer mu.Unlock()

			for key, res := range opResults {
				// Save the operation result (success/failure)
				results[key] = res

				// If successful, prepare the entry for the index
				if res.Err == nil {
					// CRITICAL: Use the backend's Output (which contains the ARN and Metadata)
					// If for some reason there's no Output, use the original entry (fallback)
					if res.Output != nil {
						toIndex = append(toIndex, *res.Output)
					} else {
						toIndex = append(toIndex, entries[findEntryIndex(entries, key)])
					}
				}
			}
		}(store, batch)
	}

	wg.Wait()

	// UNIFIED INDEXING (Fan-In)
	// Now 'toIndex' has native Dynamo entries AND SSM references.
	// We make a single massive call to DynamoDB.

	if len(toIndex) > 0 {
		idxResults := g.index.Upsert(ctx, toIndex)

		// Merge the index results.
		// If indexing failed for something already saved in SSM, mark it as partial error or warning.
		for key, res := range idxResults {
			if res.Err != nil {
				// If we already had a success result (from SSM), this is an "Eventual Consistency" error
				if existingRes, exists := results[key]; exists && existingRes.Err == nil {
					g.logger.Warn("Data saved in backend but failed to index", zap.String("key", key), zap.Error(res.Err))
					// Optional: Overwrite the error in results if you want to be strict
					// results[key] = res
				} else {
					// If it was native Dynamo, this is the main error
					results[key] = res
				}
			} else {
				// If it was native Dynamo and succeeded, register it
				if _, exists := results[key]; !exists {
					results[key] = res
				}
			}
		}
	}

	return results
}

// Simple helper to search in slice (can be optimized with a map if the batch is large).
func findEntryIndex(entries []models.Entry, key string) int {
	for i, e := range entries {
		if e.Key == key {
			return i
		}
	}
	return 0
}
