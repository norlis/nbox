package auth_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"
	"nbox/internal/auth"
	"nbox/internal/nbox"
	"nbox/internal/transport/httpx"
)

// ---- stub store --------------------------------------------------------

type stubStore struct {
	user *auth.User
	err  error
}

func (s *stubStore) FindByUsername(_ context.Context, _ string) (*auth.User, error) {
	return s.user, s.err
}

func (s *stubStore) ValidatePassword(_ context.Context, _, _ string) (*auth.User, error) {
	return s.user, s.err
}

// ---- helpers -----------------------------------------------------------

func newTestHandler(store auth.Store) *auth.TokenHandler {
	cfg := &nbox.Config{HmacSecretKey: []byte("test-secret-key-32-bytes-long!!!")}
	render := httpx.New(nil)
	return auth.NewTokenHandler(store, cfg, render, zap.NewNop())
}

func doTokenRequest(t *testing.T, h *auth.TokenHandler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/token",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.Token(rr, req)
	return rr
}

// ---- tests -------------------------------------------------------------

func TestToken_WrongPassword_Returns401(t *testing.T) {
	store := &stubStore{err: auth.ErrInvalidPassword}
	h := newTestHandler(store)

	rr := doTokenRequest(t, h, `{"username":"alice","password":"wrong"}`)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d (Unauthorized)", rr.Code, http.StatusUnauthorized)
	}
}

func TestToken_UnknownUsername_Returns401(t *testing.T) {
	store := &stubStore{err: auth.ErrUserNotFound}
	h := newTestHandler(store)

	rr := doTokenRequest(t, h, `{"username":"nobody","password":"x"}`)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d (Unauthorized)", rr.Code, http.StatusUnauthorized)
	}
}

func TestToken_UnknownUsername_ResponseBodyOpaqueCredentialsError(t *testing.T) {
	// The client should see ErrInvalidCredentials text, not the specific sentinel.
	store := &stubStore{err: auth.ErrUserNotFound}
	h := newTestHandler(store)

	rr := doTokenRequest(t, h, `{"username":"nobody","password":"x"}`)

	body := rr.Body.String()
	if strings.Contains(body, auth.ErrUserNotFound.Error()) {
		t.Errorf("response leaks ErrUserNotFound: %s", body)
	}
}

func TestToken_MalformedJSON_Returns400(t *testing.T) {
	store := &stubStore{} // not reached
	h := newTestHandler(store)

	rr := doTokenRequest(t, h, `{not valid json`)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d (Bad Request)", rr.Code, http.StatusBadRequest)
	}
}

func TestToken_UnexpectedStoreError_Returns500(t *testing.T) {
	unexpectedErr := errors.New("database connection lost")
	store := &stubStore{err: unexpectedErr}
	h := newTestHandler(store)

	rr := doTokenRequest(t, h, `{"username":"alice","password":"pw"}`)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d (Internal Server Error)", rr.Code, http.StatusInternalServerError)
	}
}

func TestToken_ValidCredentials_Returns200WithToken(t *testing.T) {
	store := &stubStore{
		user: &auth.User{
			Username: "alice",
			Roles:    []string{"admin"},
		},
	}
	h := newTestHandler(store)

	rr := doTokenRequest(t, h, `{"username":"alice","password":"correct"}`)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["token"] == "" {
		t.Error("expected non-empty token in response")
	}
}

func TestToken_EmptyBody_Returns400(t *testing.T) {
	store := &stubStore{}
	h := newTestHandler(store)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/token", bytes.NewReader([]byte{}))
	rr := httptest.NewRecorder()
	h.Token(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d (Bad Request)", rr.Code, http.StatusBadRequest)
	}
}
