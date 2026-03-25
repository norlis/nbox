package models

import (
	"time"

	"nbox/internal/domain/backend"
)

type Metadata struct {
	Fingerprint    string                     `dynamodbav:"Fingerprint,omitempty" json:"fingerprint,omitempty"`
	Secure         bool                       `dynamodbav:"Secure"                json:"-"`
	StorageBackend backend.StorageBackendType `dynamodbav:"StorageBackend"        json:"storageBackend"`
	Action         string                     `dynamodbav:"Action,omitempty"      json:"-"`
	Version        int64                      `dynamodbav:"Version,omitempty"     json:"version,omitempty"`
	UpdatedAt      time.Time                  `dynamodbav:"UpdatedAt,unixtime"    json:"updatedAt"`
	UpdatedBy      string                     `dynamodbav:"UpdatedBy,omitempty"   json:"updatedBy,omitempty"`
}
