package organizations

import (
	"database/sql"
	"github.com/alumasinde/gopass/internal/platform/apperr"
	"github.com/alumasinde/gopass/internal/platform/audit"
	"github.com/alumasinde/gopass/internal/platform/httpx"
	"github.com/alumasinde/gopass/internal/platform/rbac"
	"github.com/alumasinde/gopass/internal/platform/tenancy"
	"github.com/go-chi/chi/v5"
	"net/http"
	"strings"
)

type Handler struct {
	db    *sql.DB
	authz *rbac.Service
	audit *audit.Service
}

func NewHandler(db *sql.DB, a *rbac.Service, au *audit.Service) *Handler { return &Handler{db, a, au} }
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
	var id int64
	var name, slug string
	var active bool
	if e = h.db.QueryRowContext(r.Context(), `SELECT id,name,slug,is_active FROM organizations WHERE id=?`, org).Scan(&id, &name, &slug, &active); e != nil {
		httpx.Fail(w, apperr.NotFound.With("organization not found"))
		return
	}
	httpx.OK(w, map[string]any{"id": id, "name": name, "slug": slug, "is_active": active})
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
	if httpx.Decode(r, &in) != nil || strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.Slug) == "" {
		httpx.Fail(w, apperr.InvalidRequest.With("name and slug are required"))
		return
	}
	if _, e := h.db.ExecContext(r.Context(), `UPDATE organizations SET name=?,slug=?,updated_at=UTC_TIMESTAMP() WHERE id=?`, in.Name, in.Slug, org); e != nil {
		httpx.Fail(w, apperr.Conflict.With("organization could not be updated"))
		return
	}
	h.audit.Record(r.Context(), audit.Entry{Action: "organization.updated", ResourceType: "organization", ResourceID: org})
	httpx.OK(w, map[string]any{"id": org, "name": in.Name, "slug": in.Slug})
}
