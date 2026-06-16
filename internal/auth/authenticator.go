package auth

import "context"

// Authenticator verifies a credential and returns the principal's
// Identity. The credential format is opaque to this contract —
// each implementation defines it.
type Authenticator interface {
	// Method returns the unique identifier of this authenticator,
	// e.g. "approle", "aws-sts". Used by Registry routing.
	Method() string

	// Authenticate verifies credential and returns Identity on success.
	// Returns an implementation-specific sentinel error on failure.
	Authenticate(ctx context.Context, credential []byte) (*Identity, error)
}

// sourceIPKey is a context key for the source IP of a login request.
// Use WithSourceIP / SourceIPFromContext to read/write.
type sourceIPKey struct{}

// WithSourceIP returns ctx with the source IP attached.
func WithSourceIP(ctx context.Context, ip string) context.Context {
	return context.WithValue(ctx, sourceIPKey{}, ip)
}

// SourceIPFromContext returns the source IP attached via WithSourceIP.
// The second return value is false if no IP was set.
func SourceIPFromContext(ctx context.Context) (string, bool) {
	ip, ok := ctx.Value(sourceIPKey{}).(string)
	return ip, ok
}
