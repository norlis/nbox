package backend

import (
	"slices"
	"time"
)

type StorageBackendType string

const (
	BackendDynamoDB             StorageBackendType = "dynamodb"
	BackendParameterStore       StorageBackendType = "parameterstore"
	BackendParameterStoreSecure StorageBackendType = "parameterstore_secure"
)

func GetAllBackendTypes() []StorageBackendType {
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

type PrefixConfig struct {
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
func (c *PrefixConfig) IsBackendAllowed(backend StorageBackendType) bool {
	if backend == c.TypeDefault {
		return true
	}

	return slices.Contains(c.TypeAllowed, backend)
}
