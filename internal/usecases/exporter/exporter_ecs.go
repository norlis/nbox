package exporter

import (
	"encoding/json"
	"fmt"

	"nbox/internal/domain"
	"nbox/internal/domain/models"
)

type ECSTaskDefExporter struct{}

func NewECSTaskDefExporter() *ECSTaskDefExporter {
	return &ECSTaskDefExporter{}
}

type ECSEnvironment struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type ECSSecret struct {
	Name      string `json:"name"`
	ValueFrom string `json:"valueFrom"`
}

// ECSTaskDefinition representa la estructura de salida.
type ECSTaskDefinition struct {
	Environment []ECSEnvironment `json:"environment,omitempty"`
	Secrets     []ECSSecret      `json:"secrets,omitempty"`
}

func (e *ECSTaskDefExporter) Export(entries []models.Entry) ([]byte, error) {
	result := ECSTaskDefinition{
		Environment: []ECSEnvironment{},
		Secrets:     []ECSSecret{},
	}

	for _, entry := range entries {
		envVarName := domain.ConvertToEnvVarName(entry.ShortKey)

		if entry.Secure {
			result.Secrets = append(result.Secrets, ECSSecret{
				Name:      envVarName,
				ValueFrom: entry.Value,
			})
		} else {
			result.Environment = append(result.Environment, ECSEnvironment{
				Name:  envVarName,
				Value: entry.Value,
			})
		}
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("%w: %w", domain.ErrInvalidFileFormat, err)
	}

	return data, nil
}
