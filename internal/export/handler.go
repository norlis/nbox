package export

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/norlis/httpgate/presenter"
	"go.uber.org/zap"
	"nbox/internal/transport/httpx"
)

type Handler struct {
	generator *Generator
	render    *httpx.Render
	logger    *zap.Logger
}

func NewHandler(generator *Generator, render *httpx.Render, logger *zap.Logger) *Handler {
	return &Handler{generator: generator, render: render, logger: logger}
}

func (h *Handler) Register(api *http.ServeMux) {
	api.HandleFunc("GET /api/entry/export", h.Export)
}

// Export godoc
// @Summary      Export configuration entries
// @Description  Export entries in different formats (JSON, YAML, dotenv, ECS task definition) for backup or migration purposes
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
// @Failure      400 {object} problem.Detail "Invalid parameters (missing prefix or invalid format)"
// @Failure      401 {object} problem.Detail "Unauthorized - Missing or invalid token"
// @Failure      403 {object} problem.Detail "Forbidden - Insufficient permissions"
// @Failure      404 {object} problem.Detail "No entries found with specified prefix"
// @Failure      500 {object} problem.Detail "Internal server error"
// @Router       /api/entry/export [get].
func (h *Handler) Export(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	prefix := r.URL.Query().Get("prefix")
	if prefix == "" {
		h.logger.Warn("Export request without prefix")
		h.render.Error(w, r, errors.New("prefix parameter is required"), presenter.WithStatus(http.StatusBadRequest))
		return
	}

	formatStr := r.URL.Query().Get("format")
	if formatStr == "" {
		formatStr = "json"
	}

	opts := Options{
		Prefix: prefix,
		Format: Format(formatStr),
	}

	result, err := h.generator.Export(ctx, opts)
	if err != nil {
		h.logger.Error("Export failed",
			zap.Error(err),
			zap.String("prefix", prefix),
			zap.String("format", formatStr),
		)
		h.render.Error(w, r, err, presenter.WithStatus(http.StatusBadRequest))
		return
	}

	filename := h.generator.GetFilename(opts.Format, prefix)

	w.Header().Set("Content-Type", h.generator.GetContentType(opts.Format))
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)
	w.Header().Set("X-Export-Count", strconv.Itoa(len(result.Entries)))
	w.Header().Set("X-Export-Size", strconv.FormatInt(result.Size, 10))
	w.Header().Set("X-Content-Type-Options", "nosniff")

	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(result.Content); err != nil { //nolint:gosec // G705: Content-Type is a non-HTML export format and X-Content-Type-Options:nosniff is set; this is API data, not a web page
		h.logger.Error("Failed to write response", zap.Error(err))
	}

	h.logger.Info("Export completed",
		zap.String("prefix", prefix),
		zap.String("format", formatStr),
		zap.Int("entries", len(result.Entries)),
		zap.Int64("size", result.Size),
	)
}
