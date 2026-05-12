package export

import (
	"context"
	"fmt"

	"nbox/internal/entry"
)

type Format string

const (
	FormatJSON       Format = "json"
	FormatYAML       Format = "yaml"
	FormatDotEnv     Format = "dotenv"
	FormatECSTaskDef Format = "ecs"
)

// IsValid verifica si el formato es válido.
func (f Format) IsValid() bool {
	switch f {
	case FormatJSON, FormatYAML, FormatDotEnv, FormatECSTaskDef:
		return true
	}
	return false
}

// ContentType retorna el content-type HTTP apropiado.
func (f Format) ContentType() string {
	switch f {
	case FormatJSON:
		return "application/json"
	case FormatYAML:
		return "application/x-yaml"
	case FormatDotEnv:
		return "text/plain"
	case FormatECSTaskDef:
		return "application/json"
	default:
		return "application/octet-stream"
	}
}

// FileExtension retorna la extensión de archivo apropiada.
func (f Format) FileExtension() string {
	switch f {
	case FormatJSON:
		return ".json"
	case FormatYAML:
		return ".yaml"
	case FormatDotEnv:
		return ".env"
	case FormatECSTaskDef:
		return ".json"
	default:
		return ".json"
	}
}

// Options opciones para exportación.
type Options struct {
	Prefix string `json:"prefix,omitempty"`
	Format Format `json:"format"`
}

func (o *Options) Validate() error {
	if !o.Format.IsValid() {
		return ErrInvalidFormat
	}
	return nil
}

// Result resultado de una exportación.
type Result struct {
	Entries []entry.Entry `json:"entries" yaml:"entries"`
	Content []byte        `json:"-"       yaml:"-"`
	Size    int64         `json:"-"       yaml:"-"`
}

// ValidationError error de validación.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation error on field '%s': %s", e.Field, e.Message)
}

// ErrInvalidFormat error cuando el formato no es válido.
var ErrInvalidFormat = &ValidationError{Field: "format", Message: "invalid export format"}

// Exporter convierte una lista de entries a bytes en el formato solicitado.
type Exporter interface {
	Export(ctx context.Context, entries []entry.Entry, options Options) ([]byte, error)
}
