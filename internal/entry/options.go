package entry

// ListOptions tunes a List call.
type ListOptions struct {
	// LeavesOnly: solo hojas del subárbol (recursivo), excluyendo carpetas
	// (Key con "/" final) y records con Path en blanco.
	LeavesOnly bool
}

// ListOption mutates ListOptions.
type ListOption func(*ListOptions)

// LeavesOnly returns only the leaf entries of the whole subtree.
func LeavesOnly() ListOption { return func(o *ListOptions) { o.LeavesOnly = true } }

// NewListOptions folds opts into a ListOptions value.
func NewListOptions(opts ...ListOption) ListOptions {
	var o ListOptions
	for _, fn := range opts {
		fn(&o)
	}
	return o
}
