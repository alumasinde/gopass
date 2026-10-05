package main

import (
	"context"
	"fmt"
	"github.com/alumasinde/gopass/internal/app/config"
	"github.com/alumasinde/gopass/internal/app/database"
	"github.com/alumasinde/gopass/internal/platform/auth"
	"log"
	"os"
	"strings"
	"time"
)

func main() {
	cfg := config.Load()
	db, e := database.Open(context.Background(), cfg.DBDSN)
	if e != nil {
		log.Fatal(e)
	}
	defer db.Close()
	email := env("SEED_ADMIN_EMAIL", "admin@example.com")
	password := env("SEED_ADMIN_PASSWORD", "ChangeMe12345!")
	orgName := env("SEED_ORG_NAME", "Demo Organization")
	slug := env("SEED_ORG_SLUG", "demo")
	tx, e := db.BeginTx(context.Background(), nil)
	if e != nil {
		log.Fatal(e)
	}
	defer tx.Rollback()
	var oid int64
	e = tx.QueryRow(`SELECT id FROM organizations WHERE slug=?`, slug).Scan(&oid)
	if e != nil {
		r, e := tx.Exec(`INSERT INTO organizations(name,slug,is_active,created_at,updated_at) VALUES(?,?,1,UTC_TIMESTAMP(),UTC_TIMESTAMP())`, orgName, slug)
		if e != nil {
			log.Fatal(e)
		}
		oid, _ = r.LastInsertId()
	}
	var rid int64
	e = tx.QueryRow(`SELECT id FROM roles WHERE organization_id=? AND code='organization_admin'`, oid).Scan(&rid)
	if e != nil {
		r, e := tx.Exec(`INSERT INTO roles(organization_id,name,code,is_system,created_at,updated_at) VALUES(?,?,?,1,UTC_TIMESTAMP(),UTC_TIMESTAMP())`, oid, "Organization Administrator", "organization_admin")
		if e != nil {
			log.Fatal(e)
		}
		rid, _ = r.LastInsertId()
		_, e = tx.Exec(`INSERT IGNORE INTO role_permissions(role_id,permission_id) SELECT ?,id FROM permissions`, rid)
		if e != nil {
			log.Fatal(e)
		}
	}
	var uid int64
	e = tx.QueryRow(`SELECT id FROM users WHERE organization_id=? AND email=?`, oid, email).Scan(&uid)
	if e != nil {
		h, _ := auth.Hash(password)
		r, e := tx.Exec(`INSERT INTO users(organization_id,first_name,last_name,email,password_hash,is_active,created_at,updated_at) VALUES(?,?,?,?,?,1,UTC_TIMESTAMP(),UTC_TIMESTAMP())`, oid, "Platform", "Admin", email, h)
		if e != nil {
			log.Fatal(e)
		}
		uid, _ = r.LastInsertId()
	}
	_, e = tx.Exec(`INSERT IGNORE INTO user_roles(user_id,role_id,organization_id,scope_type,is_active,created_at,updated_at) VALUES(?,?,?,'ORGANIZATION',1,UTC_TIMESTAMP(),UTC_TIMESTAMP())`, uid, rid, oid)
	if e != nil {
		log.Fatal(e)
	}
	if e = tx.Commit(); e != nil {
		log.Fatal(e)
	}
	fmt.Printf("seeded organization=%d admin=%s password=%s\n", oid, email, password)
	_ = strings.TrimSpace
	_ = time.Now
}
func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
