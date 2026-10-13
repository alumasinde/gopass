package database

import (
	"context"
	"errors"
	"testing"
)

func TestRouterFailsClosedWithoutTenantContext(t *testing.T) {
	r := NewRouter(nil)
	if _, err := r.QueryContext(context.Background(), `SELECT 1`); !errors.Is(err, ErrTenantUnavailable) {
		t.Fatalf("query without tenant context should fail closed, got %v", err)
	}
	if _, err := r.BeginTx(context.Background(), nil); !errors.Is(err, ErrTenantUnavailable) {
		t.Fatalf("transaction without tenant context should fail closed, got %v", err)
	}
	if err := r.QueryRowContext(context.Background(), `SELECT 1`).Scan(new(int)); !errors.Is(err, ErrTenantUnavailable) {
		t.Fatalf("row query without tenant context should fail closed, got %v", err)
	}
}

func TestKeyFromEnvRejectsInvalidEncryptionKey(t *testing.T) {
	if got := keyFromEnv("short"); len(got) != 0 {
		t.Fatalf("expected invalid key to be rejected, got %d bytes", len(got))
	}
}
