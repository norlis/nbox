package storage

import (
	"context"
	"fmt"
	"nbox/internal/domain"
	"nbox/internal/domain/backend"
	"nbox/internal/domain/models"
	"nbox/internal/domain/models/operations"
	"sync"

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
) *Gateway {
	return &Gateway{
		backends:   make(map[backend.StorageBackendType]domain.EntryPartialStore),
		index:      index,
		logger:     logger,
		prefixRepo: prefixRepo,
	}
}

func (c *Gateway) RegisterBackend(t backend.StorageBackendType, adapter domain.EntryPartialStore) {
	c.backends[t] = adapter
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

// Retrieve optimizado: Check Index First
func (g *Gateway) Retrieve(ctx context.Context, key string, opts ...domain.RetrieveOption) (*models.Entry, error) {
	// 1. Paso 1: Consultar SIEMPRE al Índice (DynamoDB) primero.
	// Esto actúa como nuestra "cache de ruta" y fuente de metadatos.
	indexEntry, err := g.index.Retrieve(ctx, key)
	if err != nil {
		return nil, err
	}
	if indexEntry == nil {
		return nil, nil // No existe ni en el índice
	}

	// 2. Paso 2: "Service Discovery" a nivel de dato.
	// Miramos la metadata para saber qué backend tiene el dato real.
	backendType := indexEntry.Metadata.StorageBackend

	// Si no tiene backend definido (legacy) o dice "dynamodb",
	// entonces YA tenemos el dato (lo acabamos de leer del índice). ¡Eficiencia x2!
	if backendType == "" || backendType == backend.BackendDynamoDB {
		return indexEntry, nil
	}

	// 3. Paso 3: Si vive en otro lado (ej. SSM), delegamos.
	store, ok := g.backends[backendType]
	if !ok {
		return nil, fmt.Errorf("backend %s not registered", backendType)
	}

	return store.Retrieve(ctx, key, opts...)
}

// Resolve optimizado
func (g *Gateway) Resolve(ctx context.Context, key string) ([]byte, error) {
	// Misma lógica: Preguntar al índice dónde está el secreto
	indexEntry, err := g.index.Retrieve(ctx, key)
	if err != nil || indexEntry == nil {
		return nil, err
	}

	backendType := indexEntry.Metadata.StorageBackend

	// Si es Dynamo, resolvemos localmente (ej. desencriptar si fuera necesario)
	if backendType == backend.BackendDynamoDB {
		return []byte(indexEntry.Value), nil
	}

	// Si es externo, vamos directo a la fuente
	if store, ok := g.backends[backendType]; ok {
		return store.Resolve(ctx, key)
	}

	return nil, fmt.Errorf("backend desconocido: %s", backendType)
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

// Tracking SIEMPRE usa el Índice Global (DynamoDB)
func (g *Gateway) Tracking(ctx context.Context, key string) ([]models.Tracking, error) {
	return g.index.Tracking(ctx, key)
}

func (g *Gateway) Upsert(ctx context.Context, entries []models.Entry) operations.Results {
	// 1. AGRUPACIÓN (Serial)
	// Agrupamos las entradas por instancia de Backend para aprovechar sus métodos Upsert por lotes.
	groupedEntries := make(map[domain.EntryPartialStore][]models.Entry)
	results := make(operations.Results)

	// Mutex para proteger el mapa de resultados durante la ejecución paralela
	var mu sync.Mutex

	for _, entry := range entries {
		store, err := g.resolveBackend(ctx, entry.Key, entry.Secure)
		if err != nil {
			// Fallos de resolución se agregan inmediatamente (fase serial, sin lock)
			results.Add(entry.Key, operations.Failed, err)
			continue
		}
		groupedEntries[store] = append(groupedEntries[store], entry)
	}

	var wg sync.WaitGroup

	for store, batch := range groupedEntries {
		// Go 1.22+ captura las variables del loop (store, batch) de forma segura por defecto.

		wg.Go(func() {
			// A. Escritura en el Backend "Source of Truth" (Batch)
			opResults := store.Upsert(ctx, batch)

			// Slice temporal para las entradas que deben ir al índice
			toIndex := make([]models.Entry, 0, len(batch))

			// B. Procesamiento de resultados y preparación para Índice
			mu.Lock()
			for _, entry := range batch {
				res, ok := opResults[entry.Key]
				if !ok {
					continue
				}

				// Guardamos el resultado global
				results.Add(entry.Key, res.Action, res.Err)

				// Lógica de Indexación (Solo si hubo éxito en el primario)
				// Y solo si el backend actual NO es ya el índice global
				if res.Err == nil && store != g.index {
					// Lógica de Transparencia V2:
					// Si el backend nos devolvió una versión modificada (Output), usamos esa.
					// Si no, usamos la original.
					if res.Output != nil {
						toIndex = append(toIndex, *res.Output)
					} else {
						toIndex = append(toIndex, entry)
					}
				}
			}
			mu.Unlock()

			// C. Escritura en Índice Global (DynamoDB) - "Sidecar Indexing"
			// Se ejecuta fuera del Lock para no bloquear a otras goroutines
			if len(toIndex) > 0 {
				idxRes := g.index.Upsert(ctx, toIndex)

				// Solo logueamos errores de indexación, no fallamos la operación principal
				for key, res := range idxRes {
					if res.Err != nil {
						g.logger.Warn("Failed to update global index",
							zap.String("key", key),
							zap.Error(res.Err),
						)
					}
				}
			}
		})
	}

	// Esperamos a que todos los backends terminen
	wg.Wait()

	return results
}
