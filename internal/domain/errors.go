package domain

import "errors"

var (
	// User errors.
	ErrUserNotFound    = errors.New("user not found")
	ErrInvalidPassword = errors.New("invalid password")

	// Export/Import errors.
	ErrInvalidExportFormat      = errors.New("invalid export format")
	ErrInvalidImportFormat      = errors.New("invalid import format")
	ErrInvalidOverwriteStrategy = errors.New("invalid overwrite strategy")
	ErrExportSizeLimitExceeded  = errors.New("export size limit exceeded")
	ErrImportSizeLimitExceeded  = errors.New("import size limit exceeded")
	ErrInsufficientPermissions  = errors.New("insufficient permissions")
	ErrConflictsDetected        = errors.New("conflicts detected during import")
	ErrInvalidFileFormat        = errors.New("invalid file format")
	ErrValidationFailed         = errors.New("validation failed")

	// Entry errors.
	ErrEntryNotFound     = errors.New("entry not found")
	ErrInvalidKeyFormat  = errors.New("invalid key format")
	ErrKeyTooLong        = errors.New("key exceeds maximum length")
	ErrValueTooLong      = errors.New("value exceeds maximum length")
	ErrBatchSizeTooLarge = errors.New("batch size exceeds maximum")

	// Template errors.
	ErrTemplateNotFound = errors.New("template not found")
	ErrInvalidTemplate  = errors.New("invalid template")
	ErrMissingVariables = errors.New("template has missing variables")

	// Secret errors.
	ErrSecretAccessDenied = errors.New("access denied to secret")
	ErrSecretNotFound     = errors.New("secret not found")
)
