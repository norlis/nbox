package models

import (
	"errors"
	"path/filepath"
	"strings"
)

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

// GetSchemaFromFilename returns the SchemaType based on the file extension.
func (SchemaType) GetSchemaFromFilename(filename string) (SchemaType, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	if schemaType, ok := ExtensionMap[ext]; ok {
		return schemaType, nil
	}
	return "", errors.New("unsupported file extension")
}

func GetAllSchemaTypes() []SchemaType {
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
