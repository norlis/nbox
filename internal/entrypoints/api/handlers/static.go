package handlers

import (
	"net/http"

	"github.com/norlis/httpgate/pkg/adapter/apidriven/presenters"
	"nbox/internal/application"
)

type StaticHandler struct {
	config *application.Config
	render presenters.Presenters
}

func NewStaticHandler(config *application.Config, render presenters.Presenters) *StaticHandler {
	return &StaticHandler{
		config: config,
		render: render,
	}
}

// Stages godoc
// @Summary     List available stages
// @Description Returns the list of available stages
// @Tags        static
// @Produce     json
// @Security    BasicAuth
// @Security    BearerAuth
// @Success     200 {array}  string
// @Failure     401 {object} problem.ProblemDetail "Unauthorized"
// @Router      /api/static/stages [get].
func (s *StaticHandler) Stages(w http.ResponseWriter, r *http.Request) {
	s.render.JSON(w, r, s.config.Stages)
}

// Environments godoc
// @Summary     List allowed prefixes (Deprecated)
// @Description Deprecated: Use /api/static/stages instead.
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
