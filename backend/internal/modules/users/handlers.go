package users

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/alumasinde/gopass/internal/app/database"
	"github.com/alumasinde/gopass/internal/platform/apperr"
	"github.com/alumasinde/gopass/internal/platform/audit"
	"github.com/alumasinde/gopass/internal/platform/auth"
	"github.com/alumasinde/gopass/internal/platform/httpx"
	"github.com/alumasinde/gopass/internal/platform/ratelimit"
	"github.com/alumasinde/gopass/internal/platform/rbac"
	"github.com/alumasinde/gopass/internal/platform/tenancy"
	"github.com/go-chi/chi/v5"
)

type Handler struct {
	db          database.Handle
	central     database.Handle
	platform    database.Handle
	authz       *rbac.Service
	audit       *audit.Service
	Service     *Service
	Auth        *auth.Service
	rateLimiter *ratelimit.RateLimiter
}

func (h *Handler) IsActive(ctx context.Context, userID, orgID int64) (bool, error) {
	return h.Service.IsActive(ctx, userID, orgID)
}

func NewHandler(db any, a *rbac.Service, au *audit.Service) *Handler {
	repo := NewRepository(db)
	return &Handler{db: database.Adapt(db), authz: a, audit: au, Service: NewService(repo, nil), rateLimiter: ratelimit.NewRateLimiter()}
}

func NewAuthService(db any, tokens *auth.Service, auditServices ...*audit.Service) *Handler {
	repo := NewRepository(db)
	central := database.Adapt(db)
	if source, ok := db.(interface{ CentralHandle() database.Handle }); ok {
		central = source.CentralHandle()
	}
	h := &Handler{db: database.Adapt(db), central: central, Service: NewService(repo, tokens), Auth: tokens, rateLimiter: ratelimit.NewRateLimiter()}
	if len(auditServices) > 0 {
		h.audit = auditServices[0]
	}
	h.platform = central
	return h
}

// SetPlatformDB points public tenant resolution at the control-plane registry.
func (h *Handler) SetPlatformDB(db any) { h.platform = database.Adapt(db) }

func (h *Handler) registerRoutes(r chi.Router) {
	r.Route("/users", func(r chi.Router) { r.Get("/", h.List); r.Post("/", h.Create); r.Get("/{id}", h.Get) })
}

// registerAuthRoutes mounts the public authentication endpoints.
func (h *Handler) registerAuthRoutes(r chi.Router) {
	r.Post("/auth/login", h.Login)
	r.Post("/auth/refresh", h.Refresh)
	r.Post("/auth/logout", h.Logout)
	r.Get("/tenant/resolve", h.ResolveTenantHost)
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email            string `json:"email"`
		Password         string `json:"password"`
		OrganizationID   int64  `json:"organization_id"`
		OrganizationSlug string `json:"organization_slug"`
		TenantHost       string `json:"tenant_host"`
	}
	if httpx.Decode(r, &in) != nil || in.Email == "" || in.Password == "" {
		httpx.Fail(w, apperr.InvalidRequest.With("email and password are required"))
		return
	}

	// Rate limiting check
	ip := clientIP(r.RemoteAddr)
	// Keep the handler safe if constructed outside NewAuthService.
	if h.rateLimiter == nil {
		h.rateLimiter = ratelimit.NewRateLimiter()
	}
	if !h.rateLimiter.CheckLogin(ip, in.Email) {
		httpx.Fail(w, apperr.TooManyRequests.With("too many login attempts, please try again later"))
		return
	}

	org, orgErr := tenancy.ID(r.Context())
	if orgErr != nil {
		// Public login requests do not have an authenticated tenant context yet,
		// so resolve the tenant from the explicit organization ID or slug.
		org = in.OrganizationID
		registry := h.platform
		if registry == nil {
			registry = h.central
		}
		if org < 1 && in.OrganizationSlug != "" {
			if lookupErr := registry.QueryRowContext(r.Context(), "SELECT id FROM organizations WHERE slug=? AND is_active=1 AND lifecycle_status='ACTIVE' AND provisioning_status='READY'", strings.TrimSpace(strings.ToLower(in.OrganizationSlug))).Scan(&org); lookupErr != nil {
				org = 0
			}
		}
		if org < 1 && strings.TrimSpace(in.TenantHost) != "" {
			host, hostErr := normalizeTenantHost(in.TenantHost)
			if hostErr == nil {
				_ = registry.QueryRowContext(r.Context(), "SELECT d.organization_id FROM platform_tenant_domains d JOIN organizations o ON o.id=d.organization_id WHERE d.domain=? AND d.is_active=1 AND d.is_verified=1 AND o.is_active=1 AND o.lifecycle_status='ACTIVE' AND o.provisioning_status='READY'", host).Scan(&org)
			}
		}
		if org < 1 {
			httpx.Fail(w, apperr.InvalidRequest.With("organization_id or organization_slug is required"))
			return
		}
	}

	var organizationActive bool
	registry := h.platform
	if registry == nil {
		registry = h.central
	}
	if err := registry.QueryRowContext(r.Context(), `SELECT is_active FROM organizations WHERE id=? AND lifecycle_status='ACTIVE' AND provisioning_status='READY'`, org).Scan(&organizationActive); err != nil || !organizationActive {
		httpx.Fail(w, apperr.Unauthorized.With("invalid credentials"))
		return
	}

	tenantCtx := tenancy.With(r.Context(), org)
	user, accessToken, refreshToken, e := h.Service.Authenticate(tenantCtx, org, in.Email, in.Password)
	if e != nil {
		h.rateLimiter.RecordLoginAttempt(ip, in.Email, false)
		if h.audit != nil {
			h.audit.Record(tenancy.With(r.Context(), org), audit.Entry{OrganizationID: org, Action: "user.login_failed", ResourceType: "user", ResourceID: 0})
		}
		if e == ErrUserNotFound || e == ErrInvalidPassword {
			httpx.Fail(w, apperr.Unauthorized.With("invalid credentials"))
		} else {
			httpx.Fail(w, apperr.Database)
		}
		return
	}

	h.rateLimiter.RecordLoginAttempt(ip, in.Email, true)
	if h.audit != nil {
		h.audit.Record(tenantCtx, audit.Entry{OrganizationID: org, ActorUserID: user.ID, Action: "user.login", ResourceType: "user", ResourceID: user.ID})
	}
	httpx.OK(w, map[string]any{
		"user":          map[string]any{"id": user.ID, "first_name": user.FirstName, "last_name": user.LastName, "email": user.Email},
		"access_token":  accessToken,
		"refresh_token": refreshToken,
	})
}

func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	var in struct {
		RefreshToken string `json:"refresh_token"`
	}
	if httpx.Decode(r, &in) != nil || in.RefreshToken == "" {
		httpx.Fail(w, apperr.InvalidRequest.With("refresh_token is required"))
		return
	}

	newAccessToken, newRefreshToken, e := h.Service.RefreshToken(r.Context(), in.RefreshToken)
	if e != nil {
		httpx.Fail(w, apperr.Unauthorized.With("invalid refresh token"))
		return
	}

	if claims, err := h.Auth.Parse(in.RefreshToken); err == nil && h.audit != nil {
		h.audit.Record(tenancy.With(r.Context(), claims.OrgID), audit.Entry{OrganizationID: claims.OrgID, ActorUserID: claims.UserID, Action: "user.session_refreshed", ResourceType: "session", ResourceID: claims.UserID})
	}
	httpx.OK(w, map[string]any{"access_token": newAccessToken, "refresh_token": newRefreshToken})
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	var in struct {
		RefreshToken string `json:"refresh_token"`
	}
	if httpx.Decode(r, &in) != nil || in.RefreshToken == "" {
		httpx.Fail(w, apperr.InvalidRequest.With("refresh_token is required"))
		return
	}

	if e := h.Service.Logout(r.Context(), in.RefreshToken); e != nil {
		if errors.Is(e, ErrInvalidRefreshToken) {
			httpx.Fail(w, apperr.Unauthorized.With("invalid refresh token"))
		} else {
			httpx.Fail(w, apperr.Database)
		}
		return
	}

	if claims, err := h.Auth.Parse(in.RefreshToken); err == nil && h.audit != nil {
		h.audit.Record(tenancy.With(r.Context(), claims.OrgID), audit.Entry{OrganizationID: claims.OrgID, ActorUserID: claims.UserID, Action: "user.session_revoked", ResourceType: "session", ResourceID: claims.UserID})
	}
	httpx.OK(w, map[string]any{"message": "logged out successfully"})
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.From(r.Context())
	if !ok {
		httpx.Fail(w, apperr.Unauthorized)
		return
	}
	org, err := tenancy.ID(r.Context())
	if err != nil || org != claims.OrgID {
		httpx.Fail(w, apperr.Unauthorized)
		return
	}
	user, err := h.Service.GetUser(r.Context(), org, claims.UserID)
	if err != nil || user == nil {
		httpx.Fail(w, apperr.NotFound)
		return
	}
	registry := h.platform
	if registry == nil {
		registry = h.central
	}
	var organizationName, organizationSlug string
	if err := registry.QueryRowContext(r.Context(), `SELECT name,slug FROM organizations WHERE id=?`, org).Scan(&organizationName, &organizationSlug); err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	permissions := make([]string, 0)
	rows, queryErr := h.db.QueryContext(r.Context(), `SELECT DISTINCT p.code FROM user_roles ur JOIN users u ON u.id=ur.user_id AND u.organization_id=ur.organization_id JOIN roles ro ON ro.id=ur.role_id AND ro.organization_id=ur.organization_id JOIN role_permissions rp ON rp.role_id=ro.id JOIN permissions p ON p.id=rp.permission_id WHERE ur.user_id=? AND ur.organization_id=? AND ur.is_active=1 AND u.is_active=1 ORDER BY p.code`, claims.UserID, org)
	if queryErr != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			rows.Close()
			httpx.Fail(w, apperr.Database)
			return
		}
		permissions = append(permissions, code)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		httpx.Fail(w, apperr.Database)
		return
	}
	rows.Close()
	roles := make([]string, 0)
	roleRows, queryErr := h.db.QueryContext(r.Context(), `SELECT DISTINCT ro.code FROM user_roles ur JOIN roles ro ON ro.id=ur.role_id AND ro.organization_id=ur.organization_id WHERE ur.user_id=? AND ur.organization_id=? AND ur.is_active=1 ORDER BY ro.code`, claims.UserID, org)
	if queryErr != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	for roleRows.Next() {
		var code string
		if err := roleRows.Scan(&code); err != nil {
			roleRows.Close()
			httpx.Fail(w, apperr.Database)
			return
		}
		roles = append(roles, code)
	}
	if err := roleRows.Err(); err != nil {
		roleRows.Close()
		httpx.Fail(w, apperr.Database)
		return
	}
	roleRows.Close()
	sort.Strings(permissions)
	sort.Strings(roles)
	httpx.OK(w, map[string]any{"id": user.ID, "organization_id": org, "first_name": user.FirstName, "last_name": user.LastName, "email": user.Email, "is_active": user.IsActive, "organization_name": organizationName, "organization_slug": organizationSlug, "permissions": permissions, "roles": roles})
}

func normalizeTenantHost(raw string) (string, error) {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" || strings.Contains(raw, "://") || strings.ContainsAny(raw, "/?#@") {
		return "", errors.New("invalid tenant host")
	}
	host := raw
	if parsed, err := url.Parse("//" + raw); err == nil && parsed.Hostname() != "" {
		host = parsed.Hostname()
	}
	host = strings.TrimSuffix(host, ".")
	if host == "" || net.ParseIP(host) != nil || !strings.Contains(host, ".") {
		return "", errors.New("invalid tenant host")
	}
	for _, part := range strings.Split(host, ".") {
		if part == "" || len(part) > 63 {
			return "", errors.New("invalid tenant host")
		}
		for _, r := range part {
			if !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r != '-' {
				return "", errors.New("invalid tenant host")
			}
		}
		if part[0] == '-' || part[len(part)-1] == '-' {
			return "", errors.New("invalid tenant host")
		}
	}
	return host, nil
}

// ResolveTenantHost resolves a registered and verified custom domain to a ready tenant.
func (h *Handler) ResolveTenantHost(w http.ResponseWriter, r *http.Request) {
	host, err := normalizeTenantHost(r.URL.Query().Get("host"))
	if err != nil {
		httpx.Fail(w, apperr.InvalidRequest.With("valid host is required"))
		return
	}
	registry := h.platform
	if registry == nil {
		registry = h.central
	}
	var slug, name string
	err = registry.QueryRowContext(r.Context(), `SELECT o.slug,o.name FROM platform_tenant_domains d JOIN organizations o ON o.id=d.organization_id WHERE d.domain=? AND d.is_active=1 AND d.is_verified=1 AND o.is_active=1 AND o.lifecycle_status='ACTIVE' AND o.provisioning_status='READY'`, host).Scan(&slug, &name)
	if err != nil {
		httpx.Fail(w, apperr.NotFound.With("this domain is not connected to an active GoPass organization"))
		return
	}
	httpx.OK(w, map[string]any{"slug": slug, "name": name})
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	if h.authz.Require(r.Context(), "users.view") != nil {
		httpx.Fail(w, apperr.Forbidden)
		return
	}
	org, _ := tenancy.ID(r.Context())
	params := httpx.ParsePagination(r)
	offset := httpx.CalculateOffset(params.Page, params.Limit)
	users, total, e := h.Service.ListUsers(r.Context(), org, offset, params.Limit, params.Search)
	if e != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	out := make([]map[string]any, len(users))
	for i, u := range users {
		out[i] = map[string]any{"id": u.ID, "first_name": u.FirstName, "last_name": u.LastName, "email": u.Email, "is_active": u.IsActive}
	}
	httpx.Paginated(w, out, params.Page, params.Limit, total)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	if h.authz.Require(r.Context(), "users.create") != nil {
		httpx.Fail(w, apperr.Forbidden)
		return
	}
	var in struct {
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
		Email     string `json:"email"`
		Password  string `json:"password"`
	}
	if httpx.Decode(r, &in) != nil || in.FirstName == "" || in.LastName == "" || in.Email == "" || len(in.Password) < 12 {
		httpx.Fail(w, apperr.InvalidRequest.With("first_name, last_name, email and password of at least 12 characters are required"))
		return
	}
	org, _ := tenancy.ID(r.Context())
	user, e := h.Service.CreateUser(r.Context(), org, in.FirstName, in.LastName, in.Email, in.Password)
	if e != nil {
		if e == ErrUserAlreadyExists {
			httpx.Fail(w, apperr.Conflict.With("user already exists"))
		} else if e == ErrInvalidEmail {
			httpx.Fail(w, apperr.InvalidRequest.With("invalid email"))
		} else if e == ErrInvalidPassword {
			httpx.Fail(w, apperr.InvalidRequest.With("password must be at least 12 characters"))
		} else {
			httpx.Fail(w, apperr.Database)
		}
		return
	}
	h.audit.Record(r.Context(), audit.Entry{
		Action:       "user.created",
		ResourceType: "user",
		ResourceID:   user.ID,
		NewValues:    map[string]any{"first_name": user.FirstName, "last_name": user.LastName, "email": user.Email, "is_active": user.IsActive},
	})
	httpx.Created(w, map[string]any{"id": user.ID})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	if h.authz.Require(r.Context(), "users.view") != nil {
		httpx.Fail(w, apperr.Forbidden)
		return
	}
	id, e := httpx.ID(chi.URLParam(r, "id"))
	if e != nil {
		httpx.Fail(w, apperr.InvalidID)
		return
	}
	org, _ := tenancy.ID(r.Context())
	user, e := h.Service.GetUser(r.Context(), org, id)
	if e != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	if user == nil {
		httpx.Fail(w, apperr.NotFound.With("user not found"))
		return
	}
	httpx.OK(w, map[string]any{"id": user.ID, "first_name": user.FirstName, "last_name": user.LastName, "email": user.Email, "is_active": user.IsActive})
}

// clientIP extracts the host component from RemoteAddr. Forwarded headers are
// intentionally not trusted here; a trusted-proxy policy must be configured
// explicitly before those headers can safely be used.
func clientIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err == nil {
		return host
	}
	return strings.TrimSpace(remoteAddr)
}
