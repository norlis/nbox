package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"

	"github.com/norlis/httpgate/pkg/adapter/apidriven/presenters"
	"go.uber.org/zap"
	"nbox/internal/application"
	"nbox/internal/domain"
)

type AuthenticationMiddleware any

type Authn struct {
	credentials map[string]string
	config      *application.Config
	render      presenters.Presenters
	logger      *zap.Logger
	repository  domain.UserRepository
}

func NewAuthn(prefix string, config *application.Config, render presenters.Presenters, logger *zap.Logger, repository domain.UserRepository) *Authn {
	credentials := map[string]string{}
	_ = json.Unmarshal([]byte(os.Getenv(prefix)), &credentials)
	return &Authn{credentials: credentials, config: config, render: render, logger: logger, repository: repository}
}

func (a Authn) Handler() func(http.Handler) http.Handler {
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
				if errors.Is(err, ErrInvalidAuthHeaderFormat) || errors.Is(err, ErrInvalidCredentials) {
					w.Header().Set("WWW-Authenticate", `Basic realm="api"`)
				}
				a.render.Error(w, r, err, presenters.WithStatus(http.StatusUnauthorized))
				return
			}

			next.ServeHTTP(w, r.WithContext(newCtx))
		})
	}
}
