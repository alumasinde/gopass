package migrations

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed *.sql
var files embed.FS

const advisoryLockName = "gopass_schema_migrate"

type File struct {
	Version int
	Name    string
	SQL     string
}

type StatusEntry struct {
	Version        int        `json:"version"`
	Name           string     `json:"name"`
	Applied        bool       `json:"applied"`
	AppliedAt      *time.Time `json:"applied_at,omitempty"`
	ChecksumStatus string     `json:"checksum_status"`
	Drift          bool       `json:"drift"`
}

type StatusReport struct {
	HistoryExists bool          `json:"history_exists"`
	HasChecksums  bool          `json:"has_checksums"`
	Migrations    []StatusEntry `json:"migrations"`
	PendingCount  int           `json:"pending_count"`
	AppliedCount  int           `json:"applied_count"`
	DriftCount    int           `json:"drift_count"`
}

func Embedded() ([]File, error) {
	entries, err := fs.ReadDir(files, ".")
	if err != nil {
		return nil, err
	}
	out := make([]File, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		base := strings.TrimSuffix(e.Name(), ".sql")
		parts := strings.SplitN(base, "_", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid embedded migration filename %q", e.Name())
		}
		v, err := strconv.Atoi(parts[0])
		if err != nil || v < 1 {
			return nil, fmt.Errorf("invalid embedded migration version in %q", e.Name())
		}
		b, err := files.ReadFile(e.Name())
		if err != nil {
			return nil, err
		}
		if len(strings.TrimSpace(string(b))) == 0 {
			return nil, fmt.Errorf("embedded migration %q is empty", e.Name())
		}
		out = append(out, File{Version: v, Name: parts[1], SQL: strings.TrimPrefix(string(b), "\ufeff")})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	for i := 1; i < len(out); i++ {
		if out[i-1].Version == out[i].Version {
			return nil, fmt.Errorf("duplicate embedded migration version %d", out[i].Version)
		}
	}
	return out, nil
}

func Checksum(sqlText string) string {
	normalized := strings.TrimPrefix(sqlText, "\ufeff")
	normalized = strings.ReplaceAll(normalized, "\r\n", "\n")
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}

// Status is read-only. A database without migration history is represented as
// all-pending; the status page never creates or modifies schema objects.
func Status(ctx context.Context, db *sql.DB) (StatusReport, error) {
	all, err := Embedded()
	if err != nil {
		return StatusReport{}, err
	}
	report := StatusReport{HistoryExists: true, Migrations: make([]StatusEntry, 0, len(all))}
	var tableCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='schema_migrations'`).Scan(&tableCount); err != nil {
		return StatusReport{}, fmt.Errorf("inspect migration history: %w", err)
	}
	if tableCount == 0 {
		report.HistoryExists = false
		for _, m := range all {
			report.Migrations = append(report.Migrations, StatusEntry{Version: m.Version, Name: m.Name, ChecksumStatus: "NOT_APPLIED"})
			report.PendingCount++
		}
		return report, nil
	}

	hasChecksums, err := hasColumn(ctx, db, "schema_migrations", "checksum")
	if err != nil {
		return StatusReport{}, err
	}
	report.HasChecksums = hasChecksums

	type appliedRecord struct {
		name     string
		at       sql.NullTime
		checksum sql.NullString
	}
	applied := map[int]appliedRecord{}
	query := `SELECT version,name,applied_at,NULL AS checksum FROM schema_migrations`
	if hasChecksums {
		query = `SELECT version,name,applied_at,checksum FROM schema_migrations`
	}
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return StatusReport{}, fmt.Errorf("read migration history: %w", err)
	}
	for rows.Next() {
		var version int
		var record appliedRecord
		if err := rows.Scan(&version, &record.name, &record.at, &record.checksum); err != nil {
			rows.Close()
			return StatusReport{}, fmt.Errorf("scan migration history: %w", err)
		}
		applied[version] = record
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return StatusReport{}, err
	}
	rows.Close()

	known := make(map[int]bool, len(all))
	for _, m := range all {
		known[m.Version] = true
		record, ok := applied[m.Version]
		entry := StatusEntry{Version: m.Version, Name: m.Name, Applied: ok, ChecksumStatus: "NOT_APPLIED"}
		if !ok {
			report.PendingCount++
			report.Migrations = append(report.Migrations, entry)
			continue
		}
		report.AppliedCount++
		entry.ChecksumStatus = "UNVERIFIED"
		if record.at.Valid {
			t := record.at.Time.UTC()
			entry.AppliedAt = &t
		}
		if record.name != m.Name {
			entry.Drift = true
			entry.ChecksumStatus = "NAME_MISMATCH"
		} else if record.checksum.Valid && record.checksum.String != "" {
			if record.checksum.String == Checksum(m.SQL) {
				entry.ChecksumStatus = "MATCH"
			} else {
				entry.Drift = true
				entry.ChecksumStatus = "MISMATCH"
			}
		}
		if entry.Drift {
			report.DriftCount++
		}
		report.Migrations = append(report.Migrations, entry)
	}
	for version, record := range applied {
		if known[version] {
			continue
		}
		entry := StatusEntry{Version: version, Name: record.name, Applied: true, ChecksumStatus: "UNKNOWN_MIGRATION", Drift: true}
		if record.at.Valid {
			t := record.at.Time.UTC()
			entry.AppliedAt = &t
		}
		report.Migrations = append(report.Migrations, entry)
		report.DriftCount++
	}
	sort.Slice(report.Migrations, func(i, j int) bool { return report.Migrations[i].Version < report.Migrations[j].Version })
	return report, nil
}

// ApplyEmbedded serializes UI and CLI migration runs using the same MySQL
// advisory lock. MySQL DDL auto-commits, so migrations must be safely rerunnable.
func ApplyEmbedded(ctx context.Context, db *sql.DB) error {
	all, err := Embedded()
	if err != nil {
		return err
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("open migration connection: %w", err)
	}
	defer conn.Close()

	var got sql.NullInt64
	if err := conn.QueryRowContext(ctx, `SELECT GET_LOCK(?, 30)`, advisoryLockName).Scan(&got); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	if !got.Valid || got.Int64 != 1 {
		return errors.New("another migration run is in progress; try again later")
	}
	defer func() {
		rctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = conn.ExecContext(rctx, `SELECT RELEASE_LOCK(?)`, advisoryLockName)
	}()

	if _, err := conn.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version BIGINT UNSIGNED NOT NULL,name VARCHAR(255) NOT NULL,applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,PRIMARY KEY(version)) ENGINE=InnoDB`); err != nil {
		return fmt.Errorf("ensure schema_migrations: %w", err)
	}

	report, err := statusWithConn(ctx, conn, all)
	if err != nil {
		return err
	}
	if report.DriftCount > 0 {
		return fmt.Errorf("migration history contains %d drift/unknown entries; inspect migration status and resolve before applying", report.DriftCount)
	}

	for _, m := range all {
		var exists int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE version=?`, m.Version).Scan(&exists); err != nil {
			return err
		}
		if exists != 0 {
			continue
		}
		if m.Version == 6 {
			if err := applyCredentialTokenMigration(ctx, conn); err != nil {
				return fmt.Errorf("migration %04d_%s failed: %w", m.Version, m.Name, err)
			}
		} else if err := executeSQLStatements(ctx, conn, m.SQL); err != nil {
			return fmt.Errorf("migration %04d_%s failed: %w; MySQL DDL may have partially applied; fix the cause and retry", m.Version, m.Name, err)
		}

		hasChecksums, err := hasColumn(ctx, conn, "schema_migrations", "checksum")
		if err != nil {
			return err
		}
		if hasChecksums {
			_, err = conn.ExecContext(ctx, `INSERT INTO schema_migrations(version,name,checksum) VALUES(?,?,?)`, m.Version, m.Name, Checksum(m.SQL))
		} else {
			_, err = conn.ExecContext(ctx, `INSERT INTO schema_migrations(version,name) VALUES(?,?)`, m.Version, m.Name)
		}
		if err != nil {
			return fmt.Errorf("record migration %04d: %w", m.Version, err)
		}
	}
	return nil
}

// executeSQLStatements runs trusted migration SQL one statement at a time on
// the same connection. This preserves session variables/PREPARE state without
// enabling multiStatements on the application's general-purpose DB pool.
func executeSQLStatements(ctx context.Context, conn *sql.Conn, source string) error {
	statements, err := splitSQLStatements(source)
	if err != nil {
		return err
	}
	for i, statement := range statements {
		if _, err := conn.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("statement %d: %w", i+1, err)
		}
	}
	return nil
}

// splitSQLStatements understands MySQL quoted strings, identifiers and comments,
// so semicolons inside literals/comments do not split a statement.
func splitSQLStatements(source string) ([]string, error) {
	var statements []string
	start := 0
	state := byte(0) // 0 normal, ' single quote, " double quote, ` backtick, 1 line comment, 2 block comment
	for i := 0; i < len(source); i++ {
		c := source[i]
		switch state {
		case 1:
			if c == '\n' {
				state = 0
			}
			continue
		case 2:
			if c == '*' && i+1 < len(source) && source[i+1] == '/' {
				state = 0
				i++
			}
			continue
		case '\'', '"', '`':
			if c == '\\' && state != '`' && i+1 < len(source) {
				i++
				continue
			}
			if c == state {
				if i+1 < len(source) && source[i+1] == state {
					i++
					continue
				}
				state = 0
			}
			continue
		}
		if c == '\'' || c == '"' || c == '`' {
			state = c
			continue
		}
		if c == '#' {
			state = 1
			continue
		}
		if c == '-' && i+1 < len(source) && source[i+1] == '-' && (i+2 == len(source) || source[i+2] <= ' ') {
			state = 1
			i++
			continue
		}
		if c == '/' && i+1 < len(source) && source[i+1] == '*' {
			state = 2
			i++
			continue
		}
		if c == ';' {
			statement := strings.TrimSpace(source[start:i])
			if hasSQLCode(statement) {
				statements = append(statements, statement)
			}
			start = i + 1
		}
	}
	if state == '\'' || state == '"' || state == '`' || state == 2 {
		return nil, errors.New("unterminated quote or block comment in migration SQL")
	}
	last := strings.TrimSpace(source[start:])
	if hasSQLCode(last) {
		statements = append(statements, last)
	}
	return statements, nil
}

func hasSQLCode(statement string) bool {
	state := byte(0) // 0 normal, 1 line comment, 2 block comment
	for i := 0; i < len(statement); i++ {
		c := statement[i]
		if state == 1 {
			if c == '\n' {
				state = 0
			}
			continue
		}
		if state == 2 {
			if c == '*' && i+1 < len(statement) && statement[i+1] == '/' {
				state = 0
				i++
			}
			continue
		}
		if c == '#' || (c == '-' && i+1 < len(statement) && statement[i+1] == '-' && (i+2 == len(statement) || statement[i+2] <= ' ')) {
			state = 1
			if c == '-' {
				i++
			}
			continue
		}
		if c == '/' && i+1 < len(statement) && statement[i+1] == '*' {
			state = 2
			i++
			continue
		}
		if c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == ';' {
			continue
		}
		return true
	}
	return false
}

func statusWithConn(ctx context.Context, conn *sql.Conn, all []File) (StatusReport, error) {
	var hasTable int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='schema_migrations'`).Scan(&hasTable); err != nil {
		return StatusReport{}, err
	}
	report := StatusReport{HistoryExists: hasTable > 0, Migrations: make([]StatusEntry, 0, len(all))}
	if hasTable == 0 {
		for _, m := range all {
			report.Migrations = append(report.Migrations, StatusEntry{Version: m.Version, Name: m.Name, ChecksumStatus: "NOT_APPLIED"})
			report.PendingCount++
		}
		return report, nil
	}
	hasChecksums, err := hasColumn(ctx, conn, "schema_migrations", "checksum")
	if err != nil {
		return StatusReport{}, err
	}
	report.HasChecksums = hasChecksums
	type record struct {
		name     string
		at       sql.NullTime
		checksum sql.NullString
	}
	applied := map[int]record{}
	q := `SELECT version,name,applied_at,NULL AS checksum FROM schema_migrations`
	if hasChecksums {
		q = `SELECT version,name,applied_at,checksum FROM schema_migrations`
	}
	rows, err := conn.QueryContext(ctx, q)
	if err != nil {
		return StatusReport{}, err
	}
	for rows.Next() {
		var v int
		var rec record
		if err := rows.Scan(&v, &rec.name, &rec.at, &rec.checksum); err != nil {
			rows.Close()
			return StatusReport{}, err
		}
		applied[v] = rec
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return StatusReport{}, err
	}
	rows.Close()
	known := map[int]bool{}
	for _, m := range all {
		known[m.Version] = true
		rec, ok := applied[m.Version]
		entry := StatusEntry{Version: m.Version, Name: m.Name, Applied: ok, ChecksumStatus: "NOT_APPLIED"}
		if !ok {
			report.PendingCount++
			report.Migrations = append(report.Migrations, entry)
			continue
		}
		report.AppliedCount++
		entry.ChecksumStatus = "UNVERIFIED"
		if rec.at.Valid {
			t := rec.at.Time.UTC()
			entry.AppliedAt = &t
		}
		if rec.name != m.Name {
			entry.Drift = true
			entry.ChecksumStatus = "NAME_MISMATCH"
		} else if rec.checksum.Valid && rec.checksum.String != "" {
			if rec.checksum.String == Checksum(m.SQL) {
				entry.ChecksumStatus = "MATCH"
			} else {
				entry.Drift = true
				entry.ChecksumStatus = "MISMATCH"
			}
		}
		if entry.Drift {
			report.DriftCount++
		}
		report.Migrations = append(report.Migrations, entry)
	}
	for v, rec := range applied {
		if !known[v] {
			e := StatusEntry{Version: v, Name: rec.name, Applied: true, ChecksumStatus: "UNKNOWN_MIGRATION", Drift: true}
			if rec.at.Valid {
				t := rec.at.Time.UTC()
				e.AppliedAt = &t
			}
			report.Migrations = append(report.Migrations, e)
			report.DriftCount++
		}
	}
	sort.Slice(report.Migrations, func(i, j int) bool { return report.Migrations[i].Version < report.Migrations[j].Version })
	return report, nil
}

type migrationDB interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func applyCredentialTokenMigration(ctx context.Context, db migrationDB) error {
	hasToken, err := hasColumn(ctx, db, "credentials", "token")
	if err != nil {
		return err
	}
	hasHash, err := hasColumn(ctx, db, "credentials", "token_hash")
	if err != nil {
		return err
	}
	if !hasToken && !hasHash {
		return fmt.Errorf("credentials table has neither token nor token_hash column")
	}
	if hasToken && !hasHash {
		if _, err := db.ExecContext(ctx, `ALTER TABLE credentials ADD COLUMN token_hash VARCHAR(255) NULL AFTER token`); err != nil {
			return err
		}
		hasHash = true
	}
	if hasHash {
		if hasToken {
			if _, err := db.ExecContext(ctx, `UPDATE credentials SET token_hash=SHA2(token,256) WHERE token_hash IS NULL`); err != nil {
				return err
			}
		} else {
			var nullCount int
			if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM credentials WHERE token_hash IS NULL`).Scan(&nullCount); err != nil {
				return err
			}
			if nullCount > 0 {
				return fmt.Errorf("token_hash contains %d NULL rows but plaintext token column is already gone; manual recovery required", nullCount)
			}
		}
		nullable, err := columnNullable(ctx, db, "credentials", "token_hash")
		if err != nil {
			return err
		}
		if nullable {
			if _, err := db.ExecContext(ctx, `ALTER TABLE credentials MODIFY COLUMN token_hash VARCHAR(255) NOT NULL`); err != nil {
				return err
			}
		}
		index, err := hasIndex(ctx, db, "credentials", "uq_credentials_token_hash")
		if err != nil {
			return err
		}
		if !index {
			if _, err := db.ExecContext(ctx, `ALTER TABLE credentials ADD UNIQUE KEY uq_credentials_token_hash(token_hash)`); err != nil {
				return err
			}
		}
	}
	if hasToken {
		if _, err := db.ExecContext(ctx, `ALTER TABLE credentials DROP COLUMN token`); err != nil {
			return err
		}
	}
	return nil
}
func hasColumn(ctx context.Context, db interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, table, column string) (bool, error) {
	var n int
	err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name=? AND column_name=?`, table, column).Scan(&n)
	return n > 0, err
}
func columnNullable(ctx context.Context, db interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, table, column string) (bool, error) {
	var v string
	err := db.QueryRowContext(ctx, `SELECT is_nullable FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name=? AND column_name=?`, table, column).Scan(&v)
	return strings.EqualFold(v, "YES"), err
}
func hasIndex(ctx context.Context, db interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, table, index string) (bool, error) {
	var n int
	err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name=? AND index_name=?`, table, index).Scan(&n)
	return n > 0, err
}
