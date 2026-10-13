package migrations

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
)

//go:embed *.sql
var files embed.FS

type File struct {
	Version int
	Name    string
	SQL     string
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
			continue
		}
		v, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}
		b, err := files.ReadFile(e.Name())
		if err != nil {
			return nil, err
		}
		out = append(out, File{Version: v, Name: parts[1], SQL: string(b)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out, nil
}

// ApplyEmbedded applies this build's versioned migrations to a tenant database.
// The caller must configure the MySQL driver for multiStatements. Each migration
// is recorded only after all statements execute successfully.
func ApplyEmbedded(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version BIGINT UNSIGNED NOT NULL, name VARCHAR(255) NOT NULL, applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, PRIMARY KEY(version)) ENGINE=InnoDB`); err != nil {
		return fmt.Errorf("ensure schema_migrations: %w", err)
	}
	all, err := Embedded()
	if err != nil {
		return err
	}
	for _, m := range all {
		var exists int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE version=?`, m.Version).Scan(&exists); err != nil {
			return err
		}
		if exists != 0 {
			continue
		}
		if m.Version == 6 {
			if err := applyCredentialTokenMigration(ctx, db); err != nil {
				return fmt.Errorf("migration %04d_%s failed: %w", m.Version, m.Name, err)
			}
		} else if _, err := db.ExecContext(ctx, m.SQL); err != nil {
			return fmt.Errorf("migration %04d_%s failed: %w", m.Version, m.Name, err)
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO schema_migrations(version,name) VALUES(?,?)`, m.Version, m.Name); err != nil {
			return fmt.Errorf("record migration %04d: %w", m.Version, err)
		}
	}
	return nil
}

// applyCredentialTokenMigration makes migration 0006 recoverable after an
// interrupted DDL sequence. It converges to the same final schema as the SQL
// migration while refusing to fabricate hashes if the plaintext token is gone.
func applyCredentialTokenMigration(ctx context.Context, db *sql.DB) error {
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
				return fmt.Errorf("token_hash contains %d NULL rows but the plaintext token column is already gone; manual recovery is required", nullCount)
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
		hasIndex, err := hasIndex(ctx, db, "credentials", "uq_credentials_token_hash")
		if err != nil {
			return err
		}
		if !hasIndex {
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
func hasColumn(ctx context.Context, db *sql.DB, table, column string) (bool, error) {
	var n int
	err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name=? AND column_name=?`, table, column).Scan(&n)
	return n > 0, err
}
func columnNullable(ctx context.Context, db *sql.DB, table, column string) (bool, error) {
	var v string
	err := db.QueryRowContext(ctx, `SELECT is_nullable FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name=? AND column_name=?`, table, column).Scan(&v)
	return strings.EqualFold(v, "YES"), err
}
func hasIndex(ctx context.Context, db *sql.DB, table, index string) (bool, error) {
	var n int
	err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name=? AND index_name=?`, table, index).Scan(&n)
	return n > 0, err
}
