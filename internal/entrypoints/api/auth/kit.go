package auth

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

// extractAuthValue comprueba un esquema de autenticación y extrae su valor.
// Devuelve el valor y un booleano 'true' si el esquema coincide.
// Example: extractAuthValue("Bearer mi-token", "Bearer") -> ("mi-token", true)
// Example: extractAuthValue("basic QWxhZGRpbjpvcGVuIHNlc2FtZQ==", "Basic") -> ("QWxhZGRpbjpvcGVuIHNlc2FtZQ==", true)
// Example: extractAuthValue("Token mi-token", "Bearer") -> ("", false).
func extractAuthValue(authHeader, scheme string) (string, bool) {
	prefix := scheme + " "
	prefixLen := len(prefix)

	// ✅ Comprobación segura de la longitud y del prefijo usando EqualFold.
	if len(authHeader) >= prefixLen && strings.EqualFold(authHeader[:prefixLen], prefix) {
		return authHeader[prefixLen:], true
	}

	return "", false
}
