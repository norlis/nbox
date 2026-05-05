package exporter

import (
	"encoding/json"
	"fmt"

	"nbox/internal/domain"
	"nbox/internal/domain/models"
)

type JSONExporter struct{}

func NewJSONExporter() *JSONExporter {
	return &JSONExporter{}
}

func (e *JSONExporter) Export(entries []models.Entry) ([]byte, error) {
	normalized := make([]models.Entry, len(entries))
	for i, entry := range entries {
		normalized[i] = models.Entry{
			Key:    entry.Key,
			Value:  entry.Value,
			Secure: entry.Secure,
		}
	}

	data, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("%w: %w", domain.ErrInvalidFileFormat, err)
	}

	return data, nil
}
