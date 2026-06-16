package nboxclient_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"nbox/internal/entrypushd/nboxclient"
)

func TestSnapshot_RetriesOnServerError(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if attempts.Add(1) < 3 { // fail twice, succeed on the 3rd
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`[{"key":"k","value":"v","secure":false}]`))
	}))
	defer srv.Close()

	c := nboxclient.NewClient(srv.URL, srv.Client())
	entries, err := c.Snapshot(context.Background(), "jwt", "p/")

	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, int32(3), attempts.Load(), "5xx must be retried until success")
}

func TestResolve_DoesNotRetry4xx(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	c := nboxclient.NewClient(srv.URL, srv.Client())
	_, err := c.Resolve(context.Background(), "jwt", "k")

	require.Error(t, err)
	require.Equal(t, int32(1), attempts.Load(), "4xx is terminal — no retries")
}

func TestSnapshot_SendsBearerTokenAndEncodedPrefix(t *testing.T) {
	var gotAuth, gotQuery, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotQuery = r.URL.Query().Get("v")
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"key":"passbox/banking/api","value":"x","secure":true}]`))
	}))
	defer srv.Close()

	c := nboxclient.NewClient(srv.URL, srv.Client())
	entries, err := c.Snapshot(context.Background(), "jwt-123", "passbox/banking/")

	require.NoError(t, err)
	require.Equal(t, "Bearer jwt-123", gotAuth)
	require.Equal(t, "/api/entry/prefix", gotPath)
	require.Equal(t, "passbox/banking/", gotQuery)
	require.Len(t, entries, 1)
	require.Equal(t, "passbox/banking/api", entries[0].Key)
	require.Equal(t, "x", entries[0].Value)
	require.True(t, entries[0].Secure)
}

func TestSnapshot_ErrorsOnNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := nboxclient.NewClient(srv.URL, srv.Client())
	_, err := c.Snapshot(context.Background(), "jwt", "p/")
	require.Error(t, err)
	require.Contains(t, err.Error(), "401")
}

func TestSnapshot_ErrorsOnBadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`not-json`))
	}))
	defer srv.Close()

	c := nboxclient.NewClient(srv.URL, srv.Client())
	_, err := c.Snapshot(context.Background(), "jwt", "p/")
	require.Error(t, err)
}

func TestSnapshot_RespectsContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := nboxclient.NewClient(srv.URL, srv.Client())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := c.Snapshot(ctx, "jwt", "p/")
	require.Error(t, err)
}

func TestResolve_ReturnsPlaintextValue(t *testing.T) {
	var gotAuth, gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		gotQuery = r.URL.Query().Get("v")
		_, _ = w.Write([]byte(`{"key":"passbox/banking/api","value":"plain-secret","secure":true}`))
	}))
	defer srv.Close()

	c := nboxclient.NewClient(srv.URL, srv.Client())
	val, err := c.Resolve(context.Background(), "jwt-9", "passbox/banking/api")

	require.NoError(t, err)
	require.Equal(t, "Bearer jwt-9", gotAuth)
	require.Equal(t, "/api/entry/resolve", gotPath)
	require.Equal(t, "passbox/banking/api", gotQuery)
	require.Equal(t, "plain-secret", val)
}

func TestResolve_ErrorsOnNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	c := nboxclient.NewClient(srv.URL, srv.Client())
	_, err := c.Resolve(context.Background(), "jwt", "passbox/k")
	require.Error(t, err)
	require.Contains(t, err.Error(), "403")
}

func TestSnapshot_SendsLeavesFlag(t *testing.T) {
	var hasLeaves bool
	var gotV string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hasLeaves = r.URL.Query().Has("leaves")
		gotV = r.URL.Query().Get("v")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := nboxclient.NewClient(srv.URL, srv.Client())
	_, err := c.Snapshot(context.Background(), "jwt", "passbox/")

	require.NoError(t, err)
	require.True(t, hasLeaves, "Snapshot must request only leaves")
	require.Equal(t, "passbox/", gotV)
}
