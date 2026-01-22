package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/norlis/httpgate/pkg/adapter/apidriven/presenters"
	_ "github.com/norlis/httpgate/pkg/kit/problem"
	"go.uber.org/zap"
	"nbox/internal/domain"
)

type Claims struct {
	Username string   `json:"username"`
	Name     string   `json:"name"`
	Roles    []string `json:"roles"`
	jwt.RegisteredClaims
}

type TokenRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// TokenHandler JWT
// @Summary Token
// @Description authentication token
// @Tags auth
// @Accept       json
// @Produce      json
// @Param data body TokenRequest true "Payload"
// @Success 200 {object} object{token=string} "Token generated successfully"
// @Failure 401 {object} problem.ProblemDetail "Unauthorized"
// @Failure 500 {object} problem.ProblemDetail "Internal error"
// @Router /api/auth/token [post].
func (a Authn) TokenHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		payload := &TokenRequest{}

		if err := json.NewDecoder(r.Body).Decode(payload); err != nil {
			a.render.Error(w, r, err, presenters.WithStatus(http.StatusBadRequest))
			return
		}

		user, err := a.repository.ValidatePassword(r.Context(), payload.Username, payload.Password)
		if err != nil {
			a.logger.Error("ErrValidatePassword", zap.Error(err))
			if errors.Is(err, domain.ErrUserNotFound) || errors.Is(err, domain.ErrInvalidPassword) {
				a.render.Error(w, r, ErrInvalidCredentials, presenters.WithStatus(http.StatusBadRequest))
				return
			}

			a.render.Error(w, r, err, presenters.WithStatus(http.StatusBadRequest))
			return
		}

		expirationTime := time.Now().Add(24 * time.Hour)
		now := time.Now()
		claims := &Claims{
			Username: user.Username,
			Name:     user.Username,
			Roles:    user.Roles,
			RegisteredClaims: jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(expirationTime),
				IssuedAt:  jwt.NewNumericDate(now),
				NotBefore: jwt.NewNumericDate(now),
			},
		}

		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

		tokenString, err := token.SignedString(a.config.HmacSecretKey)
		if err != nil {
			a.render.Error(w, r, err, presenters.WithStatus(http.StatusInternalServerError))
			return
		}

		a.render.JSON(w, r, map[string]string{"token": tokenString})
	}
}
