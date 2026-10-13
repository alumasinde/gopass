package database

import (
	"context"
	"database/sql"
	_ "github.com/go-sql-driver/mysql"
	"time"
)

func Open(ctx context.Context, dsn string) (*sql.DB, error) {
	if dsn == "" {
		return nil, ErrConfig
	}
	db, e := sql.Open("mysql", dsn)
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
