package models

import (
	"fmt"
	"nbox/internal/domain/backend"
	"time"
)

type EntryOutput struct {
	Key    string `example:"development/service/var-example" json:"key"    yaml:"key"`
	Value  string `example:"value 123"                       json:"value"  yaml:"value"`
	Secure bool   `example:"false"                           json:"secure" yaml:"secure"`
}

type Entry struct {
	Path     string    `json:"path,omitempty"                     swaggerignore:"true" yaml:"path,omitempty"`
	Key      string    `example:"development/service/var-example" json:"key"           yaml:"key"`
	Value    string    `example:"value 123"                       json:"value"         yaml:"value"`
	Secure   bool      `example:"false"                           json:"secure"        yaml:"secure"`
	Metadata *Metadata `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

func (e *Entry) String() string {
	return fmt.Sprintf("Key: %s. Value: %s", e.Key, e.Value)
}

type Tracking struct {
	Key            string                     `json:"key" dynamodbav:"Key"`
	Value          string                     `json:"value" dynamodbav:"Value"`
	Secure         bool                       `json:"secure" dynamodbav:"Secure"`
	StorageBackend backend.StorageBackendType `json:"storageBackend" dynamodbav:"StorageBackend"`
	UpdatedAt      time.Time                  `json:"updatedAt" dynamodbav:"Timestamp"` //,unixtime
	UpdatedBy      string                     `json:"updatedBy" dynamodbav:"UpdatedBy"`
	Action         string                     `json:"action,omitempty" dynamodbav:"Action"`
	Version        int64                      `json:"version,omitempty" dynamodbav:"Version,omitempty"`
	Hash           string                     `dynamodbav:"Hash,omitempty"      json:"hash,omitempty"`
}

func (e *Tracking) String() string {
	return fmt.Sprintf("Key: %s. Value: %s", e.Key, e.Value)
}
