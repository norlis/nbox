package box

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"nbox/internal/box/store/spec"
)

type Box struct {
	Service string           `json:"service"`
	Stage   map[string]Stage `json:"stage"`
}

type Stage struct {
	Template Template         `json:"template"`
	Metadata TemplateMetadata `dynamodbav:"Metadata" json:"metadata"`
}

type Template struct {
	Name  string `dynamodbav:"path"  json:"name"` // s3 path
	Value string `dynamodbav:"value" json:"value"`
}

type TemplateMetadata struct {
	Hash      string    `dynamodbav:"Fingerprint,omitempty" json:"hash,omitempty"`
	Version   string    `dynamodbav:"Version,omitempty"     json:"version,omitempty"`
	UpdatedAt time.Time `dynamodbav:"UpdatedAt,unixtime"    json:"updatedAt"`
	UpdatedBy string    `dynamodbav:"UpdatedBy,omitempty"   json:"updatedBy,omitempty"`
}

type TemplateVersion struct {
	VersionId    string    `json:"versionId"`
	LastModified time.Time `json:"lastModified"`
	Size         int64     `json:"size"`
	IsLatest     bool      `json:"isLatest"`
}

type CommandName string

var (
	UpsertTemplate CommandName = "upsert.template"
	UpsertVariable CommandName = "upsert.variables"
)

type Command[T any] struct {
	Command CommandName `json:"command,omitempty"`
	Payload T           `json:"payload"`
}

type SchemaType string

const (
	JSON   SchemaType = "json"
	YAML   SchemaType = "yaml"
	DOTENV SchemaType = "dotenv"
	CFG    SchemaType = "cfg"
	INI    SchemaType = "ini"
	XML    SchemaType = "xml"
	TXT    SchemaType = "txt"
)

// ExtensionMap maps file extensions to their SchemaType.
// This is the single source of truth for extension → schema mapping.
var ExtensionMap = map[string]SchemaType{
	".json": JSON,
	".yaml": YAML,
	".yml":  YAML,
	".env":  DOTENV,
	".cfg":  CFG,
	".conf": CFG,
	".ini":  INI,
	".xml":  XML,
	".txt":  TXT,
	".text": TXT,
}

// SchemaFromFilename returns the SchemaType based on the file extension.
func SchemaFromFilename(filename string) (SchemaType, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	if schemaType, ok := ExtensionMap[ext]; ok {
		return schemaType, nil
	}
	return "", errors.New("unsupported file extension")
}

// AllSchemaTypes returns the complete list of supported schema types.
func AllSchemaTypes() []SchemaType {
	return []SchemaType{
		JSON,
		YAML,
		DOTENV,
		CFG,
		INI,
		XML,
		TXT,
	}
}

// SchemaStrategy defines the behavior for each supported file type.
type SchemaStrategy interface {
	SchemaType() SchemaType
	Transform(value string) string
	Validate(content string) error
	Format(content []byte) ([]byte, error)
}

// Store persiste templates de box.
type Store interface {
	UpsertBox(ctx context.Context, box *Box) map[string]spec.Result
	BoxExists(ctx context.Context, service, stage, template string) (bool, error)
	RetrieveBox(ctx context.Context, service, stage, template string) ([]byte, error)
	RetrieveBoxVersion(ctx context.Context, service, stage, template, versionId string) ([]byte, error)
	ListVersions(ctx context.Context, service, stage, template string) ([]TemplateVersion, error)
	List(ctx context.Context) ([]Box, error)
	Detail(ctx context.Context, service, stage string) (*Stage, error)
}
