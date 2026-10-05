package credentials

import (
	"database/sql"
	"errors"
	"github.com/alumasinde/gopass/internal/platform/apperr"
	"github.com/alumasinde/gopass/internal/platform/audit"
	"github.com/alumasinde/gopass/internal/platform/httpx"
	"github.com/alumasinde/gopass/internal/platform/rbac"
	"github.com/alumasinde/gopass/internal/platform/tenancy"
	"github.com/go-chi/chi/v5"
	"net/http"
)

type Handler struct {
	authz   *rbac.Service
	audit   *audit.Service
	service *Service
}

func NewHandler(db *sql.DB, a *rbac.Service, au *audit.Service) *Handler {
	return &Handler{a, au, NewService(db, a)}
}
func (h *Handler) registerRoutes(r chi.Router) {
	r.Route("/credentials", func(r chi.Router) {
		r.Get("/", h.List)
		r.Post("/", h.Create)
		r.Get("/{id}", h.Get)
		r.Post("/{id}/revoke", h.Revoke)
		r.Post("/verify", h.Verify)
	})
}
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	if h.authz.Require(r.Context(), "credentials.view") != nil {
		httpx.Fail(w, apperr.Forbidden)
		return
	}
	o, _ := tenancy.ID(r.Context())
	v, e := h.service.List(r.Context(), o)
	if e != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	httpx.OK(w, v)
}
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	if h.authz.Require(r.Context(), "credentials.view") != nil {
		httpx.Fail(w, apperr.Forbidden)
		return
	}
	id, e := httpx.ID(chi.URLParam(r, "id"))
	if e != nil {
		httpx.Fail(w, apperr.InvalidID)
		return
	}
	o, _ := tenancy.ID(r.Context())
	v, e := h.service.List(r.Context(), o)
	if e != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	for _, x := range v {
		if x.ID == id {
			httpx.OK(w, x)
			return
		}
	}
	httpx.Fail(w, apperr.NotFound)
}
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	if h.authz.Require(r.Context(), "credentials.issue") != nil {
		httpx.Fail(w, apperr.Forbidden)
		return
	}
	var in struct {
		GatepassID int64 `json:"gatepass_id"`
	}
	if httpx.Decode(r, &in) != nil || in.GatepassID < 1 {
		httpx.Fail(w, apperr.InvalidRequest.With("gatepass_id is required"))
		return
	}
	o, _ := tenancy.ID(r.Context())
	v, e := h.service.Issue(r.Context(), o, in.GatepassID)
	if errors.Is(e, ErrNotFound) {
		httpx.Fail(w, apperr.NotFound)
		return
	}
	if errors.Is(e, ErrUnavailable) {
		httpx.Fail(w, apperr.Conflict.With("gatepass is not eligible for credential issuance"))
		return
	}
	if errors.Is(e, rbac.ErrForbidden) {
		httpx.Fail(w, apperr.Forbidden)
		return
	}
	if e != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	h.audit.Record(r.Context(), audit.Entry{Action: "credential.issued", ResourceType: "credential", ResourceID: v.ID})
	httpx.Created(w, v)
}
func (h *Handler) Verify(w http.ResponseWriter, r *http.Request) {
	if h.authz.Require(r.Context(), "credentials.verify") != nil {
		httpx.Fail(w, apperr.Forbidden)
		return
	}
	var in struct {
		Token string `json:"token"`
	}
	if httpx.Decode(r, &in) != nil || in.Token == "" {
		httpx.Fail(w, apperr.InvalidRequest.With("token is required"))
		return
	}
	v, e := h.service.Verify(r.Context(), in.Token)
	if errors.Is(e, ErrNotFound) || errors.Is(e, ErrUnavailable) {
		httpx.Fail(w, apperr.Conflict.With("credential is invalid or unavailable"))
		return
	}
	if errors.Is(e, rbac.ErrForbidden) {
		httpx.Fail(w, apperr.Forbidden)
		return
	}
	if e != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	httpx.OK(w, map[string]any{"valid": true, "credential": v})
}
func (h *Handler) Revoke(w http.ResponseWriter, r *http.Request) {
	if h.authz.Require(r.Context(), "credentials.revoke") != nil {
		httpx.Fail(w, apperr.Forbidden)
		return
	}
	id, e := httpx.ID(chi.URLParam(r, "id"))
	if e != nil {
		httpx.Fail(w, apperr.InvalidID)
		return
	}
	o, _ := tenancy.ID(r.Context())
	e = h.service.Revoke(r.Context(), o, id)
	if errors.Is(e, ErrNotFound) {
		httpx.Fail(w, apperr.NotFound)
		return
	}
	if e != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	h.audit.Record(r.Context(), audit.Entry{Action: "credential.revoked", ResourceType: "credential", ResourceID: id})
	httpx.OK(w, map[string]any{"id": id, "is_revoked": true})
}
