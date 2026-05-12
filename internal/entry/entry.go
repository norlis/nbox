package entry

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"nbox/internal/prefix"
)

type EntryOutput struct {
	Key    string `example:"development/service/var-example" json:"key"    yaml:"key"`
	Value  string `example:"value 123"                       json:"value"  yaml:"value"`
	Secure bool   `example:"false"                           json:"secure" yaml:"secure"`
}

type Entry struct {
	Path     string    `json:"path,omitempty"                     swaggerignore:"true"      yaml:"path,omitempty"`
	Key      string    `example:"development/service/var-example" json:"key"                yaml:"key"`
	ShortKey string    `json:"-"                                  swaggerignore:"true"      yaml:"-"`
	Value    string    `example:"value 123"                       json:"value"              yaml:"value"`
	Secure   bool      `example:"false"                           json:"secure"             yaml:"secure"`
	Metadata *Metadata `json:"metadata"                           yaml:"metadata,omitempty"`
}

func (e *Entry) String() string {
	return fmt.Sprintf("Key: %s. Value: %s", e.Key, e.Value)
}

type Metadata struct {
	Fingerprint    string                    `dynamodbav:"Fingerprint,omitempty" json:"fingerprint,omitempty"`
	Secure         bool                      `dynamodbav:"Secure"                json:"-"`
	StorageBackend prefix.StorageBackendType `dynamodbav:"StorageBackend"        json:"storageBackend"`
	Action         string                    `dynamodbav:"Action,omitempty"      json:"-"`
	Version        int64                     `dynamodbav:"Version,omitempty"     json:"version,omitempty"`
	UpdatedAt      time.Time                 `dynamodbav:"UpdatedAt,unixtime"    json:"updatedAt"`
	UpdatedBy      string                    `dynamodbav:"UpdatedBy,omitempty"   json:"updatedBy,omitempty"`
}

type OperationType string

const (
	Created   OperationType = "created"
	Updated   OperationType = "updated"
	Unchanged OperationType = "unchanged" // Útil para operaciones idempotentes
	Failed    OperationType = "failed"    // Útil si quieres registrar fallos explícitos
)

func (ot OperationType) IsValid() bool {
	switch ot {
	case Created, Updated, Unchanged, Failed:
		return true
	}
	return false
}

func (ot OperationType) String() string {
	return string(ot)
}

type Result struct {
	Key    string        `json:"key"`
	Action OperationType `json:"action"`
	Err    error         `json:"-"`
	Output *Entry        `json:"-"`
}

func (r Result) MarshalJSON() ([]byte, error) {
	type Alias Result

	aux := struct {
		Alias
		ErrorMessage string `json:"error,omitempty"`
	}{
		Alias: Alias(r),
	}

	if r.Err != nil {
		aux.ErrorMessage = r.Err.Error()
	}

	return json.Marshal(aux)
}

type Results map[string]Result

func (r Results) Add(key string, action OperationType, err error) {
	r[key] = Result{
		Key:    key,
		Action: action,
		Err:    err,
	}
}

// Helper para facilitar la creación de resultados con Output.
func (r Results) AddWithOutput(key string, action OperationType, err error, output *Entry) {
	r[key] = Result{
		Key:    key,
		Action: action,
		Err:    err,
		Output: output,
	}
}

func (r Results) HasErrors() bool {
	for _, res := range r {
		if res.Err != nil {
			return true
		}
	}
	return false
}

// RetrieveConfig configura el comportamiento de Retrieve (e.g., decryption).
type RetrieveConfig struct {
	Decrypt bool
}

type RetrieveOption func(*RetrieveConfig)

func WithDecryption(decrypt bool) RetrieveOption {
	return func(c *RetrieveConfig) {
		c.Decrypt = decrypt
	}
}

// Reader es el contrato de lectura de entries.
type Reader interface {
	Retrieve(ctx context.Context, key string, opts ...RetrieveOption) (*Entry, error)
	List(ctx context.Context, prefix string) ([]Entry, error)
	RetrieveMany(ctx context.Context, keys []string) (map[string]*Entry, error)
}

// Writer es el contrato de escritura.
type Writer interface {
	Upsert(ctx context.Context, entries []Entry) Results
	Delete(ctx context.Context, key string) error
}

// Resolver expone el valor descifrado de un secreto.
type Resolver interface {
	Resolve(ctx context.Context, key string) (*Entry, error)
}

// Store es el contrato completo para un backend principal (un solo backend físico).
type Store interface {
	Reader
	Writer
	Resolver
	BackendType() prefix.StorageBackendType
}

// PartialStore es para backends que no soportan List (e.g., SSM Secure).
type PartialStore interface {
	Writer
	Resolver
	Retrieve(ctx context.Context, key string, opts ...RetrieveOption) (*Entry, error)
	BackendType() prefix.StorageBackendType
}

// Manager orquesta múltiples backends y ofrece operaciones unificadas.
type Manager interface {
	Reader
	Writer
	Resolver
	RegisterBackend(backend PartialStore)
}

// ConvertToEnvVarName converts a key like "production/myapp/db-host" to
// an upper-snake-case env variable name "PRODUCTION_MYAPP_DB_HOST".
func ConvertToEnvVarName(key string) string {
	s := strings.ToUpper(key)
	replacer := strings.NewReplacer(
		"/", "_",
		"-", "_",
		".", "_",
		" ", "_",
	)
	return replacer.Replace(s)
}
