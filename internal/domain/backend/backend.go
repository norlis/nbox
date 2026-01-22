package backend

import "time"

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

// UpsertStats representa las estadísticas de una operación Upsert
type UpsertStats struct {
	Processed int
	Failed    int
	Skipped   int
}

type PrefixConfig struct {
	Prefix      string               `json:"prefix" dynamodbav:"Prefix"` // PK
	TypeDefault StorageBackendType   `json:"typeDefault" dynamodbav:"TypeDefault"`
	TypeSecure  StorageBackendType   `json:"typeSecure" dynamodbav:"TypeSecure"`
	TypeAllowed []StorageBackendType `json:"typeAllowed" dynamodbav:"TypeAllowed"` // TODO: Deprecated
	Tags        map[string]string    `json:"tags,omitempty" dynamodbav:"Tags,omitempty"`
	CreatedAt   time.Time            `json:"-" dynamodbav:"CreatedAt,unixtime"`
	UpdatedAt   time.Time            `json:"-" dynamodbav:"UpdatedAt,unixtime"`
	UpdatedBy   string               `json:"-" dynamodbav:"UpdatedBy"`
}

// IsBackendAllowed verifica si un backend está permitido para este prefijo.
func (c *PrefixConfig) IsBackendAllowed(backend StorageBackendType) bool {
	if backend == c.TypeDefault {
		return true
	}

	for _, allowed := range c.TypeAllowed {
		if backend == allowed {
			return true
		}
	}

	return false
}
