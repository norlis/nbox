package box

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// JSONStrategy handles JSON file transformation, validation, and formatting.
type JSONStrategy struct{}

func (j *JSONStrategy) Transform(value string) string {
	escaped, err := json.Marshal(value)
	if err != nil {
		return value
	}
	return strings.Trim(string(escaped), `"`)
}

func (j *JSONStrategy) Validate(content string) error {
	return json.Unmarshal([]byte(content), &json.RawMessage{})
}

func (j *JSONStrategy) SchemaType() SchemaType {
	return JSON
}

func (j *JSONStrategy) Format(content []byte) ([]byte, error) {
	var out bytes.Buffer
	if err := json.Indent(&out, content, "", "  "); err != nil {
		return nil, fmt.Errorf("failed to indent JSON: %w", err)
	}
	return out.Bytes(), nil
}
