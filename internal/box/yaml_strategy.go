package box

import (
	"bytes"
	"fmt"
	"strings"

	"go.yaml.in/yaml/v4"
)

// YAMLStrategy handles YAML file transformation, validation, and formatting.
type YAMLStrategy struct{}

func (y *YAMLStrategy) Transform(value string) string {
	if strings.ContainsAny(value, ":{}\n\t#[]!|>&*") || strings.TrimSpace(value) != value {
		return fmt.Sprintf("%q", value)
	}
	return value
}

func (y *YAMLStrategy) Validate(content string) error {
	return yaml.Unmarshal([]byte(content), new(any))
}

func (y *YAMLStrategy) SchemaType() SchemaType {
	return YAML
}

func (y *YAMLStrategy) Format(content []byte) ([]byte, error) {
	var node yaml.Node

	if err := yaml.Unmarshal(content, &node); err != nil {
		return nil, fmt.Errorf("failed to parse YAML AST: %w", err)
	}

	var out bytes.Buffer
	enc := yaml.NewEncoder(&out)
	enc.SetIndent(2)

	if err := enc.Encode(&node); err != nil {
		return nil, fmt.Errorf("failed to format YAML AST: %w", err)
	}

	return out.Bytes(), nil
}
