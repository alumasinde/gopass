package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Row is the common scanning surface shared by sql.Row and routed row results.
type Row interface{ Scan(dest ...any) error }

// Handle is the minimal database API used by tenant-owned modules. *sql.DB is
// adapted with Adapt; Router implements this interface and selects a database
// from the organization bound to context.
type Handle interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) Row
	BeginTx(context.Context, *sql.TxOptions) (*sql.Tx, error)
	PingContext(context.Context) error
	Close() error
}

type Static struct{ DB *sql.DB }

func (s *Static) ExecContext(ctx context.Context, q string, args ...any) (sql.Result, error) {
	return s.DB.ExecContext(ctx, q, args...)
}
func (s *Static) QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
	return s.DB.QueryContext(ctx, q, args...)
}
func (s *Static) QueryRowContext(ctx context.Context, q string, args ...any) Row {
	return s.DB.QueryRowContext(ctx, q, args...)
}
func (s *Static) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	return s.DB.BeginTx(ctx, opts)
}
func (s *Static) PingContext(ctx context.Context) error { return s.DB.PingContext(ctx) }
func (s *Static) Close() error                          { return s.DB.Close() }

// Adapt preserves compatibility with callers/tests that still pass *sql.DB.
func Adapt(value any) Handle {
	switch db := value.(type) {
	case nil:
		return unavailableHandle{}
	case Handle:
		return db
	case *sql.DB:
		return &Static{DB: db}
	default:
		panic(fmt.Sprintf("unsupported database handle %T", value))
	}
}

type errorRow struct{ err error }

func (r errorRow) Scan(...any) error { return r.err }

type unavailableHandle struct{}

func (unavailableHandle) ExecContext(context.Context, string, ...any) (sql.Result, error) {
	return nil, errors.New("database handle is not configured")
}
func (unavailableHandle) QueryContext(context.Context, string, ...any) (*sql.Rows, error) {
	return nil, errors.New("database handle is not configured")
}
func (unavailableHandle) QueryRowContext(context.Context, string, ...any) Row {
	return errorRow{errors.New("database handle is not configured")}
}
func (unavailableHandle) BeginTx(context.Context, *sql.TxOptions) (*sql.Tx, error) {
	return nil, errors.New("database handle is not configured")
}
func (unavailableHandle) PingContext(context.Context) error {
	return errors.New("database handle is not configured")
}
func (unavailableHandle) Close() error { return nil }
