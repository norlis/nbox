package prefix

import (
	"context"
	"fmt"
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
	TypeAllowed []StorageBackendType `dynamodbav:"TypeAllowed"        json:"typeAllowed"`
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

// IsValid reports whether b is a known storage backend.
func (b StorageBackendType) IsValid() bool {
	return slices.Contains(AllBackendTypes(), b)
}

// NormalizeAllowed rebuilds TypeAllowed as the deduplicated set of every backend
// the prefix uses: TypeDefault first, then TypeSecure (when set), then any extra
// entries already in TypeAllowed — preserving first-seen order, skipping empties.
func (c *Config) NormalizeAllowed() {
	seen := make(map[StorageBackendType]struct{})
	out := make([]StorageBackendType, 0, len(c.TypeAllowed)+2)
	add := func(b StorageBackendType) {
		if b == "" {
			return
		}
		if _, ok := seen[b]; ok {
			return
		}
		seen[b] = struct{}{}
		out = append(out, b)
	}
	add(c.TypeDefault)
	add(c.TypeSecure)
	for _, b := range c.TypeAllowed {
		add(b)
	}
	c.TypeAllowed = out
}

// Validate rejects unknown backends. Call before persisting.
func (c *Config) Validate() error {
	if !c.TypeDefault.IsValid() {
		return fmt.Errorf("invalid default backend %q (allowed: %v)", c.TypeDefault, AllBackendTypes())
	}
	if c.TypeSecure != "" && !c.TypeSecure.IsValid() {
		return fmt.Errorf("invalid secure backend %q (allowed: %v)", c.TypeSecure, AllBackendTypes())
	}
	for _, b := range c.TypeAllowed {
		if !b.IsValid() {
			return fmt.Errorf("invalid allowed backend %q (allowed: %v)", b, AllBackendTypes())
		}
	}
	return nil
}

// Store es el contrato de persistencia de configuraciones de prefijo.
type Store interface {
	List(ctx context.Context) ([]Config, error)
	ByPrefix(ctx context.Context, prefix string) (*Config, error)
	Upsert(ctx context.Context, prefixes []Config) (UpsertStats, error)
}
