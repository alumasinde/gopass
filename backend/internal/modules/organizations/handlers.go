package organizations

import (
	"database/sql"
	"net/http"

	"github.com/alumasinde/gopass/internal/platform/apperr"
	"github.com/alumasinde/gopass/internal/platform/audit"
	"github.com/alumasinde/gopass/internal/platform/httpx"
	"github.com/alumasinde/gopass/internal/platform/rbac"
	"github.com/alumasinde/gopass/internal/platform/tenancy"
	"github.com/go-chi/chi/v5"
)

type Handler struct {
	db      *sql.DB
	authz   *rbac.Service
	audit   *audit.Service
	service *Service
}

func NewHandler(db *sql.DB, a *rbac.Service, au *audit.Service) *Handler {
	repo := NewRepository(db)
	return &Handler{db: db, authz: a, audit: au, service: NewService(repo)}
}

func (h *Handler) registerRoutes(r chi.Router) {
	r.Get("/organization", h.Get)
	r.Patch("/organization", h.Update)
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	if h.authz.Require(r.Context(), "organization.view") != nil {
		httpx.Fail(w, apperr.Forbidden)
		return
	}
	org, e := tenancy.ID(r.Context())
	if e != nil {
		httpx.Fail(w, apperr.TenantMissing)
		return
	}
	organization, e := h.service.GetOrganization(r.Context(), org)
	if e != nil {
		if e == ErrOrgNotFound {
			httpx.Fail(w, apperr.NotFound.With("organization not found"))
		} else {
			httpx.Fail(w, apperr.Database)
		}
		return
	}
	httpx.OK(w, map[string]any{"id": organization.ID, "name": organization.Name, "slug": organization.Slug, "is_active": organization.IsActive})
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	if h.authz.Require(r.Context(), "organization.update") != nil {
		httpx.Fail(w, apperr.Forbidden)
		return
	}
	org, _ := tenancy.ID(r.Context())
	var in struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	if httpx.Decode(r, &in) != nil {
		httpx.Fail(w, apperr.InvalidJSON)
		return
	}
	organization := &Organization{ID: org, Name: in.Name, Slug: in.Slug}
	if e := h.service.UpdateOrganization(r.Context(), organization); e != nil {
		if e == ErrOrgNotFound {
			httpx.Fail(w, apperr.NotFound.With("organization not found"))
		} else if e == ErrInvalidOrgName || e == ErrInvalidSlug {
			httpx.Fail(w, apperr.InvalidRequest.With(e.Error()))
		} else if e == ErrOrgAlreadyExists {
			httpx.Fail(w, apperr.Conflict.With("organization with this slug already exists"))
		} else {
			httpx.Fail(w, apperr.Database)
		}
		return
	}
	h.audit.Record(r.Context(), audit.Entry{Action: "organization.updated", ResourceType: "organization", ResourceID: org})
	httpx.OK(w, map[string]any{"id": org, "name": organization.Name, "slug": organization.Slug})
}
