package boxspec

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"nbox/internal/domain/validation"
)

// SpecRegistry acts as the central registry for spec definitions.
// Keeps specs in memory for fast O(1) access.
type SpecRegistry struct {
	store        SpecStore
	engine       SpecEngine
	specs        map[string]SpecDefinition // ID -> Spec
	patternIndex map[string]string         // pattern -> specID (pre-computed)
	mu           sync.RWMutex
}

// NewSpecRegistry creates a new spec registry.
func NewSpecRegistry(store SpecStore, engine SpecEngine) *SpecRegistry {
	return &SpecRegistry{
		store:        store,
		engine:       engine,
		specs:        make(map[string]SpecDefinition),
		patternIndex: make(map[string]string),
	}
}

// Reload loads (or reloads) all definitions from the store.
// Call this at startup and when hot-reload is triggered.
func (r *SpecRegistry) Reload(ctx context.Context) error {
	loaded, err := r.store.LoadAll(ctx)
	if err != nil {
		return fmt.Errorf("failed to load specs: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Rebuild maps
	r.specs = make(map[string]SpecDefinition, len(loaded))
	r.patternIndex = make(map[string]string)

	for _, spec := range loaded {
		r.specs[spec.ID] = spec
		// Build pattern index for fast lookup
		for _, pattern := range spec.MatchPatterns {
			r.patternIndex[pattern] = spec.ID
		}
	}

	// Invalidate engine cache
	r.engine.InvalidateCache()

	return nil
}

// GetByID retrieves a spec by its unique ID.
func (r *SpecRegistry) GetByID(id string) (SpecDefinition, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if spec, ok := r.specs[id]; ok {
		return spec, nil
	}
	return SpecDefinition{}, fmt.Errorf("spec not found: %s", id)
}

// Resolve finds the best matching spec based on filename.
func (r *SpecRegistry) Resolve(filename string) (SpecDefinition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	base := strings.ToLower(filepath.Base(filename))

	// Check pattern index
	for pattern, specID := range r.patternIndex {
		if matched, _ := filepath.Match(pattern, base); matched {
			if spec, ok := r.specs[specID]; ok {
				return spec, true
			}
		}
	}
	return SpecDefinition{}, false
}

// List returns all available specs (for API).
func (r *SpecRegistry) List() []SpecDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	list := make([]SpecDefinition, 0, len(r.specs))
	for _, spec := range r.specs {
		list = append(list, spec)
	}
	return list
}

// Validate executes validation by delegating to the engine.
func (r *SpecRegistry) Validate(ctx context.Context, specID string, data []byte, format string) (validation.Result, error) {
	spec, err := r.GetByID(specID)
	if err != nil {
		return validation.NewResult(validation.WithInternalError(err)), nil
	}
	result, err := r.engine.Validate(ctx, spec, data, format)
	result.Meta = &spec
	return result, err
}

// ExportJSONSchema resolves a spec by filename pattern and exports it as JSON Schema.
func (r *SpecRegistry) ExportJSONSchema(ctx context.Context, filename string) (SpecDefinition, []byte, error) {
	spec, found := r.Resolve(filename)
	if !found {
		return SpecDefinition{}, nil, fmt.Errorf("no spec found matching: %s", filename)
	}

	data, err := r.engine.ExportJSONSchema(ctx, spec)
	if err != nil {
		return spec, nil, err
	}

	return spec, data, nil
}

// ValidateByFilename resolves spec from filename and validates.
func (r *SpecRegistry) ValidateByFilename(ctx context.Context, filename string, data []byte, format string) (validation.Result, error) {
	spec, found := r.Resolve(filename)
	if !found {
		// No matching spec - skip semantic validation
		return validation.NewResult(), nil
	}
	result, err := r.engine.Validate(ctx, spec, data, format)
	result.Meta = &spec
	return result, err
}
