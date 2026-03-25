package handlers

import (
	"encoding/json"
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
