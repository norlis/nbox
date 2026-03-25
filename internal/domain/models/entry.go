package models

import (
	"fmt"
	"time"

	"nbox/internal/domain/backend"
)

type EntryOutput struct {
	Key    string `example:"development/service/var-example" json:"key"    yaml:"key"`
	Value  string `example:"value 123"                       json:"value"  yaml:"value"`
	Secure bool   `example:"false"                           json:"secure" yaml:"secure"`
}

type Entry struct {
	Path     string    `json:"path,omitempty"                     swaggerignore:"true"      yaml:"path,omitempty"`
	Key      string    `example:"development/service/var-example" json:"key"                yaml:"key"`
	Value    string    `example:"value 123"                       json:"value"              yaml:"value"`
	Secure   bool      `example:"false"                           json:"secure"             yaml:"secure"`
	Metadata *Metadata `json:"metadata,omitempty"                 yaml:"metadata,omitempty"`
}

func (e *Entry) String() string {
	return fmt.Sprintf("Key: %s. Value: %s", e.Key, e.Value)
}

type Tracking struct {
	Key            string                     `dynamodbav:"Key"                   json:"key"`
	Value          string                     `dynamodbav:"Value"                 json:"value"`
	Secure         bool                       `dynamodbav:"Secure"                json:"secure"`
	StorageBackend backend.StorageBackendType `dynamodbav:"StorageBackend"        json:"storageBackend"`
	UpdatedAt      time.Time                  `dynamodbav:"Timestamp"             json:"updatedAt"` // ,unixtime
	UpdatedBy      string                     `dynamodbav:"UpdatedBy"             json:"updatedBy"`
	Action         string                     `dynamodbav:"Action"                json:"action,omitempty"`
	Version        int64                      `dynamodbav:"Version,omitempty"     json:"version,omitempty"`
	Hash           string                     `dynamodbav:"Fingerprint,omitempty" json:"hash,omitempty"`
}

func (e *Tracking) String() string {
	return fmt.Sprintf("Key: %s. Value: %s", e.Key, e.Value)
}
