package export

import "errors"

var (
	// ErrInvalidExportFormat = errors.New("invalid export format").
	ErrInvalidImportFormat      = errors.New("invalid import format")
	ErrInvalidOverwriteStrategy = errors.New("invalid overwrite strategy")
	ErrExportSizeLimitExceeded  = errors.New("export size limit exceeded")
	ErrImportSizeLimitExceeded  = errors.New("import size limit exceeded")
	ErrInsufficientPermissions  = errors.New("insufficient permissions")
	ErrConflictsDetected        = errors.New("conflicts detected during import")
	ErrInvalidFileFormat        = errors.New("invalid file format")
	ErrValidationFailed         = errors.New("validation failed")
)
