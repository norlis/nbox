package exporter

import (
	"fmt"

	"gopkg.in/yaml.v3"
	"nbox/internal/domain"
	"nbox/internal/domain/models"
)

type YAMLExporter struct{}

func NewYAMLExporter() *YAMLExporter {
	return &YAMLExporter{}
}

func (e *YAMLExporter) Export(entries []models.Entry) ([]byte, error) {
	normalized := make([]models.Entry, len(entries))
	for i, entry := range entries {
		normalized[i] = models.Entry{
			Key:    fmt.Sprintf("%s/%v", entry.Path, entry.Key),
			Value:  entry.Value,
			Secure: entry.Secure,
		}
	}

	data, err := yaml.Marshal(normalized)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", domain.ErrInvalidFileFormat, err)
	}

	return data, nil
}
