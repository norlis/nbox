package auth

import "context"

// identityKey and m2mTokenKey are unexported types used as ctx keys to
// avoid collisions with values from other packages. (See pkg context docs.)
type (
	identityKey struct{}
	m2mTokenKey struct{}
)

// WithIdentity attaches an authenticated Identity to ctx. The gRPC
// auth interceptor calls this after successful authentication so
// downstream handlers can read who the caller is (audit, authorization,
// resource-scoped queries).
func WithIdentity(ctx context.Context, id *Identity) context.Context {
	return context.WithValue(ctx, identityKey{}, id)
}

// IdentityFromContext returns the Identity stashed via WithIdentity, if any.
func IdentityFromContext(ctx context.Context) (*Identity, bool) {
	id, ok := ctx.Value(identityKey{}).(*Identity)
	return id, ok
}

// WithM2MToken attaches a freshly-minted M2M JWT to ctx. The interceptor
// calls this after MintM2MJWT so downstream code (e.g. the snapshot
// handler in Fase 2.1.d) can pull it for HTTP calls to nbox without
// re-running the bcrypt path.
func WithM2MToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, m2mTokenKey{}, token)
}

// M2MTokenFromContext returns the JWT stashed via WithM2MToken, if any.
func M2MTokenFromContext(ctx context.Context) (string, bool) {
	t, ok := ctx.Value(m2mTokenKey{}).(string)
	return t, ok
}
