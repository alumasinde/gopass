package gates

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
	return &Handler{a, au, NewService(db)}
}
func (h *Handler) registerRoutes(r chi.Router) {
	r.Route("/gates", func(r chi.Router) {
		r.Get("/", h.List)
		r.Post("/", h.Create)
		r.Get("/{id}", h.Get)
		r.Put("/{id}", h.Update)
		r.Delete("/{id}", h.Delete)
	})
}
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	if h.authz.Require(r.Context(), "gates.view") != nil {
		httpx.Fail(w, apperr.Forbidden)
		return
	}
	o, e := tenancy.ID(r.Context())
	if e != nil {
		httpx.Fail(w, apperr.TenantMissing)
		return
	}
	v, e := h.service.List(r.Context(), o)
	if e != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	httpx.OK(w, v)
}
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	if h.authz.Require(r.Context(), "gates.view") != nil {
		httpx.Fail(w, apperr.Forbidden)
		return
	}
	id, e := httpx.ID(chi.URLParam(r, "id"))
	if e != nil {
		httpx.Fail(w, apperr.InvalidID)
		return
	}
	o, _ := tenancy.ID(r.Context())
	v, e := h.service.Get(r.Context(), o, id)
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
	if h.authz.Require(r.Context(), "gates.create") != nil {
		httpx.Fail(w, apperr.Forbidden)
		return
	}
	var in struct {
		SiteID     int64 `json:"site_id"`
		Name, Code string
	}
	if httpx.Decode(r, &in) != nil {
		httpx.Fail(w, apperr.InvalidJSON)
		return
	}
	o, _ := tenancy.ID(r.Context())
	v, e := h.service.Create(r.Context(), o, in.SiteID, in.Name, in.Code)
	if e != nil {
		if errors.Is(e, ErrInvalid) {
			httpx.Fail(w, apperr.InvalidRequest)
			return
		}
		httpx.Fail(w, apperr.Conflict)
		return
	}
	h.audit.Record(r.Context(), audit.Entry{Action: "gate.created", ResourceType: "gate", ResourceID: v.ID})
	httpx.Created(w, v)
}
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	if h.authz.Require(r.Context(), "gates.update") != nil {
		httpx.Fail(w, apperr.Forbidden)
		return
	}
	id, e := httpx.ID(chi.URLParam(r, "id"))
	if e != nil {
		httpx.Fail(w, apperr.InvalidID)
		return
	}
	var in struct {
		SiteID     int64 `json:"site_id"`
		Name, Code string
		IsActive   bool `json:"is_active"`
	}
	if httpx.Decode(r, &in) != nil {
		httpx.Fail(w, apperr.InvalidJSON)
		return
	}
	o, _ := tenancy.ID(r.Context())
	e = h.service.Update(r.Context(), o, id, in.SiteID, in.Name, in.Code, in.IsActive)
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
	h.audit.Record(r.Context(), audit.Entry{Action: "gate.updated", ResourceType: "gate", ResourceID: id})
	httpx.OK(w, map[string]any{"id": id})
}
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	if h.authz.Require(r.Context(), "gates.delete") != nil {
		httpx.Fail(w, apperr.Forbidden)
		return
	}
	id, e := httpx.ID(chi.URLParam(r, "id"))
	if e != nil {
		httpx.Fail(w, apperr.InvalidID)
		return
	}
	o, _ := tenancy.ID(r.Context())
	e = h.service.Delete(r.Context(), o, id)
	if errors.Is(e, ErrNotFound) {
		httpx.Fail(w, apperr.NotFound)
		return
	}
	if e != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	h.audit.Record(r.Context(), audit.Entry{Action: "gate.archived", ResourceType: "gate", ResourceID: id})
	httpx.OK(w, map[string]any{"id": id})
}
