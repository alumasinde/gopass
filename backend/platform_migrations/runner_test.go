package platformmigrations

import (
	"strings"
	"testing"
)

func TestEmbeddedPlatformMigrationsAreOrderedAndNamed(t *testing.T) {
	files, err := Embedded()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("expected embedded platform migrations")
	}
	for i, f := range files {
		if f.Version < 1 || f.Name == "" || strings.TrimSpace(f.SQL) == "" {
			t.Fatalf("invalid migration: %#v", f)
		}
		if i > 0 && files[i-1].Version >= f.Version {
			t.Fatalf("migrations are not strictly ordered: %d then %d", files[i-1].Version, f.Version)
		}
	}
}
func TestPlatformMigrationContainsControlPlaneTables(t *testing.T) {
	files, err := Embedded()
	if err != nil {
		t.Fatal(err)
	}
	sql := files[0].SQL
	for _, table := range []string{"organizations", "tenant_bootstrap_requests", "platform_users", "platform_sessions", "platform_audit_logs", "platform_migration_runs"} {
		if !strings.Contains(sql, "CREATE TABLE IF NOT EXISTS "+table) {
			t.Errorf("platform migration missing table %s", table)
		}
	}
	if strings.Contains(strings.ToLower(sql), "password ") {
		t.Fatal("platform migration must not define plaintext password fields")
	}
}
func TestChecksumStable(t *testing.T) {
	a := checksum("SELECT 1;\r\n")
	b := checksum("SELECT 1;\n")
	if a != b {
		t.Fatal("checksum should normalize line endings")
	}
	if len(a) != 64 {
		t.Fatalf("checksum length = %d", len(a))
	}
}

func TestSplitSQLStatementsKeepsSemicolonsInsideStringsAndComments(t *testing.T) {
	statements, err := splitSQLStatements("-- header; comment\nINSERT INTO t(v) VALUES('a;b'); /* block; comment */ SELECT `x;y` FROM t;")
	if err != nil {
		t.Fatal(err)
	}
	if len(statements) != 2 {
		t.Fatalf("got %d statements, want 2: %#v", len(statements), statements)
	}
	if !strings.Contains(statements[0], "a;b") || !strings.Contains(statements[1], "x;y") {
		t.Fatalf("quoted semicolons were split: %#v", statements)
	}
}

func TestOrganizationIDSequenceMigrationExists(t *testing.T) {
	files, err := Embedded()
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, f := range files {
		if f.Version == 2 {
			found = true
			for _, token := range []string{"platform_id_sequences", "next_value", "organization"} {
				if !strings.Contains(f.SQL, token) {
					t.Errorf("organization ID migration missing %q", token)
				}
			}
		}
	}
	if !found {
		t.Fatal("platform migration 0002 must provide serialized organization ID allocation")
	}
}
