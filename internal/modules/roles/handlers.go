package roles

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
	r.Route("/roles", func(r chi.Router) {
		r.Get("/", h.List)
		r.Post("/", h.Create)
		r.Post("/{id}/permissions", h.AssignPermission)
		r.Delete("/{id}/permissions/{permission_id}", h.RemovePermission)
		r.Get("/{id}/permissions", h.GetPermissions)
		r.Post("/assign", h.AssignRole)
		r.Delete("/assign", h.RemoveRole)
		r.Get("/user/{user_id}", h.GetUserRoles)
	})
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	if h.authz.Require(r.Context(), "roles.view") != nil {
		httpx.Fail(w, apperr.Forbidden)
		return
	}
	org, _ := tenancy.ID(r.Context())
	roles, e := h.service.ListRoles(r.Context(), org, 100)
	if e != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	out := make([]map[string]any, len(roles))
	for i, role := range roles {
		out[i] = map[string]any{"id": role.ID, "name": role.Name, "code": role.Code, "is_system": role.IsSystem}
	}
	httpx.OK(w, out)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	if h.authz.Require(r.Context(), "roles.create") != nil {
		httpx.Fail(w, apperr.Forbidden)
		return
	}
	var in struct {
		Name string `json:"name"`
		Code string `json:"code"`
	}
	if httpx.Decode(r, &in) != nil {
		httpx.Fail(w, apperr.InvalidJSON)
		return
	}
	org, _ := tenancy.ID(r.Context())
	role, e := h.service.CreateRole(r.Context(), org, in.Name, in.Code)
	if e != nil {
		if e == ErrRoleAlreadyExists {
			httpx.Fail(w, apperr.Conflict.With("role already exists"))
		} else if e == ErrInvalidRoleName || e == ErrInvalidRoleCode {
			httpx.Fail(w, apperr.InvalidRequest.With(e.Error()))
		} else {
			httpx.Fail(w, apperr.Database)
		}
		return
	}
	h.audit.Record(r.Context(), audit.Entry{Action: "role.created", ResourceType: "role", ResourceID: role.ID})
	httpx.Created(w, map[string]any{"id": role.ID})
}

func (h *Handler) AssignPermission(w http.ResponseWriter, r *http.Request) {
	if h.authz.Require(r.Context(), "roles.manage_permissions") != nil {
		httpx.Fail(w, apperr.Forbidden)
		return
	}
	rid, e := httpx.ID(chi.URLParam(r, "id"))
	if e != nil {
		httpx.Fail(w, apperr.InvalidID)
		return
	}
	var in struct {
		PermissionID int64 `json:"permission_id"`
	}
	if httpx.Decode(r, &in) != nil || in.PermissionID < 1 {
		httpx.Fail(w, apperr.InvalidRequest.With("permission_id is required"))
		return
	}
	org, _ := tenancy.ID(r.Context())

	// Get the permission code to check if granter holds it
	permCode, e := h.service.GetPermissionCode(r.Context(), org, in.PermissionID)
	if e != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	if permCode == "" {
		httpx.Fail(w, apperr.NotFound.With("permission not found"))
		return
	}

	// Granter must hold the permission they're assigning
	if h.authz.Require(r.Context(), permCode) != nil {
		httpx.Fail(w, apperr.Forbidden.With("you must hold the permission you are assigning"))
		return
	}

	if e := h.service.AssignPermission(r.Context(), org, rid, in.PermissionID); e != nil {
		if e == ErrRoleNotFound {
			httpx.Fail(w, apperr.NotFound.With("role not found"))
		} else if e == ErrSystemRoleImmutable {
			httpx.Fail(w, apperr.Forbidden.With("system roles cannot be modified"))
		} else {
			httpx.Fail(w, apperr.Database)
		}
		return
	}
	h.audit.Record(r.Context(), audit.Entry{Action: "role.permission_assigned", ResourceType: "role", ResourceID: rid, NewValues: map[string]any{"permission_id": in.PermissionID}})
	httpx.OK(w, map[string]any{"role_id": rid, "permission_id": in.PermissionID})
}

func (h *Handler) RemovePermission(w http.ResponseWriter, r *http.Request) {
	if h.authz.Require(r.Context(), "roles.manage_permissions") != nil {
		httpx.Fail(w, apperr.Forbidden)
		return
	}
	rid, e := httpx.ID(chi.URLParam(r, "id"))
	if e != nil {
		httpx.Fail(w, apperr.InvalidID)
		return
	}
	pid, e := httpx.ID(chi.URLParam(r, "permission_id"))
	if e != nil {
		httpx.Fail(w, apperr.InvalidID)
		return
	}
	org, _ := tenancy.ID(r.Context())
	if e := h.service.RemovePermission(r.Context(), org, rid, pid); e != nil {
		if e == ErrRoleNotFound {
			httpx.Fail(w, apperr.NotFound.With("role not found"))
		} else if e == ErrSystemRoleImmutable {
			httpx.Fail(w, apperr.Forbidden.With("system roles cannot be modified"))
		} else {
			httpx.Fail(w, apperr.Database)
		}
		return
	}
	h.audit.Record(r.Context(), audit.Entry{Action: "role.permission_removed", ResourceType: "role", ResourceID: rid, OldValues: map[string]any{"permission_id": pid}})
	httpx.OK(w, map[string]any{"role_id": rid, "permission_id": pid})
}

func (h *Handler) GetPermissions(w http.ResponseWriter, r *http.Request) {
	if h.authz.Require(r.Context(), "roles.view") != nil {
		httpx.Fail(w, apperr.Forbidden)
		return
	}
	rid, e := httpx.ID(chi.URLParam(r, "id"))
	if e != nil {
		httpx.Fail(w, apperr.InvalidID)
		return
	}
	org, _ := tenancy.ID(r.Context())
	perms, e := h.service.GetRolePermissions(r.Context(), org, rid)
	if e != nil {
		if e == ErrRoleNotFound {
			httpx.Fail(w, apperr.NotFound.With("role not found"))
		} else {
			httpx.Fail(w, apperr.Database)
		}
		return
	}
	out := make([]map[string]any, len(perms))
	for i, p := range perms {
		out[i] = map[string]any{"id": p.ID, "code": p.Code, "name": p.Name, "description": p.Description, "module": p.Module, "action": p.Action, "is_system": p.IsSystem}
	}
	httpx.OK(w, out)
}

func (h *Handler) AssignRole(w http.ResponseWriter, r *http.Request) {
	if h.authz.Require(r.Context(), "roles.assign") != nil {
		httpx.Fail(w, apperr.Forbidden)
		return
	}
	var in struct {
		UserID    int64  `json:"user_id"`
		RoleID    int64  `json:"role_id"`
		ScopeType string `json:"scope_type"`
		SiteID    *int64 `json:"site_id"`
		GateID    *int64 `json:"gate_id"`
	}
	if httpx.Decode(r, &in) != nil || in.UserID < 1 || in.RoleID < 1 {
		httpx.Fail(w, apperr.InvalidRequest.With("user_id and role_id are required"))
		return
	}
	org, _ := tenancy.ID(r.Context())
	if in.ScopeType == "" {
		in.ScopeType = ScopeOrganization
	}

	// Get role details to check if it's a system role
	role, e := h.service.GetRole(r.Context(), org, in.RoleID)
	if e != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	if role == nil {
		httpx.Fail(w, apperr.NotFound.With("role not found"))
		return
	}
	if role.IsSystem {
		httpx.Fail(w, apperr.Forbidden.With("system roles cannot be assigned"))
		return
	}

	// Get all permissions for the role being assigned
	rolePerms, e := h.service.GetRolePermissions(r.Context(), org, in.RoleID)
	if e != nil {
		httpx.Fail(w, apperr.Database)
		return
	}

	// Assigner must hold all permissions they're granting
	for _, perm := range rolePerms {
		if h.authz.Require(r.Context(), perm.Code) != nil {
			httpx.Fail(w, apperr.Forbidden.With("you must hold all permissions in the role you are assigning"))
			return
		}
	}

	// Verify user, site, and gate belong to the same org
	if in.SiteID != nil {
		siteOrg, e := h.service.GetSiteOrganization(r.Context(), *in.SiteID)
		if e != nil || siteOrg != org {
			httpx.Fail(w, apperr.InvalidRequest.With("site does not belong to this organization"))
			return
		}
	}
	if in.GateID != nil {
		gateOrg, e := h.service.GetGateOrganization(r.Context(), *in.GateID)
		if e != nil || gateOrg != org {
			httpx.Fail(w, apperr.InvalidRequest.With("gate does not belong to this organization"))
			return
		}
	}

	if e := h.service.AssignRoleToUser(r.Context(), org, in.UserID, in.RoleID, in.ScopeType, in.SiteID, in.GateID); e != nil {
		if e == ErrRoleNotFound {
			httpx.Fail(w, apperr.NotFound.With("role not found"))
		} else if e == ErrInvalidScope {
			httpx.Fail(w, apperr.InvalidRequest.With("invalid scope type"))
		} else {
			httpx.Fail(w, apperr.Database)
		}
		return
	}
	h.audit.Record(r.Context(), audit.Entry{Action: "role.assigned", ResourceType: "user_role", ResourceID: in.UserID, NewValues: map[string]any{"user_id": in.UserID, "role_id": in.RoleID, "scope_type": in.ScopeType, "site_id": in.SiteID, "gate_id": in.GateID}})
	httpx.OK(w, map[string]any{"user_id": in.UserID, "role_id": in.RoleID})
}

func (h *Handler) RemoveRole(w http.ResponseWriter, r *http.Request) {
	if h.authz.Require(r.Context(), "roles.assign") != nil {
		httpx.Fail(w, apperr.Forbidden)
		return
	}
	var in struct {
		UserID int64 `json:"user_id"`
		RoleID int64 `json:"role_id"`
	}
	if httpx.Decode(r, &in) != nil || in.UserID < 1 || in.RoleID < 1 {
		httpx.Fail(w, apperr.InvalidRequest.With("user_id and role_id are required"))
		return
	}
	org, _ := tenancy.ID(r.Context())
	if e := h.service.RemoveRoleFromUser(r.Context(), org, in.UserID, in.RoleID); e != nil {
		if e == ErrRoleNotFound {
			httpx.Fail(w, apperr.NotFound.With("role not found"))
		} else {
			httpx.Fail(w, apperr.Database)
		}
		return
	}
	h.audit.Record(r.Context(), audit.Entry{Action: "role.removed", ResourceType: "user_role", ResourceID: in.UserID, OldValues: map[string]any{"user_id": in.UserID, "role_id": in.RoleID}})
	httpx.OK(w, map[string]any{"user_id": in.UserID, "role_id": in.RoleID})
}

func (h *Handler) GetUserRoles(w http.ResponseWriter, r *http.Request) {
	if h.authz.Require(r.Context(), "roles.view") != nil {
		httpx.Fail(w, apperr.Forbidden)
		return
	}
	uid, e := httpx.ID(chi.URLParam(r, "user_id"))
	if e != nil {
		httpx.Fail(w, apperr.InvalidID)
		return
	}
	org, _ := tenancy.ID(r.Context())
	userRoles, e := h.service.GetUserRoles(r.Context(), org, uid)
	if e != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	out := make([]map[string]any, len(userRoles))
	for i, ur := range userRoles {
		out[i] = map[string]any{
			"id": ur.ID, "user_id": ur.UserID, "role_id": ur.RoleID,
			"organization_id": ur.OrganizationID, "scope_type": ur.ScopeType,
			"site_id": ur.SiteID, "gate_id": ur.GateID, "is_active": ur.IsActive,
			"role_name": ur.RoleName, "role_code": ur.RoleCode,
		}
	}
	httpx.OK(w, out)
}
