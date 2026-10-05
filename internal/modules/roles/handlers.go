package roles

import (
	"database/sql"
	"github.com/alumasinde/passnow/internal/platform/audit"
	"github.com/alumasinde/passnow/internal/platform/httpx"
	"github.com/alumasinde/passnow/internal/platform/rbac"
	"github.com/alumasinde/passnow/internal/platform/tenancy"
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
	r.Route("/roles", func(r chi.Router) {
		r.Get("/", h.List)
		r.Post("/", h.Create)
		r.Post("/{id}/permissions", h.AssignPermission)
		r.Post("/assign", h.AssignRole)
	})
}
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	if h.authz.Require(r.Context(), "roles.view") != nil {
		httpx.Error(w, 403, "forbidden", "permission denied")
		return
	}
	org, _ := tenancy.ID(r.Context())
	rows, e := h.db.QueryContext(r.Context(), `SELECT id,name,code,is_system FROM roles WHERE organization_id=? ORDER BY name`, org)
	if e != nil {
		httpx.Error(w, 500, "database_error", "database error")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var n, c string
		var sys bool
		if rows.Scan(&id, &n, &c, &sys) == nil {
			out = append(out, map[string]any{"id": id, "name": n, "code": c, "is_system": sys})
		}
	}
	httpx.OK(w, out)
}
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	if h.authz.Require(r.Context(), "roles.create") != nil {
		httpx.Error(w, 403, "forbidden", "permission denied")
		return
	}
	var in struct{ Name, Code string }
	if httpx.Decode(r, &in) != nil || strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.Code) == "" {
		httpx.Error(w, 400, "invalid_request", "name and code are required")
		return
	}
	org, _ := tenancy.ID(r.Context())
	res, e := h.db.ExecContext(r.Context(), `INSERT INTO roles(organization_id,name,code,is_system,created_at,updated_at) VALUES(?,?,?,0,UTC_TIMESTAMP(),UTC_TIMESTAMP())`, org, in.Name, in.Code)
	if e != nil {
		httpx.Error(w, 409, "conflict", "role could not be created")
		return
	}
	id, _ := res.LastInsertId()
	h.audit.Record(r.Context(), audit.Entry{Action: "role.created", ResourceType: "role", ResourceID: id})
	httpx.Created(w, map[string]any{"id": id})
}
func (h *Handler) AssignPermission(w http.ResponseWriter, r *http.Request) {
	if h.authz.Require(r.Context(), "roles.manage_permissions") != nil {
		httpx.Error(w, 403, "forbidden", "permission denied")
		return
	}
	rid, e := httpx.ID(chi.URLParam(r, "id"))
	if e != nil {
		httpx.Error(w, 400, "invalid_id", "invalid id")
		return
	}
	var in struct{ PermissionID int64 }
	if httpx.Decode(r, &in) != nil || in.PermissionID < 1 {
		httpx.Error(w, 400, "invalid_request", "permission_id is required")
		return
	}
	org, _ := tenancy.ID(r.Context())
	_, e = h.db.ExecContext(r.Context(), `INSERT IGNORE INTO role_permissions(role_id,permission_id) SELECT ?,id FROM permissions WHERE id=? AND EXISTS(SELECT 1 FROM roles WHERE id=? AND organization_id=?)`, rid, in.PermissionID, rid, org)
	if e != nil {
		httpx.Error(w, 409, "conflict", "permission could not be assigned")
		return
	}
	httpx.OK(w, map[string]any{"role_id": rid, "permission_id": in.PermissionID})
}
func (h *Handler) AssignRole(w http.ResponseWriter, r *http.Request) {
	if h.authz.Require(r.Context(), "roles.assign") != nil {
		httpx.Error(w, 403, "forbidden", "permission denied")
		return
	}
	var in struct {
		UserID, RoleID int64
		ScopeType      string
		SiteID, GateID *int64
	}
	if httpx.Decode(r, &in) != nil || in.UserID < 1 || in.RoleID < 1 {
		httpx.Error(w, 400, "invalid_request", "user_id and role_id are required")
		return
	}
	org, _ := tenancy.ID(r.Context())
	_, e := h.db.ExecContext(r.Context(), `INSERT INTO user_roles(user_id,role_id,organization_id,scope_type,site_id,gate_id,is_active,created_at,updated_at) SELECT ?,id,?,COALESCE(NULLIF(?,''),'ORGANIZATION'),?,?,1,UTC_TIMESTAMP(),UTC_TIMESTAMP() FROM roles WHERE id=? AND organization_id=?`, in.UserID, org, in.ScopeType, in.SiteID, in.GateID, in.RoleID, org)
	if e != nil {
		httpx.Error(w, 409, "conflict", "role could not be assigned")
		return
	}
	httpx.OK(w, map[string]any{"user_id": in.UserID, "role_id": in.RoleID})
}
