package spec

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/errors"
	cuejson "cuelang.org/go/encoding/json"
	cueyaml "cuelang.org/go/encoding/yaml"
	"nbox/internal/domain/boxspec"
	"nbox/internal/domain/validation"
)

// CueEngine implements boxspec.SpecEngine using CUE language.
type CueEngine struct {
	ctx   *cue.Context
	cache map[string]cue.Value // specID -> compiled #Schema
	mu    sync.RWMutex
}

// NewCueEngine creates a new CUE-based engine.
func NewCueEngine() boxspec.SpecEngine {
	return &CueEngine{
		ctx:   cuecontext.New(),
		cache: make(map[string]cue.Value),
	}
}

// Validate checks data against a CUE specification.
func (e *CueEngine) Validate(ctx context.Context, spec boxspec.SpecDefinition, data []byte, format string) (validation.Result, error) {
	// 1. Get or compile schema (cached)
	schema, err := e.getOrCompile(spec)
	if err != nil {
		return validation.NewResult(
			validation.WithInternalError(fmt.Errorf("failed to compile spec: %w", err)),
		), err
	}

	// 2. Parse input data
	dataVal, parseErr := e.parseData(data, format)
	if parseErr != nil {
		return validation.NewResult(
			validation.WithSyntaxError(parseErr),
			validation.WithSchemaErrors([]validation.Error{
				{Path: "parse", Message: fmt.Sprintf("failed to parse %s: %v", format, parseErr)},
			}),
		), parseErr
	}

	// 3. Unify schema with data
	unified := schema.Unify(dataVal)

	// 4. Validate
	if err := unified.Validate(); err != nil {
		return e.parseErrors(err), nil
	}

	return validation.NewResult(), nil
}

// InvalidateCache clears the compiled schema cache.
func (e *CueEngine) InvalidateCache() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cache = make(map[string]cue.Value)
	// Recreate context to free memory
	e.ctx = cuecontext.New()
}

// getOrCompile retrieves a cached schema or compiles it.
func (e *CueEngine) getOrCompile(spec boxspec.SpecDefinition) (cue.Value, error) {
	// Fast path: check cache
	e.mu.RLock()
	if cached, ok := e.cache[spec.ID]; ok {
		e.mu.RUnlock()
		return cached, nil
	}
	e.mu.RUnlock()

	// Slow path: compile and cache
	e.mu.Lock()
	defer e.mu.Unlock()

	// Double-check after acquiring write lock
	if cached, ok := e.cache[spec.ID]; ok {
		return cached, nil
	}

	// Compile the spec
	val := e.ctx.CompileString(spec.RawContent)
	if val.Err() != nil {
		return cue.Value{}, fmt.Errorf("invalid spec definition: %w", val.Err())
	}

	// Extract #Schema
	schema := val.LookupPath(cue.ParsePath("#Schema"))
	if !schema.Exists() {
		return cue.Value{}, fmt.Errorf("spec %s missing #Schema definition", spec.ID)
	}

	e.cache[spec.ID] = schema
	return schema, nil
}

// parseData parses JSON or YAML into a CUE value.
func (e *CueEngine) parseData(data []byte, format string) (cue.Value, error) {
	switch strings.ToLower(format) {
	case "json":
		expr, err := cuejson.Extract("input.json", data)
		if err != nil {
			return cue.Value{}, err
		}
		return e.ctx.BuildExpr(expr), nil

	case "yaml", "yml":
		file, err := cueyaml.Extract("input.yaml", data)
		if err != nil {
			return cue.Value{}, err
		}
		return e.ctx.BuildFile(file), nil

	default:
		return cue.Value{}, fmt.Errorf("unsupported format: %s", format)
	}
}

// parseErrors converts CUE errors to ValidationErrors.
func (e *CueEngine) parseErrors(err error) validation.Result {
	var validationErrors []validation.Error

	for _, ee := range errors.Errors(err) {
		path := strings.Join(ee.Path(), ".")
		msg := ee.Error()
		msg = strings.ReplaceAll(msg, "#Schema.", "")

		validationErrors = append(validationErrors, validation.Error{
			Path:    path,
			Message: msg,
		})
	}

	return validation.NewResult(
		validation.WithSchemaErrors(validationErrors),
	)
}
