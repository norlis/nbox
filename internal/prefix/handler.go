package prefix

import (
	"errors"
	"net/http"

	"github.com/norlis/httpgate/pkg/adapter/apidriven/presenters"
)

type Handler struct {
	render       presenters.Presenters
	prefixConfig Store
}

func NewHandler(render presenters.Presenters, prefixConfig Store) *Handler {
	return &Handler{render: render, prefixConfig: prefixConfig}
}

func (h *Handler) Register(api *http.ServeMux) {
	api.HandleFunc("GET /api/prefix/resolve", h.ByPrefix)
	api.HandleFunc("GET /api/prefix", h.List)
	api.HandleFunc("GET /api/prefix/backends", h.ListBackends)
}

// ByPrefix
// @Summary Prefix config
// @Description plain value
// @Tags prefix-config
// @Produce json
// @Param v query string true "prefix path"
// @Security BasicAuth
// @Security BearerAuth
// @Success 200 {object} Config ""
// @Failure 401 {object} problem.ProblemDetail "Unauthorized"
// @Failure 404 {object} problem.ProblemDetail "Not found"
// @Failure 500 {object} problem.ProblemDetail "Internal error"
// @Router /api/prefix/resolve [get].
func (h *Handler) ByPrefix(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	key := r.URL.Query().Get("v")

	if key == "" {
		h.render.Error(w, r, errors.New("empty prefix"), presenters.WithStatus(http.StatusBadRequest))
		return
	}

	cfg, err := h.prefixConfig.ByPrefix(ctx, key)
	if err != nil {
		h.render.Error(w, r, err, presenters.WithStatus(http.StatusBadRequest))
		return
	}

	if cfg == nil {
		h.render.Error(w, r, errors.New("not found prefix"), presenters.WithStatus(http.StatusNotFound))
		return
	}

	h.render.JSON(w, r, cfg)
}

// List
// @Summary List Available configs
// @Description Get the list of configurations
// @Tags prefix-config
// @Produce json
// @Security BasicAuth
// @Security BearerAuth
// @Success 200 {object} []Config "List of prefix config"
// @Failure 401 {object} problem.ProblemDetail "Unauthorized"
// @Router /api/prefix [get].
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
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
// @Router /api/prefix/backends [get].
func (h *Handler) ListBackends(w http.ResponseWriter, r *http.Request) {
	types := AllBackendTypes()
	h.render.JSON(w, r, types)
}
