package schemastrategy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"nbox/internal/domain/models"
)

// JsonStrategy handles JSON file transformation, validation, and formatting.
type JsonStrategy struct{}

// Transform escapes a value for safe injection into a JSON template.
// It marshals the value and removes surrounding quotes.
func (j *JsonStrategy) Transform(value string) string {
	escaped, err := json.Marshal(value)
	if err != nil {
		return value
	}
	return strings.Trim(string(escaped), `"`)
}

// Validate checks that the content is valid JSON syntax.
func (j *JsonStrategy) Validate(content string) error {
	return json.Unmarshal([]byte(content), &json.RawMessage{})
}

// SchemaType returns the schema type this strategy handles.
func (j *JsonStrategy) SchemaType() models.SchemaType {
	return models.JSON
}

// Format applies pretty-print formatting with 2-space indentation.
func (j *JsonStrategy) Format(content []byte) ([]byte, error) {
	var out bytes.Buffer
	if err := json.Indent(&out, content, "", "  "); err != nil {
		return nil, fmt.Errorf("failed to indent JSON: %w", err)
	}
	return out.Bytes(), nil
}
