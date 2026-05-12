package exporter

import (
	"encoding/json"
	"fmt"

	"nbox/internal/entry"
)

type JSON struct{}

func NewJSON() *JSON { return &JSON{} }

func (e *JSON) Export(entries []entry.Entry) ([]byte, error) {
	normalized := make([]entry.Entry, len(entries))
	for i, en := range entries {
		normalized[i] = entry.Entry{
			Key:    en.Key,
			Value:  en.Value,
			Secure: en.Secure,
		}
	}

	data, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("invalid file format: %w", err)
	}

	return data, nil
}
