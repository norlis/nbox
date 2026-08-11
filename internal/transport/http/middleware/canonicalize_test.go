package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestCleanKey verifies the cleanKey helper collapses path traversal sequences
// while preserving semantics expected by the OPA permission patterns.
func TestCleanKey(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "unchanged clean path",
			input: "development/app/config",
			want:  "development/app/config",
		},
		{
			name:  "single traversal collapses to production",
			input: "development/../production/secret",
			want:  "production/secret",
		},
		{
			name:  "double traversal collapses to production",
			input: "development/../../production",
			want:  "production",
		},
		{
			name:  "passbox path unchanged",
			input: "passbox/banking/key",
			want:  "passbox/banking/key",
		},
		{
			name:  "dot segment collapsed",
			input: "a/./b",
			want:  "a/b",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "trailing slash preserved",
			input: "passbox/banking/",
			want:  "passbox/banking/",
		},
		{
			name:  "leading slash stripped",
			input: "/development/app",
			want:  "development/app",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := cleanKey(tc.input)
			if got != tc.want {
				t.Errorf("cleanKey(%q) = %q; want %q", tc.input, got, tc.want)
			}
		})
	}
}

// TestCanonicalizeKey verifies the HTTP middleware rewrites traversal sequences
// in the "v" query parameter before the request reaches downstream handlers.
func TestCanonicalizeKey(t *testing.T) {
	t.Run("traversal is normalized and slashes are literal in RawQuery", func(t *testing.T) {
		var (
			gotV        string
			gotRawQuery string
		)
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotV = r.URL.Query().Get("v")
			gotRawQuery = r.URL.RawQuery
		})

		handler := CanonicalizeKey()(next)
		req := httptest.NewRequest(http.MethodGet, "/api/entry/key?v=development/../production/secret", http.NoBody)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if gotV != "production/secret" {
			t.Errorf("downstream v = %q; want %q", gotV, "production/secret")
		}
		if !strings.Contains(gotRawQuery, "v=production/secret") {
			t.Errorf("RawQuery %q does not contain literal v=production/secret (slashes must not be percent-encoded)", gotRawQuery)
		}
	})

	t.Run("clean path passes through unchanged (no mutation)", func(t *testing.T) {
		originalRawQuery := "v=development/app/config"
		var gotRawQuery string
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotRawQuery = r.URL.RawQuery
		})

		handler := CanonicalizeKey()(next)
		req := httptest.NewRequest(http.MethodGet, "/api/entry/key?"+originalRawQuery, http.NoBody)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if gotRawQuery != originalRawQuery {
			t.Errorf("RawQuery mutated: got %q; want %q (must be byte-for-byte equal)", gotRawQuery, originalRawQuery)
		}
	})

	t.Run("no v param passes through unchanged", func(t *testing.T) {
		var gotRawQuery string
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotRawQuery = r.URL.RawQuery
		})

		handler := CanonicalizeKey()(next)
		req := httptest.NewRequest(http.MethodGet, "/api/entry/prefix", http.NoBody)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if gotRawQuery != "" {
			t.Errorf("RawQuery should be empty, got %q", gotRawQuery)
		}
	})
}
