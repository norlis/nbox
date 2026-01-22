package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"nbox/internal/domain"
	"nbox/internal/domain/backend"
	"nbox/internal/domain/models"
	"nbox/internal/domain/models/operations"
	"sync"
	"time"

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

func (g *Gateway) calculateChecksum(entry *models.Entry) {
	if entry.Metadata == nil {
		entry.Metadata = &models.Metadata{}
	}
	hash := sha256.Sum256([]byte(entry.Value))
	entry.Metadata.Hash = hex.EncodeToString(hash[:])
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

// Retrieve Index First
func (g *Gateway) Retrieve(ctx context.Context, key string, _ ...domain.RetrieveOption) (*models.Entry, error) {
	return g.index.Retrieve(ctx, key)
}

// Resolve optimizado
// Resolve obtiene la metadata del índice y el valor real del backend
func (g *Gateway) Resolve(ctx context.Context, key string) (*models.Entry, error) {
	// 1. Recuperamos la "cáscara" y metadata desde DynamoDB (Índice)
	indexEntry, err := g.index.Retrieve(ctx, key)
	if err != nil {
		return nil, err
	}
	if indexEntry == nil {
		return nil, domain.ErrEntryNotFound
	}

	backendType := indexEntry.Metadata.StorageBackend

	// 2. Si el backend es local (Dynamo) o Legacy, ya tenemos el valor final
	if backendType == "" || backendType == backend.BackendDynamoDB {
		// Opcional: Si quieres asegurarte de que Secure sea consistente
		// indexEntry.Secure = false
		return indexEntry, nil
	}

	// 3. Buscamos el adaptador correcto
	store, ok := g.backends[backendType]
	if !ok {
		g.logger.Error("Critical: Index points to unknown backend",
			zap.String("key", key),
			zap.String("backend", string(backendType)))
		return nil, fmt.Errorf("backend no registrado: %s", backendType)
	}

	// 4. Obtenemos SOLO el valor secreto (bytes) desde la fuente (SSM)
	// No usamos store.Retrieve() para evitar una llamada extra y para confiar en la metadata del índice.
	entry, err := store.Resolve(ctx, key)
	if err != nil {
		return nil, err
	}

	// 5. "Hidratamos" el entry del índice con el entry externo
	indexEntry.Value = entry.Value
	indexEntry.Secure = entry.Secure

	return indexEntry, nil
}

func (g *Gateway) Delete(ctx context.Context, key string) error {
	store, err := g.resolveBackend(ctx, key, false) // TODO: mejorar esto, en este paso no se sabe si es un secret
	if err != nil {
		return err
	}

	// 1. Borrar de la fuente (SSM)
	if err := store.Delete(ctx, key); err != nil {
		return err
	}

	// 2. Borrar del índice (DynamoDB) si son distintos
	if store != g.index {
		// Ignoramos error de índice para no bloquear, o lo manejamos según política
		_ = g.index.Delete(ctx, key)
	}

	return nil
}

// List SIEMPRE usa el Índice Global (DynamoDB)
func (g *Gateway) List(ctx context.Context, prefix string) ([]models.Entry, error) {
	// No importa dónde estén guardados los datos reales, DynamoDB tiene la copia indexada.
	return g.index.List(ctx, prefix)
}

func (g *Gateway) Upsert(ctx context.Context, entries []models.Entry) operations.Results {
	results := make(operations.Results)

	// Listas para separar el trabajo
	externalWrites := make(map[domain.EntryPartialStore][]models.Entry)
	toIndex := make([]models.Entry, 0, len(entries))

	// 1. CLASIFICACIÓN (Routing)
	for _, entry := range entries {
		entry.Metadata = &models.Metadata{
			UpdatedAt: time.Now().UTC(),
		}
		g.calculateChecksum(&entry) // calcular checksum

		store, err := g.resolveBackend(ctx, entry.Key, entry.Secure)
		if err != nil {
			results.Add(entry.Key, operations.Failed, err)
			continue
		}

		// Si el destino ES el índice (DynamoDB), va directo a la cola de indexación.
		if store == g.index {
			// Aseguramos que tenga metadata base si no la tiene
			if entry.Metadata == nil {
				entry.Metadata = &models.Metadata{}
			}
			entry.Metadata.StorageBackend = store.BackendType()
			toIndex = append(toIndex, entry)
		} else {
			// Si es externo (SSM), lo encolamos para ejecución remota
			externalWrites[store] = append(externalWrites[store], entry)
		}
	}

	// 2. EJECUCIÓN EXTERNA (Parallel Fan-Out)
	var wg sync.WaitGroup
	var mu sync.Mutex // Para proteger 'toIndex' y 'results'

	for store, batch := range externalWrites {
		wg.Add(1)
		go func(s domain.EntryPartialStore, entries []models.Entry) {
			defer wg.Done()

			// Escribimos en SSM
			opResults := s.Upsert(ctx, entries)

			mu.Lock()
			defer mu.Unlock()

			for key, res := range opResults {
				// Guardamos el resultado de la operación (éxito/fallo)
				results[key] = res

				// Si fue exitoso, preparamos la entrada para el índice
				if res.Err == nil {
					// CRÍTICO: Usamos el Output del backend (que trae el ARN y Metadata)
					// Si por alguna razón no hay Output, usamos la entry original (fallback)
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

	// 3. INDEXACIÓN UNIFICADA (Fan-In)
	// Ahora 'toIndex' tiene nativo de Dynamo Y las referencias de SSM.
	// Hacemos una sola llamada masiva a DynamoDB.

	if len(toIndex) > 0 {
		idxResults := g.index.Upsert(ctx, toIndex)

		// Mezclamos los resultados del índice.
		// Si falló la indexación de algo que ya se guardó en SSM, lo marcamos como error parcial o warning.
		for key, res := range idxResults {
			if res.Err != nil {
				// Si ya teníamos un resultado de éxito (de SSM), esto es un error de "Consistencia Eventual"
				if existingRes, exists := results[key]; exists && existingRes.Err == nil {
					g.logger.Warn("Data saved in backend but failed to index", zap.String("key", key), zap.Error(res.Err))
					// Opcional: Sobrescribir el error en results si quieres ser estricto
					// results[key] = res
				} else {
					// Si era nativo de Dynamo, este es el error principal
					results[key] = res
				}
			} else {
				// Si era nativo de Dynamo y salió bien, lo registramos
				if _, exists := results[key]; !exists {
					results[key] = res
				}
			}
		}
	}

	return results
}

// Helper simple para buscar en slice (puedes optimizarlo con un map si el batch es grande)
func findEntryIndex(entries []models.Entry, key string) int {
	for i, e := range entries {
		if e.Key == key {
			return i
		}
	}
	return 0
}
