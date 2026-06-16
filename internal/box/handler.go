package box

import (
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/norlis/httpgate/presenter"
	_ "github.com/norlis/httpgate/problem"
	"nbox/internal/box/store/spec"
	"nbox/internal/transport/httpx"
)

type Input struct {
	Content string `json:"content"`
}

type ValidateInput struct {
	Filename string `json:"filename"`
	Content  string `json:"content"`
}

type Handler struct {
	store    Store
	renderer *Renderer
	registry *spec.SpecRegistry
	render   *httpx.Render
	resolver *StrategyResolver
}

func NewHandler(
	store Store,
	renderer *Renderer,
	registry *spec.SpecRegistry,
	render *httpx.Render,
	resolver *StrategyResolver,
) *Handler {
	return &Handler{
		store:    store,
		renderer: renderer,
		registry: registry,
		render:   render,
		resolver: resolver,
	}
}

func (h *Handler) Register(api *http.ServeMux) {
	// box / template routes
	api.HandleFunc("POST /api/box", h.UpsertBox) // Deprecated
	api.HandleFunc("GET /api/box", h.List)
	api.HandleFunc("POST /api/box/{service}/{stage}/{template}", h.UpsertBoxV2)
	api.HandleFunc("HEAD /api/box/{service}/{stage}/{template}", h.Exist)
	api.HandleFunc("GET /api/box/{service}/{stage}/{template}", h.Retrieve)
	api.HandleFunc("GET /api/box/{service}/{stage}", h.Stage)
	api.HandleFunc("GET /api/box/{service}/{stage}/{template}/build", h.Build)
	api.HandleFunc("GET /api/box/{service}/{stage}/{template}/vars", h.ListVars)
	api.HandleFunc("GET /api/box/schemas", h.ListSchemaTypes)

	// boxspec routes
	api.HandleFunc("GET /api/boxspec/specs", h.ListSpecs)
	api.HandleFunc("GET /api/boxspec/resolve", h.ResolveSpec)
	api.HandleFunc("POST /api/boxspec/reload", h.ReloadSpecs)
	api.HandleFunc("POST /api/boxspec/validate", h.ValidateTemplate)
}

// UpsertBox
// @Summary Upsert templates
// @Description insert or update templates on s3
// @Tags templates
// @Accept json
// @Produce json
// @Security BasicAuth
// @Security BearerAuth
// @Param data body Command[Box] true "Upsert template"
// @Success 200 {object} []string ""
// @Failure 400 {object} problem.Detail "Bad Request"
// @Failure 401 {object} problem.Detail "Unauthorized"
// @Failure 500 {object} problem.Detail "Internal error"
// @Router /api/box [post].
func (h *Handler) UpsertBox(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	command := &Command[Box]{}
	if err := json.NewDecoder(r.Body).Decode(command); err != nil {
		h.render.Error(w, r, err, presenter.WithStatus(http.StatusBadRequest))
		return
	}

	result := h.store.UpsertBox(ctx, &command.Payload)
	h.render.JSON(w, r, result)
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
// @Param data body Input true "Template content (Base64 encoded)"
// @Success 200 {object} []string "List of updated paths"
// @Failure 400 {object} problem.Detail "Bad Request"
// @Failure 401 {object} problem.Detail "Unauthorized"
// @Failure 500 {object} problem.Detail "Internal error"
// @Router /api/box/{service}/{stage}/{template} [post].
func (h *Handler) UpsertBoxV2(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	service := r.PathValue("service")
	stage := r.PathValue("stage")
	templateName := r.PathValue("template")

	var req Input
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.render.Error(w, r, err, presenter.WithStatus(http.StatusBadRequest))
		return
	}

	bx := &Box{
		Service: service,
		Stage: map[string]Stage{
			stage: {
				Template: Template{
					Name:  templateName,
					Value: req.Content,
				},
			},
		},
	}

	result := h.store.UpsertBox(ctx, bx)
	status := http.StatusOK
	for _, v := range result {
		if !v.Valid {
			status = http.StatusBadRequest
			break
		}
	}
	h.render.JSON(w, r, result, presenter.WithStatusCode(status))
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
// @Security BasicAuth
// @Security BearerAuth
// @Success 200 {object} object{exist=bool} ""
// @Failure 401 {object} problem.Detail "Unauthorized"
// @Failure 500 {object} problem.Detail "Internal error"
// @Router /api/box/{service}/{stage}/{template} [head].
func (h *Handler) Exist(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	service := r.PathValue("service")
	stage := r.PathValue("stage")
	template := r.PathValue("template")

	exists, err := h.store.BoxExists(ctx, service, stage, template)
	if err != nil {
		h.render.Error(w, r, err, presenter.WithStatus(http.StatusNotFound))
		return
	}

	h.render.JSON(w, r, map[string]bool{"exist": exists})
}

// Retrieve
// @Summary Retrieve template
// @Description detail
// @Tags templates
// @Produce plain
// @Param service path string true "service name"
// @Param stage path string true "stage"
// @Param template path string true "template name"
// @Security BasicAuth
// @Security BearerAuth
// @Success 200 {object} string ""
// @Failure 401 {object} problem.Detail "Unauthorized"
// @Failure 500 {object} problem.Detail "Internal error"
// @Router /api/box/{service}/{stage}/{template} [get].
func (h *Handler) Retrieve(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	service := r.PathValue("service")
	stage := r.PathValue("stage")
	template := r.PathValue("template")

	data, err := h.store.RetrieveBox(ctx, service, stage, template)
	if err != nil {
		h.render.Error(w, r, err, presenter.WithStatus(http.StatusNotFound))
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(data) //nolint:gosec // G705: text/plain + nosniff; template content served as data, not HTML
}

// Build
// @Summary Build template
// @Description replace vars patterns
// @Tags templates
// @Produce plain
// @Param service path string true "service name"
// @Param stage path string true "stage"
// @Param template path string true "template name"
// @Security BasicAuth
// @Security BearerAuth
// @Success 200 {object} string ""
// @Failure 401 {object} problem.Detail "Unauthorized"
// @Failure 500 {object} problem.Detail "Internal error"
// @Router /api/box/{service}/{stage}/{template}/build [get].
func (h *Handler) Build(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	service := r.PathValue("service")
	stage := r.PathValue("stage")
	template := r.PathValue("template")

	q := r.URL.Query()
	args := make(map[string]string)
	for key := range q {
		if key == "service" || key == "stage" || key == "template" {
			continue
		}
		args[key] = q.Get(key)
	}

	var opts []BuildOption
	if _, isStrict := q["strict"]; isStrict {
		opts = append(opts, WithBuildTemplateStrict())
	}

	data, err := h.renderer.BuildBox(ctx, service, stage, template, args, opts...)
	if err != nil {
		h.render.Error(w, r, err, presenter.WithStatus(h.resolveErrorStatus(err)))
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write([]byte(data)) //nolint:gosec // G705: text/plain + nosniff; rendered template served as data, not HTML
}

// List
// @Summary List templates
// @Description all templates
// @Tags templates
// @Accept json
// @Produce json
// @Security BasicAuth
// @Security BearerAuth
// @Success 200 {object} []Box ""
// @Failure 400 {object} problem.Detail "Bad Request"
// @Failure 401 {object} problem.Detail "Unauthorized"
// @Failure 500 {object} problem.Detail "Internal error"
// @Router /api/box [get].
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	data, err := h.store.List(ctx)
	if err != nil {
		h.render.Error(w, r, err, presenter.WithStatus(http.StatusNotFound))
		return
	}

	h.render.JSON(w, r, data)
}

// Stage
// @Summary Stage detail
// @Description show metadata & template names
// @Tags templates
// @Produce json
// @Param service path string true "service name"
// @Param stage path string true "stage"
// @Security BasicAuth
// @Security BearerAuth
// @Success 200 {object} Stage ""
// @Failure 401 {object} problem.Detail "Unauthorized"
// @Failure 500 {object} problem.Detail "Internal error"
// @Router /api/box/{service}/{stage} [get].
func (h *Handler) Stage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	service := r.PathValue("service")
	stage := r.PathValue("stage")
	data, err := h.renderer.Detail(ctx, service, stage)
	if err != nil {
		h.render.Error(w, r, err, presenter.WithStatus(http.StatusNotFound))
		return
	}
	h.render.JSON(w, r, data)
}

// ListVars
// @Summary List vars template
// @Description show all vars in template
// @Tags templates
// @Produce json
// @Param service path string true "service name"
// @Param stage path string true "stage"
// @Param template path string true "template name"
// @Security BasicAuth
// @Security BearerAuth
// @Success 200 {object} []string ""
// @Failure 401 {object} problem.Detail "Unauthorized"
// @Failure 500 {object} problem.Detail "Internal error"
// @Router /api/box/{service}/{stage}/{template}/vars [get].
func (h *Handler) ListVars(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	service := r.PathValue("service")
	stage := r.PathValue("stage")
	template := r.PathValue("template")

	data := h.renderer.ListVars(ctx, service, stage, template)
	h.render.JSON(w, r, data)
}

// ListSchemaTypes
// @Summary List Available Schema Types
// @Description Get the list of supported storage backends
// @Tags templates
// @Produce json
// @Security BasicAuth
// @Security BearerAuth
// @Success 200 {object} []string "List of schema types (e.g. json, txt)"
// @Failure 401 {object} problem.Detail "Unauthorized"
// @Router /api/box/schemas [get].
func (h *Handler) ListSchemaTypes(w http.ResponseWriter, r *http.Request) {
	types := h.resolver.GetImplementedSchemaTypes()
	h.render.JSON(w, r, types)
}

// ListSpecs
// @Summary     List available validation specs
// @Description Get list of schemas available for template validation
// @Tags        boxspec
// @Produce     json
// @Security    BasicAuth
// @Security    BearerAuth
// @Success     200 {array} []spec.SpecDefinition
// @Failure     401 {object} problem.Detail "Unauthorized"
// @Failure     500 {object} problem.Detail "Internal error"
// @Router      /api/boxspec/specs [get].
func (h *Handler) ListSpecs(w http.ResponseWriter, r *http.Request) {
	specs := h.registry.List()
	h.render.JSON(w, r, specs)
}

// ReloadSpecs
// @Summary     Reload validation specs
// @Description Hot-reload specs from filesystem without restart
// @Tags        boxspec
// @Produce     json
// @Security    BasicAuth
// @Security    BearerAuth
// @Success     200 {object} map[string]int "count of loaded specs"
// @Failure     401 {object} problem.Detail "Unauthorized"
// @Failure     500 {object} problem.Detail "Internal error"
// @Router      /api/boxspec/reload [post].
func (h *Handler) ReloadSpecs(w http.ResponseWriter, r *http.Request) {
	if err := h.registry.Reload(r.Context()); err != nil {
		h.render.Error(w, r, err, presenter.WithStatus(http.StatusInternalServerError))
		return
	}
	specs := h.registry.List()
	h.render.JSON(w, r, map[string]int{"loaded": len(specs)})
}

// ResolveSpec
// @Summary     Resolve spec and export as JSON Schema
// @Description Given a filename pattern, resolves the matching spec and returns its JSON Schema representation
// @Tags        boxspec
// @Produce     json
// @Security    BasicAuth
// @Security    BearerAuth
// @Param       pattern query string true "Filename pattern to match (e.g., task-definition.json)"
// @Success     200 {object} map[string]interface{} "JSON Schema"
// @Failure     400 {object} problem.Detail "Missing pattern parameter"
// @Failure     404 {object} problem.Detail "No matching spec found"
// @Failure     500 {object} problem.Detail "Internal error"
// @Router      /api/boxspec/resolve [get].
func (h *Handler) ResolveSpec(w http.ResponseWriter, r *http.Request) {
	pattern := r.URL.Query().Get("pattern")
	if pattern == "" {
		h.render.Error(w, r, errors.New("missing required query parameter: pattern"), presenter.WithStatus(http.StatusBadRequest))
		return
	}

	s, schemaBytes, err := h.registry.ExportJSONSchema(r.Context(), pattern)
	if err != nil {
		h.render.Error(w, r, err, presenter.WithStatus(http.StatusUnprocessableEntity))
		return
	}

	var schema any
	if err := json.Unmarshal(schemaBytes, &schema); err != nil {
		h.render.Error(w, r, err, presenter.WithStatus(http.StatusInternalServerError))
		return
	}

	h.render.JSON(w, r, map[string]any{"spec": s, "schema": schema})
}

// ValidateTemplate
// @Summary Validate template
// @Description Check if a template complies with defined specs without saving
// @Tags templates
// @Accept json
// @Produce json
// @Param data body ValidateInput true "Template data"
// @Success 200 {object} spec.ValidationResult
// @Failure 400 {object} problem.Detail "Validation Failed"
// @Router /api/boxspec/validate [post].
func (h *Handler) ValidateTemplate(w http.ResponseWriter, r *http.Request) {
	var input ValidateInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		h.render.Error(w, r, err, presenter.WithStatus(http.StatusBadRequest))
		return
	}

	contentBytes, _, err := h.resolver.Process(input.Filename, input.Content)
	if err != nil {
		h.render.Error(w, r, err, presenter.WithStatus(http.StatusBadRequest))
		return
	}

	format := strings.TrimPrefix(filepath.Ext(input.Filename), ".")

	result, err := h.registry.ValidateByFilename(r.Context(), input.Filename, contentBytes, format)
	if err != nil {
		h.render.Error(w, r, err, presenter.WithStatus(http.StatusInternalServerError))
		return
	}

	h.render.JSON(w, r, result)
}

// resolveErrorStatus maps domain errors to appropriate HTTP status codes.
func (h *Handler) resolveErrorStatus(err error) int {
	switch {
	case errors.Is(err, ErrMissingVariables):
		return http.StatusBadRequest
	case errors.Is(err, ErrInvalidSyntax):
		return http.StatusBadRequest
	case errors.Is(err, ErrInvalidTemplate):
		return http.StatusBadRequest
	case errors.Is(err, ErrTemplateNotFound):
		return http.StatusNotFound
	default:
		return http.StatusInternalServerError
	}
}
