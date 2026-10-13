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
	"log"
	"net"
	"net/http"
	"net/mail"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	appdatabase "github.com/alumasinde/gopass/internal/app/database"
	"github.com/alumasinde/gopass/internal/app/http/middleware"
	"github.com/alumasinde/gopass/internal/platform/apperr"
	"github.com/alumasinde/gopass/internal/platform/auth"
	"github.com/alumasinde/gopass/internal/platform/httpx"
	"github.com/alumasinde/gopass/internal/platform/ratelimit"
	platformmigrations "github.com/alumasinde/gopass/platform_migrations"
	"github.com/go-chi/chi/v5"
	"github.com/go-sql-driver/mysql"
)

type organizationIDAllocator interface {
	AllocateOrganizationID(context.Context, *sql.Tx) (int64, error)
}

type Handler struct {
	db            *sql.DB
	tokens        *auth.Service
	encryptionKey []byte
	rateLimiter   *ratelimit.RateLimiter
	tenantPools   interface {
		Invalidate(int64)
		RevokeSessions(context.Context, int64) error
		SyncOrganizationMetadata(context.Context, int64, string, string) error
	}
}

func New(db *sql.DB, tokens *auth.Service, pools ...interface {
	Invalidate(int64)
	RevokeSessions(context.Context, int64) error
	SyncOrganizationMetadata(context.Context, int64, string, string) error
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
		p.Patch("/platform/admins/{id}/resume", h.ResumeAdmin)
		p.Put("/platform/admins/{id}", h.UpdateAdmin)
		p.Get("/platform/organizations", h.ListOrganizations)
		p.Post("/platform/organizations", h.CreateOrganization)
		p.Get("/platform/organizations/{id}", h.GetOrganization)
		p.Put("/platform/organizations/{id}", h.UpdateOrganization)
		p.Patch("/platform/organizations/{id}/suspend", h.Suspend)
		p.Patch("/platform/organizations/{id}/resume", h.Resume)
		p.Post("/platform/organizations/{id}/database-config", h.SetDatabaseConfig)
		p.Get("/platform/organizations/{id}/domains", h.ListTenantDomains)
		p.Post("/platform/organizations/{id}/domains", h.CreateTenantDomain)
		p.Post("/platform/organizations/{id}/domains/{domainID}/verify", h.VerifyTenantDomain)
		p.Post("/platform/organizations/{id}/deprovision", h.Deprovision)
		p.Post("/platform/organizations/{id}/provision/retry", h.RetryProvisioning)
		p.Get("/platform/migrations", h.MigrationStatus)
		p.Post("/platform/migrations/run", h.RunMigrations)
		p.Get("/platform/migrations/runs/{id}", h.GetMigrationRun)
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

var tenantDomainPattern = regexp.MustCompile(`^(?i)[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+$`)

func (h *Handler) ListTenantDomains(w http.ResponseWriter, r *http.Request) {
	orgID, ok := pathID(w, r)
	if !ok {
		return
	}
	rows, err := h.db.QueryContext(r.Context(), `SELECT id,domain,verification_token,is_verified,is_active,created_at,updated_at FROM platform_tenant_domains WHERE organization_id=? ORDER BY id DESC`, orgID)
	if err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var id int64
		var domain, token string
		var verified, active bool
		var created, updated time.Time
		if err := rows.Scan(&id, &domain, &token, &verified, &active, &created, &updated); err != nil {
			httpx.Fail(w, apperr.Database)
			return
		}
		item := map[string]any{"id": id, "domain": domain, "is_verified": verified, "is_active": active, "created_at": created, "updated_at": updated}
		if !verified {
			item["verification_token"] = token
			item["verification_host"] = "_gopass-verification." + domain
		}
		out = append(out, item)
	}
	if rows.Err() != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	httpx.OK(w, out)
}

func (h *Handler) CreateTenantDomain(w http.ResponseWriter, r *http.Request) {
	orgID, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		Domain string `json:"domain"`
	}
	if httpx.Decode(r, &in) != nil {
		httpx.Fail(w, apperr.InvalidJSON)
		return
	}
	domain := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(in.Domain)), ".")
	if strings.Contains(domain, "://") || strings.ContainsAny(domain, "/?#:@") || !tenantDomainPattern.MatchString(domain) {
		httpx.Fail(w, apperr.InvalidRequest.With("enter a valid hostname only, without scheme, port or path"))
		return
	}
	var exists int
	if err := h.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM organizations WHERE id=?`, orgID).Scan(&exists); err != nil || exists == 0 {
		httpx.Fail(w, apperr.NotFound.With("organization not found"))
		return
	}
	randomBytes := make([]byte, 32)
	if _, err := rand.Read(randomBytes); err != nil {
		httpx.Fail(w, apperr.Internal)
		return
	}
	token := base64.RawURLEncoding.EncodeToString(randomBytes)
	res, err := h.db.ExecContext(r.Context(), `INSERT INTO platform_tenant_domains(organization_id,domain,verification_token,is_verified,is_active,created_at,updated_at) VALUES(?,?,?,0,0,UTC_TIMESTAMP(),UTC_TIMESTAMP())`, orgID, domain, token)
	if err != nil {
		httpx.Fail(w, apperr.Conflict.With("domain already registered or could not be added"))
		return
	}
	id, err := res.LastInsertId()
	if err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	claims, _ := auth.From(r.Context())
	if err := h.auditStandalone(r.Context(), claims.UserID, "platform.tenant_domain_added", "tenant_domain", id, nil, map[string]any{"organization_id": orgID, "domain": domain}); err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	httpx.Created(w, map[string]any{"id": id, "organization_id": orgID, "domain": domain, "is_verified": false, "is_active": false, "verification_host": "_gopass-verification." + domain, "verification_token": token})
}

func (h *Handler) VerifyTenantDomain(w http.ResponseWriter, r *http.Request) {
	orgID, ok := pathID(w, r)
	if !ok {
		return
	}
	domainID, err := strconv.ParseInt(chi.URLParam(r, "domainID"), 10, 64)
	if err != nil || domainID < 1 {
		httpx.Fail(w, apperr.InvalidID)
		return
	}
	var domain, token string
	err = h.db.QueryRowContext(r.Context(), `SELECT domain,verification_token FROM platform_tenant_domains WHERE id=? AND organization_id=?`, domainID, orgID).Scan(&domain, &token)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.Fail(w, apperr.NotFound)
		return
	}
	if err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	txt, err := net.LookupTXT("_gopass-verification." + domain)
	if err != nil {
		httpx.Fail(w, apperr.Conflict.With("DNS TXT verification record was not found yet"))
		return
	}
	verified := false
	for _, value := range txt {
		if strings.TrimSpace(value) == token {
			verified = true
			break
		}
	}
	if !verified {
		httpx.Fail(w, apperr.Conflict.With("DNS TXT record does not match the verification token"))
		return
	}
	if _, err := h.db.ExecContext(r.Context(), `UPDATE platform_tenant_domains SET is_verified=1,is_active=1,updated_at=UTC_TIMESTAMP() WHERE id=? AND organization_id=?`, domainID, orgID); err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	claims, _ := auth.From(r.Context())
	if err := h.auditStandalone(r.Context(), claims.UserID, "platform.tenant_domain_verified", "tenant_domain", domainID, map[string]any{"is_verified": false, "is_active": false}, map[string]any{"domain": domain, "is_verified": true, "is_active": true}); err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	httpx.OK(w, map[string]any{"id": domainID, "domain": domain, "is_verified": true, "is_active": true})
}

func (h *Handler) ListOrganizations(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.QueryContext(r.Context(), `SELECT id,name,slug,is_active,lifecycle_status,database_mode,provisioning_status,database_config_ciphertext IS NOT NULL AND database_config_ciphertext<>'',provisioning_error,provisioned_at,suspended_at,created_at,updated_at FROM organizations ORDER BY id DESC LIMIT 500`)
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
	row := h.db.QueryRowContext(r.Context(), `SELECT id,name,slug,is_active,lifecycle_status,database_mode,provisioning_status,database_config_ciphertext IS NOT NULL AND database_config_ciphertext<>'',provisioning_error,provisioned_at,suspended_at,created_at,updated_at FROM organizations WHERE id=?`, id)
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
	provisioningStatus := "BOOTSTRAPPING"
	if h.tenantPools == nil {
		provisioningStatus = "PENDING"
	} // Legacy single-database compatibility for integration tests/local upgrades.
	var id int64
	if allocator, ok := h.tenantPools.(organizationIDAllocator); ok {
		id, err = allocator.AllocateOrganizationID(r.Context(), tx)
		if err != nil {
			httpx.Fail(w, apperr.Database.With("could not allocate a tenant ID; apply platform migrations and check the shared tenant database"))
			return
		}
	}
	var res sql.Result
	if id > 0 {
		res, err = tx.ExecContext(r.Context(), `INSERT INTO organizations(id,name,slug,is_active,lifecycle_status,database_mode,provisioning_status,created_at,updated_at) VALUES(?,?,?,0,'PROVISIONING',?,?,UTC_TIMESTAMP(),UTC_TIMESTAMP())`, id, in.Name, in.Slug, mode, provisioningStatus)
	} else {
		res, err = tx.ExecContext(r.Context(), `INSERT INTO organizations(name,slug,is_active,lifecycle_status,database_mode,provisioning_status,created_at,updated_at) VALUES(?,?,0,'PROVISIONING',?,?,UTC_TIMESTAMP(),UTC_TIMESTAMP())`, in.Name, in.Slug, mode, provisioningStatus)
	}
	if err != nil {
		httpx.Fail(w, apperr.Conflict.With("organization slug already exists or organization could not be created"))
		return
	}
	if id == 0 {
		id, err = res.LastInsertId()
		if err != nil {
			httpx.Fail(w, apperr.Database)
			return
		}
	}
	response := map[string]any{"id": id, "name": in.Name, "slug": in.Slug, "database_mode": mode, "lifecycle_status": "PROVISIONING", "provisioning_status": provisioningStatus}
	if h.tenantPools != nil {
		if _, err = tx.ExecContext(r.Context(), `INSERT INTO tenant_bootstrap_requests(organization_id,first_name,last_name,email,password_hash,created_at,updated_at) VALUES(?,?,?,?,?,UTC_TIMESTAMP(),UTC_TIMESTAMP())`, id, in.AdminFirstName, in.AdminLastName, in.AdminEmail, passwordHash); err != nil {
			httpx.Fail(w, apperr.Database)
			return
		}
		response["message"] = "Tenant registered. Initial tenant identity will be created in the tenant data plane before schema provisioning."
	} else {
		roleRes, e := tx.ExecContext(r.Context(), `INSERT INTO roles(organization_id,name,code,is_system,created_at,updated_at) VALUES(?, 'Organization Administrator','organization_admin',1,UTC_TIMESTAMP(),UTC_TIMESTAMP())`, id)
		if e != nil {
			httpx.Fail(w, apperr.Database)
			return
		}
		roleID, e := roleRes.LastInsertId()
		if e != nil {
			httpx.Fail(w, apperr.Database)
			return
		}
		userRes, e := tx.ExecContext(r.Context(), `INSERT INTO users(organization_id,first_name,last_name,email,password_hash,is_active,created_at,updated_at) VALUES(?,?,?,?,?,1,UTC_TIMESTAMP(),UTC_TIMESTAMP())`, id, in.AdminFirstName, in.AdminLastName, in.AdminEmail, passwordHash)
		if e != nil {
			httpx.Fail(w, apperr.Conflict.With("tenant administrator email may already exist or user could not be created"))
			return
		}
		userID, e := userRes.LastInsertId()
		if e != nil {
			httpx.Fail(w, apperr.Database)
			return
		}
		if _, e = tx.ExecContext(r.Context(), `INSERT INTO role_permissions(role_id,permission_id) SELECT ?,id FROM permissions`, roleID); e != nil {
			httpx.Fail(w, apperr.Database)
			return
		}
		if _, e = tx.ExecContext(r.Context(), `INSERT INTO user_roles(user_id,role_id,organization_id,scope_type,is_active,created_at,updated_at) VALUES(?,?,?,'ORGANIZATION',1,UTC_TIMESTAMP(),UTC_TIMESTAMP())`, userID, roleID, id); e != nil {
			httpx.Fail(w, apperr.Database)
			return
		}
		response["admin_user_id"] = userID
	}
	if err = h.auditTx(r.Context(), tx, claims.UserID, "organization.provisioning_requested", "organization", id, nil, map[string]any{"name": in.Name, "slug": in.Slug, "database_mode": mode, "provisioning_status": provisioningStatus, "admin_email": in.AdminEmail}); err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	if err = tx.Commit(); err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	httpx.Created(w, response)
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
	var mode, status, provisioningError string
	var hasConfig bool
	err = tx.QueryRowContext(r.Context(), `SELECT database_mode,provisioning_status,database_config_ciphertext IS NOT NULL AND database_config_ciphertext<>'',COALESCE(provisioning_error,'') FROM organizations WHERE id=? FOR UPDATE`, id).Scan(&mode, &status, &hasConfig, &provisioningError)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.Fail(w, apperr.NotFound)
		return
	}
	if err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	var bootstrapCount int
	if err = tx.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM tenant_bootstrap_requests WHERE organization_id=?`, id).Scan(&bootstrapCount); err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	hasBootstrap := bootstrapCount > 0
	// Recover tenants created before the platform/shared ID ranges were coordinated.
	// Only re-key a failed, not-yet-bootstrapped organization when the worker has
	// explicitly reported a conflicting tenant identity row; no tenant data is moved.
	if hasBootstrap && status == "FAILED" && strings.Contains(provisioningError, "belongs to a different slug") {
		allocator, supported := h.tenantPools.(organizationIDAllocator)
		if !supported {
			httpx.Fail(w, apperr.Conflict.With("tenant ID conflicts with an existing shared tenant; configure the tenant router and apply platform migrations before retrying"))
			return
		}
		var firstName, lastName, email, passwordHash string
		if err = tx.QueryRowContext(r.Context(), `SELECT first_name,last_name,email,password_hash FROM tenant_bootstrap_requests WHERE organization_id=? FOR UPDATE`, id).Scan(&firstName, &lastName, &email, &passwordHash); err != nil {
			httpx.Fail(w, apperr.Database)
			return
		}
		newID, allocErr := allocator.AllocateOrganizationID(r.Context(), tx)
		if allocErr != nil {
			httpx.Fail(w, apperr.Database.With("could not allocate a safe tenant ID; apply platform migrations first"))
			return
		}
		if _, err = tx.ExecContext(r.Context(), `DELETE FROM tenant_bootstrap_requests WHERE organization_id=?`, id); err != nil {
			httpx.Fail(w, apperr.Database)
			return
		}
		if _, err = tx.ExecContext(r.Context(), `UPDATE organizations SET id=?,is_active=0,lifecycle_status='PROVISIONING',provisioning_status='BOOTSTRAPPING',provisioning_error=NULL,updated_at=UTC_TIMESTAMP() WHERE id=?`, newID, id); err != nil {
			httpx.Fail(w, apperr.Database)
			return
		}
		if _, err = tx.ExecContext(r.Context(), `INSERT INTO tenant_bootstrap_requests(organization_id,first_name,last_name,email,password_hash,created_at,updated_at) VALUES(?,?,?,?,?,UTC_TIMESTAMP(),UTC_TIMESTAMP())`, newID, firstName, lastName, email, passwordHash); err != nil {
			httpx.Fail(w, apperr.Database)
			return
		}
		if err = h.auditTx(r.Context(), tx, claims.UserID, "organization.id_conflict_recovered", "organization", newID, map[string]any{"previous_id": id}, map[string]any{"id": newID, "provisioning_status": "BOOTSTRAPPING"}); err != nil {
			httpx.Fail(w, apperr.Database)
			return
		}
		if err = tx.Commit(); err != nil {
			httpx.Fail(w, apperr.Database)
			return
		}
		httpx.OK(w, map[string]any{"id": newID, "previous_id": id, "provisioning_status": "BOOTSTRAPPING", "message": "Conflicting ID safely reassigned. Tenant bootstrap has been queued again."})
		return
	}
	if !(mode == "SHARED" || (mode == "DEDICATED" && (hasConfig || hasBootstrap))) || !(status == "FAILED" || status == "BLOCKED") {
		httpx.Fail(w, apperr.Conflict.With("tenant provisioning cannot be retried until its database configuration is valid"))
		return
	}
	nextStatus := "PENDING"
	if hasBootstrap {
		nextStatus = "BOOTSTRAPPING"
	}
	if _, err = tx.ExecContext(r.Context(), `UPDATE organizations SET is_active=0,lifecycle_status='PROVISIONING',provisioning_status=?,provisioning_error=NULL,updated_at=UTC_TIMESTAMP() WHERE id=?`, nextStatus, id); err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	if err = h.auditTx(r.Context(), tx, claims.UserID, "organization.provisioning_retried", "organization", id, map[string]any{"provisioning_status": status}, map[string]any{"provisioning_status": nextStatus}); err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	if err = tx.Commit(); err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	httpx.OK(w, map[string]any{"id": id, "provisioning_status": nextStatus})
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
	var hasConfig bool
	if err = tx.QueryRowContext(r.Context(), `SELECT database_mode,provisioning_status,database_config_ciphertext IS NOT NULL AND database_config_ciphertext<>'' FROM organizations WHERE id=? FOR UPDATE`, id).Scan(&oldMode, &oldProv, &hasConfig); errors.Is(err, sql.ErrNoRows) {
		httpx.Fail(w, apperr.NotFound)
		return
	} else if err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	if oldMode == "DEDICATED" && hasConfig && oldProv != "FAILED" && oldProv != "BLOCKED" {
		httpx.Fail(w, apperr.Conflict.With("a configured dedicated database cannot be replaced while provisioning or active; suspend and use a reviewed database migration workflow"))
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
	// Keep bootstrap work queued ahead of database provisioning when the initial
	// tenant administrator has not yet been copied into the tenant data plane.
	var bootstrapCount int
	if err = tx.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM tenant_bootstrap_requests WHERE organization_id=?`, id).Scan(&bootstrapCount); err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	nextStatus := "PENDING"
	if bootstrapCount > 0 {
		nextStatus = "BOOTSTRAPPING"
	}
	// Credentials are encrypted at rest. The dedicated provisioner migrates and
	// validates the target schema before the tenant can become READY/ACTIVE.
	_, err = tx.ExecContext(r.Context(), `UPDATE organizations SET database_mode='DEDICATED',database_config_ciphertext=?,provisioning_status=?,provisioning_error=NULL,is_active=0,lifecycle_status='PROVISIONING',updated_at=UTC_TIMESTAMP() WHERE id=?`, ciphertext, nextStatus, id)
	if err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	if err = h.auditTx(r.Context(), tx, claims.UserID, "organization.database_configured", "organization", id, map[string]any{"database_mode": oldMode, "provisioning_status": oldProv}, map[string]any{"database_mode": "DEDICATED", "provisioning_status": nextStatus}); err != nil {
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
	httpx.OK(w, map[string]any{"id": id, "database_mode": "DEDICATED", "provisioning_status": nextStatus, "message": "Database credentials validated and encrypted. Tenant will activate only after bootstrap, migrations and schema validation succeed."})
}

// hasTenantOperationalData prevents an unsafe shared-to-dedicated switch.
// Current provisioning migrates identity and role grants, but deliberately does
// not pretend to move historical gatepass, visitor, audit, or notification data.
func (h *Handler) hasTenantOperationalData(ctx context.Context, orgID int64) (bool, error) {
	if checker, ok := h.tenantPools.(interface {
		HasOperationalData(context.Context, int64) (bool, error)
	}); ok {
		return checker.HasOperationalData(ctx, orgID)
	}
	// Compatibility for legacy single-database tests/development.
	tables := []string{"sites", "gates", "visitors", "pass_types", "gatepasses", "approval_requests", "approval_workflows", "credentials", "check_ins", "check_outs", "audit_logs", "notifications", "organization_settings"}
	for _, table := range tables {
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
		return errors.New("dedicated database cannot be the platform control-plane database")
	}
	if sharedNamer, ok := h.tenantPools.(interface {
		SharedDatabaseName(context.Context) (string, error)
	}); ok {
		sharedName, nameErr := sharedNamer.SharedDatabaseName(ctx)
		if nameErr != nil {
			return nameErr
		}
		if strings.EqualFold(target.DBName, sharedName) {
			return errors.New("dedicated database cannot be the shared tenant database")
		}
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
func (h *Handler) auditStandalone(ctx context.Context, actor int64, action, resource string, id int64, oldv, newv any) error {
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := h.auditTx(ctx, tx, actor, action, resource, id, oldv, newv); err != nil {
		return err
	}
	return tx.Commit()
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

// MigrationStatus reports the actual embedded migration inventory and database
// history. It is read-only and requires a valid platform administrator session.
func (h *Handler) MigrationStatus(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	report, err := platformmigrations.Status(ctx, h.db)
	if err != nil {
		log.Printf("platform migration status failed: %v", err)
		httpx.Fail(w, apperr.Database)
		return
	}
	var databaseName string
	if err := h.db.QueryRowContext(ctx, `SELECT DATABASE()`).Scan(&databaseName); err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	response := map[string]any{
		"target_database":   databaseName,
		"execution_enabled": migrationUIEnabled(),
		"report":            report,
	}
	var runTableCount int
	if err := h.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='platform_migration_runs'`).Scan(&runTableCount); err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	if runTableCount > 0 {
		var runID, runStatus, targetDB string
		var versionsJSON []byte
		var updated time.Time
		err := h.db.QueryRowContext(ctx, `SELECT run_id,status,target_database,expected_versions,updated_at FROM platform_migration_runs WHERE active_key='ACTIVE' ORDER BY created_at DESC LIMIT 1`).Scan(&runID, &runStatus, &targetDB, &versionsJSON, &updated)
		if err == nil {
			var versions []int
			if json.Unmarshal(versionsJSON, &versions) != nil {
				httpx.Fail(w, apperr.Database)
				return
			}
			response["active_run"] = map[string]any{"run_id": runID, "status": runStatus, "target_database": targetDB, "expected_versions": versions, "updated_at": updated.UTC()}
		} else if !errors.Is(err, sql.ErrNoRows) {
			httpx.Fail(w, apperr.Database)
			return
		}
	}
	if runTableCount > 0 {
		var runID, runStatus, targetDB string
		var requestedBy int64
		var versionsJSON []byte
		var appliedJSON, errorMessage sql.NullString
		var createdAt, updatedAt time.Time
		var startedAt, finishedAt sql.NullTime
		err := h.db.QueryRowContext(ctx, `SELECT run_id,requested_by,status,target_database,expected_versions,CAST(applied_versions AS CHAR),error_message,created_at,updated_at,started_at,finished_at FROM platform_migration_runs ORDER BY created_at DESC LIMIT 1`).Scan(&runID, &requestedBy, &runStatus, &targetDB, &versionsJSON, &appliedJSON, &errorMessage, &createdAt, &updatedAt, &startedAt, &finishedAt)
		if err == nil {
			var expectedVersions []int
			appliedVersions := make([]int, 0)
			if json.Unmarshal(versionsJSON, &expectedVersions) != nil {
				httpx.Fail(w, apperr.Database)
				return
			}
			if appliedJSON.Valid && json.Unmarshal([]byte(appliedJSON.String), &appliedVersions) != nil {
				httpx.Fail(w, apperr.Database)
				return
			}
			latest := map[string]any{
				"run_id": runID, "requested_by": requestedBy, "status": runStatus,
				"target_database": targetDB, "expected_versions": expectedVersions,
				"applied_versions": appliedVersions, "created_at": createdAt.UTC(), "updated_at": updatedAt.UTC(),
			}
			if errorMessage.Valid && errorMessage.String != "" {
				latest["error_message"] = errorMessage.String
			}
			if startedAt.Valid {
				latest["started_at"] = startedAt.Time.UTC()
			}
			if finishedAt.Valid {
				latest["finished_at"] = finishedAt.Time.UTC()
			}
			response["latest_run"] = latest
		} else if !errors.Is(err, sql.ErrNoRows) {
			httpx.Fail(w, apperr.Database)
			return
		}
	}
	httpx.OK(w, response)
}

// RunMigrations executes only the migrations embedded in this exact backend
// build. It is intentionally disabled by default and protected by platform
// session middleware. The caller must confirm the exact pending version list.
func (h *Handler) RunMigrations(w http.ResponseWriter, r *http.Request) {
	if !migrationUIEnabled() {
		httpx.Error(w, http.StatusForbidden, "migration_ui_disabled", "migration execution through the UI is disabled; enable PLATFORM_MIGRATIONS_UI_ENABLED during an approved maintenance window")
		return
	}
	var in struct {
		Confirmation     string `json:"confirmation"`
		ExpectedVersions []int  `json:"expected_versions"`
	}
	if httpx.Decode(r, &in) != nil {
		httpx.Fail(w, apperr.InvalidJSON)
		return
	}
	if in.Confirmation != "APPLY PENDING MIGRATIONS" {
		httpx.Fail(w, apperr.InvalidRequest.With("confirmation must exactly equal APPLY PENDING MIGRATIONS"))
		return
	}
	claims, ok := auth.From(r.Context())
	if !ok {
		httpx.Fail(w, apperr.Unauthorized)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	before, err := platformmigrations.Status(ctx, h.db)
	if err != nil {
		log.Printf("platform migration preflight failed: %v", err)
		httpx.Fail(w, apperr.Database)
		return
	}
	if !before.HistoryExists {
		httpx.Fail(w, apperr.Conflict.With("migration history table is missing; bootstrap or repair this database using the migration CLI before using the UI"))
		return
	}
	if before.AppliedCount == 0 && before.PendingCount > 0 {
		httpx.Fail(w, apperr.Conflict.With("no applied migration history exists; bootstrap this database using the migration CLI before using the UI"))
		return
	}
	if before.DriftCount > 0 {
		httpx.Fail(w, apperr.Conflict.With("migration history drift detected; resolve drift before applying pending migrations"))
		return
	}
	pending := make([]int, 0, before.PendingCount)
	for _, item := range before.Migrations {
		if !item.Applied {
			pending = append(pending, item.Version)
		}
	}
	if !sameVersions(pending, in.ExpectedVersions) {
		httpx.Fail(w, apperr.Conflict.With("pending migrations changed since the page was loaded; refresh migration status and confirm again"))
		return
	}
	if len(pending) == 0 {
		httpx.OK(w, map[string]any{"message": "No pending migrations.", "applied": []int{}})
		return
	}
	var databaseName string
	if err := h.db.QueryRowContext(ctx, `SELECT DATABASE()`).Scan(&databaseName); err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	// The job table is included in migration 0010, but it must also be available
	// to track the first UI-triggered run that applies 0010 on an older database.
	if err := ensureMigrationRunsTable(ctx, h.db); err != nil {
		log.Printf("ensure migration run tracking table failed: %v", err)
		httpx.Fail(w, apperr.Database)
		return
	}
	expectedJSON, err := json.Marshal(pending)
	if err != nil {
		httpx.Fail(w, apperr.Internal)
		return
	}
	runID, err := auth.NewSessionID()
	if err != nil {
		httpx.Fail(w, apperr.Internal)
		return
	}

	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	defer tx.Rollback()

	// Recover a run left active by a process crash only after the maximum
	// migration execution window plus a safety margin has elapsed.
	var activeID, activeStatus string
	var activeUpdated time.Time
	err = tx.QueryRowContext(ctx, `SELECT run_id,status,updated_at FROM platform_migration_runs WHERE active_key='ACTIVE' LIMIT 1 FOR UPDATE`).Scan(&activeID, &activeStatus, &activeUpdated)
	if err == nil {
		if time.Since(activeUpdated.UTC()) < 15*time.Minute {
			httpx.Fail(w, apperr.Conflict.With("another migration run is already queued or running"))
			return
		}
		oldState := map[string]any{"run_id": activeID, "status": activeStatus}
		newState := map[string]any{"run_id": activeID, "status": "FAILED", "reason": "stale run recovered after process interruption"}
		if _, err := tx.ExecContext(ctx, `UPDATE platform_migration_runs SET status='FAILED',active_key=NULL,error_message=?,finished_at=UTC_TIMESTAMP(),updated_at=UTC_TIMESTAMP() WHERE run_id=? AND active_key='ACTIVE'`, "stale run recovered after process interruption", activeID); err != nil {
			httpx.Fail(w, apperr.Database)
			return
		}
		if err := h.auditTx(ctx, tx, claims.UserID, "platform.migrations_stale_run_recovered", "database_migrations", 0, oldState, newState); err != nil {
			httpx.Fail(w, apperr.Database)
			return
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		httpx.Fail(w, apperr.Database)
		return
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO platform_migration_runs(run_id,requested_by,target_database,status,expected_versions,active_key,created_at,updated_at) VALUES(?,?,?,'QUEUED',?,'ACTIVE',UTC_TIMESTAMP(),UTC_TIMESTAMP())`, runID, claims.UserID, databaseName, expectedJSON); err != nil {
		// The unique active_key is a cross-process guard against simultaneous queue requests.
		httpx.Fail(w, apperr.Conflict.With("another migration run was queued concurrently; refresh migration status"))
		return
	}
	queued := map[string]any{"run_id": runID, "database": databaseName, "versions": pending, "status": "QUEUED"}
	if err := h.auditTx(ctx, tx, claims.UserID, "platform.migrations_queued", "database_migrations", 0, nil, queued); err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	if err := tx.Commit(); err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}

	// Run outside the HTTP request so the global HTTP write timeout cannot
	// truncate a long DDL operation. Progress is persisted for UI polling.
	go h.executeMigrationRun(runID, claims.UserID, databaseName, pending)
	httpx.JSON(w, http.StatusAccepted, map[string]any{"data": map[string]any{
		"run_id": runID, "status": "QUEUED", "expected_versions": pending,
		"message": "Migration run queued. Poll the run status until it completes.",
	}})
}

func ensureMigrationRunsTable(ctx context.Context, db *sql.DB) error {
	const q = `CREATE TABLE IF NOT EXISTS platform_migration_runs (
		run_id CHAR(32) NOT NULL PRIMARY KEY,
		requested_by BIGINT UNSIGNED NOT NULL,
		target_database VARCHAR(128) NOT NULL,
		status VARCHAR(20) NOT NULL,
		expected_versions JSON NOT NULL,
		applied_versions JSON NULL,
		error_message TEXT NULL,
		active_key VARCHAR(20) NULL,
		started_at DATETIME NULL,
		finished_at DATETIME NULL,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		UNIQUE KEY uq_platform_migration_runs_active_key (active_key),
		KEY idx_platform_migration_runs_created (created_at),
		KEY idx_platform_migration_runs_status (status)
	) ENGINE=InnoDB`
	_, err := db.ExecContext(ctx, q)
	return err
}

func (h *Handler) executeMigrationRun(runID string, actorID int64, databaseName string, expected []int) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	startTx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		log.Printf("migration run %s could not begin: %v", runID, err)
		return
	}
	startState := map[string]any{"run_id": runID, "status": "QUEUED"}
	runningState := map[string]any{"run_id": runID, "status": "RUNNING", "started_at": time.Now().UTC()}
	if _, err = startTx.ExecContext(ctx, `UPDATE platform_migration_runs SET status='RUNNING',started_at=UTC_TIMESTAMP(),updated_at=UTC_TIMESTAMP() WHERE run_id=? AND status='QUEUED'`, runID); err == nil {
		err = h.auditTx(ctx, startTx, actorID, "platform.migrations_started", "database_migrations", 0, startState, runningState)
	}
	if err == nil {
		err = startTx.Commit()
	} else {
		_ = startTx.Rollback()
	}
	if err != nil {
		log.Printf("migration run %s could not enter RUNNING: %v", runID, err)
		return
	}

	runErr := platformmigrations.ApplyEmbedded(ctx, h.db)
	finishCtx, finishCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer finishCancel()
	report, statusErr := platformmigrations.Status(finishCtx, h.db)
	if statusErr != nil {
		log.Printf("migration run %s status refresh failed: %v", runID, statusErr)
	}
	appliedVersions := make([]int, 0)
	if report.Migrations != nil {
		for _, item := range report.Migrations {
			if item.Applied {
				for _, expectedVersion := range expected {
					if item.Version == expectedVersion {
						appliedVersions = append(appliedVersions, item.Version)
						break
					}
				}
			}
		}
	}
	appliedJSON, _ := json.Marshal(appliedVersions)
	finalStatus := "SUCCEEDED"
	action := "platform.migrations_succeeded"
	message := ""
	if runErr != nil {
		finalStatus = "FAILED"
		action = "platform.migrations_failed"
		message = runErr.Error()
		if len(message) > 4000 {
			message = message[:4000]
		}
	}
	if statusErr != nil && runErr == nil {
		finalStatus = "FAILED"
		action = "platform.migrations_failed"
		message = "migration command returned success but final status could not be verified"
	}
	finishTx, txErr := h.db.BeginTx(finishCtx, nil)
	if txErr != nil {
		log.Printf("migration run %s could not record final state: %v", runID, txErr)
		return
	}
	defer finishTx.Rollback()
	oldState := map[string]any{"run_id": runID, "status": "RUNNING"}
	newState := map[string]any{"run_id": runID, "status": finalStatus, "applied_versions": appliedVersions}
	if message != "" {
		newState["error"] = message
	}
	if _, txErr = finishTx.ExecContext(finishCtx, `UPDATE platform_migration_runs SET status=?,applied_versions=?,error_message=NULLIF(?,''),active_key=NULL,finished_at=UTC_TIMESTAMP(),updated_at=UTC_TIMESTAMP() WHERE run_id=?`, finalStatus, appliedJSON, message, runID); txErr == nil {
		txErr = h.auditTx(finishCtx, finishTx, actorID, action, "database_migrations", 0, oldState, newState)
	}
	if txErr == nil {
		txErr = finishTx.Commit()
	}
	if txErr != nil {
		log.Printf("migration run %s final state/audit could not be committed: %v", runID, txErr)
		return
	}
	if runErr != nil {
		log.Printf("migration run %s failed for database %q: %v", runID, databaseName, runErr)
	}
}

func (h *Handler) GetMigrationRun(w http.ResponseWriter, r *http.Request) {
	runID := strings.TrimSpace(chi.URLParam(r, "id"))
	if len(runID) != 32 {
		httpx.Fail(w, apperr.InvalidID)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	var requestedBy int64
	var databaseName, status string
	var expectedJSON []byte
	var appliedJSON, errorMessage sql.NullString
	var startedAt, finishedAt sql.NullTime
	var createdAt, updatedAt time.Time
	err := h.db.QueryRowContext(ctx, `SELECT requested_by,target_database,status,expected_versions,CAST(applied_versions AS CHAR),error_message,started_at,finished_at,created_at,updated_at FROM platform_migration_runs WHERE run_id=?`, runID).Scan(&requestedBy, &databaseName, &status, &expectedJSON, &appliedJSON, &errorMessage, &startedAt, &finishedAt, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.Fail(w, apperr.NotFound)
		return
	}
	if err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	var expectedVersions []int
	appliedVersions := make([]int, 0)
	if err := json.Unmarshal(expectedJSON, &expectedVersions); err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	if appliedJSON.Valid {
		if err := json.Unmarshal([]byte(appliedJSON.String), &appliedVersions); err != nil {
			httpx.Fail(w, apperr.Database)
			return
		}
	}
	result := map[string]any{
		"run_id": runID, "requested_by": requestedBy, "target_database": databaseName,
		"status": status, "expected_versions": expectedVersions, "applied_versions": appliedVersions,
		"created_at": createdAt.UTC(), "updated_at": updatedAt.UTC(),
	}
	if errorMessage.Valid && errorMessage.String != "" {
		result["error_message"] = errorMessage.String
	}
	if startedAt.Valid {
		result["started_at"] = startedAt.Time.UTC()
	}
	if finishedAt.Valid {
		result["finished_at"] = finishedAt.Time.UTC()
	}
	httpx.OK(w, result)
}

func migrationUIEnabled() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("PLATFORM_MIGRATIONS_UI_ENABLED")), "true")
}

func sameVersions(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	aa := append([]int(nil), a...)
	bb := append([]int(nil), b...)
	sort.Ints(aa)
	sort.Ints(bb)
	for i := range aa {
		if aa[i] != bb[i] {
			return false
		}
	}
	return true
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
	var active, databaseConfigured bool
	var errorText sql.NullString
	var provisioned, suspended sql.NullTime
	var created, updated time.Time
	err := s.Scan(&id, &name, &slug, &active, &lifecycle, &mode, &prov, &databaseConfigured, &errorText, &provisioned, &suspended, &created, &updated)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"id": id, "name": name, "slug": slug, "is_active": active, "lifecycle_status": lifecycle, "database_mode": mode, "provisioning_status": prov, "database_configured": databaseConfigured, "created_at": created, "updated_at": updated}
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

func (h *Handler) ResumeAdmin(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	claims, _ := auth.From(r.Context())
	var email, name string
	var active bool
	err := h.db.QueryRowContext(r.Context(), `SELECT email,display_name,is_active FROM platform_users WHERE id=?`, id).Scan(&email, &name, &active)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.Fail(w, apperr.NotFound)
		return
	}
	if err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	if _, err = h.db.ExecContext(r.Context(), `UPDATE platform_users SET is_active=1,updated_at=UTC_TIMESTAMP() WHERE id=?`, id); err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	if err = h.auditStandalone(r.Context(), claims.UserID, "platform.admin_resumed", "platform_user", id, map[string]any{"email": email, "is_active": active}, map[string]any{"email": email, "is_active": true}); err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	httpx.OK(w, map[string]any{"id": id, "email": email, "display_name": name, "is_active": true, "requires_login": !active})
}
func (h *Handler) UpdateAdmin(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		Email       string `json:"email"`
		DisplayName string `json:"display_name"`
	}
	if httpx.Decode(r, &in) != nil {
		httpx.Fail(w, apperr.InvalidJSON)
		return
	}
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	in.DisplayName = strings.TrimSpace(in.DisplayName)
	addr, err := mail.ParseAddress(in.Email)
	if err != nil || strings.ToLower(addr.Address) != in.Email || in.DisplayName == "" {
		httpx.Fail(w, apperr.InvalidRequest.With("valid email and display_name are required"))
		return
	}
	var oldEmail, oldName string
	if err = h.db.QueryRowContext(r.Context(), `SELECT email,display_name FROM platform_users WHERE id=?`, id).Scan(&oldEmail, &oldName); errors.Is(err, sql.ErrNoRows) {
		httpx.Fail(w, apperr.NotFound)
		return
	} else if err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	if _, err = h.db.ExecContext(r.Context(), `UPDATE platform_users SET email=?,display_name=?,updated_at=UTC_TIMESTAMP() WHERE id=?`, in.Email, in.DisplayName, id); err != nil {
		httpx.Fail(w, apperr.Conflict.With("email already exists or administrator could not be updated"))
		return
	}
	claims, _ := auth.From(r.Context())
	if err = h.auditStandalone(r.Context(), claims.UserID, "platform.admin_updated", "platform_user", id, map[string]any{"email": oldEmail, "display_name": oldName}, map[string]any{"email": in.Email, "display_name": in.DisplayName}); err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	httpx.OK(w, map[string]any{"id": id, "email": in.Email, "display_name": in.DisplayName})
}

func (h *Handler) UpdateOrganization(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	if httpx.Decode(r, &in) != nil {
		httpx.Fail(w, apperr.InvalidJSON)
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Slug = strings.ToLower(strings.TrimSpace(in.Slug))
	if in.Name == "" || !regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`).MatchString(in.Slug) {
		httpx.Fail(w, apperr.InvalidRequest.With("name and a valid lowercase slug are required"))
		return
	}
	var oldName, oldSlug string
	var lifecycle, provisioning string
	if err := h.db.QueryRowContext(r.Context(), `SELECT name,slug,lifecycle_status,provisioning_status FROM organizations WHERE id=?`, id).Scan(&oldName, &oldSlug, &lifecycle, &provisioning); errors.Is(err, sql.ErrNoRows) {
		httpx.Fail(w, apperr.NotFound)
		return
	} else if err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	if lifecycle != "ACTIVE" || provisioning != "READY" {
		httpx.Fail(w, apperr.Conflict.With("only active, ready tenants can be edited"))
		return
	}
	claims, _ := auth.From(r.Context())
	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(r.Context(), `UPDATE organizations SET name=?,slug=?,updated_at=UTC_TIMESTAMP() WHERE id=?`, in.Name, in.Slug, id); err != nil {
		httpx.Fail(w, apperr.Conflict.With("tenant slug is already in use or details could not be saved"))
		return
	}
	if err = tx.Commit(); err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	if h.tenantPools != nil {
		if err = h.tenantPools.SyncOrganizationMetadata(r.Context(), id, in.Name, in.Slug); err != nil {
			_, _ = h.db.ExecContext(r.Context(), `UPDATE organizations SET name=?,slug=?,updated_at=UTC_TIMESTAMP() WHERE id=?`, oldName, oldSlug, id)
			httpx.Fail(w, apperr.Conflict.With("tenant database could not be synchronized; previous platform values were restored where possible"))
			return
		}
	}
	if err = h.auditStandalone(r.Context(), claims.UserID, "platform.organization_updated", "organization", id, map[string]any{"name": oldName, "slug": oldSlug}, map[string]any{"name": in.Name, "slug": in.Slug}); err != nil {
		httpx.Fail(w, apperr.Database)
		return
	}
	if h.tenantPools != nil {
		h.tenantPools.Invalidate(id)
	}
	httpx.OK(w, map[string]any{"id": id, "name": in.Name, "slug": in.Slug})
}
