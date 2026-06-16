package auth

// Registry resolves an Authenticator by its method name. The Registry
// is read-only by design — populate it via NewRegistry at startup.
// The map is immutable after construction; no mutex is needed.
type Registry interface {
	Get(method string) (Authenticator, bool)
}

type mapRegistry struct {
	by map[string]Authenticator
}

// NewRegistry builds a Registry seeded with the given authenticators.
// When multiple authenticators report the same Method(), the last
// occurrence in the argument list wins. The caller is responsible for
// providing unique methods when that matters.
func NewRegistry(auths ...Authenticator) Registry {
	r := &mapRegistry{by: make(map[string]Authenticator, len(auths))}
	for _, a := range auths {
		r.by[a.Method()] = a
	}
	return r
}

func (r *mapRegistry) Get(method string) (Authenticator, bool) {
	a, ok := r.by[method]
	return a, ok
}
