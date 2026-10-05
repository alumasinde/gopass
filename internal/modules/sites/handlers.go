package sites

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
	return &Handler{authz: a, audit: au, service: NewService(db)}
}
func (h *Handler) registerRoutes(r chi.Router) {
	r.Route("/sites", func(r chi.Router) {
		r.Get("/", h.List)
		r.Post("/", h.Create)
		r.Get("/{id}", h.Get)
		r.Put("/{id}", h.Update)
		r.Delete("/{id}", h.Delete)
	})
}
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	if e := h.authz.Require(r.Context(), "sites.view"); e != nil {
		httpx.Fail(w, apperr.Forbidden)
		return
	}
	org, e := tenancy.ID(r.Context())
	if e != nil {
		httpx.Fail(w, apperr.TenantMissing)
		return
	}
	v, e := h.service.List(r.Context(), org, 100)
	if e != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	httpx.OK(w, v)
}
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	if e := h.authz.Require(r.Context(), "sites.view"); e != nil {
		httpx.Fail(w, apperr.Forbidden)
		return
	}
	id, e := httpx.ID(chi.URLParam(r, "id"))
	if e != nil {
		httpx.Fail(w, apperr.InvalidID)
		return
	}
	org, _ := tenancy.ID(r.Context())
	v, e := h.service.Get(r.Context(), org, id)
	if errors.Is(e, ErrNotFound) {
		httpx.Fail(w, apperr.NotFound)
		return
	}
	if e != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	httpx.OK(w, v)
}
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	if e := h.authz.Require(r.Context(), "sites.create"); e != nil {
		httpx.Fail(w, apperr.Forbidden)
		return
	}
	var in struct{ Name, Code string }
	if httpx.Decode(r, &in) != nil {
		httpx.Fail(w, apperr.InvalidJSON)
		return
	}
	org, _ := tenancy.ID(r.Context())
	v, e := h.service.Create(r.Context(), org, in.Name, in.Code)
	if e != nil {
		if errors.Is(e, ErrInvalid) {
			httpx.Fail(w, apperr.InvalidRequest.With("name and code are required"))
		} else {
			httpx.Fail(w, apperr.Conflict.With("site code already exists"))
		}
		return
	}
	h.audit.Record(r.Context(), audit.Entry{Action: "site.created", ResourceType: "site", ResourceID: v.ID})
	httpx.Created(w, v)
}
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	if e := h.authz.Require(r.Context(), "sites.update"); e != nil {
		httpx.Fail(w, apperr.Forbidden)
		return
	}
	id, e := httpx.ID(chi.URLParam(r, "id"))
	if e != nil {
		httpx.Fail(w, apperr.InvalidID)
		return
	}
	var in struct {
		Name, Code string
		IsActive   bool
	}
	if httpx.Decode(r, &in) != nil {
		httpx.Fail(w, apperr.InvalidJSON)
		return
	}
	org, _ := tenancy.ID(r.Context())
	e = h.service.Update(r.Context(), org, id, in.Name, in.Code, in.IsActive)
	if errors.Is(e, ErrNotFound) {
		httpx.Fail(w, apperr.NotFound)
		return
	}
	if errors.Is(e, ErrInvalid) {
		httpx.Fail(w, apperr.InvalidRequest)
		return
	}
	if e != nil {
		httpx.Fail(w, apperr.Conflict)
		return
	}
	h.audit.Record(r.Context(), audit.Entry{Action: "site.updated", ResourceType: "site", ResourceID: id})
	httpx.OK(w, map[string]any{"id": id})
}
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	if e := h.authz.Require(r.Context(), "sites.delete"); e != nil {
		httpx.Fail(w, apperr.Forbidden)
		return
	}
	id, e := httpx.ID(chi.URLParam(r, "id"))
	if e != nil {
		httpx.Fail(w, apperr.InvalidID)
		return
	}
	org, _ := tenancy.ID(r.Context())
	e = h.service.Delete(r.Context(), org, id)
	if errors.Is(e, ErrNotFound) {
		httpx.Fail(w, apperr.NotFound)
		return
	}
	if e != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	h.audit.Record(r.Context(), audit.Entry{Action: "site.archived", ResourceType: "site", ResourceID: id})
	httpx.OK(w, map[string]any{"id": id})
}
