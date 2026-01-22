package models

import (
	"nbox/internal/domain/backend"
	"time"
)

type Metadata struct {
	Hash           string                     `dynamodbav:"Hash,omitempty"      json:"hash,omitempty"`
	Secure         bool                       `dynamodbav:"Secure"              json:"-"`
	StorageBackend backend.StorageBackendType `dynamodbav:"StorageBackend"      json:"storageBackend"`
	Action         string                     `dynamodbav:"Action,omitempty"    json:"-"`
	Version        int64                      `dynamodbav:"Version,omitempty" json:"version,omitempty"`
	UpdatedAt      time.Time                  `dynamodbav:"UpdatedAt,unixtime"  json:"updatedAt,omitempty"`
	UpdatedBy      string                     `dynamodbav:"UpdatedBy,omitempty" json:"updatedBy,omitempty"`
}
