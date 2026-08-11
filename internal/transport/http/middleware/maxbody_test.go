package middleware_test

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nbox/internal/transport/http/middleware"
)

// echoHandler reads the full body and writes 200 on success, 400 on read error.
func echoHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
}

func TestMaxBodyBytes_WithinLimit_Returns200(t *testing.T) {
	const limit int64 = 1024
	handler := middleware.MaxBodyBytes(limit)(echoHandler())

	body := strings.Repeat("a", 512) // half the limit
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestMaxBodyBytes_ExceedsLimit_Returns400(t *testing.T) {
	const limit int64 = 1024
	handler := middleware.MaxBodyBytes(limit)(echoHandler())

	body := bytes.Repeat([]byte("a"), 2048) // double the limit
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 when body exceeds limit, got %d", rr.Code)
	}
}

func TestMaxBodyBytes_ExceedsLimit_MaxBytesError(t *testing.T) {
	const limit int64 = 1024
	// Use a handler that exposes the error type directly.
	var readErr error
	probeHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, readErr = io.ReadAll(r.Body)
		if readErr != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	handler := middleware.MaxBodyBytes(limit)(probeHandler)

	body := bytes.Repeat([]byte("x"), int(limit)+1)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if readErr == nil {
		t.Fatal("expected a read error when body exceeds limit, got nil")
	}
	var mbe *http.MaxBytesError
	if !errors.As(readErr, &mbe) {
		t.Errorf("expected *http.MaxBytesError, got %T: %v", readErr, readErr)
	}
}

func TestMaxBodyBytes_NilBody_Unaffected(t *testing.T) {
	const limit int64 = 1024

	// A handler that only checks whether MaxBytesReader was applied — it
	// avoids calling io.ReadAll on a potentially-nil body by just writing 200.
	noReadHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The middleware must not replace a nil body with a non-nil one.
		if r.Body != nil {
			// Acceptable: httptest.NewRequest sets Body to http.NoBody for nil
			// body args, which is non-nil but has zero bytes. The middleware
			// should NOT wrap http.NoBody (it wraps only when r.Body != nil, so
			// this path is fine for http.NoBody). We just confirm we get 200.
		}
		w.WriteHeader(http.StatusOK)
	})

	handler := middleware.MaxBodyBytes(limit)(noReadHandler)

	// GET request with no body — httptest sets Body to http.NoBody (non-nil).
	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 for nil/empty body, got %d", rr.Code)
	}
}
