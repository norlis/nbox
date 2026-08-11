package entry

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/norlis/httpgate/presenter"
	_ "github.com/norlis/httpgate/problem"
	"nbox/internal/transport/httpx"
)

type Handler struct {
	store  Manager
	render *httpx.Render
}

func NewHandler(store Manager, render *httpx.Render) *Handler {
	return &Handler{store: store, render: render}
}

func (h *Handler) Register(api *http.ServeMux) {
	api.HandleFunc("POST /api/entry", h.Upsert)
	api.HandleFunc("GET /api/entry/key", h.GetByKey)
	api.HandleFunc("GET /api/entry/prefix", h.ListByPrefix)
	api.HandleFunc("DELETE /api/entry/key", h.DeleteKey)
	api.HandleFunc("GET /api/entry/resolve", h.Resolve)
	// Deprecated: legacy alias for /api/entry/resolve, kept for a client still on the old path. Remove once migrated.
	api.HandleFunc("GET /api/entry/secret-value", h.Resolve)
	api.HandleFunc("POST /api/entry/lookup", h.RetrieveMany)
}

// Upsert
// @Summary Upsert entries
// @Description insert / update vars
// @Tags entry
// @Accept json
// @Produce json
// @Param data body []Entry true "Upsert template"
// @Security BasicAuth
// @Security BearerAuth
// @Success 200 {object} map[string]string ""
// @Failure 400 {object} problem.Detail "Bad Request"
// @Failure 401 {object} problem.Detail "Unauthorized"
// @Failure 500 {object} problem.Detail "Internal error"
// @Router /api/entry [post].
func (h *Handler) Upsert(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var entries []Entry

	if err := json.NewDecoder(r.Body).Decode(&entries); err != nil {
		h.render.Error(w, r, err, presenter.WithStatus(http.StatusBadRequest))
		return
	}

	h.render.JSON(w, r, h.store.Upsert(ctx, entries))
}

// ListByPrefix
// @Summary Filter by prefix
// @Description list all keys by path
// @Tags entry
// @Produce json
// @Param v query string true "key path"
// @Param leaves query bool false "every real key under the prefix, flat (no folder markers / empty values)"
// @Security BasicAuth
// @Security BearerAuth
// @Success 200 {object} []Entry ""
// @Failure 401 {object} problem.Detail "Unauthorized"
// @Failure 500 {object} problem.Detail "Internal error"
// @Router /api/entry/prefix [get].
func (h *Handler) ListByPrefix(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	prefix := r.URL.Query().Get("v")

	var opts []ListOption
	if r.URL.Query().Has("leaves") {
		opts = append(opts, LeavesOnly())
	}

	entries, err := h.store.List(ctx, prefix, opts...)
	if err != nil {
		h.render.Error(w, r, err, presenter.WithStatus(http.StatusBadRequest))
		return
	}
	if entries == nil {
		entries = []Entry{} // serialize [] not null
	}

	h.render.JSON(w, r, entries)
}

// GetByKey
// @Summary Retrieve key
// @Description detail
// @Tags entry
// @Produce json
// @Param v query string true "key path"
// @Security BasicAuth
// @Security BearerAuth
// @Success 200 {object} Entry ""
// @Failure 401 {object} problem.Detail "Unauthorized"
// @Failure 500 {object} problem.Detail "Internal error"
// @Router /api/entry/key [get].
func (h *Handler) GetByKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	key := r.URL.Query().Get("v")
	e, err := h.store.Retrieve(ctx, key)
	if err != nil {
		h.render.Error(w, r, err, presenter.WithStatus(http.StatusBadRequest))
		return
	}

	h.render.JSON(w, r, e)
}

// DeleteKey
// @Summary Delete
// @Description delete keys & children
// @Tags entry
// @Produce json
// @Param v query string true "key path"
// @Security BasicAuth
// @Security BearerAuth
// @Success 200 {object} object{message=string} ""
// @Failure 401 {object} problem.Detail "Unauthorized"
// @Failure 500 {object} problem.Detail "Internal error"
// @Router /api/entry/key [delete].
func (h *Handler) DeleteKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	key := r.URL.Query().Get("v")
	err := h.store.Delete(ctx, key)
	if err != nil {
		h.render.Error(w, r, err, presenter.WithStatus(http.StatusBadRequest))
		return
	}

	h.render.JSON(w, r, map[string]string{"message": "ok"})
}

// Resolve
// @Summary Resolve value
// @Description plain value
// @Tags entry
// @Produce json
// @Param v query string true "key path"
// @Security BasicAuth
// @Security BearerAuth
// @Success 200 {object} Entry ""
// @Failure 401 {object} problem.Detail "Unauthorized"
// @Failure 404 {object} problem.Detail "Not found"
// @Failure 500 {object} problem.Detail "Internal error"
// @Router /api/entry/resolve [get].
func (h *Handler) Resolve(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	key := r.URL.Query().Get("v")

	if key == "" {
		h.render.Error(w, r, errors.New("empty key"), presenter.WithStatus(http.StatusBadRequest))
		return
	}

	e, err := h.store.Resolve(ctx, key)
	if err != nil {
		h.render.Error(w, r, err, presenter.WithStatus(http.StatusBadRequest))
		return
	}

	if e == nil {
		h.render.Error(w, r, errors.New("not found key"), presenter.WithStatus(http.StatusNotFound))
		return
	}

	h.render.JSON(w, r, e)
}

// RetrieveMany
// @Summary Retrieve multiple entries
// @Description Retrieve values and metadata for a list of keys in a single request.
// @Tags entry
// @Accept json
// @Produce json
// @Param request body []string true "List of keys to retrieve"
// @Security BasicAuth
// @Security BearerAuth
// @Success 200 {object} map[string]Entry
// @Failure 400 {object} problem.Detail "Invalid request body"
// @Failure 500 {object} problem.Detail "Internal server error"
// @Router /api/entry/lookup [post].
func (h *Handler) RetrieveMany(w http.ResponseWriter, r *http.Request) {
	keys := make([]string, 0)

	if err := json.NewDecoder(r.Body).Decode(&keys); err != nil {
		h.render.Error(w, r, fmt.Errorf("invalid json body: %w", err), presenter.WithStatus(http.StatusBadRequest))
		return
	}

	if len(keys) == 0 {
		h.render.JSON(w, r, map[string]Entry{})
		return
	}

	results, err := h.store.RetrieveMany(r.Context(), keys)
	if err != nil {
		h.render.Error(w, r, err, presenter.WithStatus(http.StatusInternalServerError))
		return
	}

	h.render.JSON(w, r, results)
}
