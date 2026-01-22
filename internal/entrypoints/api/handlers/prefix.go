package handlers

import (
	"errors"
	"nbox/internal/domain"
	"nbox/internal/domain/backend"
	"net/http"

	"github.com/norlis/httpgate/pkg/adapter/apidriven/presenters"
)

type PrefixConfigHandler struct {
	render       presenters.Presenters
	prefixConfig domain.PrefixConfigRepository
}

func NewPrefixConfigHandler(render presenters.Presenters, prefixConfig domain.PrefixConfigRepository) *PrefixConfigHandler {
	return &PrefixConfigHandler{
		render:       render,
		prefixConfig: prefixConfig,
	}
}

// GetByPrefix
// @Summary Prefix config
// @Description plain value
// @Tags prefix-config
// @Produce json
// @Param v query string true "prefix path"
// @Security 	 BasicAuth
// @Security 	 BearerAuth
// @Success 200 {object} backend.PrefixConfig ""
// @Failure 401 {object} problem.ProblemDetail "Unauthorized"
// @Failure 404 {object} problem.ProblemDetail "Not found"
// @Failure 500 {object} problem.ProblemDetail "Internal error"
// @Router /api/prefix/resolve [get].
func (h *PrefixConfigHandler) GetByPrefix(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	key := r.URL.Query().Get("v")

	if key == "" {
		h.render.Error(w, r, errors.New("empty prefix"), presenters.WithStatus(http.StatusBadRequest))
		return
	}

	prefix, err := h.prefixConfig.GetByPrefix(ctx, key)
	if err != nil {
		h.render.Error(w, r, err, presenters.WithStatus(http.StatusBadRequest))
		return
	}

	if prefix == nil {
		h.render.Error(w, r, errors.New("not found prefix"), presenters.WithStatus(http.StatusNotFound))
		return
	}

	h.render.JSON(w, r, prefix)
}

// List
// @Summary List Available configs
// @Description Get the list of configurations
// @Tags prefix-config
// @Produce json
// @Security BasicAuth
// @Security BearerAuth
// @Success 200 {object} []backend.PrefixConfig "List of prefix config"
// @Failure 401 {object} problem.ProblemDetail "Unauthorized"
// @Router /api/prefix [get]
func (h *PrefixConfigHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	configs, err := h.prefixConfig.List(ctx)
	if err != nil {
		h.render.Error(w, r, err, presenters.WithStatus(http.StatusInternalServerError))
		return
	}

	h.render.JSON(w, r, configs)
}

// ListBackends
// @Summary List Available Backend Types
// @Description Get the list of supported storage backends
// @Tags prefix-config
// @Produce json
// @Security BasicAuth
// @Security BearerAuth
// @Success 200 {object} []string "List of backend types (e.g. dynamodb, parameterstore)"
// @Failure 401 {object} problem.ProblemDetail "Unauthorized"
// @Router /api/prefix/backends [get]
func (h *PrefixConfigHandler) ListBackends(w http.ResponseWriter, r *http.Request) {
	types := backend.GetAllBackendTypes()
	h.render.JSON(w, r, types)
}
