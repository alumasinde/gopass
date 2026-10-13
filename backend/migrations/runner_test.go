package migrations

import (
	"strings"
	"testing"
)

func TestEmbeddedMigrationsAreOrderedAndNamed(t *testing.T) {
	all, err := Embedded()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) < 12 {
		t.Fatalf("expected at least 12 embedded migrations, got %d", len(all))
	}
	for i, migration := range all {
		if migration.Version < 1 || migration.Name == "" || migration.SQL == "" {
			t.Fatalf("invalid migration at index %d: %#v", i, migration)
		}
		if i > 0 && all[i-1].Version >= migration.Version {
			t.Fatalf("migrations are not strictly ordered: %d then %d", all[i-1].Version, migration.Version)
		}
	}
	if all[len(all)-1].Version != 12 || all[len(all)-1].Name != "visitors_archive" {
		t.Fatalf("expected migration 0012_visitors_archive, got %#v", all[len(all)-1])
	}
}

func TestChecksumChangesWhenMigrationContentChanges(t *testing.T) {
	a := Checksum("SELECT 1;")
	b := Checksum("SELECT 2;")
	if a == b {
		t.Fatal("different migration content must produce different checksums")
	}
	if a != Checksum("SELECT 1;") {
		t.Fatal("checksum should be stable")
	}
	if Checksum("SELECT 1;\r\n") != Checksum("SELECT 1;\n") {
		t.Fatal("checksum should normalize Windows line endings")
	}
}

func TestSplitSQLStatementsRespectsStringsAndComments(t *testing.T) {
	source := "-- migration comment with semicolon ;\nINSERT INTO example(value) VALUES ('inside;string'); /* block ; comment */\nSELECT 2; # trailing comment ;\n"
	got, err := splitSQLStatements(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 statements, got %d: %#v", len(got), got)
	}
	if !strings.Contains(got[0], "'inside;string'") || !strings.Contains(got[1], "SELECT 2") {
		t.Fatalf("unexpected split statements: %#v", got)
	}
}

func TestSplitSQLStatementsRejectsUnterminatedString(t *testing.T) {
	if _, err := splitSQLStatements("SELECT 'unterminated;"); err == nil {
		t.Fatal("expected an unterminated string error")
	}
}

func TestAllEmbeddedMigrationsSplitWithoutLexicalErrors(t *testing.T) {
	all, err := Embedded()
	if err != nil {
		t.Fatal(err)
	}
	for _, migration := range all {
		if migration.Version == 6 {
			// Version 6 is intentionally applied by the recovery helper.
			continue
		}
		statements, err := splitSQLStatements(migration.SQL)
		if err != nil {
			t.Fatalf("migration %04d_%s: %v", migration.Version, migration.Name, err)
		}
		if len(statements) == 0 {
			t.Fatalf("migration %04d_%s has no executable statements", migration.Version, migration.Name)
		}
	}
}
