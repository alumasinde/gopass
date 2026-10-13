package platformmigrations

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

const lockName = "gopass_platform_schema_migrate"

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
			return nil, fmt.Errorf("invalid platform migration filename %q", e.Name())
		}
		v, err := strconv.Atoi(parts[0])
		if err != nil || v < 1 {
			return nil, fmt.Errorf("invalid platform migration version %q", e.Name())
		}
		b, err := files.ReadFile(e.Name())
		if err != nil {
			return nil, err
		}
		out = append(out, File{Version: v, Name: parts[1], SQL: strings.TrimPrefix(string(b), "\ufeff")})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	for i := 1; i < len(out); i++ {
		if out[i-1].Version == out[i].Version {
			return nil, fmt.Errorf("duplicate platform migration version %d", out[i].Version)
		}
	}
	return out, nil
}
func checksum(s string) string {
	s = strings.ReplaceAll(strings.TrimPrefix(s, "\ufeff"), "\r\n", "\n")
	v := sha256.Sum256([]byte(s))
	return hex.EncodeToString(v[:])
}
func Status(ctx context.Context, db *sql.DB) (StatusReport, error) {
	all, err := Embedded()
	if err != nil {
		return StatusReport{}, err
	}
	report := StatusReport{HistoryExists: true, Migrations: make([]StatusEntry, 0, len(all))}
	var n int
	if err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='platform_schema_migrations'`).Scan(&n); err != nil {
		return report, err
	}
	if n == 0 {
		report.HistoryExists = false
		for _, m := range all {
			report.Migrations = append(report.Migrations, StatusEntry{Version: m.Version, Name: m.Name, ChecksumStatus: "NOT_APPLIED"})
			report.PendingCount++
		}
		return report, nil
	}
	rows, err := db.QueryContext(ctx, `SELECT version,name,applied_at,checksum FROM platform_schema_migrations`)
	if err != nil {
		return report, err
	}
	type record struct {
		name string
		at   sql.NullTime
		sum  sql.NullString
	}
	applied := map[int]record{}
	for rows.Next() {
		var v int
		var r record
		if err = rows.Scan(&v, &r.name, &r.at, &r.sum); err != nil {
			rows.Close()
			return report, err
		}
		applied[v] = r
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return report, err
	}
	rows.Close()
	report.HasChecksums = true
	known := map[int]bool{}
	for _, m := range all {
		known[m.Version] = true
		r, ok := applied[m.Version]
		e := StatusEntry{Version: m.Version, Name: m.Name, Applied: ok, ChecksumStatus: "NOT_APPLIED"}
		if !ok {
			report.PendingCount++
			report.Migrations = append(report.Migrations, e)
			continue
		}
		report.AppliedCount++
		e.ChecksumStatus = "UNVERIFIED"
		if r.at.Valid {
			t := r.at.Time.UTC()
			e.AppliedAt = &t
		}
		if r.name != m.Name {
			e.Drift = true
			e.ChecksumStatus = "NAME_MISMATCH"
		} else if r.sum.Valid && r.sum.String != "" {
			if r.sum.String == checksum(m.SQL) {
				e.ChecksumStatus = "MATCH"
			} else {
				e.Drift = true
				e.ChecksumStatus = "MISMATCH"
			}
		}
		if e.Drift {
			report.DriftCount++
		}
		report.Migrations = append(report.Migrations, e)
	}
	for v, r := range applied {
		if known[v] {
			continue
		}
		e := StatusEntry{Version: v, Name: r.name, Applied: true, ChecksumStatus: "UNKNOWN_MIGRATION", Drift: true}
		if r.at.Valid {
			t := r.at.Time.UTC()
			e.AppliedAt = &t
		}
		report.Migrations = append(report.Migrations, e)
		report.DriftCount++
	}
	sort.Slice(report.Migrations, func(i, j int) bool { return report.Migrations[i].Version < report.Migrations[j].Version })
	return report, nil
}
func ApplyEmbedded(ctx context.Context, db *sql.DB) error {
	all, err := Embedded()
	if err != nil {
		return err
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	var got sql.NullInt64
	if err = conn.QueryRowContext(ctx, `SELECT GET_LOCK(?,30)`, lockName).Scan(&got); err != nil {
		return err
	}
	if !got.Valid || got.Int64 != 1 {
		return errors.New("another platform migration is running")
	}
	defer func() {
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = conn.ExecContext(c, `SELECT RELEASE_LOCK(?)`, lockName)
	}()
	if _, err = conn.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS platform_schema_migrations(version BIGINT UNSIGNED NOT NULL PRIMARY KEY,name VARCHAR(255) NOT NULL,checksum CHAR(64) NOT NULL,applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP) ENGINE=InnoDB`); err != nil {
		return err
	}
	report, err := Status(ctx, db)
	if err != nil {
		return err
	}
	if report.DriftCount > 0 {
		return fmt.Errorf("platform migration history has %d drift/unknown entries", report.DriftCount)
	}
	for _, m := range all {
		var exists int
		if err = conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM platform_schema_migrations WHERE version=?`, m.Version).Scan(&exists); err != nil {
			return err
		}
		if exists > 0 {
			continue
		}
		statements, splitErr := splitSQLStatements(m.SQL)
		if splitErr != nil {
			return fmt.Errorf("platform migration %04d_%s SQL parsing failed: %w", m.Version, m.Name, splitErr)
		}
		for i, stmt := range statements {
			if _, err = conn.ExecContext(ctx, stmt); err != nil {
				return fmt.Errorf("platform migration %04d_%s statement %d failed: %w; MySQL DDL may have partially applied, fix the cause and retry", m.Version, m.Name, i+1, err)
			}
		}
		if _, err = conn.ExecContext(ctx, `INSERT INTO platform_schema_migrations(version,name,checksum) VALUES(?,?,?)`, m.Version, m.Name, checksum(m.SQL)); err != nil {
			return err
		}
	}
	return nil
}

// splitSQLStatements understands MySQL quoted strings, identifiers and comments,
// so semicolons inside literals/comments do not split a statement.
func splitSQLStatements(source string) ([]string, error) {
	var statements []string
	start := 0
	state := byte(0) // 0 normal, quotes/backticks, 1 line comment, 2 block comment
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
	state := byte(0)
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
