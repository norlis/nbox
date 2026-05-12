package prefix

import (
	"context"
	"slices"
	"time"
)

type StorageBackendType string

const (
	BackendDynamoDB             StorageBackendType = "dynamodb"
	BackendParameterStore       StorageBackendType = "parameterstore"
	BackendParameterStoreSecure StorageBackendType = "parameterstore_secure"
)

func AllBackendTypes() []StorageBackendType {
	return []StorageBackendType{
		BackendDynamoDB,
		BackendParameterStore,
		BackendParameterStoreSecure,
	}
}

// UpsertStats representa las estadísticas de una operación Upsert.
type UpsertStats struct {
	Processed int
	Failed    int
	Skipped   int
}

type Config struct {
	Prefix      string               `dynamodbav:"Prefix"             json:"prefix"` // PK
	TypeDefault StorageBackendType   `dynamodbav:"TypeDefault"        json:"typeDefault"`
	TypeSecure  StorageBackendType   `dynamodbav:"TypeSecure"         json:"typeSecure"`
	TypeAllowed []StorageBackendType `dynamodbav:"TypeAllowed"        json:"typeAllowed"` // TODO: Deprecated
	Tags        map[string]string    `dynamodbav:"Tags,omitempty"     json:"tags,omitempty"`
	CreatedAt   time.Time            `dynamodbav:"CreatedAt,unixtime" json:"-"`
	UpdatedAt   time.Time            `dynamodbav:"UpdatedAt,unixtime" json:"-"`
	UpdatedBy   string               `dynamodbav:"UpdatedBy"          json:"-"`
}

// IsBackendAllowed verifica si un backend está permitido para este prefijo.
func (c *Config) IsBackendAllowed(backend StorageBackendType) bool {
	if backend == c.TypeDefault {
		return true
	}

	return slices.Contains(c.TypeAllowed, backend)
}

// Store es el contrato de persistencia de configuraciones de prefijo.
type Store interface {
	List(ctx context.Context) ([]Config, error)
	ByPrefix(ctx context.Context, prefix string) (*Config, error)
	Upsert(ctx context.Context, prefixes []Config) (UpsertStats, error)
}
