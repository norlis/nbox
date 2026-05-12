package transporthttp

import (
	"html/template"
	"io/fs"
	"net/http"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"nbox/web"
)

type UIHandler struct {
	logger  *zap.Logger
	tpl     *template.Template
	statics http.Handler
}

func NewUIHandler(logger *zap.Logger) *UIHandler {
	tpl, err := template.ParseFS(web.Files, "assets/templates/events.html")
	if err != nil {
		logger.Error("ErrParseTemplate", zap.Error(err))
		return nil
	}

	_, err = fs.Sub(web.Files, "assets")
	if err != nil {
		logger.Error("Failed to create sub filesystem for assets", zap.Error(err))
		return nil
	}

	return &UIHandler{
		logger:  logger,
		tpl:     tpl,
		statics: http.FileServer(http.FS(web.Files)),
	}
}

func (h *UIHandler) EventsPage(w http.ResponseWriter, r *http.Request) {
	data := struct {
		ClientID string
	}{
		ClientID: uuid.NewString(),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.tpl.Execute(w, data); err != nil {
		h.logger.Error("Failed to execute events template", zap.Error(err))
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func (h *UIHandler) ServeAssets() http.Handler {
	return h.statics
}
