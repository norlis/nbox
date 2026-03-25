package strategies

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"nbox/internal/domain/models"
	schemastrategy2 "nbox/internal/domain/strategies/schemastrategy"
)

// StrategyResolver acts as a Facade to simplify strategy retrieval.
// Uses models.ExtensionMap as the single source of truth for extension mapping.
type StrategyResolver struct {
	strategies map[models.SchemaType]SchemaStrategy
	extCache   map[string]SchemaStrategy // Direct extension → strategy cache
	mu         sync.RWMutex
}

// NewStrategyResolver creates a new resolver with default strategies registered.
func NewStrategyResolver() *StrategyResolver {
	r := &StrategyResolver{
		strategies: make(map[models.SchemaType]SchemaStrategy),
		extCache:   make(map[string]SchemaStrategy),
	}

	r.Register(&schemastrategy2.JsonStrategy{})
	r.Register(&schemastrategy2.YamlStrategy{})
	r.Register(&schemastrategy2.TextStrategy{})

	return r
}

// Register allows adding or overwriting handlers dynamically at runtime.
// Updates the extension cache using models.ExtensionMap as source of truth.
func (r *StrategyResolver) Register(strategy SchemaStrategy) {
	r.mu.Lock()
	defer r.mu.Unlock()

	schemaType := strategy.SchemaType()
	r.strategies[schemaType] = strategy

	// Update extension cache for all extensions that map to this schema type
	for ext, st := range models.ExtensionMap {
		if st == schemaType {
			r.extCache[ext] = strategy
		}
	}
}

// Resolve determines the correct strategy based on the filename extension.
func (r *StrategyResolver) Resolve(filename string) SchemaStrategy {
	ext := strings.ToLower(filepath.Ext(filename))

	r.mu.RLock()
	defer r.mu.RUnlock()

	// Fast path: O(1) lookup by extension
	if strategy, ok := r.extCache[ext]; ok {
		return strategy
	}

	// Fallback to default text strategy
	if defaultStrat, ok := r.strategies[models.TXT]; ok {
		return defaultStrat
	}

	// Last resort fallback (safety net)
	return &schemastrategy2.TextStrategy{}
}

// GetImplementedSchemaTypes returns all schema types that have registered strategies.
func (r *StrategyResolver) GetImplementedSchemaTypes() []models.SchemaType {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make([]models.SchemaType, 0, len(r.strategies))
	for schemaType := range r.strategies {
		types = append(types, schemaType)
	}

	slices.Sort(types)
	return types
}

// Process orquesta la decodificación, validación, formateo y cálculo de integridad.
// Retorna los bytes listos para guardar y su hash SHA-256.
func (r *StrategyResolver) Process(filename, contentBase64 string) ([]byte, string, error) {
	raw, err := base64.StdEncoding.DecodeString(contentBase64)
	if err != nil {
		return nil, "", fmt.Errorf("base64 decode failed: %w", err)
	}

	strategy := r.Resolve(filename)

	if err = strategy.Validate(string(raw)); err != nil {
		return nil, "", fmt.Errorf("validation failed for %s: %w", strategy.SchemaType(), err)
	}

	formatted, err := strategy.Format(raw)
	if err != nil {
		return nil, "", fmt.Errorf("formatting failed: %w", err)
	}

	hash := sha256.Sum256(formatted)

	return formatted, hex.EncodeToString(hash[:]), nil
}
