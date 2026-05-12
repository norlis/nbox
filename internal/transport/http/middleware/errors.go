package middleware

import (
	"errors"
	"strings"
)

var (
	ErrMissingAuthHeader       = errors.New("authorization header is required")
	ErrUnsupportedAuthScheme   = errors.New("unsupported or missing authentication scheme")
	ErrInvalidAuthHeaderFormat = errors.New("invalid authorization header format")

	ErrTokenInvalid      = errors.New("token is invalid")
	ErrTokenExpired      = errors.New("token has expired")
	ErrTokenClaimInvalid = errors.New("token contains an invalid or missing claim")

	ErrInvalidCredentials = errors.New("invalid username or password")
)

func isCredentialError(err error) bool {
	return errors.Is(err, ErrInvalidAuthHeaderFormat) || errors.Is(err, ErrInvalidCredentials)
}

// extractAuthValue checks an authentication scheme and extracts its value.
// Returns the value and true if the scheme matches.
// Example: extractAuthValue("Bearer my-token", "Bearer") -> ("my-token", true).
func extractAuthValue(authHeader, scheme string) (string, bool) {
	prefix := scheme + " "
	prefixLen := len(prefix)

	if len(authHeader) >= prefixLen && strings.EqualFold(authHeader[:prefixLen], prefix) {
		return authHeader[prefixLen:], true
	}

	return "", false
}
