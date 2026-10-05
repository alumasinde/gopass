package passtypes

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

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

func NewHandler(db *sql.DB, a *rbac.Service, au *audit.Service) *Handler {
	return &Handler{authz: a, audit: au, service: NewService(db)}
}

func (h *Handler) registerRoutes(r chi.Router) {
	r.Route("/pass-types", func(r chi.Router) {
		r.Get("/", h.List)
		r.Post("/", h.Create)
		r.Get("/{id}", h.Get)
		r.Put("/{id}", h.Update)
		r.Delete("/{id}", h.Delete)
	})
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	if h.authz.Require(r.Context(), "pass_types.view") != nil {
		httpx.Fail(w, apperr.Forbidden)
		return
	}
	org, err := tenancy.ID(r.Context())
	if err != nil {
		httpx.Fail(w, apperr.TenantMissing)
		return
	}
	items, err := h.service.List(r.Context(), org)
	if err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	httpx.OK(w, items)
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	if h.authz.Require(r.Context(), "pass_types.view") != nil {
		httpx.Fail(w, apperr.Forbidden)
		return
	}
	id, err := httpx.ID(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, apperr.InvalidID)
		return
	}
	org, err := tenancy.ID(r.Context())
	if err != nil {
		httpx.Fail(w, apperr.TenantMissing)
		return
	}
	item, err := h.service.Get(r.Context(), org, id)
	if errors.Is(err, ErrNotFound) {
		httpx.Fail(w, apperr.NotFound)
		return
	}
	if err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	httpx.OK(w, item)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	if h.authz.Require(r.Context(), "pass_types.create") != nil {
		httpx.Fail(w, apperr.Forbidden)
		return
	}
	var in struct {
		Name             string `json:"name"`
		Code             string `json:"code"`
		RequiresApproval bool   `json:"requires_approval"`
		ValidityMinutes  int    `json:"validity_minutes"`
	}
	if httpx.Decode(r, &in) != nil {
		httpx.Fail(w, apperr.InvalidJSON)
		return
	}
	if strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.Code) == "" || in.ValidityMinutes < 1 {
		httpx.Fail(w, apperr.InvalidRequest.With("name, code and positive validity_minutes are required"))
		return
	}
	org, err := tenancy.ID(r.Context())
	if err != nil {
		httpx.Fail(w, apperr.TenantMissing)
		return
	}
	item, err := h.service.Create(r.Context(), org, in.Name, in.Code, in.RequiresApproval, in.ValidityMinutes)
	if errors.Is(err, ErrInvalid) {
		httpx.Fail(w, apperr.InvalidRequest)
		return
	}
	if err != nil {
		httpx.Fail(w, apperr.Conflict.With("pass type could not be created"))
		return
	}
	h.audit.Record(r.Context(), audit.Entry{Action: "pass_type.created", ResourceType: "pass_type", ResourceID: item.ID})
	httpx.Created(w, item)
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	if h.authz.Require(r.Context(), "pass_types.update") != nil {
		httpx.Fail(w, apperr.Forbidden)
		return
	}
	id, err := httpx.ID(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, apperr.InvalidID)
		return
	}
	var in struct {
		Name             string `json:"name"`
		Code             string `json:"code"`
		RequiresApproval bool   `json:"requires_approval"`
		ValidityMinutes  int    `json:"validity_minutes"`
		IsActive         bool   `json:"is_active"`
	}
	if httpx.Decode(r, &in) != nil {
		httpx.Fail(w, apperr.InvalidJSON)
		return
	}
	org, err := tenancy.ID(r.Context())
	if err != nil {
		httpx.Fail(w, apperr.TenantMissing)
		return
	}
	err = h.service.Update(r.Context(), org, id, in.Name, in.Code, in.RequiresApproval, in.ValidityMinutes, in.IsActive)
	if errors.Is(err, ErrNotFound) {
		httpx.Fail(w, apperr.NotFound)
		return
	}
	if errors.Is(err, ErrInvalid) {
		httpx.Fail(w, apperr.InvalidRequest)
		return
	}
	if err != nil {
		httpx.Fail(w, apperr.Conflict)
		return
	}
	h.audit.Record(r.Context(), audit.Entry{Action: "pass_type.updated", ResourceType: "pass_type", ResourceID: id})
	httpx.OK(w, map[string]any{"id": id})
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	if h.authz.Require(r.Context(), "pass_types.delete") != nil {
		httpx.Fail(w, apperr.Forbidden)
		return
	}
	id, err := httpx.ID(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, apperr.InvalidID)
		return
	}
	org, err := tenancy.ID(r.Context())
	if err != nil {
		httpx.Fail(w, apperr.TenantMissing)
		return
	}
	err = h.service.Delete(r.Context(), org, id)
	if errors.Is(err, ErrNotFound) {
		httpx.Fail(w, apperr.NotFound)
		return
	}
	if err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	h.audit.Record(r.Context(), audit.Entry{Action: "pass_type.archived", ResourceType: "pass_type", ResourceID: id})
	httpx.OK(w, map[string]any{"id": id, "is_active": false})
}
