//go:build integration

package integration

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alumasinde/gopass/internal/app/database"
	"github.com/alumasinde/gopass/internal/platform/tenancy"
	mysql "github.com/go-sql-driver/mysql"
)

func encryptTenantDSN(t *testing.T, dsn string) string {
	t.Helper()
	raw := strings.TrimSpace(os.Getenv("PLATFORM_DB_ENCRYPTION_KEY"))
	var key []byte
	if len(raw) == 32 {
		key = []byte(raw)
	} else if b, err := base64.StdEncoding.DecodeString(raw); err == nil && len(b) == 32 {
		key = b
	} else if b, err := base64.RawStdEncoding.DecodeString(raw); err == nil && len(b) == 32 {
		key = b
	}
	if len(key) != 32 {
		t.Skip("set PLATFORM_DB_ENCRYPTION_KEY to a 32-byte key to run dedicated database integration tests")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		t.Fatal(err)
	}
	sealed := gcm.Seal(nonce, nonce, []byte(dsn), []byte("gopass:organization-db-dsn:v1"))
	return base64.RawStdEncoding.EncodeToString(sealed)
}

func TestDedicatedProvisioningRefusesSharedTenantWithOperationalData(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(fmt.Sprintf("dedicated-refuse-%d", time.Now().UnixNano()))
	h.createSite(org.id, fmt.Sprintf("KEEP-%d", time.Now().UnixNano()))
	if _, err := h.db.Exec(`UPDATE organizations SET is_active=0,lifecycle_status='PROVISIONING',database_mode='DEDICATED',provisioning_status='PENDING',database_config_ciphertext='not-used-because-existing-data-is-detected' WHERE id=?`, org.id); err != nil {
		t.Fatal(err)
	}
	router := database.NewRouter(h.db)
	t.Cleanup(func() { router.Invalidate(org.id) })
	err := router.ProvisionDedicated(context.Background(), org.id)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "operational data") {
		t.Fatalf("expected provisioning to fail closed when shared operational data exists, got %v", err)
	}
	var count int
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM sites WHERE organization_id=?`, org.id).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("refused provisioning must preserve shared records; sites count=%d", count)
	}
}

func TestDedicatedTenantRouterMigratesAndIsolatesTenantSchema(t *testing.T) {
	h := newHarness(t)
	dsn := strings.TrimSpace(os.Getenv("GOPASS_DEDICATED_TEST_DSN"))
	if dsn == "" {
		t.Skip("set GOPASS_DEDICATED_TEST_DSN to a separate empty MySQL database ending in _test")
	}
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(strings.ToLower(cfg.DBName), "_test") {
		t.Fatalf("refusing dedicated test database %q: name must end in _test", cfg.DBName)
	}
	var centralName string
	if err := h.db.QueryRow(`SELECT DATABASE()`).Scan(&centralName); err != nil {
		t.Fatal(err)
	}
	if strings.EqualFold(centralName, cfg.DBName) {
		t.Fatal("dedicated test database must differ from the central test database")
	}
	cfg.MultiStatements = true
	cfg.ParseTime = true
	cfg.Loc = time.UTC
	target, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = target.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := target.PingContext(ctx); err != nil {
		t.Fatalf("connect dedicated test database: %v", err)
	}
	var existing int
	if err := target.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=? AND table_name='organizations'`, cfg.DBName).Scan(&existing); err != nil {
		t.Fatal(err)
	}
	if existing == 1 {
		var orgCount int
		if err := target.QueryRowContext(ctx, `SELECT COUNT(*) FROM organizations`).Scan(&orgCount); err == nil && orgCount > 0 {
			t.Skip("dedicated test DB contains organization rows; clean it before running the isolation test")
		}
	}

	ciphertext := encryptTenantDSN(t, dsn)
	org := h.createOrg(fmt.Sprintf("dedicated-%d", time.Now().UnixNano()))
	t.Cleanup(func() {
		statements := []string{
			`DELETE FROM audit_logs WHERE organization_id=?`, `DELETE FROM refresh_tokens WHERE organization_id=?`, `DELETE FROM check_outs WHERE organization_id=?`, `DELETE FROM check_ins WHERE organization_id=?`, `DELETE FROM approval_requests WHERE organization_id=?`, `DELETE FROM credentials WHERE organization_id=?`, `DELETE FROM gatepasses WHERE organization_id=?`, `DELETE FROM approval_workflow_steps WHERE workflow_id IN (SELECT id FROM approval_workflows WHERE organization_id=?)`, `DELETE FROM approval_workflows WHERE organization_id=?`, `DELETE FROM user_roles WHERE organization_id=?`, `DELETE FROM role_permissions WHERE role_id IN (SELECT id FROM roles WHERE organization_id=?)`, `DELETE FROM roles WHERE organization_id=?`, `DELETE FROM visitors WHERE organization_id=?`, `DELETE FROM gates WHERE organization_id=?`, `DELETE FROM sites WHERE organization_id=?`, `DELETE FROM pass_types WHERE organization_id=?`, `DELETE FROM organization_settings WHERE organization_id=?`, `DELETE FROM notifications WHERE organization_id=?`, `DELETE FROM users WHERE organization_id=?`, `DELETE FROM organizations WHERE id=?`,
		}
		for _, stmt := range statements {
			if _, err := target.ExecContext(context.Background(), stmt, org.id); err != nil {
				t.Errorf("cleanup dedicated test tenant with %q: %v", stmt, err)
			}
		}
	})
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	user := h.createUser(org.id, "dedicated-user-"+suffix+"@example.test")
	if _, err := h.db.Exec(`UPDATE organizations SET is_active=0,lifecycle_status='PROVISIONING',database_mode='DEDICATED',provisioning_status='PENDING',database_config_ciphertext=? WHERE id=?`, ciphertext, org.id); err != nil {
		t.Fatal(err)
	}

	router := database.NewRouter(h.db)
	t.Cleanup(func() { router.Invalidate(org.id) })
	if err := router.ProvisionDedicated(ctx, org.id); err != nil {
		t.Fatalf("dedicated tenant provisioning failed: %v", err)
	}
	if _, err := h.db.Exec(`UPDATE organizations SET is_active=1,lifecycle_status='ACTIVE',provisioning_status='READY',provisioned_at=UTC_TIMESTAMP() WHERE id=?`, org.id); err != nil {
		t.Fatal(err)
	}

	tenantCtx := tenancy.With(ctx, org.id)
	var localOrgCount, localUserCount int
	if err := router.QueryRowContext(tenantCtx, `SELECT COUNT(*) FROM organizations`).Scan(&localOrgCount); err != nil {
		t.Fatal(err)
	}
	if err := router.QueryRowContext(tenantCtx, `SELECT COUNT(*) FROM users WHERE organization_id=? AND id=?`, org.id, user.id).Scan(&localUserCount); err != nil {
		t.Fatal(err)
	}
	if localOrgCount != 1 {
		t.Fatalf("dedicated DB exposed %d organization rows; want exactly 1", localOrgCount)
	}
	if localUserCount != 1 {
		t.Fatalf("dedicated DB did not receive its user identity: count=%d", localUserCount)
	}
	var centralOrgCount int
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM organizations`).Scan(&centralOrgCount); err != nil {
		t.Fatal(err)
	}
	if centralOrgCount <= localOrgCount {
		t.Fatalf("test fixture needs multiple central organizations to prove isolation: central=%d dedicated=%d", centralOrgCount, localOrgCount)
	}

	// Router must fail closed immediately when central lifecycle is disabled.
	if _, err := h.db.Exec(`UPDATE organizations SET is_active=0,lifecycle_status='SUSPENDED' WHERE id=?`, org.id); err != nil {
		t.Fatal(err)
	}
	if _, err := router.QueryContext(tenantCtx, `SELECT 1`); err == nil {
		t.Fatal("suspended dedicated tenant was still routed")
	}
	if err := router.RevokeSessions(ctx, org.id); err != nil {
		t.Fatalf("revoke dedicated sessions during suspension: %v", err)
	}
	router.Invalidate(org.id)
	if _, err := h.db.Exec(`UPDATE organizations SET is_active=1,lifecycle_status='ACTIVE',provisioning_status='READY' WHERE id=?`, org.id); err != nil {
		t.Fatal(err)
	}
	if _, err := router.QueryContext(tenantCtx, `SELECT 1`); err != nil {
		t.Fatalf("resumed dedicated tenant did not route: %v", err)
	}

	if _, err := h.db.Exec(`UPDATE organizations SET is_active=0,lifecycle_status='FAILED',provisioning_status='FAILED',provisioning_error='integration failure simulation' WHERE id=?`, org.id); err != nil {
		t.Fatal(err)
	}
	if _, err := router.QueryContext(tenantCtx, `SELECT 1`); err == nil {
		t.Fatal("failed dedicated tenant was still routed")
	}
	if _, err := h.db.Exec(`UPDATE organizations SET is_active=0,lifecycle_status='PROVISIONING',provisioning_status='PENDING',provisioning_error=NULL WHERE id=?`, org.id); err != nil {
		t.Fatal(err)
	}
	if err := router.ProvisionDedicated(ctx, org.id); err != nil {
		t.Fatalf("retry dedicated tenant provisioning: %v", err)
	}
	if _, err := h.db.Exec(`UPDATE organizations SET is_active=1,lifecycle_status='ACTIVE',provisioning_status='READY',provisioned_at=UTC_TIMESTAMP() WHERE id=?`, org.id); err != nil {
		t.Fatal(err)
	}
	if _, err := router.QueryContext(tenantCtx, `SELECT 1`); err != nil {
		t.Fatalf("recovered dedicated tenant did not route: %v", err)
	}

}
