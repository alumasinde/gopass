package organizations

import (
	"github.com/alumasinde/gopass/internal/app/database"
	"net/http"

	"github.com/alumasinde/gopass/internal/platform/apperr"
	"github.com/alumasinde/gopass/internal/platform/audit"
	"github.com/alumasinde/gopass/internal/platform/httpx"
	"github.com/alumasinde/gopass/internal/platform/rbac"
	"github.com/alumasinde/gopass/internal/platform/tenancy"
	"github.com/go-chi/chi/v5"
)

type Handler struct {
	db      database.Handle
	central database.Handle
	authz   *rbac.Service
	audit   *audit.Service
	service *Service
}

func NewHandler(db any, a *rbac.Service, au *audit.Service) *Handler {
	repo := NewRepository(db)
	central := database.Adapt(db)
	if source, ok := db.(interface{ CentralHandle() database.Handle }); ok {
		central = source.CentralHandle()
	}
	return &Handler{db: database.Adapt(db), central: central, authz: a, audit: au, service: NewService(repo)}
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
	var mode, oldName, oldSlug string
	if e := h.central.QueryRowContext(r.Context(), `SELECT database_mode,name,slug FROM organizations WHERE id=?`, org).Scan(&mode, &oldName, &oldSlug); e != nil {
		httpx.Fail(w, apperr.Database)
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
	if _, err := h.central.ExecContext(r.Context(), `UPDATE organizations SET name=?,slug=?,updated_at=UTC_TIMESTAMP() WHERE id=?`, in.Name, in.Slug, org); err != nil {
		// Best-effort compensation keeps the dedicated copy aligned if the
		// globally unique central slug update fails.
		_ = h.service.UpdateOrganization(r.Context(), &Organization{ID: org, Name: oldName, Slug: oldSlug})
		httpx.Fail(w, apperr.Conflict.With("organization metadata could not be synchronized; previous values restored where possible"))
		return
	}
	h.audit.Record(r.Context(), audit.Entry{Action: "organization.updated", ResourceType: "organization", ResourceID: org})
	httpx.OK(w, map[string]any{"id": org, "name": organization.Name, "slug": organization.Slug})
}
