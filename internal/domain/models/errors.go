package models

import "fmt"

// ValidationError error de validación.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation error on field '%s': %s", e.Field, e.Message)
}

// ErrInvalidExportFormat error cuando el formato no es válido.
var ErrInvalidExportFormat = &ValidationError{Field: "format", Message: "invalid export format"}
