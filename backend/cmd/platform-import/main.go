package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/alumasinde/gopass/internal/app/config"
	"github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
	sourceDSN := config.DSN()
	targetDSN := config.PlatformDSN()
	if sourceDSN == "" || targetDSN == "" {
		fatal(errors.New("configure DB_DSN (legacy/shared source) and PLATFORM_DB_DSN (new platform target)"))
	}
	srcCfg, e := mysql.ParseDSN(sourceDSN)
	if e != nil {
		fatal(e)
	}
	dstCfg, e := mysql.ParseDSN(targetDSN)
	if e != nil {
		fatal(e)
	}
	if strings.EqualFold(srcCfg.DBName, dstCfg.DBName) && strings.EqualFold(srcCfg.Addr, dstCfg.Addr) && strings.EqualFold(srcCfg.Net, dstCfg.Net) {
		fatal(errors.New("source and target point to the same database; use a separate PLATFORM_DB_DSN"))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	src, e := sql.Open("mysql", sourceDSN)
	if e != nil {
		fatal(e)
	}
	defer src.Close()
	dst, e := sql.Open("mysql", targetDSN)
	if e != nil {
		fatal(e)
	}
	defer dst.Close()
	if e = src.PingContext(ctx); e != nil {
		fatal(fmt.Errorf("connect to legacy source: %w", e))
	}
	if e = dst.PingContext(ctx); e != nil {
		fatal(fmt.Errorf("connect to platform target: %w", e))
	}
	for _, table := range []string{"organizations", "tenant_bootstrap_requests", "platform_users", "platform_sessions", "platform_audit_logs", "platform_migration_runs"} {
		if !exists(ctx, dst, table) {
			fatal(fmt.Errorf("target table %s is missing; run go run ./cmd/platform-migrate up first", table))
		}
		var n int
		if e = dst.QueryRowContext(ctx, "SELECT COUNT(*) FROM `"+table+"`").Scan(&n); e != nil {
			fatal(e)
		}
		if n != 0 {
			fatal(fmt.Errorf("target table %s is not empty; import is intentionally refusing to overwrite existing platform data", table))
		}
	}
	tx, e := dst.BeginTx(ctx, nil)
	if e != nil {
		fatal(e)
	}
	defer tx.Rollback()
	counts := map[string]int{}
	if exists(ctx, src, "organizations") {
		counts["organizations"], e = copyOrganizations(ctx, src, tx)
		if e != nil {
			fatal(e)
		}
	}
	if exists(ctx, src, "platform_users") {
		counts["platform_users"], e = copyAdmins(ctx, src, tx)
		if e != nil {
			fatal(e)
		}
	}
	if exists(ctx, src, "platform_sessions") {
		counts["platform_sessions"], e = copySessions(ctx, src, tx)
		if e != nil {
			fatal(e)
		}
	}
	if exists(ctx, src, "platform_audit_logs") {
		counts["platform_audit_logs"], e = copyAudit(ctx, src, tx)
		if e != nil {
			fatal(e)
		}
	}
	if exists(ctx, src, "platform_migration_runs") {
		counts["platform_migration_runs"], e = copyRuns(ctx, src, tx)
		if e != nil {
			fatal(e)
		}
	}
	if e = tx.Commit(); e != nil {
		fatal(e)
	}
	fmt.Println("Platform data import completed. Legacy source was not modified.")
	for _, t := range []string{"organizations", "tenant_bootstrap_requests", "platform_users", "platform_sessions", "platform_audit_logs", "platform_migration_runs"} {
		fmt.Printf("  %-24s %d rows\n", t, counts[t])
	}
}
func exists(ctx context.Context, db *sql.DB, table string) bool {
	var n int
	return db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name=?`, table).Scan(&n) == nil && n == 1
}
func copyOrganizations(ctx context.Context, src *sql.DB, tx *sql.Tx) (int, error) {
	rows, e := src.QueryContext(ctx, `SELECT id,name,slug,is_active,lifecycle_status,database_mode,provisioning_status,database_config_ciphertext,provisioning_error,provisioned_at,suspended_at,created_at,updated_at FROM organizations`)
	if e != nil {
		return 0, e
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var id int64
		var name, slug, life, mode, status string
		var active bool
		var cipher, problem sql.NullString
		var provisioned, suspended sql.NullTime
		var created, updated time.Time
		if e = rows.Scan(&id, &name, &slug, &active, &life, &mode, &status, &cipher, &problem, &provisioned, &suspended, &created, &updated); e != nil {
			return n, e
		}
		_, e = tx.ExecContext(ctx, `INSERT INTO organizations(id,name,slug,is_active,lifecycle_status,database_mode,provisioning_status,database_config_ciphertext,provisioning_error,provisioned_at,suspended_at,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`, id, name, slug, active, life, mode, status, nullString(cipher), nullString(problem), nullTime(provisioned), nullTime(suspended), created, updated)
		if e != nil {
			return n, e
		}
		n++
	}
	return n, rows.Err()
}
func copyAdmins(ctx context.Context, src *sql.DB, tx *sql.Tx) (int, error) {
	rows, e := src.QueryContext(ctx, `SELECT id,email,password_hash,display_name,is_active,last_login_at,created_at,updated_at FROM platform_users`)
	if e != nil {
		return 0, e
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var id int64
		var email, hash, name string
		var active bool
		var last sql.NullTime
		var created, updated time.Time
		if e = rows.Scan(&id, &email, &hash, &name, &active, &last, &created, &updated); e != nil {
			return n, e
		}
		_, e = tx.ExecContext(ctx, `INSERT INTO platform_users(id,email,password_hash,display_name,is_active,last_login_at,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)`, id, email, hash, name, active, nullTime(last), created, updated)
		if e != nil {
			return n, e
		}
		n++
	}
	return n, rows.Err()
}
func copySessions(ctx context.Context, src *sql.DB, tx *sql.Tx) (int, error) {
	rows, e := src.QueryContext(ctx, `SELECT session_id,platform_user_id,expires_at,revoked_at,created_at,updated_at FROM platform_sessions`)
	if e != nil {
		return 0, e
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var id string
		var uid int64
		var expires, created, updated time.Time
		var revoked sql.NullTime
		if e = rows.Scan(&id, &uid, &expires, &revoked, &created, &updated); e != nil {
			return n, e
		}
		_, e = tx.ExecContext(ctx, `INSERT INTO platform_sessions(session_id,platform_user_id,expires_at,revoked_at,created_at,updated_at) VALUES(?,?,?,?,?,?)`, id, uid, expires, nullTime(revoked), created, updated)
		if e != nil {
			return n, e
		}
		n++
	}
	return n, rows.Err()
}
func copyAudit(ctx context.Context, src *sql.DB, tx *sql.Tx) (int, error) {
	rows, e := src.QueryContext(ctx, `SELECT id,actor_platform_user_id,action,resource_type,resource_id,old_values,new_values,created_at FROM platform_audit_logs`)
	if e != nil {
		return 0, e
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var id int64
		var actor, resource sql.NullInt64
		var action, typ string
		var oldv, newv []byte
		var created time.Time
		if e = rows.Scan(&id, &actor, &action, &typ, &resource, &oldv, &newv, &created); e != nil {
			return n, e
		}
		_, e = tx.ExecContext(ctx, `INSERT INTO platform_audit_logs(id,actor_platform_user_id,action,resource_type,resource_id,old_values,new_values,created_at) VALUES(?,?,?,?,?,?,?,?)`, id, nullInt(actor), action, typ, nullInt(resource), nullableJSON(oldv), nullableJSON(newv), created)
		if e != nil {
			return n, e
		}
		n++
	}
	return n, rows.Err()
}
func copyRuns(ctx context.Context, src *sql.DB, tx *sql.Tx) (int, error) {
	rows, e := src.QueryContext(ctx, `SELECT run_id,requested_by,target_database,status,expected_versions,applied_versions,error_message,active_key,started_at,finished_at,created_at,updated_at FROM platform_migration_runs`)
	if e != nil {
		return 0, e
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var id, target, status string
		var actor int64
		var expected []byte
		var applied sql.NullString
		var message, active sql.NullString
		var started, finished sql.NullTime
		var created, updated time.Time
		if e = rows.Scan(&id, &actor, &target, &status, &expected, &applied, &message, &active, &started, &finished, &created, &updated); e != nil {
			return n, e
		}
		_, e = tx.ExecContext(ctx, `INSERT INTO platform_migration_runs(run_id,requested_by,target_database,status,expected_versions,applied_versions,error_message,active_key,started_at,finished_at,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, id, actor, target, status, expected, nullableJSON([]byte(applied.String)), nullString(message), nullString(active), nullTime(started), nullTime(finished), created, updated)
		if e != nil {
			return n, e
		}
		n++
	}
	return n, rows.Err()
}
func nullTime(v sql.NullTime) any {
	if v.Valid {
		return v.Time
	}
	return nil
}
func nullString(v sql.NullString) any {
	if v.Valid {
		return v.String
	}
	return nil
}
func nullInt(v sql.NullInt64) any {
	if v.Valid {
		return v.Int64
	}
	return nil
}
func nullableJSON(v []byte) any {
	if len(v) == 0 {
		return nil
	}
	return v
}
func fatal(e error) { fmt.Fprintln(os.Stderr, "platform-import:", e); os.Exit(1) }
