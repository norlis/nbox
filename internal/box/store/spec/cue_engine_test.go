package spec

import (
	"bytes"
	"context"
	"encoding/json"
	"sync"
	"testing"
)

// inlineSpec is a minimal self-contained CUE spec fixture used across all tests.
// Using an inline fixture avoids fragile relative-path dependencies on asset files.
// The shape matches what ExportJSONSchema expects: a #Schema definition that
// openapi.Generate can turn into components.schemas.Schema.
const inlineSpec = `
#Meta: {
	id:      "test:simple:v1"
	name:    "Simple Test Schema"
	version: "1.0"
}

#Schema: {
	name!: string
	port!: int & >=1 & <=65535
}
`

func newTestDef() SpecDefinition {
	return SpecDefinition{
		ID:         "test:simple:v1",
		Name:       "Simple Test Schema",
		Version:    "1.0",
		RawContent: inlineSpec,
	}
}

// conformingJSON satisfies #Schema.
const conformingJSON = `{"name":"myservice","port":8080}`

// nonConformingJSON has port > 65535.
const nonConformingJSON = `{"name":"myservice","port":99999}`

// wrongTypeJSON has the wrong type for "name" (int instead of string).
const wrongTypeJSON = `{"name":42,"port":8080}`

// ─── Characterization test 1: ExportJSONSchema returns non-empty valid JSON ──

func TestExportJSONSchema_ReturnsValidJSON(t *testing.T) {
	engine := NewCueEngine()
	ctx := context.Background()
	def := newTestDef()

	b, err := engine.ExportJSONSchema(ctx, def)
	if err != nil {
		t.Fatalf("ExportJSONSchema returned unexpected error: %v", err)
	}
	if len(b) == 0 {
		t.Fatal("ExportJSONSchema returned empty bytes")
	}
	if !json.Valid(b) {
		t.Fatalf("ExportJSONSchema returned invalid JSON:\n%s", string(b))
	}

	// The JSON Schema must contain "properties" with "name" and "port".
	var schema map[string]any
	if err := json.Unmarshal(b, &schema); err != nil {
		t.Fatalf("failed to unmarshal JSON Schema: %v", err)
	}
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("JSON Schema missing 'properties' key; got: %s", string(b))
	}
	if _, ok := props["name"]; !ok {
		t.Error("JSON Schema properties missing 'name' field")
	}
	if _, ok := props["port"]; !ok {
		t.Error("JSON Schema properties missing 'port' field")
	}
}

// ─── Characterization test 2: Idempotence — two calls are byte-for-byte equal ─

func TestExportJSONSchema_Idempotent(t *testing.T) {
	engine := NewCueEngine()
	ctx := context.Background()
	def := newTestDef()

	b1, err := engine.ExportJSONSchema(ctx, def)
	if err != nil {
		t.Fatalf("first ExportJSONSchema call failed: %v", err)
	}
	b2, err := engine.ExportJSONSchema(ctx, def)
	if err != nil {
		t.Fatalf("second ExportJSONSchema call failed: %v", err)
	}
	if !bytes.Equal(b1, b2) {
		t.Fatalf("ExportJSONSchema is not idempotent:\nfirst:  %s\nsecond: %s",
			string(b1), string(b2))
	}
}

// ─── Characterization test 3a: Validate conforming data → Valid=true ─────────

func TestValidate_ConformingData_OK(t *testing.T) {
	engine := NewCueEngine()
	ctx := context.Background()
	def := newTestDef()

	result, err := engine.Validate(ctx, def, []byte(conformingJSON), "json")
	if err != nil {
		t.Fatalf("Validate returned unexpected error: %v", err)
	}
	if !result.Valid {
		t.Fatalf("expected Valid=true for conforming data, got errors: %+v", result.Errors)
	}
	if result.Kind != KindSuccess {
		t.Errorf("expected KindSuccess, got %q", result.Kind)
	}
}

// ─── Characterization test 3b: Validate non-conforming data → Valid=false ────

func TestValidate_NonConformingData_Invalid(t *testing.T) {
	engine := NewCueEngine()
	ctx := context.Background()
	def := newTestDef()

	result, err := engine.Validate(ctx, def, []byte(nonConformingJSON), "json")
	if err != nil {
		t.Fatalf("Validate returned unexpected error: %v", err)
	}
	if result.Valid {
		t.Fatal("expected Valid=false for non-conforming data (port out of range)")
	}
	if len(result.Errors) == 0 {
		t.Fatal("expected at least one validation error, got none")
	}
}

// ─── Characterization test 3c: Validate wrong-type field → Valid=false ───────
// Note: CUE's open-world unification means absent fields are not immediately
// rejected (they remain "unsatisfied" constraints rather than errors).
// A type mismatch (int where string expected) IS a concrete error.

func TestValidate_WrongTypeField_Invalid(t *testing.T) {
	engine := NewCueEngine()
	ctx := context.Background()
	def := newTestDef()

	result, err := engine.Validate(ctx, def, []byte(wrongTypeJSON), "json")
	if err != nil {
		t.Fatalf("Validate returned unexpected error: %v", err)
	}
	if result.Valid {
		t.Fatal("expected Valid=false for data with wrong type (name: int instead of string)")
	}
	if len(result.Errors) == 0 {
		t.Fatal("expected at least one validation error, got none")
	}
}

// ─── Characterization test 4: InvalidateCache preserves correctness ──────────
// After invalidating, ExportJSONSchema must still return the same bytes.

func TestExportJSONSchema_AfterInvalidateCache_SameOutput(t *testing.T) {
	engine := NewCueEngine()
	ctx := context.Background()
	def := newTestDef()

	before, err := engine.ExportJSONSchema(ctx, def)
	if err != nil {
		t.Fatalf("ExportJSONSchema before invalidate failed: %v", err)
	}

	engine.InvalidateCache()

	after, err := engine.ExportJSONSchema(ctx, def)
	if err != nil {
		t.Fatalf("ExportJSONSchema after invalidate failed: %v", err)
	}

	if !bytes.Equal(before, after) {
		t.Fatalf("ExportJSONSchema output changed after InvalidateCache:\nbefore: %s\nafter:  %s",
			string(before), string(after))
	}
}

// ─── Cache-inspection test 5: schemaJSON cache is populated after first call ──
// Requires the schemaJSON field added in the optimization step.

func TestExportJSONSchema_CachePopulated(t *testing.T) {
	e := NewCueEngine().(*CueEngine)
	ctx := context.Background()
	def := newTestDef()

	// Before any call: cache must be empty.
	e.mu.RLock()
	_, cached := e.schemaJSON[def.ID]
	e.mu.RUnlock()
	if cached {
		t.Fatal("expected schemaJSON cache to be empty before first call")
	}

	_, err := e.ExportJSONSchema(ctx, def)
	if err != nil {
		t.Fatalf("ExportJSONSchema failed: %v", err)
	}

	// After first call: cache must be populated.
	e.mu.RLock()
	_, cached = e.schemaJSON[def.ID]
	e.mu.RUnlock()
	if !cached {
		t.Fatal("expected schemaJSON cache to be populated after first call")
	}
}

// ─── Cache-inspection test 6: InvalidateCache clears schemaJSON ──────────────

func TestInvalidateCache_ClearsSchemaJSON(t *testing.T) {
	e := NewCueEngine().(*CueEngine)
	ctx := context.Background()
	def := newTestDef()

	_, err := e.ExportJSONSchema(ctx, def)
	if err != nil {
		t.Fatalf("ExportJSONSchema failed: %v", err)
	}

	e.InvalidateCache()

	e.mu.RLock()
	n := len(e.schemaJSON)
	e.mu.RUnlock()
	if n != 0 {
		t.Fatalf("expected schemaJSON empty after InvalidateCache, got %d entries", n)
	}
}

// ─── Concurrency test 7: no races, always valid JSON ─────────────────────────
// Run with: go test -race ./internal/box/store/spec/

func TestExportJSONSchema_Concurrent(t *testing.T) {
	const goroutines = 20
	const iterations = 10

	engine := NewCueEngine()
	ctx := context.Background()
	def := newTestDef()

	var wg sync.WaitGroup
	errs := make(chan error, goroutines*iterations)

	for g := range goroutines {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := range iterations {
				// Goroutine 0 invalidates every 5 iterations to stress the path.
				if id == 0 && i%5 == 0 {
					engine.InvalidateCache()
				}
				b, err := engine.ExportJSONSchema(ctx, def)
				if err != nil {
					errs <- err
					return
				}
				if !json.Valid(b) {
					errs <- &invalidJSONError{data: b}
					return
				}
			}
		}(g)
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent call failed: %v", err)
	}
}

type invalidJSONError struct{ data []byte }

func (e *invalidJSONError) Error() string {
	return "invalid JSON returned from ExportJSONSchema: " + string(e.data)
}
