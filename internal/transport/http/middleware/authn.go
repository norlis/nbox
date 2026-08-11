package middleware

import (
	"context"
	"net/http"

	"github.com/norlis/httpgate/presenter"
	"go.uber.org/zap"
	auth "nbox/internal/auth"
	"nbox/internal/nbox"
	"nbox/internal/transport/httpx"
)

type Authn struct {
	config     *nbox.Config
	render     *httpx.Render
	logger     *zap.Logger
	repository auth.Store
}

func NewAuthn(config *nbox.Config, render *httpx.Render, logger *zap.Logger, repository auth.Store) *Authn {
	return &Authn{config: config, render: render, logger: logger, repository: repository}
}

func (a *Authn) Handler() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authorization := r.Header.Get("Authorization")

			var newCtx context.Context
			var err error

			if _, ok := extractAuthValue(authorization, "Basic"); ok {
				newCtx, err = a.tryBasicAuth(r)
			} else if _, ok := extractAuthValue(authorization, "Bearer"); ok {
				newCtx, err = a.tryJwt(r)
			} else if authorization == "" {
				err = ErrMissingAuthHeader
			} else {
				err = ErrUnsupportedAuthScheme
			}

			if err != nil {
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
