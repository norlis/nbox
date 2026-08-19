package transporthttp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/norlis/httpgate/middleware"
	"github.com/norlis/httpgate/trace"
	"github.com/stretchr/testify/require"
	"nbox/pkg/logger"
)

func TestBaseChain_TraceGeneratedAndEchoed(t *testing.T) {
	var buf bytes.Buffer
	slg := logger.NewWithWriter(&buf, logger.Config{Level: "info"}, "nbox", "test")
	chain := middleware.New(
		middleware.TraceContext(middleware.WithResponseHeader("x-transaction-id")),
		middleware.RequestLogger(slg),
	)
	h := chain.ThenFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/x", nil))

	tid := rec.Header().Get("x-transaction-id")
	require.Regexp(t, `^[0-9a-f]{32}$`, tid)
	var doc map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &doc))
	require.Equal(t, "request completed", doc["message"])
	require.Equal(t, tid, doc["trace_id"])
}

func TestBaseChain_TraceInherited(t *testing.T) {
	var buf bytes.Buffer
	slg := logger.NewWithWriter(&buf, logger.Config{Level: "info"}, "nbox", "test")
	chain := middleware.New(middleware.TraceContext(middleware.WithResponseHeader("x-transaction-id")), middleware.RequestLogger(slg))
	h := chain.ThenFunc(func(w http.ResponseWriter, r *http.Request) {})

	tc := trace.New()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/x", nil)
	req.Header.Set(trace.Header, tc.Traceparent())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	require.Equal(t, tc.TraceID, rec.Header().Get("x-transaction-id"))
}
