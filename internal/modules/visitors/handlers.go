package visitors

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
	r.Route("/visitors", func(r chi.Router) {
		r.Get("/", h.List)
		r.Post("/", h.Create)
		r.Get("/{id}", h.Get)
		r.Put("/{id}", h.Update)
		r.Post("/{id}/blacklist", h.Blacklist)
		r.Post("/{id}/unblacklist", h.Unblacklist)
	})
}
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	if h.authz.Require(r.Context(), "visitors.view") != nil {
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
	if h.authz.Require(r.Context(), "visitors.view") != nil {
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
	if h.authz.Require(r.Context(), "visitors.register") != nil {
		httpx.Fail(w, apperr.Forbidden)
		return
	}
	var in struct{ FirstName, LastName, Phone, Email string }
	if httpx.Decode(r, &in) != nil {
		httpx.Fail(w, apperr.InvalidJSON)
		return
	}
	o, _ := tenancy.ID(r.Context())
	v, e := h.service.Create(r.Context(), o, in.FirstName, in.LastName, in.Phone, in.Email)
	if e != nil {
		if errors.Is(e, ErrInvalid) {
			httpx.Fail(w, apperr.InvalidRequest)
			return
		}
		httpx.Fail(w, apperr.Database)
		return
	}
	h.audit.Record(r.Context(), audit.Entry{Action: "visitor.created", ResourceType: "visitor", ResourceID: v.ID})
	httpx.Created(w, v)
}
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	if h.authz.Require(r.Context(), "visitors.update") != nil {
		httpx.Fail(w, apperr.Forbidden)
		return
	}
	id, e := httpx.ID(chi.URLParam(r, "id"))
	if e != nil {
		httpx.Fail(w, apperr.InvalidID)
		return
	}
	var in struct{ FirstName, LastName, Phone, Email string }
	if httpx.Decode(r, &in) != nil {
		httpx.Fail(w, apperr.InvalidJSON)
		return
	}
	o, _ := tenancy.ID(r.Context())
	e = h.service.Update(r.Context(), o, id, in.FirstName, in.LastName, in.Phone, in.Email)
	if errors.Is(e, ErrNotFound) {
		httpx.Fail(w, apperr.NotFound)
		return
	}
	if errors.Is(e, ErrInvalid) {
		httpx.Fail(w, apperr.InvalidRequest)
		return
	}
	if e != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	h.audit.Record(r.Context(), audit.Entry{Action: "visitor.updated", ResourceType: "visitor", ResourceID: id})
	httpx.OK(w, map[string]any{"id": id})
}
func (h *Handler) Blacklist(w http.ResponseWriter, r *http.Request)   { h.setBlacklist(w, r, true) }
func (h *Handler) Unblacklist(w http.ResponseWriter, r *http.Request) { h.setBlacklist(w, r, false) }
func (h *Handler) setBlacklist(w http.ResponseWriter, r *http.Request, v bool) {
	if h.authz.Require(r.Context(), "visitors.blacklist") != nil {
		httpx.Fail(w, apperr.Forbidden)
		return
	}
	id, e := httpx.ID(chi.URLParam(r, "id"))
	if e != nil {
		httpx.Fail(w, apperr.InvalidID)
		return
	}
	o, _ := tenancy.ID(r.Context())
	e = h.service.SetBlacklist(r.Context(), o, id, v)
	if errors.Is(e, ErrNotFound) {
		httpx.Fail(w, apperr.NotFound)
		return
	}
	if e != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	h.audit.Record(r.Context(), audit.Entry{Action: "visitor.blacklist_changed", ResourceType: "visitor", ResourceID: id})
	httpx.OK(w, map[string]any{"id": id, "is_blacklisted": v})
}
