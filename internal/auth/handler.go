package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/norlis/httpgate/presenter"
	_ "github.com/norlis/httpgate/problem"
	"go.uber.org/zap"
	"nbox/internal/nbox"
	"nbox/internal/transport/httpx"
)

type Claims struct {
	Username   string            `json:"username"`
	Name       string            `json:"name"`
	Roles      []string          `json:"roles"`
	Attributes map[string]string `json:"attributes,omitempty"` // ABAC seed: extra key/value pairs from the authenticator's metadata (e.g. allowed_prefixes, env)
	jwt.RegisteredClaims
}

type TokenRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type TokenHandler struct {
	repo   Store
	config *nbox.Config
	render *httpx.Render
	logger *zap.Logger
}

func NewTokenHandler(repo Store, config *nbox.Config, render *httpx.Render, logger *zap.Logger) *TokenHandler {
	return &TokenHandler{repo: repo, config: config, render: render, logger: logger}
}

func (h *TokenHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/auth/token", h.Token)
}

// Token godoc
// @Summary Token
// @Description authentication token
// @Tags auth
// @Accept json
// @Produce json
// @Param data body TokenRequest true "Payload"
// @Success 200 {object} object{token=string} "Token generated successfully"
// @Failure 401 {object} problem.Detail "Unauthorized"
// @Failure 500 {object} problem.Detail "Internal error"
// @Router /api/auth/token [post].
func (h *TokenHandler) Token(w http.ResponseWriter, r *http.Request) {
	payload := &TokenRequest{}

	if err := json.NewDecoder(r.Body).Decode(payload); err != nil {
		h.render.Error(w, r, err, presenter.WithStatus(http.StatusBadRequest))
		return
	}

	user, err := h.repo.ValidatePassword(r.Context(), payload.Username, payload.Password)
	if err != nil {
		h.logger.Error("ErrValidatePassword", zap.Error(err))
		if errors.Is(err, ErrUserNotFound) || errors.Is(err, ErrInvalidPassword) {
			h.render.Error(w, r, ErrInvalidCredentials, presenter.WithStatus(http.StatusUnauthorized))
			return
		}
		h.render.Error(w, r, err, presenter.WithStatus(http.StatusInternalServerError))
		return
	}

	now := time.Now()
	claims := &Claims{
		Username: user.Username,
		Name:     user.Username,
		Roles:    user.Roles,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(h.config.HmacSecretKey)
	if err != nil {
		h.render.Error(w, r, err, presenter.WithStatus(http.StatusInternalServerError))
		return
	}

	h.render.JSON(w, r, map[string]string{"token": tokenString})
}
