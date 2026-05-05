package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/norlis/httpgate/pkg/adapter/apidriven/presenters"
	_ "github.com/norlis/httpgate/pkg/kit/problem"
	"nbox/internal/domain"
	"nbox/internal/domain/models"
	"nbox/internal/domain/strategies"
	"nbox/internal/usecases"
)

type BoxInput struct {
	Content string `json:"content"`
}

type BoxHandler struct {
	store      domain.TemplateAdapter
	boxUseCase *usecases.BoxUseCase
	render     presenters.Presenters
	resolver   *strategies.StrategyResolver
}

type CommandBox struct {
	ID      string     `example:"123"  json:"id"`
	Payload models.Box `json:"payload"`
}

func NewBoxHandler(
	store domain.TemplateAdapter,
	boxUseCase *usecases.BoxUseCase,
	render presenters.Presenters,
	resolver *strategies.StrategyResolver,
) *BoxHandler {
	return &BoxHandler{store: store, boxUseCase: boxUseCase, render: render, resolver: resolver}
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

// UpsertBoxV2
// @Summary Upsert template
// @Description insert or update a specific template on s3
// @Tags templates
// @Accept json
// @Produce json
// @Security BasicAuth
// @Security BearerAuth
// @Param service path string true "service name"
// @Param stage path string true "stage name"
// @Param template path string true "template name"
// @Param data body BoxInput true "Template content (Base64 encoded)"
// @Success 200 {object} []string "List of updated paths"
// @Failure 400 {object} problem.ProblemDetail "Bad Request"
// @Failure 401 {object} problem.ProblemDetail "Unauthorized"
// @Failure 500 {object} problem.ProblemDetail "Internal error"
// @Router /api/box/{service}/{stage}/{template} [post].
func (b *BoxHandler) UpsertBoxV2(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	service := r.PathValue("service")
	stage := r.PathValue("stage")
	templateName := r.PathValue("template")

	var req BoxInput

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		b.render.Error(w, r, err, presenters.WithStatus(http.StatusBadRequest))
		return
	}

	box := &models.Box{
		Service: service,
		Stage: map[string]models.Stage{
			stage: {
				Template: models.Template{
					Name:  templateName,
					Value: req.Content,
				},
			},
		},
	}

	result := b.store.UpsertBox(ctx, box)
	status := http.StatusOK
	for _, v := range result {
		if !v.Valid {
			status = http.StatusBadRequest
			break
		}
	}
	b.render.JSON(w, r, result, presenters.WithStatusCode(status))
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

	opts := make([]usecases.BuildOption, 0)
	if _, isStrict := r.URL.Query()["strict"]; isStrict {
		opts = append(opts, usecases.WithBuildTemplateStrict())
	}

	data, err := b.boxUseCase.BuildBox(ctx, service, stage, template, args, opts...)
	if err != nil {
		status := b.resolveErrorStatus(err)
		b.render.Error(w, r, err, presenters.WithStatus(status))
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

// ListSchemaTypes
// @Summary List Available Schema Types
// @Description Get the list of supported storage backends
// @Tags templates
// @Produce json
// @Security BasicAuth
// @Security BearerAuth
// @Success 200 {object} []string "List of schema types (e.g. json, txt)"
// @Failure 401 {object} problem.ProblemDetail "Unauthorized"
// @Router /api/box/schemas [get].
func (b *BoxHandler) ListSchemaTypes(w http.ResponseWriter, r *http.Request) {
	types := b.resolver.GetImplementedSchemaTypes()
	b.render.JSON(w, r, types)
}

// resolveErrorStatus maps domain errors to appropriate HTTP status codes.
func (b *BoxHandler) resolveErrorStatus(err error) int {
	switch {
	case errors.Is(err, domain.ErrMissingVariables):
		return http.StatusBadRequest
	case errors.Is(err, domain.ErrInvalidSyntax):
		return http.StatusBadRequest
	case errors.Is(err, domain.ErrInvalidTemplate):
		return http.StatusBadRequest
	case errors.Is(err, domain.ErrTemplateNotFound):
		return http.StatusNotFound
	default:
		return http.StatusInternalServerError
	}
}
