package models

type ExportFormat string

const (
	ExportFormatJSON       ExportFormat = "json"
	ExportFormatYAML       ExportFormat = "yaml"
	ExportFormatDotEnv     ExportFormat = "dotenv"
	ExportFormatECSTaskDef ExportFormat = "ecs"
)

// IsValid verifica si el formato es válido.
func (f ExportFormat) IsValid() bool {
	switch f {
	case ExportFormatJSON, ExportFormatYAML, ExportFormatDotEnv, ExportFormatECSTaskDef:
		return true
	}
	return false
}

// ContentType retorna el content-type HTTP apropiado.
func (f ExportFormat) ContentType() string {
	switch f {
	case ExportFormatJSON:
		return "application/json"
	case ExportFormatYAML:
		return "application/x-yaml"
	case ExportFormatDotEnv:
		return "text/plain"
	case ExportFormatECSTaskDef:
		return "application/json"
	default:
		return "application/octet-stream"
	}
}

// FileExtension retorna la extensión de archivo apropiada.
func (f ExportFormat) FileExtension() string {
	switch f {
	case ExportFormatJSON:
		return ".json"
	case ExportFormatYAML:
		return ".yaml"
	case ExportFormatDotEnv:
		return ".env"
	case ExportFormatECSTaskDef:
		return ".json"
	default:
		return ".json"
	}
}

// ExportOptions opciones para exportación.
type ExportOptions struct {
	Prefix string       `json:"prefix,omitempty"`
	Format ExportFormat `json:"format"`
}

func (o *ExportOptions) Validate() error {
	if !o.Format.IsValid() {
		return ErrInvalidExportFormat
	}
	return nil
}

// ExportResult resultado de una exportación.
type ExportResult struct {
	Entries []Entry `json:"entries" yaml:"entries"`
	Content []byte  `json:"-"       yaml:"-"`
	Size    int64   `json:"-"       yaml:"-"`
}
