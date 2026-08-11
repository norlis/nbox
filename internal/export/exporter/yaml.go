package exporter

import (
	"fmt"

	"gopkg.in/yaml.v3"
	"nbox/internal/entry"
)

type YAML struct{}

func NewYAML() *YAML { return &YAML{} }

func (e *YAML) Export(entries []entry.Entry) ([]byte, error) {
	normalized := make([]entry.Entry, len(entries))
	for i, en := range entries {
		normalized[i] = entry.Entry{
			Key:    en.Key,
			Value:  en.Value,
			Secure: en.Secure,
		}
	}

	data, err := yaml.Marshal(normalized)
	if err != nil {
		return nil, fmt.Errorf("invalid file format: %w", err)
	}

	return data, nil
}
