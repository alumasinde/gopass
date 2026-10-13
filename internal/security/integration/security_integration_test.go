package integration

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alumasinde/gopass/internal/app/http/middleware"
	"github.com/alumasinde/gopass/internal/modules/approvals"
	"github.com/alumasinde/gopass/internal/modules/checkins"
	"github.com/alumasinde/gopass/internal/modules/checkouts"
	"github.com/alumasinde/gopass/internal/modules/credentials"
	"github.com/alumasinde/gopass/internal/modules/gatepasses"
	"github.com/alumasinde/gopass/internal/modules/gates"
	"github.com/alumasinde/gopass/internal/modules/organizations"
	"github.com/alumasinde/gopass/internal/modules/passtypes"
	"github.com/alumasinde/gopass/internal/modules/roles"
	"github.com/alumasinde/gopass/internal/modules/sites"
	"github.com/alumasinde/gopass/internal/modules/users"
	"github.com/alumasinde/gopass/internal/modules/visitors"
	"github.com/alumasinde/gopass/internal/platform/audit"
	"github.com/alumasinde/gopass/internal/platform/auth"
	"github.com/alumasinde/gopass/internal/platform/rbac"
	"github.com/alumasinde/gopass/internal/routes"
	"github.com/go-chi/chi/v5"
	mysql "github.com/go-sql-driver/mysql"
)

const (
	integrationPassword = "Integration-Test-Password-42!"
	integrationEmail    = "security-admin@example.test"
)

type harness struct {
	t      *testing.T
	db     *sql.DB
	router http.Handler
	tokens *auth.Service
}

type seededOrg struct {
	id int64
}

type seededUser struct {
	id    int64
	email string
}

type tokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("GOPASS_TEST_DSN"))
	if dsn == "" {
		t.Skip("set GOPASS_TEST_DSN to a dedicated MySQL database ending in _test to run integration security tests")
	}
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse GOPASS_TEST_DSN: %v", err)
	}
	if !strings.HasSuffix(strings.ToLower(cfg.DBName), "_test") {
		t.Fatalf("refusing to run destructive integration tests against database %q: database name must end in _test", cfg.DBName)
	}
	cfg.ParseTime = true
	cfg.MultiStatements = true
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(3 * time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		t.Fatalf("connect to test MySQL: %v", err)
	}
	var applied int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE version >= 7`).Scan(&applied); err != nil {
		db.Close()
		t.Fatalf("cannot read schema_migrations; apply migrations 0001-0007 to the dedicated test database first: %v", err)
	}
	if applied == 0 {
		db.Close()
		t.Fatalf("migration 0007_revocable_sessions.sql is not recorded; run go run ./cmd/migrate up against the dedicated test database first")
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tokens := auth.New("integration-only-secret-change-me", "gopass-integration-tests", 15*time.Minute, 2*time.Hour)
	authz := rbac.New(db)
	auditService := audit.New(db, logger)
	authHandler := users.NewAuthService(db, tokens, auditService)
	userHandler := users.NewHandler(db, authz, auditService)
	r := chi.NewRouter()
	r.Use(middleware.Request, middleware.Security, middleware.Recover(logger))
	routes.RegisterAPI(r, routes.Dependencies{
		Auth: authHandler, Authz: authz, Audit: auditService,
		Users:         userHandler,
		Organizations: organizations.NewHandler(db, authz, auditService),
		Roles:         roles.NewHandler(db, authz, auditService),
		Sites:         sites.NewHandler(db, authz, auditService),
		Gates:         gates.NewHandler(db, authz, auditService),
		Visitors:      visitors.NewHandler(db, authz, auditService),
		Gatepasses:    gatepasses.NewHandler(db, authz, auditService),
		Approvals:     approvals.NewHandler(db, authz, auditService),
		Credentials:   credentials.NewHandler(db, authz, auditService),
		Checkins:      checkins.NewHandler(db, authz, auditService),
		Checkouts:     checkouts.NewHandler(db, authz, auditService),
		PassTypes:     passtypes.NewHandler(db, authz, auditService),
	})
	h := &harness{t: t, db: db, router: r, tokens: tokens}
	t.Cleanup(func() { _ = db.Close() })
	return h
}

func (h *harness) request(method, path, token string, body any) *httptest.ResponseRecorder {
	h.t.Helper()
	var b []byte
	if body != nil {
		var err error
		b, err = json.Marshal(body)
		if err != nil {
			h.t.Fatalf("marshal request: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(b))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.router.ServeHTTP(rec, req)
	return rec
}

func (h *harness) login(user seededUser, orgID int64) tokenPair {
	h.t.Helper()
	rec := h.request(http.MethodPost, "/api/v1/auth/login", "", map[string]any{
		"organization_id": orgID,
		"email":           user.email,
		"password":        integrationPassword,
	})
	if rec.Code != http.StatusOK {
		h.t.Fatalf("login failed: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var envelope struct {
		Data tokenPair `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		h.t.Fatalf("decode login response: %v; body=%s", err, rec.Body.String())
	}
	if envelope.Data.AccessToken == "" || envelope.Data.RefreshToken == "" {
		h.t.Fatalf("login returned empty token pair: %s", rec.Body.String())
	}
	return envelope.Data
}

func (h *harness) createOrg(label string) seededOrg {
	h.t.Helper()
	slug := fmt.Sprintf("it-%s-%d", label, time.Now().UnixNano())
	res, err := h.db.Exec(`INSERT INTO organizations(name,slug,is_active,created_at,updated_at) VALUES(?,?,1,UTC_TIMESTAMP(),UTC_TIMESTAMP())`, "Integration "+label, slug)
	if err != nil {
		h.t.Fatalf("create organization: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		h.t.Fatalf("organization id: %v", err)
	}
	h.t.Cleanup(func() { h.cleanupOrg(id) })
	return seededOrg{id: id}
}

func (h *harness) cleanupOrg(orgID int64) {
	// Delete in dependency order. Cleanup errors are intentionally ignored so
	// they do not hide the test's original assertion failure.
	statements := []string{
		`DELETE FROM audit_logs WHERE organization_id=?`,
		`DELETE FROM refresh_tokens WHERE organization_id=?`,
		`DELETE FROM check_outs WHERE organization_id=?`,
		`DELETE FROM check_ins WHERE organization_id=?`,
		`DELETE FROM approval_requests WHERE organization_id=?`,
		`DELETE FROM credentials WHERE organization_id=?`,
		`DELETE FROM gatepasses WHERE organization_id=?`,
		`DELETE FROM approval_workflow_steps WHERE workflow_id IN (SELECT id FROM approval_workflows WHERE organization_id=?)`,
		`DELETE FROM approval_workflows WHERE organization_id=?`,
		`DELETE FROM user_roles WHERE organization_id=?`,
		`DELETE FROM role_permissions WHERE role_id IN (SELECT id FROM roles WHERE organization_id=?)`,
		`DELETE FROM roles WHERE organization_id=?`,
		`DELETE FROM visitors WHERE organization_id=?`,
		`DELETE FROM gates WHERE organization_id=?`,
		`DELETE FROM sites WHERE organization_id=?`,
		`DELETE FROM pass_types WHERE organization_id=?`,
		`DELETE FROM organization_settings WHERE organization_id=?`,
		`DELETE FROM users WHERE organization_id=?`,
		`DELETE FROM organizations WHERE id=?`,
	}
	for _, stmt := range statements {
		if _, err := h.db.Exec(stmt, orgID); err != nil {
			h.t.Errorf("cleanup integration organization %d with %q: %v", orgID, stmt, err)
		}
	}
}

func (h *harness) createUser(orgID int64, email string) seededUser {
	h.t.Helper()
	hash, err := auth.Hash(integrationPassword)
	if err != nil {
		h.t.Fatalf("hash test password: %v", err)
	}
	res, err := h.db.Exec(`INSERT INTO users(organization_id,first_name,last_name,email,password_hash,is_active,created_at,updated_at) VALUES(?,?,?,?,?,1,UTC_TIMESTAMP(),UTC_TIMESTAMP())`, orgID, "Integration", "User", email, hash)
	if err != nil {
		h.t.Fatalf("create user %s: %v", email, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		h.t.Fatalf("user id: %v", err)
	}
	return seededUser{id: id, email: email}
}

func (h *harness) createRole(orgID int64, code string, permissionCodes ...string) int64 {
	h.t.Helper()
	res, err := h.db.Exec(`INSERT INTO roles(organization_id,name,code,is_system,created_at,updated_at) VALUES(?,?,?,0,UTC_TIMESTAMP(),UTC_TIMESTAMP())`, orgID, "Integration "+code, code)
	if err != nil {
		h.t.Fatalf("create role: %v", err)
	}
	roleID, err := res.LastInsertId()
	if err != nil {
		h.t.Fatalf("role id: %v", err)
	}
	for _, code := range permissionCodes {
		var permissionID int64
		if err := h.db.QueryRow(`SELECT id FROM permissions WHERE code=?`, code).Scan(&permissionID); err != nil {
			h.t.Fatalf("permission %q missing; apply permission catalogue migrations: %v", code, err)
		}
		if _, err := h.db.Exec(`INSERT INTO role_permissions(role_id,permission_id) VALUES(?,?)`, roleID, permissionID); err != nil {
			h.t.Fatalf("assign permission %s to role: %v", code, err)
		}
	}
	return roleID
}

func (h *harness) assignRole(orgID, userID, roleID int64, scope string, siteID, gateID *int64) {
	h.t.Helper()
	if _, err := h.db.Exec(`INSERT INTO user_roles(user_id,role_id,organization_id,scope_type,site_id,gate_id,is_active,created_at,updated_at) VALUES(?,?,?,?,?,?,1,UTC_TIMESTAMP(),UTC_TIMESTAMP())`, userID, roleID, orgID, scope, siteID, gateID); err != nil {
		h.t.Fatalf("assign role %d to user %d: %v", roleID, userID, err)
	}
}

func (h *harness) createSite(orgID int64, code string) int64 {
	h.t.Helper()
	res, err := h.db.Exec(`INSERT INTO sites(organization_id,name,code,is_active,created_at,updated_at) VALUES(?,?,?,1,UTC_TIMESTAMP(),UTC_TIMESTAMP())`, orgID, "Site "+code, code)
	if err != nil {
		h.t.Fatalf("create site %s: %v", code, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		h.t.Fatal(err)
	}
	return id
}

func (h *harness) createGate(orgID, siteID int64, code string) int64 {
	h.t.Helper()
	res, err := h.db.Exec(`INSERT INTO gates(organization_id,site_id,name,code,is_active,created_at,updated_at) VALUES(?,?,?,?,1,UTC_TIMESTAMP(),UTC_TIMESTAMP())`, orgID, siteID, "Gate "+code, code)
	if err != nil {
		h.t.Fatalf("create gate %s: %v", code, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		h.t.Fatal(err)
	}
	return id
}

func (h *harness) createGatepass(orgID, visitorID, gateID, passTypeID int64) int64 {
	h.t.Helper()
	start := time.Now().UTC().Add(-time.Hour).Format("2006-01-02 15:04:05")
	end := time.Now().UTC().Add(24 * time.Hour).Format("2006-01-02 15:04:05")
	res, err := h.db.Exec(`INSERT INTO gatepasses(organization_id,visitor_id,gate_id,pass_type_id,status,valid_from,valid_until,created_at,updated_at) VALUES(?,?,?,?, 'APPROVED',?,?,UTC_TIMESTAMP(),UTC_TIMESTAMP())`, orgID, visitorID, gateID, passTypeID, start, end)
	if err != nil {
		h.t.Fatalf("create gatepass: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		h.t.Fatal(err)
	}
	return id
}

func (h *harness) createVisitor(orgID int64, suffix string) int64 {
	h.t.Helper()
	res, err := h.db.Exec(`INSERT INTO visitors(organization_id,first_name,last_name,phone,email,is_blacklisted,created_at,updated_at) VALUES(?,?,?,?,?,0,UTC_TIMESTAMP(),UTC_TIMESTAMP())`, orgID, "Visitor", suffix, "0700000000", "visitor-"+suffix+"@example.test")
	if err != nil {
		h.t.Fatalf("create visitor: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		h.t.Fatal(err)
	}
	return id
}

func (h *harness) createPassType(orgID int64, suffix string) int64 {
	h.t.Helper()
	res, err := h.db.Exec(`INSERT INTO pass_types(organization_id,name,code,requires_approval,validity_minutes,is_active,created_at,updated_at) VALUES(?,?,?,0,480,1,UTC_TIMESTAMP(),UTC_TIMESTAMP())`, orgID, "Visitor "+suffix, "IT-"+suffix)
	if err != nil {
		h.t.Fatalf("create pass type: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		h.t.Fatal(err)
	}
	return id
}

func (h *harness) permissionID(code string) int64 {
	h.t.Helper()
	var id int64
	if err := h.db.QueryRow(`SELECT id FROM permissions WHERE code=?`, code).Scan(&id); err != nil {
		h.t.Fatalf("permission %s: %v", code, err)
	}
	return id
}

// TestCrossOrganizationReadUpdateDeleteDenied exercises the actual HTTP layer
// and verifies the foreign organization's row remains unchanged in MySQL.
func TestCrossOrganizationReadUpdateDeleteDenied(t *testing.T) {
	h := newHarness(t)
	orgA := h.createOrg("crud-a")
	orgB := h.createOrg("crud-b")
	admin := h.createUser(orgA.id, "crud-admin@example.test")
	roleID := h.createRole(orgA.id, "crud-admin", "sites.view", "sites.update", "sites.delete")
	h.assignRole(orgA.id, admin.id, roleID, "ORGANIZATION", nil, nil)
	siteA := h.createSite(orgA.id, "CRUD-A")
	siteB := h.createSite(orgB.id, "CRUD-B")
	_ = siteA
	tokens := h.login(admin, orgA.id)

	get := h.request(http.MethodGet, fmt.Sprintf("/api/v1/sites/%d", siteB), tokens.AccessToken, nil)
	if get.Code != http.StatusNotFound {
		t.Fatalf("cross-org read: expected 404, got %d: %s", get.Code, get.Body.String())
	}

	update := h.request(http.MethodPut, fmt.Sprintf("/api/v1/sites/%d", siteB), tokens.AccessToken, map[string]any{"name": "COMPROMISED", "code": "CRUD-B", "is_active": true})
	if update.Code != http.StatusNotFound {
		t.Fatalf("cross-org update: expected 404, got %d: %s", update.Code, update.Body.String())
	}

	delete := h.request(http.MethodDelete, fmt.Sprintf("/api/v1/sites/%d", siteB), tokens.AccessToken, nil)
	if delete.Code != http.StatusNotFound {
		t.Fatalf("cross-org delete: expected 404, got %d: %s", delete.Code, delete.Body.String())
	}

	var name string
	var active bool
	if err := h.db.QueryRow(`SELECT name,is_active FROM sites WHERE id=? AND organization_id=?`, siteB, orgB.id).Scan(&name, &active); err != nil {
		t.Fatalf("read target site after denied requests: %v", err)
	}
	if name != "Site CRUD-B" || !active {
		t.Fatalf("cross-org requests changed foreign record: name=%q active=%v", name, active)
	}
}

// TestSiteAndGateScopedRBAC checks both positive and negative access using
// gatepass endpoints backed by real sites, gates, and gatepasses in MySQL.
func TestSiteAndGateScopedRBAC(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg("scope")
	site1 := h.createSite(org.id, "SCOPE-1")
	site2 := h.createSite(org.id, "SCOPE-2")
	gate1 := h.createGate(org.id, site1, "SCOPE-G1")
	gate2 := h.createGate(org.id, site2, "SCOPE-G2")
	gate3 := h.createGate(org.id, site1, "SCOPE-G3")
	visitor := h.createVisitor(org.id, "scope")
	passType := h.createPassType(org.id, "scope")
	pass1 := h.createGatepass(org.id, visitor, gate1, passType)
	pass2 := h.createGatepass(org.id, visitor, gate2, passType)
	pass3 := h.createGatepass(org.id, visitor, gate3, passType)

	siteUser := h.createUser(org.id, "site-scoped@example.test")
	siteRole := h.createRole(org.id, "site-scope", "gatepasses.view")
	h.assignRole(org.id, siteUser.id, siteRole, "SITE", &site1, nil)
	siteTokens := h.login(siteUser, org.id)
	if rec := h.request(http.MethodGet, fmt.Sprintf("/api/v1/gatepasses/%d", pass1), siteTokens.AccessToken, nil); rec.Code != http.StatusOK {
		t.Fatalf("site-scoped user should read pass at assigned site: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := h.request(http.MethodGet, fmt.Sprintf("/api/v1/gatepasses/%d", pass2), siteTokens.AccessToken, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("site-scoped user must not read pass at another site: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := h.request(http.MethodGet, fmt.Sprintf("/api/v1/gatepasses/%d", pass3), siteTokens.AccessToken, nil); rec.Code != http.StatusOK {
		t.Fatalf("site-scoped user should access another gate at the assigned site: status=%d body=%s", rec.Code, rec.Body.String())
	}

	gateUser := h.createUser(org.id, "gate-scoped@example.test")
	gateRole := h.createRole(org.id, "gate-scope", "gatepasses.view")
	h.assignRole(org.id, gateUser.id, gateRole, "GATE", nil, &gate1)
	gateTokens := h.login(gateUser, org.id)
	if rec := h.request(http.MethodGet, fmt.Sprintf("/api/v1/gatepasses/%d", pass1), gateTokens.AccessToken, nil); rec.Code != http.StatusOK {
		t.Fatalf("gate-scoped user should read pass at assigned gate: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := h.request(http.MethodGet, fmt.Sprintf("/api/v1/gatepasses/%d", pass2), gateTokens.AccessToken, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("gate-scoped user must not read pass at another site: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := h.request(http.MethodGet, fmt.Sprintf("/api/v1/gatepasses/%d", pass3), gateTokens.AccessToken, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("gate-scoped user must not read another gate at the same site: status=%d body=%s", rec.Code, rec.Body.String())
	}
	// Ensure the gates are distinct resources even within the same org.
	if gate1 == gate2 || gate1 == gate3 || gate2 == gate3 {
		t.Fatal("fixture error: gates should differ")
	}
}

func TestLogoutImmediatelyRevokesExistingAccessToken(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg("logout")
	user := h.createUser(org.id, integrationEmail)
	tokens := h.login(user, org.id)

	before := h.request(http.MethodGet, "/api/v1/auth/me", tokens.AccessToken, nil)
	if before.Code != http.StatusOK {
		t.Fatalf("access token should work before logout: %d %s", before.Code, before.Body.String())
	}
	logout := h.request(http.MethodPost, "/api/v1/auth/logout", "", map[string]string{"refresh_token": tokens.RefreshToken})
	if logout.Code != http.StatusOK {
		t.Fatalf("logout failed: %d %s", logout.Code, logout.Body.String())
	}
	after := h.request(http.MethodGet, "/api/v1/auth/me", tokens.AccessToken, nil)
	if after.Code != http.StatusUnauthorized {
		t.Fatalf("access token must be invalid immediately after logout: got %d body=%s", after.Code, after.Body.String())
	}

	var revoked int
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM refresh_tokens WHERE organization_id=? AND user_id=? AND revoked_at IS NOT NULL`, org.id, user.id).Scan(&revoked); err != nil {
		t.Fatalf("check revoked refresh token: %v", err)
	}
	if revoked == 0 {
		t.Fatal("logout did not persist refresh-token revocation")
	}
	assertAuditEvent(t, h.db, org.id, user.id, "user.session_revoked")
}

func TestRefreshTokenReplayRejectedAndReplacementWorks(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg("refresh")
	user := h.createUser(org.id, "refresh-user@example.test")
	initial := h.login(user, org.id)

	first := h.request(http.MethodPost, "/api/v1/auth/refresh", "", map[string]string{"refresh_token": initial.RefreshToken})
	if first.Code != http.StatusOK {
		t.Fatalf("first refresh failed: %d %s", first.Code, first.Body.String())
	}
	var envelope struct {
		Data tokenPair `json:"data"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode refresh response: %v", err)
	}
	rotated := envelope.Data
	if rotated.AccessToken == "" || rotated.RefreshToken == "" {
		t.Fatalf("refresh did not return both rotated tokens: %s", first.Body.String())
	}
	if rotated.RefreshToken == initial.RefreshToken {
		t.Fatal("refresh token was not rotated")
	}

	replay := h.request(http.MethodPost, "/api/v1/auth/refresh", "", map[string]string{"refresh_token": initial.RefreshToken})
	if replay.Code != http.StatusUnauthorized {
		t.Fatalf("replayed refresh token must be rejected: status=%d body=%s", replay.Code, replay.Body.String())
	}

	next := h.request(http.MethodPost, "/api/v1/auth/refresh", "", map[string]string{"refresh_token": rotated.RefreshToken})
	if next.Code != http.StatusOK {
		t.Fatalf("replacement refresh token should remain valid after old-token replay: %d %s", next.Code, next.Body.String())
	}
	assertAuditEvent(t, h.db, org.id, user.id, "user.session_refreshed")
}

func TestRolePermissionChangesPersistAuditRecords(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg("role-audit")
	admin := h.createUser(org.id, "role-audit-admin@example.test")
	target := h.createUser(org.id, "role-audit-target@example.test")
	adminRole := h.createRole(org.id, "role-audit-admin", "roles.manage_permissions", "roles.assign", "roles.view", "visitors.view")
	h.assignRole(org.id, admin.id, adminRole, "ORGANIZATION", nil, nil)
	targetRole, err := h.db.Exec(`INSERT INTO roles(organization_id,name,code,is_system,created_at,updated_at) VALUES(?,?,?,0,UTC_TIMESTAMP(),UTC_TIMESTAMP())`, org.id, "Temporary visitor viewer", "temporary-visitor-viewer")
	if err != nil {
		t.Fatalf("create target role: %v", err)
	}
	targetRoleID, err := targetRole.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	visitorViewID := h.permissionID("visitors.view")
	tokens := h.login(admin, org.id)

	assignedPermission := h.request(http.MethodPost, fmt.Sprintf("/api/v1/roles/%d/permissions", targetRoleID), tokens.AccessToken, map[string]int64{"permission_id": visitorViewID})
	if assignedPermission.Code != http.StatusOK {
		t.Fatalf("assign permission: status=%d body=%s", assignedPermission.Code, assignedPermission.Body.String())
	}

	assignedRole := h.request(http.MethodPost, "/api/v1/roles/assign", tokens.AccessToken, map[string]any{"user_id": target.id, "role_id": targetRoleID, "scope_type": "ORGANIZATION"})
	if assignedRole.Code != http.StatusOK {
		t.Fatalf("assign role: status=%d body=%s", assignedRole.Code, assignedRole.Body.String())
	}

	removedRole := h.request(http.MethodDelete, "/api/v1/roles/assign", tokens.AccessToken, map[string]int64{"user_id": target.id, "role_id": targetRoleID})
	if removedRole.Code != http.StatusOK {
		t.Fatalf("remove role: status=%d body=%s", removedRole.Code, removedRole.Body.String())
	}

	removedPermission := h.request(http.MethodDelete, fmt.Sprintf("/api/v1/roles/%d/permissions/%d", targetRoleID, visitorViewID), tokens.AccessToken, nil)
	if removedPermission.Code != http.StatusOK {
		t.Fatalf("remove permission: status=%d body=%s", removedPermission.Code, removedPermission.Body.String())
	}

	for _, action := range []string{"role.permission_assigned", "role.assigned", "role.removed", "role.permission_removed"} {
		assertAuditEvent(t, h.db, org.id, admin.id, action)
	}
	assertAuditPayload(t, h.db, org.id, admin.id, "role.permission_assigned", false, true)
	assertAuditPayload(t, h.db, org.id, admin.id, "role.assigned", false, true)
	assertAuditPayload(t, h.db, org.id, admin.id, "role.removed", true, false)
	assertAuditPayload(t, h.db, org.id, admin.id, "role.permission_removed", true, false)
}

func assertAuditPayload(t *testing.T, db *sql.DB, orgID, actorID int64, action string, wantOld, wantNew bool) {
	t.Helper()
	var oldPresent, newPresent bool
	err := db.QueryRow(`SELECT old_values IS NOT NULL,new_values IS NOT NULL FROM audit_logs WHERE organization_id=? AND actor_user_id=? AND action=? ORDER BY id DESC LIMIT 1`, orgID, actorID, action).Scan(&oldPresent, &newPresent)
	if err != nil {
		t.Fatalf("query audit payload %q: %v", action, err)
	}
	if oldPresent != wantOld || newPresent != wantNew {
		t.Fatalf("audit payload %q: old_values present=%v (want %v), new_values present=%v (want %v)", action, oldPresent, wantOld, newPresent, wantNew)
	}
}

func assertAuditEvent(t *testing.T, db *sql.DB, orgID, actorID int64, action string) {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE organization_id=? AND actor_user_id=? AND action=? AND resource_type<>'' AND resource_id IS NOT NULL`, orgID, actorID, action).Scan(&n); err != nil {
		t.Fatalf("query audit event %q: %v", action, err)
	}
	if n == 0 {
		t.Fatalf("expected persistent audit event %q with organization, actor, and resource attribution for org=%d actor=%d", action, orgID, actorID)
	}
}
