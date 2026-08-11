// Package middleware provides HTTP middleware for the nbox transport layer.
package middleware

import (
	"net/http"
	pkgpath "path"
	"strings"
)

// cleanKey canonicalizes a raw "v" query-parameter value by collapsing any
// path-traversal sequences ("..") and dot segments, matching exactly what the
// SSM/DynamoDB storage backends do via path.Clean.
//
// Rules:
//   - Empty input → empty output.
//   - Trailing slash in the original is preserved in the output.
//   - Leading slash is stripped so the result has the no-leading-slash form
//     expected by the OPA permission patterns (e.g. "development/app/config").
//   - ".." cannot escape above the virtual root; attempts to do so collapse to
//     the topmost segment (e.g. "development/../../production" → "production").
func cleanKey(v string) string {
	if v == "" {
		return ""
	}

	trailingSlash := strings.HasSuffix(v, "/")

	// Join with "/" acts as path.Clean on an absolute path, preventing any
	// escape above root. The result always starts with "/".
	c := pkgpath.Join("/", v)

	// Strip the leading "/" so the value matches the relative form that OPA
	// patterns expect (e.g. "development/app/config", not "/development/app/config").
	c = strings.TrimPrefix(c, "/")

	// Re-append trailing slash if the original had one and the result is non-empty.
	if trailingSlash && c != "" {
		c += "/"
	}

	return c
}

// CanonicalizeKey returns middleware that normalizes the "v" query parameter on
// every inbound request BEFORE the Authorize middleware evaluates the OPA
// policy. This is the normalize-then-authorize guard that closes the
// parser-differential authz bypass: the SSM backend collapses ".." via
// path.Clean at storage time, so without this middleware a request like
// v=development/../production/secret would match a non_production OPA pattern
// at authz time yet resolve to /production/secret at storage time.
//
// Behavior:
//   - If "v" is absent, the request is forwarded untouched.
//   - If "v" is already canonical, the request is forwarded untouched (zero
//     side-effects for the vast majority of legitimate requests).
//   - If "v" contains traversal sequences, the canonical form replaces it and
//     RawQuery is rewritten with literal slashes (not %2F) so that the OPA
//     regex patterns continue to match correctly.
//
// This middleware MUST be placed before the Authorize middleware in the chain.
func CanonicalizeKey() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			q := r.URL.Query()
			v := q.Get("v")

			if v == "" {
				next.ServeHTTP(w, r)
				return
			}

			cleaned := cleanKey(v)
			if cleaned == v {
				// No change — pass through without touching anything.
				next.ServeHTTP(w, r)
				return
			}

			// Traversal detected: rewrite the "v" value and rebuild RawQuery.
			// url.Values.Encode percent-encodes slashes as %2F, which would
			// break the OPA regex patterns that expect literal "/" characters.
			// We restore them with a ReplaceAll; this path only runs when a
			// traversal sequence was present, so re-ordering of other params
			// in the rare multi-param case is acceptable.
			q.Set("v", cleaned)
			r = r.Clone(r.Context())
			r.URL.RawQuery = strings.ReplaceAll(q.Encode(), "%2F", "/")

			next.ServeHTTP(w, r)
		})
	}
}
