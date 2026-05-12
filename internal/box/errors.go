package box

import "errors"

var (
	ErrTemplateNotFound = errors.New("template not found")
	ErrInvalidTemplate  = errors.New("invalid template")
	ErrMissingVariables = errors.New("template has missing variables")
	ErrInvalidSyntax    = errors.New("template has invalid syntax")
)
