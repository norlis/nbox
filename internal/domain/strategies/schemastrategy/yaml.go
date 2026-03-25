package schemastrategy

import (
	"bytes"
	"fmt"
	"strings"

	"go.yaml.in/yaml/v4"
	"nbox/internal/domain/models"
)

// YamlStrategy handles YAML file transformation, validation, and formatting.
type YamlStrategy struct{}

// Transform escapes a value for safe injection into a YAML template.
// YAML is complex - if the value contains special characters that could break
// the structure (like ':', '{', '}', newlines, '#'), we force double quoting.
func (y *YamlStrategy) Transform(value string) string {
	if strings.ContainsAny(value, ":{}\n\t#[]!|>&*") || strings.TrimSpace(value) != value {
		return fmt.Sprintf("%q", value)
	}
	return value
}

// Validate checks that the content is valid YAML syntax.
// Uses 'new(interface{})' to accept any valid structure (map, list, scalar).
func (y *YamlStrategy) Validate(content string) error {
	return yaml.Unmarshal([]byte(content), new(any))
}

// SchemaType returns the schema type this strategy handles.
func (y *YamlStrategy) SchemaType() models.SchemaType {
	return models.YAML
}

// Format applies canonical YAML formatting to the content.
// It parses and re-serializes the YAML to normalize whitespace and ordering.
func (y *YamlStrategy) Format(content []byte) ([]byte, error) {
	var node yaml.Node

	// Decodificar en un Nodo AST (preserva comentarios)
	if err := yaml.Unmarshal(content, &node); err != nil {
		return nil, fmt.Errorf("failed to parse YAML AST: %w", err)
	}

	// Codificar de nuevo (aplica indentación estándar de 2 espacios)
	var out bytes.Buffer
	enc := yaml.NewEncoder(&out)
	enc.SetIndent(2)

	if err := enc.Encode(&node); err != nil {
		return nil, fmt.Errorf("failed to format YAML AST: %w", err)
	}

	return out.Bytes(), nil
}
