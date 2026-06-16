package exporter

import (
	"bytes"
	"fmt"
	"strings"

	"nbox/internal/entry"
)

type Dotenv struct{}

func NewDotenv() *Dotenv { return &Dotenv{} }

func (e *Dotenv) Export(entries []entry.Entry) ([]byte, error) {
	var buf bytes.Buffer
	buf.Grow(len(entries) * 64)

	for _, en := range entries {
		rawKey := en.ShortKey
		if rawKey == "" {
			rawKey = en.Key
		}
		key := entry.ConvertToEnvVarName(rawKey)
		value := e.escapeValue(en.Value)
		_, _ = fmt.Fprintf(&buf, "%s=%s\n", key, value)
	}

	return buf.Bytes(), nil
}

func (e *Dotenv) escapeValue(value string) string {
	if strings.ContainsAny(value, " \t\n\"'#$\\") {
		escaped := strings.ReplaceAll(value, "\\", "\\\\")
		escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
		return fmt.Sprintf("%q", escaped)
	}
	return value
}
