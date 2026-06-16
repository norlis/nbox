package me

import (
	"errors"
	"net/http"

	"github.com/norlis/httpgate/authz"
	"github.com/norlis/httpgate/authz/opa"
	"github.com/norlis/httpgate/presenter"
	"nbox/internal/application"
)

var errUnauthenticated = errors.New("no user found in context")

type Handler struct {
	opa *opa.Client
}

func NewHandler(opaClient *opa.Client) *Handler {
	return &Handler{opa: opaClient}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/me/permissions", h.Permissions)
}

// Permissions godoc
// @Summary     Current caller capabilities
// @Description Returns the permission names and resource patterns granted to the authenticated caller's roles (UI hints, not a security boundary).
// @Tags        me
// @Produce     json
// @Security    BearerAuth
// @Success     200 {object} map[string]any
// @Failure     401 {object} problem.Detail "Unauthorized"
// @Failure     500 {object} problem.Detail "Internal error"
// @Router      /api/me/permissions [get].
func (h *Handler) Permissions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user, ok := application.UserFromContext(ctx)
	if !ok {
		presenter.Error(w, r, errUnauthenticated, presenter.WithStatus(http.StatusUnauthorized))
		return
	}

	in := authz.Input{Payload: map[string]any{"roles": user.Roles}}

	perms, err := h.opa.Permissions(ctx, in)
	if err != nil {
		presenter.Error(w, r, err, presenter.WithStatus(http.StatusInternalServerError))
		return
	}
	res, err := h.opa.AllowedResources(ctx, in)
	if err != nil {
		presenter.Error(w, r, err, presenter.WithStatus(http.StatusInternalServerError))
		return
	}

	presenter.JSON(w, r, map[string]any{
		"permissions":       perms,
		"allowed_resources": res,
	})
}
