package transporthttp

import (
	"net/http"

	"nbox/internal/nbox"
	"nbox/internal/transport/httpx"
)

type StaticHandler struct {
	config *nbox.Config
	render *httpx.Render
}

func NewStaticHandler(config *nbox.Config, render *httpx.Render) *StaticHandler {
	return &StaticHandler{config: config, render: render}
}

func (s *StaticHandler) Register(api *http.ServeMux) {
	api.HandleFunc("GET /api/static/stages", s.Stages)
}

// Stages godoc
// @Summary     List available stages
// @Description Returns the list of available stages
// @Tags        static
// @Produce     json
// @Security    BasicAuth
// @Security    BearerAuth
// @Success     200 {array}  string
// @Failure     401 {object} problem.Detail "Unauthorized"
// @Router      /api/static/stages [get].
func (s *StaticHandler) Stages(w http.ResponseWriter, r *http.Request) {
	s.render.JSON(w, r, s.config.Stages)
}
