package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/mail"
	"os"
	"strings"
	"time"

	"github.com/alumasinde/gopass/internal/app/config"
	"github.com/alumasinde/gopass/internal/platform/auth"
	_ "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
	email := strings.ToLower(strings.TrimSpace(os.Getenv("PLATFORM_ADMIN_EMAIL")))
	password := os.Getenv("PLATFORM_ADMIN_PASSWORD")
	name := strings.TrimSpace(os.Getenv("PLATFORM_ADMIN_NAME"))
	if name == "" {
		name = "GoPass Platform Administrator"
	}
	if _, err := mail.ParseAddress(email); err != nil || email == "" {
		fatal(errors.New("set PLATFORM_ADMIN_EMAIL to a valid email address"))
	}
	if len(password) < 8 {
		fatal(errors.New("PLATFORM_ADMIN_PASSWORD must be at least 8 characters"))
	}
	dsn := config.PlatformDSN()
	if dsn == "" {
		fatal(errors.New("configure PLATFORM_DB_DSN first (or DB_DSN for local legacy development)"))
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		fatal(err)
	}
	hash, err := auth.Hash(password)
	if err != nil {
		fatal(err)
	}
	res, err := db.ExecContext(ctx, `INSERT INTO platform_users(email,password_hash,display_name,is_active,created_at,updated_at) VALUES(?,?,?,1,UTC_TIMESTAMP(),UTC_TIMESTAMP())`, email, hash, name)
	if err != nil {
		fatal(fmt.Errorf("create platform administrator (email may already exist): %w", err))
	}
	id, err := res.LastInsertId()
	if err != nil {
		fatal(err)
	}
	fmt.Printf("Platform administrator created: id=%d email=%s\n", id, email)
}
func fatal(err error) { fmt.Fprintln(os.Stderr, "platform-admin-create:", err); os.Exit(1) }
