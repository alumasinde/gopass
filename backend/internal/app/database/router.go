package database

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/alumasinde/gopass/internal/platform/tenancy"
	"github.com/alumasinde/gopass/migrations"
	mysql "github.com/go-sql-driver/mysql"
)

var (
	ErrTenantUnavailable      = errors.New("tenant database unavailable")
	ErrTenantInactive         = errors.New("tenant is inactive")
	ErrDedicatedNotConfigured = errors.New("dedicated tenant database is not configured")
)

type tenantConn struct {
	ciphertext string
	db         *sql.DB
}

// Router is the database boundary for tenant-owned application modules. The
// central DB holds platform metadata and shared tenants; dedicated tenants are
// routed only when their central lifecycle says ACTIVE/READY.
type Router struct {
	central *sql.DB
	key     []byte
	mu      sync.Mutex
	tenants map[int64]tenantConn
}

func NewRouter(central *sql.DB) *Router {
	return &Router{central: central, key: keyFromEnv(os.Getenv("PLATFORM_DB_ENCRYPTION_KEY")), tenants: make(map[int64]tenantConn)}
}

// CentralHandle exposes the platform metadata database only to explicitly
// constructed control-plane code (e.g. resolving an organization before login).
// Tenant modules must use Router and a tenant-bound context.
func (r *Router) CentralHandle() Handle { return &Static{DB: r.central} }

func keyFromEnv(value string) []byte {
	value = strings.TrimSpace(value)
	if len(value) == 32 {
		return []byte(value)
	}
	if b, err := base64.StdEncoding.DecodeString(value); err == nil && len(b) == 32 {
		return b
	}
	if b, err := base64.RawStdEncoding.DecodeString(value); err == nil && len(b) == 32 {
		return b
	}
	return nil
}

func (r *Router) resolve(ctx context.Context) (*sql.DB, error) {
	orgID, err := tenancy.ID(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: tenant context is required", ErrTenantUnavailable)
	}
	var active bool
	var lifecycle, mode, provision string
	var ciphertext sql.NullString
	err = r.central.QueryRowContext(ctx, `SELECT is_active,lifecycle_status,database_mode,provisioning_status,database_config_ciphertext FROM organizations WHERE id=?`, orgID).Scan(&active, &lifecycle, &mode, &provision, &ciphertext)
	if err != nil {
		return nil, fmt.Errorf("%w: organization metadata: %v", ErrTenantUnavailable, err)
	}
	if !active || lifecycle != "ACTIVE" || provision != "READY" {
		return nil, ErrTenantInactive
	}
	if mode == "SHARED" {
		return r.central, nil
	}
	if mode != "DEDICATED" || !ciphertext.Valid || strings.TrimSpace(ciphertext.String) == "" {
		return nil, ErrDedicatedNotConfigured
	}
	return r.dedicated(ctx, orgID, ciphertext.String)
}

func (r *Router) dedicated(ctx context.Context, orgID int64, ciphertext string) (*sql.DB, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c, ok := r.tenants[orgID]; ok && c.ciphertext == ciphertext {
		return c.db, nil
	}
	dsn, err := decryptDSN(ciphertext, r.key)
	if err != nil {
		return nil, fmt.Errorf("%w: encrypted DSN cannot be decrypted", ErrTenantUnavailable)
	}
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil || strings.TrimSpace(cfg.DBName) == "" {
		return nil, fmt.Errorf("%w: invalid tenant DSN", ErrTenantUnavailable)
	}
	cfg.MultiStatements = true
	cfg.ParseTime = true
	cfg.Loc = time.UTC
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return nil, fmt.Errorf("%w: open tenant DB: %v", ErrTenantUnavailable, err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(20 * time.Minute)
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		db.Close()
		return nil, fmt.Errorf("%w: ping tenant DB: %v", ErrTenantUnavailable, err)
	}
	if old, ok := r.tenants[orgID]; ok {
		_ = old.db.Close()
	}
	r.tenants[orgID] = tenantConn{ciphertext: ciphertext, db: db}
	return db, nil
}

func decryptDSN(encoded string, key []byte) (string, error) {
	if len(key) != 32 {
		return "", errors.New("PLATFORM_DB_ENCRYPTION_KEY must be exactly 32 bytes or base64-encoded 32 bytes")
	}
	raw, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
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

// ValidateDSN verifies credentials and confirms the target DB is either empty or
// already a GoPass schema. It never creates a database or mutates its schema.
func ValidateDSN(ctx context.Context, dsn string) error {
	cfg, err := mysql.ParseDSN(strings.TrimSpace(dsn))
	if err != nil {
		return fmt.Errorf("invalid MySQL DSN: %w", err)
	}
	if cfg.DBName == "" {
		return errors.New("DSN must include a database name")
	}
	cfg.MultiStatements = true
	cfg.ParseTime = true
	cfg.Loc = time.UTC
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return err
	}
	defer db.Close()
	pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := db.PingContext(pctx); err != nil {
		return fmt.Errorf("database credentials/connectivity validation failed: %w", err)
	}
	var tables int
	if err := db.QueryRowContext(pctx, `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=?`, cfg.DBName).Scan(&tables); err != nil {
		return err
	}
	if tables > 0 {
		var migrationsTable int
		if err := db.QueryRowContext(pctx, `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=? AND table_name='schema_migrations'`, cfg.DBName).Scan(&migrationsTable); err != nil {
			return err
		}
		if migrationsTable == 0 {
			return errors.New("target database is not empty and has no GoPass schema_migrations table; refusing to migrate an unrelated database")
		}
	}
	return nil
}

// ProvisionDedicated migrates and validates the configured tenant database,
// seeds the organization and copies tenant users/roles/role-permission grants.
// The tenant is not made active here; the central provisioner commits READY only
// after this method succeeds.
func (r *Router) ProvisionDedicated(ctx context.Context, orgID int64) error {
	// Never switch a tenant whose business records still live in the shared
	// schema. Identity/roles are copied below; operational history is not. This
	// second guard also protects retries or internal callers that bypass the API.
	for _, table := range []string{"sites", "gates", "visitors", "pass_types", "gatepasses", "approval_requests", "approval_workflows", "credentials", "check_ins", "check_outs", "audit_logs", "notifications", "organization_settings"} {
		var count int64
		query := "SELECT COUNT(*) FROM " + table + " WHERE organization_id=?"
		if err := r.central.QueryRowContext(ctx, query, orgID).Scan(&count); err != nil {
			return fmt.Errorf("check shared tenant data in %s: %w", table, err)
		}
		if count > 0 {
			return fmt.Errorf("organization %d has operational data in shared table %s; migrate that data before dedicated provisioning", orgID, table)
		}
	}
	var name, slug, ciphertext string
	if err := r.central.QueryRowContext(ctx, `SELECT name,slug,database_config_ciphertext FROM organizations WHERE id=? AND database_mode='DEDICATED'`, orgID).Scan(&name, &slug, &ciphertext); err != nil {
		return err
	}
	if strings.TrimSpace(ciphertext) == "" {
		return ErrDedicatedNotConfigured
	}
	dsn, err := decryptDSN(ciphertext, r.key)
	if err != nil {
		return err
	}
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil || cfg.DBName == "" {
		return errors.New("invalid dedicated database DSN")
	}
	var centralName string
	if err := r.central.QueryRowContext(ctx, `SELECT DATABASE()`).Scan(&centralName); err != nil {
		return err
	}
	if strings.EqualFold(cfg.DBName, centralName) {
		return errors.New("dedicated database cannot be the central platform database")
	}
	otherRows, err := r.central.QueryContext(ctx, `SELECT id,database_config_ciphertext FROM organizations WHERE id<>? AND database_mode='DEDICATED' AND database_config_ciphertext IS NOT NULL AND database_config_ciphertext<>''`, orgID)
	if err != nil {
		return err
	}
	for otherRows.Next() {
		var otherID int64
		var encrypted string
		if err := otherRows.Scan(&otherID, &encrypted); err != nil {
			otherRows.Close()
			return err
		}
		otherDSN, err := decryptDSN(encrypted, r.key)
		if err != nil {
			otherRows.Close()
			return fmt.Errorf("cannot validate existing tenant database configuration for organization %d", otherID)
		}
		other, err := mysql.ParseDSN(otherDSN)
		if err != nil {
			otherRows.Close()
			return fmt.Errorf("invalid stored tenant database configuration for organization %d", otherID)
		}
		if strings.EqualFold(cfg.DBName, other.DBName) && strings.EqualFold(cfg.Addr, other.Addr) && strings.EqualFold(cfg.Net, other.Net) {
			otherRows.Close()
			return fmt.Errorf("dedicated database is already assigned to organization %d", otherID)
		}
	}
	if err := otherRows.Err(); err != nil {
		otherRows.Close()
		return err
	}
	otherRows.Close()
	cfg.MultiStatements = true
	cfg.ParseTime = true
	cfg.Loc = time.UTC
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(20 * time.Minute)
	pctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err = db.PingContext(pctx); err != nil {
		return fmt.Errorf("dedicated database connectivity check failed: %w", err)
	}
	var tableCount int
	if err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=?`, cfg.DBName).Scan(&tableCount); err != nil {
		return err
	}
	if tableCount > 0 {
		var migrationTable int
		if err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=? AND table_name='schema_migrations'`, cfg.DBName).Scan(&migrationTable); err != nil {
			return err
		}
		if migrationTable == 0 {
			return errors.New("refusing to migrate non-empty database without GoPass schema_migrations")
		}
		var orgTable int
		if err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=? AND table_name='organizations'`, cfg.DBName).Scan(&orgTable); err != nil {
			return err
		}
		if orgTable == 1 {
			var existingCount int
			if err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM organizations`).Scan(&existingCount); err != nil {
				return err
			}
			if existingCount > 0 {
				var match int
				if err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM organizations WHERE id=? AND slug=?`, orgID, slug).Scan(&match); err != nil {
					return err
				}
				if existingCount != 1 || match != 1 {
					return errors.New("dedicated database contains another organization; refusing to migrate or attach it")
				}
			}
		}
	}
	if err = migrations.ApplyEmbedded(ctx, db); err != nil {
		return err
	}
	required := []string{"organizations", "users", "roles", "permissions", "role_permissions", "user_roles", "sites", "gates", "visitors", "pass_types", "gatepasses", "approval_requests", "credentials", "check_ins", "check_outs", "audit_logs", "notifications"}
	for _, table := range required {
		var count int
		if err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=? AND table_name=?`, cfg.DBName, table).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			return fmt.Errorf("dedicated schema validation failed: required table %s missing", table)
		}
	}
	requiredColumns := map[string][]string{
		"organizations":     {"id", "slug", "is_active", "lifecycle_status", "database_mode", "provisioning_status"},
		"users":             {"id", "organization_id", "email", "password_hash", "is_active"},
		"user_roles":        {"user_id", "role_id", "organization_id", "scope_type", "is_active"},
		"sites":             {"id", "organization_id", "is_active"},
		"gates":             {"id", "organization_id", "site_id", "is_active"},
		"gatepasses":        {"id", "organization_id", "gate_id", "created_by", "status", "valid_from", "valid_until"},
		"refresh_tokens":    {"user_id", "organization_id", "session_id", "token_hash", "revoked_at", "expires_at"},
		"audit_logs":        {"organization_id", "action", "resource_type", "created_at"},
		"notifications":     {"organization_id", "status", "attempts", "max_attempts", "next_attempt_at"},
		"credentials":       {"token_hash", "active_gatepass_id"},
		"approval_requests": {"pending_gatepass_id"},
	}
	for table, columns := range requiredColumns {
		for _, column := range columns {
			var count int
			if err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=? AND table_name=? AND column_name=?`, cfg.DBName, table, column).Scan(&count); err != nil {
				return err
			}
			if count != 1 {
				return fmt.Errorf("dedicated schema validation failed: required column %s.%s missing", table, column)
			}
		}
	}
	requiredIndexes := map[string][]string{
		"check_ins":         {"uq_checkins_org_gatepass"},
		"check_outs":        {"uq_checkouts_org_gatepass"},
		"credentials":       {"uq_credentials_one_active_gatepass"},
		"approval_requests": {"uq_approval_one_pending_gatepass"},
	}
	for table, indexes := range requiredIndexes {
		for _, index := range indexes {
			var count int
			if err = db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT index_name) FROM information_schema.statistics WHERE table_schema=? AND table_name=? AND index_name=?`, cfg.DBName, table, index).Scan(&count); err != nil {
				return err
			}
			if count != 1 {
				return fmt.Errorf("dedicated schema validation failed: required index %s.%s missing", table, index)
			}
		}
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var existingSlug string
	err = tx.QueryRowContext(ctx, `SELECT slug FROM organizations WHERE id=? FOR UPDATE`, orgID).Scan(&existingSlug)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if err == sql.ErrNoRows {
		var n int
		if e := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM organizations`).Scan(&n); e != nil {
			return e
		}
		if n != 0 {
			return errors.New("dedicated database contains organization records; refusing to attach it to another tenant")
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO organizations(id,name,slug,is_active,lifecycle_status,database_mode,provisioning_status,created_at,updated_at) VALUES(?,?,?,0,'PROVISIONING','DEDICATED','PROVISIONING',UTC_TIMESTAMP(),UTC_TIMESTAMP())`, orgID, name, slug); err != nil {
			return err
		}
	} else if existingSlug != slug {
		return errors.New("dedicated database organization ID belongs to a different slug")
	}
	if err = r.copyTenantIdentity(ctx, tx, orgID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE organizations SET is_active=1,lifecycle_status='ACTIVE',database_mode='DEDICATED',provisioning_status='READY',provisioning_error=NULL,provisioned_at=UTC_TIMESTAMP(),updated_at=UTC_TIMESTAMP() WHERE id=?`, orgID); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	// Close this provisioning connection; the router lazily opens and caches a
	// fresh pool after the central lifecycle transaction marks the tenant READY.
	return nil
}

func (r *Router) copyTenantIdentity(ctx context.Context, tx *sql.Tx, orgID int64) error {
	rows, err := r.central.QueryContext(ctx, `SELECT id,name,code,is_system,created_at,updated_at FROM roles WHERE organization_id=?`, orgID)
	if err != nil {
		return err
	}
	type role struct {
		id               int64
		name, code       string
		system           bool
		created, updated time.Time
	}
	var roles []role
	for rows.Next() {
		var x role
		if err := rows.Scan(&x.id, &x.name, &x.code, &x.system, &x.created, &x.updated); err != nil {
			rows.Close()
			return err
		}
		roles = append(roles, x)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, x := range roles {
		if _, err := tx.ExecContext(ctx, `INSERT INTO roles(id,organization_id,name,code,is_system,created_at,updated_at) VALUES(?,?,?,?,?,?,?) ON DUPLICATE KEY UPDATE name=VALUES(name),code=VALUES(code),is_system=VALUES(is_system),updated_at=VALUES(updated_at)`, x.id, orgID, x.name, x.code, x.system, x.created, x.updated); err != nil {
			return err
		}
	}
	users, err := r.central.QueryContext(ctx, `SELECT id,first_name,last_name,email,password_hash,is_active,last_login_at,created_at,updated_at FROM users WHERE organization_id=?`, orgID)
	if err != nil {
		return err
	}
	type user struct {
		id                       int64
		first, last, email, hash string
		active                   bool
		login                    sql.NullTime
		created, updated         time.Time
	}
	var allUsers []user
	for users.Next() {
		var x user
		if err := users.Scan(&x.id, &x.first, &x.last, &x.email, &x.hash, &x.active, &x.login, &x.created, &x.updated); err != nil {
			users.Close()
			return err
		}
		allUsers = append(allUsers, x)
	}
	if err := users.Err(); err != nil {
		users.Close()
		return err
	}
	users.Close()
	for _, x := range allUsers {
		if _, err := tx.ExecContext(ctx, `INSERT INTO users(id,organization_id,first_name,last_name,email,password_hash,is_active,last_login_at,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?) ON DUPLICATE KEY UPDATE first_name=VALUES(first_name),last_name=VALUES(last_name),email=VALUES(email),password_hash=VALUES(password_hash),is_active=VALUES(is_active),last_login_at=VALUES(last_login_at),updated_at=VALUES(updated_at)`, x.id, orgID, x.first, x.last, x.email, x.hash, x.active, nullTime(x.login), x.created, x.updated); err != nil {
			return err
		}
	}
	grants, err := r.central.QueryContext(ctx, `SELECT ur.id,ur.user_id,ur.role_id,ur.scope_type,ur.site_id,ur.gate_id,ur.is_active,ur.created_at,ur.updated_at FROM user_roles ur WHERE ur.organization_id=?`, orgID)
	if err != nil {
		return err
	}
	type grant struct {
		id, userID, roleID int64
		scope              string
		site, gate         sql.NullInt64
		active             bool
		created, updated   time.Time
	}
	var allGrants []grant
	for grants.Next() {
		var x grant
		if err := grants.Scan(&x.id, &x.userID, &x.roleID, &x.scope, &x.site, &x.gate, &x.active, &x.created, &x.updated); err != nil {
			grants.Close()
			return err
		}
		allGrants = append(allGrants, x)
	}
	if err := grants.Err(); err != nil {
		grants.Close()
		return err
	}
	grants.Close()
	for _, x := range allGrants {
		if _, err := tx.ExecContext(ctx, `INSERT INTO user_roles(id,user_id,role_id,organization_id,scope_type,site_id,gate_id,is_active,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?) ON DUPLICATE KEY UPDATE user_id=VALUES(user_id),role_id=VALUES(role_id),scope_type=VALUES(scope_type),site_id=VALUES(site_id),gate_id=VALUES(gate_id),is_active=VALUES(is_active),updated_at=VALUES(updated_at)`, x.id, x.userID, x.roleID, orgID, x.scope, nullInt(x.site), nullInt(x.gate), x.active, x.created, x.updated); err != nil {
			return err
		}
	}
	rp, err := r.central.QueryContext(ctx, `SELECT rp.role_id,p.code FROM role_permissions rp JOIN roles r ON r.id=rp.role_id AND r.organization_id=? JOIN permissions p ON p.id=rp.permission_id`, orgID)
	if err != nil {
		return err
	}
	type grantPermission struct {
		roleID int64
		code   string
	}
	var allRP []grantPermission
	for rp.Next() {
		var x grantPermission
		if err := rp.Scan(&x.roleID, &x.code); err != nil {
			rp.Close()
			return err
		}
		allRP = append(allRP, x)
	}
	if err := rp.Err(); err != nil {
		rp.Close()
		return err
	}
	rp.Close()
	for _, x := range allRP {
		var permissionID int64
		if err := tx.QueryRowContext(ctx, `SELECT id FROM permissions WHERE code=?`, x.code).Scan(&permissionID); err != nil {
			return fmt.Errorf("permission %s is absent from dedicated schema: %w", x.code, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT IGNORE INTO role_permissions(role_id,permission_id) VALUES(?,?)`, x.roleID, permissionID); err != nil {
			return err
		}
	}
	return nil
}
func nullTime(v sql.NullTime) any {
	if v.Valid {
		return v.Time
	}
	return nil
}
func nullInt(v sql.NullInt64) any {
	if v.Valid {
		return v.Int64
	}
	return nil
}

func (r *Router) TenantIDs(ctx context.Context) ([]int64, error) {
	rows, err := r.central.QueryContext(ctx, `SELECT id FROM organizations WHERE is_active=1 AND lifecycle_status='ACTIVE' AND provisioning_status='READY' ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *Router) ExecContext(ctx context.Context, q string, args ...any) (sql.Result, error) {
	db, err := r.resolve(ctx)
	if err != nil {
		return nil, err
	}
	return db.ExecContext(ctx, q, args...)
}
func (r *Router) QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
	db, err := r.resolve(ctx)
	if err != nil {
		return nil, err
	}
	return db.QueryContext(ctx, q, args...)
}
func (r *Router) QueryRowContext(ctx context.Context, q string, args ...any) Row {
	db, err := r.resolve(ctx)
	if err != nil {
		return errorRow{err}
	}
	return db.QueryRowContext(ctx, q, args...)
}
func (r *Router) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	db, err := r.resolve(ctx)
	if err != nil {
		return nil, err
	}
	return db.BeginTx(ctx, opts)
}
func (r *Router) PingContext(ctx context.Context) error {
	db, err := r.resolve(ctx)
	if err != nil {
		return err
	}
	return db.PingContext(ctx)
}

// RevokeSessions invalidates tenant refresh sessions before suspension or a
// database-mode change. It deliberately bypasses the ACTIVE/READY resolver so
// platform control-plane actions can revoke sessions while changing lifecycle.
func (r *Router) RevokeSessions(ctx context.Context, orgID int64) error {
	var mode string
	var encrypted sql.NullString
	if err := r.central.QueryRowContext(ctx, `SELECT database_mode,database_config_ciphertext FROM organizations WHERE id=?`, orgID).Scan(&mode, &encrypted); err != nil {
		return err
	}
	if mode == "SHARED" {
		_, err := r.central.ExecContext(ctx, `UPDATE refresh_tokens SET revoked_at=COALESCE(revoked_at,UTC_TIMESTAMP()) WHERE organization_id=?`, orgID)
		return err
	}
	if mode != "DEDICATED" || !encrypted.Valid || encrypted.String == "" {
		return nil
	}
	r.mu.Lock()
	if c, ok := r.tenants[orgID]; ok && c.ciphertext == encrypted.String {
		db := c.db
		r.mu.Unlock()
		_, err := db.ExecContext(ctx, `UPDATE refresh_tokens SET revoked_at=COALESCE(revoked_at,UTC_TIMESTAMP()) WHERE organization_id=?`, orgID)
		return err
	}
	r.mu.Unlock()
	dsn, err := decryptDSN(encrypted.String, r.key)
	if err != nil {
		return err
	}
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return err
	}
	cfg.MultiStatements = true
	cfg.ParseTime = true
	cfg.Loc = time.UTC
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return err
	}
	defer db.Close()
	pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := db.PingContext(pctx); err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `UPDATE refresh_tokens SET revoked_at=COALESCE(revoked_at,UTC_TIMESTAMP()) WHERE organization_id=?`, orgID)
	return err
}

// Invalidate closes and forgets a tenant pool after suspension, deprovisioning,
// or database credential rotation.
func (r *Router) Invalidate(orgID int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c, ok := r.tenants[orgID]; ok {
		_ = c.db.Close()
		delete(r.tenants, orgID)
	}
}

func (r *Router) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	var first error
	for id, c := range r.tenants {
		if err := c.db.Close(); err != nil && first == nil {
			first = err
		}
		delete(r.tenants, id)
	}
	if err := r.central.Close(); err != nil && first == nil {
		first = err
	}
	return first
}

// ParseDatabaseName is exposed for safe configuration diagnostics without
// returning a password or the full DSN to logs.
func ParseDatabaseName(dsn string) (string, error) {
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return "", err
	}
	if cfg.DBName == "" {
		return "", errors.New("database name required")
	}
	return cfg.DBName, nil
}
