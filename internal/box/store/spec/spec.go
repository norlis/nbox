package spec

import (
	"context"
)

// SpecDefinition represents a validation rule loaded from a CUE file.
type SpecDefinition struct {
	ID            string   `json:"id"`            // Unique identifier (e.g., "aws:ecs:task-def-v1")
	Name          string   `json:"name"`          // Human-readable name
	Version       string   `json:"version"`       // Schema version
	MatchPatterns []string `json:"matchPatterns"` // Glob patterns (e.g., "taskdef*.json")
	RawContent    string   `json:"-"`             // CUE source (not exposed in JSON)
}

// ValidationResult contains the result of a validation.
type ValidationResult struct {
	Valid  bool              `json:"valid"`
	Spec   *SpecDefinition   `json:"spec,omitempty"`
	Errors []ValidationError `json:"errors,omitempty"`
}

// ValidationError represents a single validation error with path context.
type ValidationError struct {
	Path    string `json:"path"` // e.g., "containerDefinitions.0.memory"
	Message string `json:"message"`
}

// NewValidationResult creates a successful validation result.
func NewValidationResult() ValidationResult {
	return ValidationResult{Valid: true}
}

// Failed creates a failed validation result with errors.
func Failed(errors ...ValidationError) ValidationResult {
	return ValidationResult{Valid: false, Errors: errors}
}

// SpecStore loads spec definitions from storage sources (FS, S3, Embed).
type SpecStore interface {
	// LoadAll scans the source and returns all valid specs found.
	LoadAll(ctx context.Context) ([]SpecDefinition, error)
}

// SpecEngine executes validation against loaded specs (CUE, JSON Schema, etc.).
type SpecEngine interface {
	// Validate checks data against a specific spec definition.
	Validate(ctx context.Context, spec SpecDefinition, data []byte, format string) (Result, error)

	// ExportJSONSchema exports a spec definition as JSON Schema.
	ExportJSONSchema(ctx context.Context, spec SpecDefinition) ([]byte, error)

	// InvalidateCache clears any cached compiled schemas (called on reload).
	InvalidateCache()
}
