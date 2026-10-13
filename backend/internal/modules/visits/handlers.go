package visits

import (
	"errors"
	"net/http"

	"github.com/alumasinde/gopass/internal/platform/apperr"
	"github.com/alumasinde/gopass/internal/platform/audit"
	"github.com/alumasinde/gopass/internal/platform/httpx"
	"github.com/alumasinde/gopass/internal/platform/rbac"
	"github.com/alumasinde/gopass/internal/platform/tenancy"
	"github.com/go-chi/chi/v5"
)

type Handler struct {
	authz   *rbac.Service
	audit   *audit.Service
	service *Service
}

func NewHandler(db any, a *rbac.Service, au *audit.Service) *Handler {
	return &Handler{a, au, NewService(db)}
}

func (h *Handler) registerRoutes(r chi.Router) {
	r.Route("/visits", func(r chi.Router) {
		r.Get("/", h.List)
		r.Post("/", h.Create)
		r.Get("/{id}", h.Get)
		r.Put("/{id}", h.Update)
		r.Delete("/{id}", h.Delete)
	})
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	if h.authz.Require(r.Context(), "visits.view") != nil {
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
	if h.authz.Require(r.Context(), "visits.view") != nil {
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
	if h.authz.Require(r.Context(), "visits.create") != nil {
		httpx.Fail(w, apperr.Forbidden)
		return
	}
	var in CreateInput
	if httpx.Decode(r, &in) != nil {
		httpx.Fail(w, apperr.InvalidJSON)
		return
	}
	org, err := tenancy.ID(r.Context())
	if err != nil {
		httpx.Fail(w, apperr.TenantMissing)
		return
	}
	v, err := h.service.Create(r.Context(), org, in)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalid):
			httpx.Fail(w, apperr.InvalidRequest.With(err.Error()))
		case errors.Is(err, ErrBlacklisted):
			httpx.Fail(w, apperr.Forbidden.With(err.Error()))
		default:
			httpx.Fail(w, apperr.Database)
		}
		return
	}
	h.audit.Record(r.Context(), audit.Entry{Action: "visit.created", ResourceType: "visit", ResourceID: v.ID})
	httpx.Created(w, v)
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	if h.authz.Require(r.Context(), "visits.manage") != nil {
		httpx.Fail(w, apperr.Forbidden)
		return
	}
	id, err := httpx.ID(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, apperr.InvalidID)
		return
	}
	var in UpdateInput
	if httpx.Decode(r, &in) != nil {
		httpx.Fail(w, apperr.InvalidJSON)
		return
	}
	org, err := tenancy.ID(r.Context())
	if err != nil {
		httpx.Fail(w, apperr.TenantMissing)
		return
	}
	err = h.service.Update(r.Context(), org, id, in)
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.Fail(w, apperr.NotFound)
	case errors.Is(err, ErrInvalid):
		httpx.Fail(w, apperr.InvalidRequest.With(err.Error()))
	case errors.Is(err, ErrAlreadyClosed):
		httpx.Fail(w, apperr.InvalidRequest.With(err.Error()))
	case err != nil:
		httpx.Fail(w, apperr.Database)
	default:
		h.audit.Record(r.Context(), audit.Entry{Action: "visit.updated", ResourceType: "visit", ResourceID: id})
		httpx.OK(w, map[string]any{"id": id})
	}
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	if h.authz.Require(r.Context(), "visits.manage") != nil {
		httpx.Fail(w, apperr.Forbidden)
		return
	}
	id, e := httpx.ID(chi.URLParam(r, "id"))
	if e != nil {
		httpx.Fail(w, apperr.InvalidID)
		return
	}
	o, _ := tenancy.ID(r.Context())
	err := h.service.Delete(r.Context(), o, id)
	if errors.Is(err, ErrNotFound) {
		httpx.Fail(w, apperr.NotFound)
		return
	}
	if err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	h.audit.Record(r.Context(), audit.Entry{Action: "visit.deleted", ResourceType: "visit", ResourceID: id})
	httpx.OK(w, map[string]any{"id": id})
}
