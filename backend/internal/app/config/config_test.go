package config

import (
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
)

func TestDSNAlwaysParsesDatabaseTimesAsUTC(t *testing.T) {
	t.Setenv("DB_DSN", "user:pass@tcp(127.0.0.1:3306)/gopass?parseTime=false")
	t.Setenv("DB_NAME", "")
	dsn := DSN()
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse normalized DSN: %v", err)
	}
	if !cfg.ParseTime {
		t.Fatal("ParseTime must be enabled so DATETIME values are scanned as time.Time")
	}
	if cfg.Loc != time.UTC {
		t.Fatalf("DSN location = %v, want UTC", cfg.Loc)
	}
	if cfg.Params["time_zone"] != "'+00:00'" {
		t.Fatalf("MySQL session time_zone = %q, want '+00:00'", cfg.Params["time_zone"])
	}
}

func TestDSNUsesUTCForComponentConfiguration(t *testing.T) {
	t.Setenv("DB_DSN", "")
	t.Setenv("DB_NAME", "gopass_test")
	t.Setenv("DB_USER", "test")
	t.Setenv("DB_PASSWORD", "test")
	dsn := DSN()
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse generated DSN: %v", err)
	}
	if cfg.Loc != time.UTC {
		t.Fatalf("DSN location = %v, want UTC", cfg.Loc)
	}
	if !cfg.ParseTime {
		t.Fatal("ParseTime must be enabled")
	}
	if cfg.Params["time_zone"] != "'+00:00'" {
		t.Fatalf("MySQL session time_zone = %q, want '+00:00'", cfg.Params["time_zone"])
	}
}

func TestPlatformDSNUsesDedicatedDatabaseWhenConfigured(t *testing.T) {
	t.Setenv("PLATFORM_DB_DSN", "user:pass@tcp(127.0.0.1:3306)/gopass_platform?parseTime=false")
	dsn := PlatformDSN()
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse platform DSN: %v", err)
	}
	if cfg.DBName != "gopass_platform" {
		t.Fatalf("platform DB = %q, want gopass_platform", cfg.DBName)
	}
	if !cfg.ParseTime || cfg.Loc != time.UTC || cfg.Params["time_zone"] != "'+00:00'" {
		t.Fatal("platform DSN must enforce UTC time parsing and session timezone")
	}
}

func TestProductionRequiresDistinctPlatformDatabase(t *testing.T) {
	base := Config{AppEnv: "production", DBDSN: "user:pass@tcp(127.0.0.1:3306)/gopass", PlatformDBDSN: "user:pass@tcp(127.0.0.1:3306)/gopass", JWTSecret: "a-strong-secret-value-that-is-long-enough", AccessTTL: 15 * time.Minute, RefreshTTL: 24 * time.Hour, CORS: []string{"https://example.com"}}
	if err := Validate(base); err == nil {
		t.Fatal("expected production validation to reject same platform and shared schema")
	}
	base.PlatformDBDSN = "user:pass@tcp(127.0.0.1:3306)/gopass_platform"
	if err := Validate(base); err != nil {
		t.Fatalf("separate platform database should validate: %v", err)
	}
}
