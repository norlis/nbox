package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/norlis/httpgate/pkg/adapter/apidriven/presenters"
	_ "github.com/norlis/httpgate/pkg/kit/problem"
	"nbox/internal/domain"
	"nbox/internal/domain/models"
)

type EntryHandler struct {
	// entryAdapter  domain.EntryAdapter
	// entryUseCase  domain.EntryUseCase
	// secretAdapter domain.SecretAdapter
	store  domain.EntryManager
	render presenters.Presenters
}

// func NewEntryHandler(entryAdapter domain.EntryAdapter, secretAdapter domain.SecretAdapter, entryUseCase domain.EntryUseCase, render presenters.Presenters) *EntryHandler {
//	return &EntryHandler{entryAdapter: entryAdapter, secretAdapter: secretAdapter, entryUseCase: entryUseCase, render: render}
//}

func NewEntryHandler(store domain.EntryManager, render presenters.Presenters) *EntryHandler {
	return &EntryHandler{store: store, render: render}
}

// Upsert
// @Summary Upsert entries
// @Description insert / update vars
// @Tags entry
// @Accept json
// @Produce json
// @Param data body []models.Entry true "Upsert template"
// @Security 	 BasicAuth
// @Security 	 BearerAuth
// @Success 200 {object} map[string]string ""
// @Failure 400 {object} problem.ProblemDetail "Bad Request"
// @Failure 401 {object} problem.ProblemDetail "Unauthorized"
// @Failure 500 {object} problem.ProblemDetail "Internal error"
// @Router /api/entry [post].
func (h *EntryHandler) Upsert(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var entries []models.Entry

	if err := json.NewDecoder(r.Body).Decode(&entries); err != nil {
		h.render.Error(w, r, err, presenters.WithStatus(http.StatusBadRequest))
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
// @Security 	 BasicAuth
// @Security 	 BearerAuth
// @Success 200 {object} []models.Entry ""
// @Failure 401 {object} problem.ProblemDetail "Unauthorized"
// @Failure 500 {object} problem.ProblemDetail "Internal error"
// @Router /api/entry/prefix [get].
func (h *EntryHandler) ListByPrefix(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	prefix := r.URL.Query().Get("v")

	entries, err := h.store.List(ctx, prefix)
	if err != nil {
		h.render.Error(w, r, err, presenters.WithStatus(http.StatusBadRequest))
		return
	}

	h.render.JSON(w, r, entries)
}

// GetByKey
// @Summary Retrieve key
// @Description detail
// @Tags entry
// @Produce json
// @Param v query string true "key path"
// @Security 	 BasicAuth
// @Security 	 BearerAuth
// @Success 200 {object} models.Entry ""
// @Failure 401 {object} problem.ProblemDetail "Unauthorized"
// @Failure 500 {object} problem.ProblemDetail "Internal error"
// @Router /api/entry/key [get].
func (h *EntryHandler) GetByKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	key := r.URL.Query().Get("v")
	entry, err := h.store.Retrieve(ctx, key)
	if err != nil {
		h.render.Error(w, r, err, presenters.WithStatus(http.StatusBadRequest))
		return
	}

	h.render.JSON(w, r, entry)
}

// DeleteKey
// @Summary Delete
// @Description delete keys & children
// @Tags entry
// @Produce json
// @Param v query string true "key path"
// @Security 	 BasicAuth
// @Security 	 BearerAuth
// @Success 200 {object} object{message=string} ""
// @Failure 401 {object} problem.ProblemDetail "Unauthorized"
// @Failure 500 {object} problem.ProblemDetail "Internal error"
// @Router /api/entry/key [delete].
func (h *EntryHandler) DeleteKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	key := r.URL.Query().Get("v")
	err := h.store.Delete(ctx, key)
	if err != nil {
		h.render.Error(w, r, err, presenters.WithStatus(http.StatusBadRequest))
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
// @Security 	 BasicAuth
// @Security 	 BearerAuth
// @Success 200 {object} models.Entry ""
// @Failure 401 {object} problem.ProblemDetail "Unauthorized"
// @Failure 404 {object} problem.ProblemDetail "Not found"
// @Failure 500 {object} problem.ProblemDetail "Internal error"
// @Router /api/entry/resolve [get].
func (h *EntryHandler) Resolve(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	key := r.URL.Query().Get("v")

	if key == "" {
		h.render.Error(w, r, errors.New("empty key"), presenters.WithStatus(http.StatusBadRequest))
		return
	}

	entry, err := h.store.Resolve(ctx, key)
	if err != nil {
		h.render.Error(w, r, err, presenters.WithStatus(http.StatusBadRequest))
		return
	}

	if entry == nil {
		h.render.Error(w, r, errors.New("not found key"), presenters.WithStatus(http.StatusNotFound))
		return
	}

	h.render.JSON(w, r, entry)
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
// @Success 200 {object} map[string]models.Entry
// @Failure 400 {object} problem.ProblemDetail "Invalid request body"
// @Failure 500 {object} problem.ProblemDetail "Internal server error"
// @Router /api/entry/lookup [post].
func (h *EntryHandler) RetrieveMany(w http.ResponseWriter, r *http.Request) {
	keys := make([]string, 0)

	if err := json.NewDecoder(r.Body).Decode(&keys); err != nil {
		h.render.Error(w, r, fmt.Errorf("invalid json body: %w", err), presenters.WithStatus(http.StatusBadRequest))
		return
	}

	if len(keys) == 0 {
		h.render.JSON(w, r, map[string]models.Entry{})
		return
	}

	results, err := h.store.RetrieveMany(r.Context(), keys)
	if err != nil {
		h.render.Error(w, r, err, presenters.WithStatus(http.StatusInternalServerError))
		return
	}

	h.render.JSON(w, r, results)
}
