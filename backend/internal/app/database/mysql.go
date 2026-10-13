package database

import (
	"context"
	"database/sql"
	"time"

	mysql "github.com/go-sql-driver/mysql"
)

func Open(ctx context.Context, dsn string) (*sql.DB, error) {
	if dsn == "" {
		return nil, ErrConfig
	}
	cfg, e := mysql.ParseDSN(dsn)
	if e != nil {
		return nil, e
	}
	cfg.ParseTime = true
	cfg.Loc = time.UTC
	// Never enable multi-statement execution on the general application pool.
	cfg.MultiStatements = false
	if cfg.Params == nil {
		cfg.Params = map[string]string{}
	}
	cfg.Params["time_zone"] = "'+00:00'"
	db, e := sql.Open("mysql", cfg.FormatDSN())
	if e != nil {
		return nil, e
	}
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(30 * time.Minute)
	if e = db.PingContext(ctx); e != nil {
		db.Close()
		return nil, e
	}
	return db, nil
}

var ErrConfig = errorString("DB_DSN is required")

type errorString string

func (e errorString) Error() string { return string(e) }
