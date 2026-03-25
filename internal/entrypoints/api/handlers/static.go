package handlers

import (
	"net/http"

	"github.com/norlis/httpgate/pkg/adapter/apidriven/presenters"
	"nbox/internal/application"
)

// Deprecated.
type StaticHandler struct {
	config *application.Config
	render presenters.Presenters
}

// Deprecated.
func NewStaticHandler(config *application.Config, render presenters.Presenters) *StaticHandler {
	return &StaticHandler{
		config: config,
		render: render,
	}
}

// Environments godoc
// @Summary     List allowed prefixes (Deprecated)
// @Description Returns the list of allowed prefixes. Use /api/static/tree-config instead.
// @Tags        static
// @Produce     json
// @Security    BasicAuth
// @Security    BearerAuth
// @Success     200 {array}  string
// @Failure     401 {object} problem.ProblemDetail "Unauthorized"
// @Router      /api/static/environments [get]
// @Deprecated.
func (s *StaticHandler) Environments(w http.ResponseWriter, r *http.Request) {
	s.render.JSON(w, r, s.config.AllowedPrefixes)
}
