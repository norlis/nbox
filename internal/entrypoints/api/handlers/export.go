package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/norlis/httpgate/pkg/adapter/apidriven/presenters"
	"go.uber.org/zap"
	"nbox/internal/domain/models"
	"nbox/internal/usecases"
)

// ExportHandler maneja las peticiones de exportación.
type ExportHandler struct {
	exportUseCase *usecases.ExportUseCase
	render        presenters.Presenters
	logger        *zap.Logger
}

// NewExportHandler crea una nueva instancia.
func NewExportHandler(
	exportUseCase *usecases.ExportUseCase,
	render presenters.Presenters,
	logger *zap.Logger,
) *ExportHandler {
	return &ExportHandler{
		exportUseCase: exportUseCase,
		render:        render,
		logger:        logger,
	}
}

// Export godoc
// @Summary      Export configuration entries
// @Description  Export entries in different formats (JSON, YAML, dotenv, ECS tack definition) for backup or migration purposes
// @Description  Requires authentication via Bearer token
// @Tags         export
// @Security 	 BasicAuth
// @Security 	 BearerAuth
// @Param        prefix query string true "Prefix to filter entries (required). Example: 'production/', 'staging/myapp/'"
// @Param        format query string false "Output format" Enums(json, yaml, dotenv, ecs) default(json)
// @Produce      json
// @Produce      application/x-yaml
// @Produce      text/plain
// @Success      200 {file} binary "Exported file with entries"
// @Header       200 {string} Content-Disposition "attachment; filename=nbox-export-{prefix}-{timestamp}.{ext}"
// @Header       200 {string} X-Export-Count "Number of entries exported"
// @Header       200 {string} X-Export-Size "Size in bytes of exported file"
// @Failure      400 {object} problem.ProblemDetail "Invalid parameters (missing prefix or invalid format)"
// @Failure      401 {object} problem.ProblemDetail "Unauthorized - Missing or invalid token"
// @Failure      403 {object} problem.ProblemDetail "Forbidden - Insufficient permissions"
// @Failure      404 {object} problem.ProblemDetail "No entries found with specified prefix"
// @Failure      500 {object} problem.ProblemDetail "Internal server error"
// @Router       /api/entry/export [get].
func (h *ExportHandler) Export(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	prefix := r.URL.Query().Get("prefix")

	if prefix == "" {
		h.logger.Warn("Export request without prefix")
		h.render.Error(w, r,
			errors.New("prefix parameter is required"),
			presenters.WithStatus(http.StatusBadRequest))
		return
	}

	formatStr := r.URL.Query().Get("format")
	if formatStr == "" {
		formatStr = "json"
	}

	format := models.ExportFormat(formatStr)

	opts := models.ExportOptions{
		Prefix: prefix,
		Format: format,
	}

	result, err := h.exportUseCase.Export(ctx, opts)
	if err != nil {
		h.logger.Error("Export failed",
			zap.Error(err),
			zap.String("prefix", prefix),
			zap.String("format", formatStr),
		)
		h.render.Error(w, r, err, presenters.WithStatus(http.StatusBadRequest))
		return
	}

	filename := h.exportUseCase.GetFilename(format, prefix)

	w.Header().Set("Content-Type", h.exportUseCase.GetContentType(format))
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)
	w.Header().Set("X-Export-Count", strconv.Itoa(len(result.Entries)))
	w.Header().Set("X-Export-Size", strconv.FormatInt(result.Size, 10))

	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(result.Content); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}

	h.logger.Info("Export completed",
		zap.String("prefix", prefix),
		zap.String("format", formatStr),
		zap.Int("entries", len(result.Entries)),
		zap.Int64("size", result.Size),
	)
}
