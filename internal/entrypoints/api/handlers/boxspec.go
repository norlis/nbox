package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/norlis/httpgate/pkg/adapter/apidriven/presenters"
	"nbox/internal/domain/boxspec"
	"nbox/internal/domain/strategies"
)

type ValidateInput struct {
	Filename string `json:"filename"`
	Content  string `json:"content"`
}

type BoxSpecHandler struct {
	registry *boxspec.SpecRegistry
	render   presenters.Presenters
	resolver *strategies.StrategyResolver
}

func NewBoxSpecHandler(registry *boxspec.SpecRegistry, render presenters.Presenters, resolver *strategies.StrategyResolver) *BoxSpecHandler {
	return &BoxSpecHandler{registry: registry, render: render, resolver: resolver}
}

// List godoc
// @Summary     List available validation specs
// @Description Get list of schemas available for template validation
// @Tags        boxspec
// @Produce     json
// @Security 	BasicAuth
// @Security 	BearerAuth
// @Success     200 {array}  []boxspec.SpecDefinition
// @Failure     401 {object} problem.ProblemDetail "Unauthorized"
// @Failure     500 {object} problem.ProblemDetail "Internal error"
// @Router      /api/boxspec/specs [get].
func (h *BoxSpecHandler) List(w http.ResponseWriter, r *http.Request) {
	specs := h.registry.List()
	h.render.JSON(w, r, specs)
}

// Reload godoc
// @Summary     Reload validation specs
// @Description Hot-reload specs from filesystem without restart
// @Tags        boxspec
// @Produce     json
// @Security 	BasicAuth
// @Security 	BearerAuth
// @Success     200 {object} map[string]int "count of loaded specs"
// @Failure     401 {object} problem.ProblemDetail "Unauthorized"
// @Failure     500 {object} problem.ProblemDetail "Internal error"
// @Router      /api/boxspec/reload [post].
func (h *BoxSpecHandler) Reload(w http.ResponseWriter, r *http.Request) {
	if err := h.registry.Reload(r.Context()); err != nil {
		h.render.Error(w, r, err, presenters.WithStatus(http.StatusInternalServerError))
		return
	}
	specs := h.registry.List()
	h.render.JSON(w, r, map[string]int{"loaded": len(specs)})
}

// Resolve godoc
// @Summary     Resolve spec and export as JSON Schema
// @Description Given a filename pattern, resolves the matching spec and returns its JSON Schema representation
// @Tags        boxspec
// @Produce     json
// @Security 	BasicAuth
// @Security 	BearerAuth
// @Param       pattern query    string true "Filename pattern to match (e.g., task-definition.json)"
// @Success     200 {object} map[string]interface{} "JSON Schema"
// @Failure     400 {object} problem.ProblemDetail "Missing pattern parameter"
// @Failure     404 {object} problem.ProblemDetail "No matching spec found"
// @Failure     500 {object} problem.ProblemDetail "Internal error"
// @Router      /api/boxspec/resolve [get].
func (h *BoxSpecHandler) Resolve(w http.ResponseWriter, r *http.Request) {
	pattern := r.URL.Query().Get("pattern")
	if pattern == "" {
		h.render.Error(w, r, errors.New("missing required query parameter: pattern"), presenters.WithStatus(http.StatusBadRequest))
		return
	}

	spec, schemaBytes, err := h.registry.ExportJSONSchema(r.Context(), pattern)
	if err != nil {
		h.render.Error(w, r, err, presenters.WithStatus(http.StatusUnprocessableEntity))
		return
	}

	var schema any
	if err := json.Unmarshal(schemaBytes, &schema); err != nil {
		h.render.Error(w, r, err, presenters.WithStatus(http.StatusInternalServerError))
		return
	}

	h.render.JSON(w, r, map[string]any{
		"spec":   spec,
		"schema": schema,
	})
}

// ValidateTemplate
// @Summary Validate template
// @Description Check if a template complies with defined specs without saving
// @Tags templates
// @Accept json
// @Produce json
// @Param data body ValidateInput true "Template data"
// @Success 200 {object} boxspec.ValidationResult
// @Failure 400 {object} problem.ProblemDetail "Validation Failed"
// @Router /api/boxspec/validate [post].
func (h *BoxSpecHandler) ValidateTemplate(w http.ResponseWriter, r *http.Request) {
	var input ValidateInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		h.render.Error(w, r, err, presenters.WithStatus(http.StatusBadRequest))
		return
	}

	// 1. Procesar el contenido (Decodificar Base64 + Formatear según extensión)
	// Usamos el resolver para obtener los bytes limpios tal como se guardarían.
	contentBytes, _, err := h.resolver.Process(input.Filename, input.Content)
	if err != nil {
		h.render.Error(w, r, err, presenters.WithStatus(http.StatusBadRequest))
		return
	}

	// 2. Detectar formato para el motor de validación (json, yaml)
	format := strings.TrimPrefix(filepath.Ext(input.Filename), ".")

	// 3. Validar contra el registro
	result, err := h.registry.ValidateByFilename(r.Context(), input.Filename, contentBytes, format)
	if err != nil {
		// Error técnico (ej. sintaxis CUE inválida interna)
		h.render.Error(w, r, err, presenters.WithStatus(http.StatusInternalServerError))
		return
	}

	// Retornamos el resultado (que incluye valid: true/false y lista de errores)
	h.render.JSON(w, r, result)
}
