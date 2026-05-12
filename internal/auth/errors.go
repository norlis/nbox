package auth

import "errors"

var (
	ErrUserNotFound       = errors.New("user not found")
	ErrInvalidPassword    = errors.New("invalid password")
	ErrInvalidCredentials = errors.New("invalid username or password")
)
