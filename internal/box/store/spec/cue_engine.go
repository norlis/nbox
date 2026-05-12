package spec

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/errors"
	cuejson "cuelang.org/go/encoding/json"
	"cuelang.org/go/encoding/openapi"
	cueyaml "cuelang.org/go/encoding/yaml"
)

// CueEngine implementa SpecEngine usando el lenguaje CUE.
type CueEngine struct {
	ctx   *cue.Context
	cache map[string]cue.Value // specID -> compiled #Schema
	mu    sync.RWMutex
}

// NewCueEngine creates a new CUE-based engine.
func NewCueEngine() SpecEngine {
	return &CueEngine{
		ctx:   cuecontext.New(),
		cache: make(map[string]cue.Value),
	}
}

// Validate checks data against a CUE specification.
func (e *CueEngine) Validate(_ context.Context, def SpecDefinition, data []byte, format string) (Result, error) {
	schema, err := e.getOrCompile(def)
	if err != nil {
		return NewResult(
			WithInternalError(fmt.Errorf("failed to compile spec: %w", err)),
		), err
	}

	dataVal, parseErr := e.parseData(data, format)
	if parseErr != nil {
		return NewResult(
			WithSyntaxError(parseErr),
			WithSchemaErrors([]Error{
				{Path: "parse", Message: fmt.Sprintf("failed to parse %s: %v", format, parseErr)},
			}),
		), parseErr
	}

	unified := schema.Unify(dataVal)

	if err := unified.Validate(); err != nil {
		return e.parseErrors(err), nil
	}

	return NewResult(), nil
}

// ExportJSONSchema exports only the #Schema definition from a CUE spec as JSON Schema.
func (e *CueEngine) ExportJSONSchema(_ context.Context, def SpecDefinition) ([]byte, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	val := e.ctx.CompileString(def.RawContent)
	if val.Err() != nil {
		return nil, fmt.Errorf("failed to compile spec: %w", val.Err())
	}

	schema := val.LookupPath(cue.ParsePath("#Schema"))
	if !schema.Exists() {
		return nil, fmt.Errorf("spec %s missing #Schema definition", def.ID)
	}

	wrapped := e.ctx.CompileString("#Schema: _", cue.Scope(val))
	wrapped = wrapped.FillPath(cue.ParsePath("#Schema"), schema)

	f, err := openapi.Generate(wrapped, &openapi.Config{
		SelfContained: true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to generate OpenAPI schema: %w", err)
	}

	built := e.ctx.BuildFile(f)
	if built.Err() != nil {
		return nil, fmt.Errorf("failed to build OpenAPI value: %w", built.Err())
	}

	schemaComponent := built.LookupPath(cue.ParsePath("components.schemas.Schema"))
	if !schemaComponent.Exists() {
		return nil, errors.New("failed to extract Schema from OpenAPI output")
	}

	var raw any
	if err := schemaComponent.Decode(&raw); err != nil {
		return nil, fmt.Errorf("failed to decode JSON Schema: %w", err)
	}

	b, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSON Schema: %w", err)
	}

	return b, nil
}

// InvalidateCache clears the compiled schema cache.
func (e *CueEngine) InvalidateCache() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cache = make(map[string]cue.Value)
	e.ctx = cuecontext.New()
}

func (e *CueEngine) getOrCompile(def SpecDefinition) (cue.Value, error) {
	e.mu.RLock()
	if cached, ok := e.cache[def.ID]; ok {
		e.mu.RUnlock()
		return cached, nil
	}
	e.mu.RUnlock()

	e.mu.Lock()
	defer e.mu.Unlock()

	if cached, ok := e.cache[def.ID]; ok {
		return cached, nil
	}

	val := e.ctx.CompileString(def.RawContent)
	if val.Err() != nil {
		return cue.Value{}, fmt.Errorf("invalid spec definition: %w", val.Err())
	}

	schema := val.LookupPath(cue.ParsePath("#Schema"))
	if !schema.Exists() {
		return cue.Value{}, fmt.Errorf("spec %s missing #Schema definition", def.ID)
	}

	e.cache[def.ID] = schema
	return schema, nil
}

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

func (e *CueEngine) parseErrors(err error) Result {
	var validationErrors []Error

	for _, ee := range errors.Errors(err) {
		path := strings.Join(ee.Path(), ".")
		msg := ee.Error()
		msg = strings.ReplaceAll(msg, "#Schema.", "")

		validationErrors = append(validationErrors, Error{
			Path:    path,
			Message: msg,
		})
	}

	return NewResult(
		WithSchemaErrors(validationErrors),
	)
}
