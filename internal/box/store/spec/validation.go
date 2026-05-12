package spec

type Kind string

const (
	KindSuccess       Kind = "success"
	KindSyntaxError   Kind = "syntax_error"
	KindSchemaError   Kind = "schema_error"
	KindInternalError Kind = "internal_error"
)

// Result represents a unified validation result structure.
type Result struct {
	Valid  bool    `json:"valid"`
	Kind   Kind    `json:"kind"`
	Errors []Error `json:"errors,omitempty"`
	Meta   any     `json:"meta,omitempty"`
}

// Error represents a single validation error with path and message.
type Error struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

// Option defines the functional option signature for Result construction.
type Option func(*Result)

// NewResult is the main constructor for Result.
// By default assumes success unless options indicate otherwise.
func NewResult(opts ...Option) Result {
	// Default state: Success
	r := Result{
		Valid: true,
		Kind:  KindSuccess,
	}

	for _, opt := range opts {
		opt(&r)
	}

	// Safety logic: If there are errors or Kind is not success, Valid must be false
	if len(r.Errors) > 0 || r.Kind != KindSuccess {
		r.Valid = false
	}

	return r
}

// WithMeta adds metadata to the result (e.g. SpecDefinition).
func WithMeta(meta any) Option {
	return func(r *Result) {
		r.Meta = meta
	}
}

// WithSyntaxError configures the result as a syntax error (file level).
func WithSyntaxError(err error) Option {
	return func(r *Result) {
		r.Kind = KindSyntaxError
		r.Errors = append(r.Errors, Error{
			Path:    "file",
			Message: err.Error(),
		})
	}
}

// WithInternalError configures the result as a system error.
func WithInternalError(err error) Option {
	return func(r *Result) {
		r.Kind = KindInternalError
		r.Errors = append(r.Errors, Error{
			Path:    "system",
			Message: err.Error(),
		})
	}
}

// WithSchemaErrors configures the result as a business validation error.
// Accepts a list of pre-calculated Error values (from boxspec).
func WithSchemaErrors(errors []Error) Option {
	return func(r *Result) {
		r.Kind = KindSchemaError
		r.Errors = append(r.Errors, errors...)
	}
}

// WithKind forces a specific kind (useful for manual cases).
func WithKind(kind Kind) Option {
	return func(r *Result) {
		r.Kind = kind
	}
}
