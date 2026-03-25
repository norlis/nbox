package schemastrategy

import "nbox/internal/domain/models"

// TextStrategy handles plain text files with no transformation or validation.
type TextStrategy struct{}

// Transform returns the value unchanged (no escaping needed for plain text).
func (t *TextStrategy) Transform(value string) string { return value }

// Validate always returns nil (plain text is always valid).
func (t *TextStrategy) Validate(content string) error { return nil }

// SchemaType returns the schema type this strategy handles.
func (t *TextStrategy) SchemaType() models.SchemaType { return models.TXT }

// Format returns the content unchanged (no formatting for plain text).
func (t *TextStrategy) Format(content []byte) ([]byte, error) { return content, nil }
