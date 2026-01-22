package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/norlis/httpgate/pkg/adapter/apidriven/presenters"
	_ "github.com/norlis/httpgate/pkg/kit/problem"
	"nbox/internal/domain"
	"nbox/internal/domain/models"
	"nbox/internal/usecases"
)

type BoxHandler struct {
	store      domain.TemplateAdapter
	boxUseCase *usecases.BoxUseCase
	render     presenters.Presenters
}

type CommandBox struct {
	ID      string     `example:"123"  json:"id"`
	Payload models.Box `json:"payload"`
}

func NewBoxHandler(store domain.TemplateAdapter, boxUseCase *usecases.BoxUseCase, render presenters.Presenters) *BoxHandler {
	return &BoxHandler{store: store, boxUseCase: boxUseCase, render: render}
}

// UpsertBox
// @Summary Upsert templates
// @Description insert or update templates on s3
// @Tags templates
// @Accept json
// @Produce json
// @Security 	 BasicAuth
// @Security 	 BearerAuth
// @Param data body CommandBox true "Upsert template"
// @Success 200 {object} []string ""
// @Failure 400 {object} problem.ProblemDetail "Bad Request"
// @Failure 401 {object} problem.ProblemDetail "Unauthorized"
// @Failure 500 {object} problem.ProblemDetail "Internal error"
// @Router /api/box [post].
func (b *BoxHandler) UpsertBox(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	command := &models.Command[models.Box]{}
	if err := json.NewDecoder(r.Body).Decode(command); err != nil {
		b.render.Error(w, r, err, presenters.WithStatus(http.StatusBadRequest))
		return
	}

	result := b.store.UpsertBox(ctx, &command.Payload)
	b.render.JSON(w, r, result)
}

// Exist
// @Summary Exist template
// @Description Check the existence of the template
// @Tags templates
// @Accept json
// @Produce json
// @Param service path string true "service name"
// @Param stage path string true "stage"
// @Param template path string true "template name"
// @Security 	 BasicAuth
// @Security 	 BearerAuth
// @Success 200 {object}  object{exit=bool} ""
// @Failure 401 {object} problem.ProblemDetail "Unauthorized"
// @Failure 500 {object} problem.ProblemDetail "Internal error"
// @Router /api/box/{service}/{stage}/{template} [head].
func (b *BoxHandler) Exist(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	service := r.PathValue("service")
	stage := r.PathValue("stage")
	template := r.PathValue("template")

	exists, err := b.store.BoxExists(ctx, service, stage, template)
	if err != nil {
		b.render.Error(w, r, err, presenters.WithStatus(http.StatusNotFound))
		return
	}

	b.render.JSON(w, r, map[string]bool{"exist": exists})
}

// Retrieve
// @Summary Retrieve template
// @Description detail
// @Tags templates
// @Produce plain
// @Param service path string true "service name"
// @Param stage path string true "stage"
// @Param template path string true "template name"
// @Security 	 BasicAuth
// @Security 	 BearerAuth
// @Success 200 {object}  string ""
// @Failure 401 {object} problem.ProblemDetail "Unauthorized"
// @Failure 500 {object} problem.ProblemDetail "Internal error"
// @Router /api/box/{service}/{stage}/{template} [get].
func (b *BoxHandler) Retrieve(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	service := r.PathValue("service")
	stage := r.PathValue("stage")
	template := r.PathValue("template")

	data, err := b.store.RetrieveBox(ctx, service, stage, template)
	if err != nil {
		b.render.Error(w, r, err, presenters.WithStatus(http.StatusNotFound))
		return
	}

	// b.render.PlainText(w, r, data)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write(data)
}

// Build
// @Summary Build template
// @Description replace vars patterns
// @Tags templates
// @Produce plain
// @Param service path string true "service name"
// @Param stage path string true "stage"
// @Param template path string true "template name"
// @Security 	 BasicAuth
// @Security 	 BearerAuth
// @Success 200 {object}  string ""
// @Failure 401 {object} problem.ProblemDetail "Unauthorized"
// @Failure 500 {object} problem.ProblemDetail "Internal error"
// @Router /api/box/{service}/{stage}/{template}/build [get].
func (b *BoxHandler) Build(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	service := r.PathValue("service")
	stage := r.PathValue("stage")
	template := r.PathValue("template")
	args := make(map[string]string)

	for key := range r.URL.Query() {
		if key == "service" || key == "stage" || key == "template" {
			continue
		}
		args[key] = r.URL.Query().Get(key)
	}

	data, err := b.boxUseCase.BuildBox(ctx, service, stage, template, args)
	if err != nil {
		b.render.Error(w, r, err, presenters.WithStatus(http.StatusNotFound))
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(data))
}

// List
// @Summary List templates
// @Description all templates
// @Tags templates
// @Accept json
// @Produce json
// @Security 	 BasicAuth
// @Security 	 BearerAuth
// @Success 200 {object} []models.Box ""
// @Failure 400 {object} problem.ProblemDetail "Bad Request"
// @Failure 401 {object} problem.ProblemDetail "Unauthorized"
// @Failure 500 {object} problem.ProblemDetail "Internal error"
// @Router /api/box [get].
func (b *BoxHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	data, err := b.store.List(ctx)
	if err != nil {
		b.render.Error(w, r, err, presenters.WithStatus(http.StatusNotFound))
		return
	}

	b.render.JSON(w, r, data)
}

// ListVars
// @Summary List vars template
// @Description show all vars in template
// @Tags templates
// @Produce json
// @Param service path string true "service name"
// @Param stage path string true "stage"
// @Param template path string true "template name"
// @Security 	 BasicAuth
// @Security 	 BearerAuth
// @Success 200 {object}  []string ""
// @Failure 401 {object} problem.ProblemDetail "Unauthorized"
// @Failure 500 {object} problem.ProblemDetail "Internal error"
// @Router /api/box/{service}/{stage}/{template}/vars [get].
func (b *BoxHandler) ListVars(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	service := r.PathValue("service")
	stage := r.PathValue("stage")
	template := r.PathValue("template")

	data := b.boxUseCase.ListVars(ctx, service, stage, template)
	b.render.JSON(w, r, data)
}
