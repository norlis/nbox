package entry

import "errors"

var (
	ErrEntryNotFound     = errors.New("entry not found")
	ErrInvalidKeyFormat  = errors.New("invalid key format")
	ErrKeyTooLong        = errors.New("key exceeds maximum length")
	ErrValueTooLong      = errors.New("value exceeds maximum length")
	ErrBatchSizeTooLarge = errors.New("batch size exceeds maximum")
)
