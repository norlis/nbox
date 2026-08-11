package tracking

import (
	"context"
	"fmt"
	"time"

	"nbox/internal/prefix"
)

type Record struct {
	Key            string                    `dynamodbav:"Key"                   json:"key"`
	Value          string                    `dynamodbav:"Value"                 json:"value"`
	Secure         bool                      `dynamodbav:"Secure"                json:"secure"`
	StorageBackend prefix.StorageBackendType `dynamodbav:"StorageBackend"        json:"storageBackend"`
	UpdatedAt      time.Time                 `dynamodbav:"Timestamp"             json:"updatedAt"`
	UpdatedBy      string                    `dynamodbav:"UpdatedBy"             json:"updatedBy"`
	Action         string                    `dynamodbav:"Action,omitempty"      json:"action,omitempty"`
	Version        int64                     `dynamodbav:"Version,omitempty"     json:"version,omitempty"`
	Hash           string                    `dynamodbav:"Fingerprint,omitempty" json:"hash,omitempty"`
}

func (r *Record) String() string {
	return fmt.Sprintf("Key: %s. Value: %s", r.Key, r.Value)
}

type HistoryConfig struct {
	Since time.Time
	To    time.Time
	Limit int32
}

type HistoryOption func(*HistoryConfig)

func WithHistorySince(t time.Time) HistoryOption {
	return func(c *HistoryConfig) {
		c.Since = t
	}
}

func WithHistoryTo(t time.Time) HistoryOption {
	return func(c *HistoryConfig) {
		c.To = t
	}
}

func WithHistoryLimit(limit int32) HistoryOption {
	return func(c *HistoryConfig) {
		c.Limit = limit
	}
}

// Store es el contrato de persistencia del historial de cambios.
type Store interface {
	CreateBatch(ctx context.Context, tracks []Record) error
	History(ctx context.Context, key string, opts ...HistoryOption) ([]Record, error)
}
