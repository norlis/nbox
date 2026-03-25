package strategies

import "nbox/internal/domain/models"

// SchemaStrategy defines the behavior for each file type.
type SchemaStrategy interface {
	// SchemaType returns the schema type this strategy handles (e.g., JSON, YAML).
	SchemaType() models.SchemaType

	// Transform escapes an individual value for safe insertion into a template.
	Transform(value string) string

	// Validate checks that the final content is syntactically valid.
	Validate(content string) error

	// Format applies pretty-print or schema-specific formatting.
	Format(content []byte) ([]byte, error)
}
