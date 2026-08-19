package middleware

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/norlis/httpgate/logging"
	"github.com/norlis/httpgate/presenter"
	auth "nbox/internal/auth"
	"nbox/internal/logfields"
	"nbox/internal/nbox"
	"nbox/internal/transport/httpx"
)

type Authn struct {
	config     *nbox.Config
	render     *httpx.Render
	logger     *slog.Logger
	repository auth.Store
}

func NewAuthn(config *nbox.Config, render *httpx.Render, logger *slog.Logger, repository auth.Store) *Authn {
	return &Authn{config: config, render: render, logger: logger, repository: repository}
}

func (a *Authn) Handler() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authorization := r.Header.Get("Authorization")

			var newCtx context.Context
			var err error
			scheme := "none"

			if _, ok := extractAuthValue(authorization, "Basic"); ok {
				scheme = "basic"
				newCtx, err = a.tryBasicAuth(r)
			} else if _, ok := extractAuthValue(authorization, "Bearer"); ok {
				scheme = "bearer"
				newCtx, err = a.tryJwt(r)
			} else if authorization == "" {
				err = ErrMissingAuthHeader
			} else {
				scheme = "unsupported"
				err = ErrUnsupportedAuthScheme
			}

			if err != nil {
				attrs := []any{slog.String(logfields.KeyAuthScheme, scheme)}
				if username, _, ok := r.BasicAuth(); ok {
					attrs = append(attrs, slog.String(logfields.KeyUserID, username))
				}
				if isClientAuthError(err) {
					// Client input failure, not a service fault — WARN, never the credentials themselves.
					a.logger.WarnContext(r.Context(), "authentication failed", attrs...)
				} else {
					// e.g. the credential/store lookup itself failed (backend fault) — ERROR, once, here.
					a.logger.ErrorContext(r.Context(), "authentication failed", append(attrs, logging.Err(err))...)
				}
				if isCredentialError(err) {
					w.Header().Set("WWW-Authenticate", `Basic realm="api"`)
				}
				a.render.Error(w, r, err, presenter.WithStatus(http.StatusUnauthorized))
				return
			}

			next.ServeHTTP(w, r.WithContext(newCtx))
		})
	}
}
