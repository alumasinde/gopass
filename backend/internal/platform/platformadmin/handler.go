package platformadmin

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/mail"
	"os"
	"regexp"
	"strings"
	"time"

	appdatabase "github.com/alumasinde/gopass/internal/app/database"
	"github.com/alumasinde/gopass/internal/app/http/middleware"
	"github.com/alumasinde/gopass/internal/platform/apperr"
	"github.com/alumasinde/gopass/internal/platform/auth"
	"github.com/alumasinde/gopass/internal/platform/httpx"
	"github.com/alumasinde/gopass/internal/platform/ratelimit"
	"github.com/go-chi/chi/v5"
	"github.com/go-sql-driver/mysql"
)

type Handler struct {
	db            *sql.DB
	tokens        *auth.Service
	encryptionKey []byte
	rateLimiter   *ratelimit.RateLimiter
	tenantPools   interface {
		Invalidate(int64)
		RevokeSessions(context.Context, int64) error
	}
}

func New(db *sql.DB, tokens *auth.Service, pools ...interface {
	Invalidate(int64)
	RevokeSessions(context.Context, int64) error
}) *Handler {
	h := &Handler{db: db, tokens: tokens, encryptionKey: encryptionKeyFromEnv(os.Getenv("PLATFORM_DB_ENCRYPTION_KEY")), rateLimiter: ratelimit.NewRateLimiter()}
	if len(pools) > 0 {
		h.tenantPools = pools[0]
	}
	return h
}
func encryptionKeyFromEnv(value string) []byte {
	value = strings.TrimSpace(value)
	if len(value) == 32 {
		return []byte(value)
	}
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err == nil && len(decoded) == 32 {
		return decoded
	}
	decoded, err = base64.RawStdEncoding.DecodeString(value)
	if err == nil && len(decoded) == 32 {
		return decoded
	}
	return []byte(value)
}
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Post("/platform/auth/login", h.Login)
	r.Group(func(p chi.Router) {
		p.Use(middleware.PlatformBearer(h.tokens, h.db))
		p.Post("/platform/auth/logout", h.Logout)
		p.Get("/platform/admins", h.ListAdmins)
		p.Post("/platform/admins", h.CreateAdmin)
		p.Patch("/platform/admins/{id}/suspend", h.SuspendAdmin)
		p.Get("/platform/organizations", h.ListOrganizations)
		p.Post("/platform/organizations", h.CreateOrganization)
		p.Get("/platform/organizations/{id}", h.GetOrganization)
		p.Patch("/platform/organizations/{id}/suspend", h.Suspend)
		p.Patch("/platform/organizations/{id}/resume", h.Resume)
		p.Post("/platform/organizations/{id}/database-config", h.SetDatabaseConfig)
		p.Post("/platform/organizations/{id}/deprovision", h.Deprovision)
		p.Post("/platform/organizations/{id}/provision/retry", h.RetryProvisioning)
	})
}
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if httpx.Decode(r, &in) != nil || strings.TrimSpace(in.Email) == "" || in.Password == "" {
		httpx.Fail(w, apperr.InvalidRequest.With("email and password are required"))
		return
	}
	addr, err := mail.ParseAddress(strings.TrimSpace(in.Email))
	if err != nil {
		httpx.Fail(w, apperr.Unauthorized)
		return
	}
	ip := r.RemoteAddr
	if host, _, splitErr := net.SplitHostPort(r.RemoteAddr); splitErr == nil {
		ip = host
	}
	if h.rateLimiter == nil {
		h.rateLimiter = ratelimit.NewRateLimiter()
	}
	if !h.rateLimiter.CheckLogin(ip, strings.ToLower(addr.Address)) {
		httpx.Fail(w, apperr.TooManyRequests.With("too many login attempts, please try again later"))
		return
	}
	var id int64
	var hash string
	if err = h.db.QueryRowContext(r.Context(), `SELECT id,password_hash FROM platform_users WHERE email=? AND is_active=1`, strings.ToLower(addr.Address)).Scan(&id, &hash); err != nil || auth.Check(hash, in.Password) != nil {
		h.rateLimiter.RecordLoginAttempt(ip, strings.ToLower(addr.Address), false)
		httpx.Fail(w, apperr.Unauthorized.With("invalid credentials"))
		return
	}
	h.rateLimiter.RecordLoginAttempt(ip, strings.ToLower(addr.Address), true)
	sid, err := auth.NewSessionID()
	if err != nil {
		httpx.Fail(w, apperr.Internal)
		return
	}
	token, err := h.tokens.IssuePlatformAccess(id, sid)
	if err != nil {
		httpx.Fail(w, apperr.Internal)
		return
	}
	expires := time.Now().UTC().Add(h.tokens.AccessTTL)
	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	defer tx.Rollback()
	// Calculate expiration in MySQL to ensure UTC consistency with UTC_TIMESTAMP()
	expiresSeconds := int64(h.tokens.AccessTTL.Seconds())
	if _, err = tx.ExecContext(r.Context(), `INSERT INTO platform_sessions(session_id,platform_user_id,expires_at,created_at,updated_at) VALUES(?,?,DATE_ADD(UTC_TIMESTAMP(), INTERVAL ? SECOND),UTC_TIMESTAMP(),UTC_TIMESTAMP())`, sid, id, expiresSeconds); err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	if err = h.auditTx(r.Context(), tx, id, "platform.login", "platform_user", id, nil, map[string]any{"session_id": sid}); err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	if _, err = tx.ExecContext(r.Context(), `UPDATE platform_users SET last_login_at=UTC_TIMESTAMP(),updated_at=UTC_TIMESTAMP() WHERE id=?`, id); err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	if err = tx.Commit(); err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	httpx.OK(w, map[string]any{"access_token": token, "token_type": "Bearer", "expires_at": expires})
}
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	c, ok := auth.From(r.Context())
	if !ok {
		httpx.Fail(w, apperr.Unauthorized)
		return
	}
	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(r.Context(), `UPDATE platform_sessions SET revoked_at=UTC_TIMESTAMP(),updated_at=UTC_TIMESTAMP() WHERE session_id=? AND platform_user_id=? AND revoked_at IS NULL`, c.SessionID, c.UserID)
	if err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	n, err := res.RowsAffected()
	if err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	if n > 0 {
		if err = h.auditTx(r.Context(), tx, c.UserID, "platform.logout", "platform_user", c.UserID, map[string]any{"session_id": c.SessionID, "status": "ACTIVE"}, map[string]any{"session_id": c.SessionID, "status": "REVOKED"}); err != nil {
			httpx.Fail(w, apperr.Database)
			return
		}
	}
	if err = tx.Commit(); err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	httpx.OK(w, map[string]any{"status": "revoked"})
}
func (h *Handler) ListAdmins(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.QueryContext(r.Context(), `SELECT id,email,display_name,is_active,last_login_at,created_at,updated_at FROM platform_users ORDER BY id DESC LIMIT 200`)
	if err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var id int64
		var email, name string
		var active bool
		var last sql.NullTime
		var created, updated time.Time
		if err := rows.Scan(&id, &email, &name, &active, &last, &created, &updated); err != nil {
			httpx.Fail(w, apperr.Database)
			return
		}
		v := map[string]any{"id": id, "email": email, "display_name": name, "is_active": active, "created_at": created, "updated_at": updated}
		if last.Valid {
			v["last_login_at"] = last.Time
		}
		out = append(out, v)
	}
	if rows.Err() != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	httpx.OK(w, out)
}
func (h *Handler) CreateAdmin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email       string `json:"email"`
		DisplayName string `json:"display_name"`
		Password    string `json:"password"`
	}
	if httpx.Decode(r, &in) != nil {
		httpx.Fail(w, apperr.InvalidJSON)
		return
	}
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	in.DisplayName = strings.TrimSpace(in.DisplayName)
	addr, err := mail.ParseAddress(in.Email)
	if err != nil || strings.ToLower(addr.Address) != in.Email || in.DisplayName == "" || len(in.Password) < 14 {
		httpx.Fail(w, apperr.InvalidRequest.With("valid email, display_name and password of at least 14 characters are required"))
		return
	}
	hash, err := auth.Hash(in.Password)
	if err != nil {
		httpx.Fail(w, apperr.Internal)
		return
	}
	claims, _ := auth.From(r.Context())
	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(r.Context(), `INSERT INTO platform_users(email,password_hash,display_name,is_active,created_at,updated_at) VALUES(?,?,?,1,UTC_TIMESTAMP(),UTC_TIMESTAMP())`, in.Email, hash, in.DisplayName)
	if err != nil {
		httpx.Fail(w, apperr.Conflict.With("platform administrator email already exists or could not be created"))
		return
	}
	id, err := res.LastInsertId()
	if err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	if err = h.auditTx(r.Context(), tx, claims.UserID, "platform.admin_created", "platform_user", id, nil, map[string]any{"email": in.Email, "display_name": in.DisplayName, "is_active": true}); err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	if err = tx.Commit(); err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	httpx.Created(w, map[string]any{"id": id, "email": in.Email, "display_name": in.DisplayName, "is_active": true})
}
func (h *Handler) SuspendAdmin(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	claims, _ := auth.From(r.Context())
	if id == claims.UserID {
		httpx.Fail(w, apperr.Conflict.With("cannot suspend the currently authenticated platform administrator"))
		return
	}
	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	defer tx.Rollback()
	var email string
	var active bool
	err = tx.QueryRowContext(r.Context(), `SELECT email,is_active FROM platform_users WHERE id=? FOR UPDATE`, id).Scan(&email, &active)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.Fail(w, apperr.NotFound)
		return
	}
	if err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	if _, err = tx.ExecContext(r.Context(), `UPDATE platform_users SET is_active=0,updated_at=UTC_TIMESTAMP() WHERE id=?`, id); err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	if _, err = tx.ExecContext(r.Context(), `UPDATE platform_sessions SET revoked_at=UTC_TIMESTAMP(),updated_at=UTC_TIMESTAMP() WHERE platform_user_id=? AND revoked_at IS NULL`, id); err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	if err = h.auditTx(r.Context(), tx, claims.UserID, "platform.admin_suspended", "platform_user", id, map[string]any{"email": email, "is_active": active}, map[string]any{"email": email, "is_active": false}); err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	if err = tx.Commit(); err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	httpx.OK(w, map[string]any{"id": id, "is_active": false})
}

func (h *Handler) ListOrganizations(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.QueryContext(r.Context(), `SELECT id,name,slug,is_active,lifecycle_status,database_mode,provisioning_status,provisioning_error,provisioned_at,suspended_at,created_at,updated_at FROM organizations ORDER BY id DESC LIMIT 500`)
	if err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		item, err := scanOrg(rows)
		if err != nil {
			httpx.Fail(w, apperr.Database)
			return
		}
		out = append(out, item)
	}
	if rows.Err() != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	httpx.OK(w, out)
}
func (h *Handler) GetOrganization(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	row := h.db.QueryRowContext(r.Context(), `SELECT id,name,slug,is_active,lifecycle_status,database_mode,provisioning_status,provisioning_error,provisioned_at,suspended_at,created_at,updated_at FROM organizations WHERE id=?`, id)
	item, err := scanOrg(row)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.Fail(w, apperr.NotFound)
		return
	}
	if err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	httpx.OK(w, item)
}
func (h *Handler) CreateOrganization(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name           string `json:"name"`
		Slug           string `json:"slug"`
		DatabaseMode   string `json:"database_mode"`
		AdminFirstName string `json:"admin_first_name"`
		AdminLastName  string `json:"admin_last_name"`
		AdminEmail     string `json:"admin_email"`
		AdminPassword  string `json:"admin_password"`
	}
	if httpx.Decode(r, &in) != nil {
		httpx.Fail(w, apperr.InvalidJSON)
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Slug = strings.TrimSpace(strings.ToLower(in.Slug))
	in.AdminFirstName = strings.TrimSpace(in.AdminFirstName)
	in.AdminLastName = strings.TrimSpace(in.AdminLastName)
	in.AdminEmail = strings.ToLower(strings.TrimSpace(in.AdminEmail))
	if in.Name == "" || !regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`).MatchString(in.Slug) || in.AdminFirstName == "" || in.AdminLastName == "" || len(in.AdminPassword) < 14 {
		httpx.Fail(w, apperr.InvalidRequest.With("name, valid slug, admin first/last name, admin email and admin password of at least 14 characters are required"))
		return
	}
	if addr, parseErr := mail.ParseAddress(in.AdminEmail); parseErr != nil || strings.ToLower(addr.Address) != in.AdminEmail {
		httpx.Fail(w, apperr.InvalidRequest.With("admin_email must be a valid email address"))
		return
	}
	passwordHash, err := auth.Hash(in.AdminPassword)
	if err != nil {
		httpx.Fail(w, apperr.Internal)
		return
	}
	mode := strings.ToUpper(strings.TrimSpace(in.DatabaseMode))
	if mode == "" {
		mode = "SHARED"
	}
	if mode != "SHARED" && mode != "DEDICATED" {
		httpx.Fail(w, apperr.InvalidRequest.With("database_mode must be SHARED or DEDICATED"))
		return
	}
	claims, _ := auth.From(r.Context())
	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(r.Context(), `INSERT INTO organizations(name,slug,is_active,lifecycle_status,database_mode,provisioning_status,created_at,updated_at) VALUES(?,?,0,'PROVISIONING',?,'PENDING',UTC_TIMESTAMP(),UTC_TIMESTAMP())`, in.Name, in.Slug, mode)
	if err != nil {
		httpx.Fail(w, apperr.Conflict.With("organization slug already exists or organization could not be created"))
		return
	}
	id, err := res.LastInsertId()
	if err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	roleRes, err := tx.ExecContext(r.Context(), `INSERT INTO roles(organization_id,name,code,is_system,created_at,updated_at) VALUES(?, 'Organization Administrator','organization_admin',1,UTC_TIMESTAMP(),UTC_TIMESTAMP())`, id)
	if err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	roleID, err := roleRes.LastInsertId()
	if err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	userRes, err := tx.ExecContext(r.Context(), `INSERT INTO users(organization_id,first_name,last_name,email,password_hash,is_active,created_at,updated_at) VALUES(?,?,?,?,?,1,UTC_TIMESTAMP(),UTC_TIMESTAMP())`, id, in.AdminFirstName, in.AdminLastName, in.AdminEmail, passwordHash)
	if err != nil {
		httpx.Fail(w, apperr.Conflict.With("tenant administrator email may already exist or user could not be created"))
		return
	}
	userID, err := userRes.LastInsertId()
	if err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	if _, err = tx.ExecContext(r.Context(), `INSERT INTO role_permissions(role_id,permission_id) SELECT ?,id FROM permissions`, roleID); err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	if _, err = tx.ExecContext(r.Context(), `INSERT INTO user_roles(user_id,role_id,organization_id,scope_type,is_active,created_at,updated_at) VALUES(?,?,?,'ORGANIZATION',1,UTC_TIMESTAMP(),UTC_TIMESTAMP())`, userID, roleID, id); err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	if err = h.auditTx(r.Context(), tx, claims.UserID, "organization.provisioning_requested", "organization", id, nil, map[string]any{"name": in.Name, "slug": in.Slug, "database_mode": mode, "provisioning_status": "PENDING", "admin_email": in.AdminEmail}); err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	if err = tx.Commit(); err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	httpx.Created(w, map[string]any{"id": id, "name": in.Name, "slug": in.Slug, "admin_user_id": userID, "database_mode": mode, "lifecycle_status": "PROVISIONING", "provisioning_status": "PENDING"})
}
func (h *Handler) Suspend(w http.ResponseWriter, r *http.Request) {
	h.changeLifecycle(w, r, "SUSPENDED")
}
func (h *Handler) Resume(w http.ResponseWriter, r *http.Request) { h.changeLifecycle(w, r, "ACTIVE") }
func (h *Handler) Deprovision(w http.ResponseWriter, r *http.Request) {
	h.changeLifecycle(w, r, "DEPROVISIONED")
}
func (h *Handler) changeLifecycle(w http.ResponseWriter, r *http.Request, to string) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	claims, _ := auth.From(r.Context())
	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	defer tx.Rollback()
	var active bool
	var oldLife, prov, mode string
	err = tx.QueryRowContext(r.Context(), `SELECT is_active,lifecycle_status,provisioning_status,database_mode FROM organizations WHERE id=? FOR UPDATE`, id).Scan(&active, &oldLife, &prov, &mode)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.Fail(w, apperr.NotFound)
		return
	}
	if err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	if to == "ACTIVE" && prov != "READY" {
		httpx.Fail(w, apperr.Conflict.With("organization is not provisioned for activation"))
		return
	}
	if to == "SUSPENDED" && prov != "READY" {
		httpx.Fail(w, apperr.Conflict.With("only a READY organization can be suspended; deprovision pending tenants instead"))
		return
	}
	if oldLife == "DEPROVISIONED" && to != "DEPROVISIONED" {
		httpx.Fail(w, apperr.Conflict.With("deprovisioned organizations cannot be reactivated"))
		return
	}
	// A resume is blocked until every previous session has been revoked in the
	// tenant's actual database. If the dedicated DB is unavailable, stay suspended.
	if to == "ACTIVE" && h.tenantPools != nil {
		if err := h.tenantPools.RevokeSessions(r.Context(), id); err != nil {
			httpx.Fail(w, apperr.Database)
			return
		}
	}
	newActive := to == "ACTIVE"
	var suspended any
	if to == "SUSPENDED" {
		suspended = time.Now().UTC()
	}
	if _, err = tx.ExecContext(r.Context(), `UPDATE organizations SET is_active=?,lifecycle_status=?,suspended_at=?,updated_at=UTC_TIMESTAMP() WHERE id=?`, newActive, to, suspended, id); err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	if to == "SUSPENDED" || to == "DEPROVISIONED" {
		if _, err = tx.ExecContext(r.Context(), `UPDATE refresh_tokens SET revoked_at=UTC_TIMESTAMP() WHERE organization_id=? AND revoked_at IS NULL`, id); err != nil {
			httpx.Fail(w, apperr.Database)
			return
		}
	}
	if err = h.auditTx(r.Context(), tx, claims.UserID, "organization."+strings.ToLower(to), "organization", id, map[string]any{"is_active": active, "lifecycle_status": oldLife}, map[string]any{"is_active": newActive, "lifecycle_status": to}); err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	if err = tx.Commit(); err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	if (to == "SUSPENDED" || to == "DEPROVISIONED") && h.tenantPools != nil {
		revokeErr := h.tenantPools.RevokeSessions(r.Context(), id)
		h.tenantPools.Invalidate(id)
		if revokeErr != nil {
			httpx.Fail(w, apperr.Database)
			return
		}
	}
	httpx.OK(w, map[string]any{"id": id, "is_active": newActive, "lifecycle_status": to})
}

func (h *Handler) RetryProvisioning(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	claims, _ := auth.From(r.Context())
	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	defer tx.Rollback()
	var mode, status string
	var hasConfig bool
	err = tx.QueryRowContext(r.Context(), `SELECT database_mode,provisioning_status,database_config_ciphertext IS NOT NULL AND database_config_ciphertext<>'' FROM organizations WHERE id=? FOR UPDATE`, id).Scan(&mode, &status, &hasConfig)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.Fail(w, apperr.NotFound)
		return
	}
	if err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	if !(mode == "SHARED" || (mode == "DEDICATED" && hasConfig)) || !(status == "FAILED" || status == "BLOCKED") {
		httpx.Fail(w, apperr.Conflict.With("tenant provisioning cannot be retried until its database configuration is valid"))
		return
	}
	if _, err = tx.ExecContext(r.Context(), `UPDATE organizations SET is_active=0,lifecycle_status='PROVISIONING',provisioning_status='PENDING',provisioning_error=NULL,updated_at=UTC_TIMESTAMP() WHERE id=?`, id); err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	if err = h.auditTx(r.Context(), tx, claims.UserID, "organization.provisioning_retried", "organization", id, map[string]any{"provisioning_status": status}, map[string]any{"provisioning_status": "PENDING"}); err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	if err = tx.Commit(); err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	httpx.OK(w, map[string]any{"id": id, "provisioning_status": "PENDING"})
}

func (h *Handler) SetDatabaseConfig(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		DSN string `json:"dsn"`
	}
	if httpx.Decode(r, &in) != nil || strings.TrimSpace(in.DSN) == "" {
		httpx.Fail(w, apperr.InvalidRequest.With("dsn is required"))
		return
	}
	parsedDSN, parseErr := mysql.ParseDSN(strings.TrimSpace(in.DSN))
	if parseErr != nil || parsedDSN.DBName == "" {
		httpx.Fail(w, apperr.InvalidRequest.With("dsn must be a valid MySQL DSN with a database name"))
		return
	}
	if err := h.validateDedicatedTarget(r.Context(), id, strings.TrimSpace(in.DSN)); err != nil {
		httpx.Fail(w, apperr.InvalidRequest.With("cannot use the supplied tenant database: credentials, connectivity, schema or isolation validation failed"))
		return
	}
	ciphertext, err := h.encrypt(strings.TrimSpace(in.DSN))
	if err != nil {
		httpx.Fail(w, apperr.InvalidRequest.With("PLATFORM_DB_ENCRYPTION_KEY must be configured as exactly 32 bytes"))
		return
	}
	claims, _ := auth.From(r.Context())
	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	defer tx.Rollback()
	var oldMode, oldProv string
	if err = tx.QueryRowContext(r.Context(), `SELECT database_mode,provisioning_status FROM organizations WHERE id=? FOR UPDATE`, id).Scan(&oldMode, &oldProv); errors.Is(err, sql.ErrNoRows) {
		httpx.Fail(w, apperr.NotFound)
		return
	} else if err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	if oldMode == "SHARED" {
		// This implementation copies identity and role grants, not historical
		// operational records. Refuse a shared-to-dedicated cutover if it would
		// strand existing business data in the central schema.
		hasData, checkErr := h.hasTenantOperationalData(r.Context(), id)
		if checkErr != nil {
			httpx.Fail(w, apperr.Database)
			return
		}
		if hasData {
			httpx.Fail(w, apperr.Conflict.With("tenant has operational records in the shared database; export/migrate those records before switching database mode"))
			return
		}
	}
	if h.tenantPools != nil {
		if err := h.tenantPools.RevokeSessions(r.Context(), id); err != nil {
			httpx.Fail(w, apperr.Database)
			return
		}
	}
	// Credentials are encrypted at rest. The dedicated provisioner migrates and
	// validates the target schema before the tenant can become READY/ACTIVE.
	_, err = tx.ExecContext(r.Context(), `UPDATE organizations SET database_mode='DEDICATED',database_config_ciphertext=?,provisioning_status='PENDING',provisioning_error=NULL,is_active=0,lifecycle_status='PROVISIONING',updated_at=UTC_TIMESTAMP() WHERE id=?`, ciphertext, id)
	if err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	if err = h.auditTx(r.Context(), tx, claims.UserID, "organization.database_configured", "organization", id, map[string]any{"database_mode": oldMode, "provisioning_status": oldProv}, map[string]any{"database_mode": "DEDICATED", "provisioning_status": "PENDING"}); err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	if err = tx.Commit(); err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	if h.tenantPools != nil {
		h.tenantPools.Invalidate(id)
	}
	httpx.OK(w, map[string]any{"id": id, "database_mode": "DEDICATED", "provisioning_status": "PENDING", "message": "Database credentials validated and encrypted. Tenant will activate only after migrations and schema validation succeed."})
}

// hasTenantOperationalData prevents an unsafe shared-to-dedicated switch.
// Current provisioning migrates identity and role grants, but deliberately does
// not pretend to move historical gatepass, visitor, audit, or notification data.
func (h *Handler) hasTenantOperationalData(ctx context.Context, orgID int64) (bool, error) {
	tables := []string{
		"sites", "gates", "visitors", "pass_types", "gatepasses",
		"approval_requests", "approval_workflows", "credentials", "check_ins",
		"check_outs", "audit_logs", "notifications", "organization_settings",
	}
	for _, table := range tables {
		// Table names are a compile-time allow-list; only the organization ID is
		// parameterized. Every table in this list has organization_id.
		query := "SELECT COUNT(*) FROM " + table + " WHERE organization_id=?"
		var count int64
		if err := h.db.QueryRowContext(ctx, query, orgID).Scan(&count); err != nil {
			return false, err
		}
		if count > 0 {
			return true, nil
		}
	}
	return false, nil
}

func (h *Handler) validateDedicatedTarget(ctx context.Context, orgID int64, dsn string) error {
	if err := appdatabase.ValidateDSN(ctx, dsn); err != nil {
		return err
	}
	target, err := mysql.ParseDSN(dsn)
	if err != nil {
		return err
	}
	var centralName string
	if err := h.db.QueryRowContext(ctx, `SELECT DATABASE()`).Scan(&centralName); err != nil {
		return err
	}
	if strings.EqualFold(target.DBName, centralName) {
		return errors.New("dedicated database cannot be the central platform database")
	}
	rows, err := h.db.QueryContext(ctx, `SELECT id,database_config_ciphertext FROM organizations WHERE id<>? AND database_mode='DEDICATED' AND database_config_ciphertext IS NOT NULL AND database_config_ciphertext<>''`, orgID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var otherID int64
		var encrypted string
		if err := rows.Scan(&otherID, &encrypted); err != nil {
			return err
		}
		plain, err := h.decrypt(encrypted)
		if err != nil {
			return fmt.Errorf("cannot validate existing tenant database config for organization %d", otherID)
		}
		other, err := mysql.ParseDSN(plain)
		if err != nil {
			return fmt.Errorf("invalid stored database config for organization %d", otherID)
		}
		if strings.EqualFold(target.DBName, other.DBName) && strings.EqualFold(target.Addr, other.Addr) && strings.EqualFold(target.Net, other.Net) {
			return fmt.Errorf("dedicated database is already assigned to organization %d", otherID)
		}
	}
	return rows.Err()
}

func (h *Handler) encrypt(plain string) (string, error) {
	if len(h.encryptionKey) != 32 {
		return "", errors.New("encryption key must be 32 bytes")
	}
	block, err := aes.NewCipher(h.encryptionKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plain), []byte("gopass:organization-db-dsn:v1"))
	return base64.RawStdEncoding.EncodeToString(sealed), nil
}
func (h *Handler) decrypt(encoded string) (string, error) {
	if len(h.encryptionKey) != 32 {
		return "", errors.New("encryption key must be 32 bytes")
	}
	raw, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(h.encryptionKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize()+gcm.Overhead() {
		return "", errors.New("encrypted DSN is truncated")
	}
	nonce, ciphertext := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ciphertext, []byte("gopass:organization-db-dsn:v1"))
	if err != nil {
		return "", err
	}
	return string(plain), nil
}
func (h *Handler) auditTx(ctx context.Context, tx *sql.Tx, actor int64, action, resource string, id int64, oldv, newv any) error {
	var oldJSON, newJSON any
	var actorValue any
	if actor > 0 {
		actorValue = actor
	}
	var err error
	if oldv != nil {
		oldJSON, err = json.Marshal(oldv)
		if err != nil {
			return err
		}
	}
	if newv != nil {
		newJSON, err = json.Marshal(newv)
		if err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO platform_audit_logs(actor_platform_user_id,action,resource_type,resource_id,old_values,new_values,created_at) VALUES(?,?,?,?,?,?,UTC_TIMESTAMP())`, actorValue, action, resource, id, oldJSON, newJSON)
	return err
}
func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := httpx.ID(chi.URLParam(r, "id"))
	if err != nil || id < 1 {
		httpx.Fail(w, apperr.InvalidID)
		return 0, false
	}
	return id, true
}

type scanner interface{ Scan(...any) error }

func scanOrg(s scanner) (map[string]any, error) {
	var id int64
	var name, slug, lifecycle, mode, prov string
	var active bool
	var errorText sql.NullString
	var provisioned, suspended sql.NullTime
	var created, updated time.Time
	err := s.Scan(&id, &name, &slug, &active, &lifecycle, &mode, &prov, &errorText, &provisioned, &suspended, &created, &updated)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"id": id, "name": name, "slug": slug, "is_active": active, "lifecycle_status": lifecycle, "database_mode": mode, "provisioning_status": prov, "created_at": created, "updated_at": updated}
	if errorText.Valid {
		out["provisioning_error"] = errorText.String
	}
	if provisioned.Valid {
		out["provisioned_at"] = provisioned.Time
	}
	if suspended.Valid {
		out["suspended_at"] = suspended.Time
	}
	return out, nil
}
