package middleware

import "net/http"

// MaxBodyBytes caps the request body size to limit bytes. Reads beyond the
// limit fail with *http.MaxBytesError, which handlers' decode-error paths
// surface as a 4xx. GET/HEAD requests (no body) are unaffected.
func MaxBodyBytes(limit int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, limit)
			}
			next.ServeHTTP(w, r)
		})
	}
}
