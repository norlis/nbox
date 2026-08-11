package box

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

// StrategyResolver selects the right SchemaStrategy by file extension.
type StrategyResolver struct {
	strategies map[SchemaType]SchemaStrategy
	extCache   map[string]SchemaStrategy
	mu         sync.RWMutex
}

func NewStrategyResolver() *StrategyResolver {
	r := &StrategyResolver{
		strategies: make(map[SchemaType]SchemaStrategy),
		extCache:   make(map[string]SchemaStrategy),
	}
	r.Register(&JSONStrategy{})
	r.Register(&YAMLStrategy{})
	r.Register(&TextStrategy{})
	return r
}

func (r *StrategyResolver) Register(strategy SchemaStrategy) {
	r.mu.Lock()
	defer r.mu.Unlock()

	schemaType := strategy.SchemaType()
	r.strategies[schemaType] = strategy

	for ext, st := range ExtensionMap {
		if st == schemaType {
			r.extCache[ext] = strategy
		}
	}
}

func (r *StrategyResolver) Resolve(filename string) SchemaStrategy {
	ext := strings.ToLower(filepath.Ext(filename))

	r.mu.RLock()
	defer r.mu.RUnlock()

	if strategy, ok := r.extCache[ext]; ok {
		return strategy
	}

	if defaultStrat, ok := r.strategies[TXT]; ok {
		return defaultStrat
	}

	return &TextStrategy{}
}

func (r *StrategyResolver) GetImplementedSchemaTypes() []SchemaType {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make([]SchemaType, 0, len(r.strategies))
	for schemaType := range r.strategies {
		types = append(types, schemaType)
	}

	slices.Sort(types)
	return types
}

// Process decodes a base64 content string, validates it, formats it, and returns
// the canonical bytes plus its SHA-256 hex hash.
func (r *StrategyResolver) Process(filename, contentBase64 string) (formatted []byte, checksum string, err error) {
	raw, err := base64.StdEncoding.DecodeString(contentBase64)
	if err != nil {
		return nil, "", fmt.Errorf("base64 decode failed: %w", err)
	}

	strategy := r.Resolve(filename)

	if err = strategy.Validate(string(raw)); err != nil {
		return nil, "", fmt.Errorf("validation failed for %s: %w", strategy.SchemaType(), err)
	}

	formatted, err = strategy.Format(raw)
	if err != nil {
		return nil, "", fmt.Errorf("formatting failed: %w", err)
	}

	hash := sha256.Sum256(formatted)

	return formatted, hex.EncodeToString(hash[:]), nil
}
