package exporter

import (
	"encoding/json"
	"fmt"

	"nbox/internal/entry"
)

type ECS struct{}

func NewECS() *ECS { return &ECS{} }

type ecsEnvironment struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type ecsSecret struct {
	Name      string `json:"name"`
	ValueFrom string `json:"valueFrom"`
}

type ecsTaskDefinition struct {
	Environment []ecsEnvironment `json:"environment,omitempty"`
	Secrets     []ecsSecret      `json:"secrets,omitempty"`
}

func (e *ECS) Export(entries []entry.Entry) ([]byte, error) {
	result := ecsTaskDefinition{
		Environment: []ecsEnvironment{},
		Secrets:     []ecsSecret{},
	}

	for _, en := range entries {
		envVarName := entry.ConvertToEnvVarName(en.ShortKey)

		if en.Secure {
			result.Secrets = append(result.Secrets, ecsSecret{
				Name:      envVarName,
				ValueFrom: en.Value,
			})
		} else {
			result.Environment = append(result.Environment, ecsEnvironment{
				Name:  envVarName,
				Value: en.Value,
			})
		}
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("invalid file format: %w", err)
	}

	return data, nil
}
