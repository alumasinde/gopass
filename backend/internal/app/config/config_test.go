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
