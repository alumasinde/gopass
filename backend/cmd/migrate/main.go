package main

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"

	"github.com/alumasinde/gopass/internal/app/config"
)

const (
	defaultMigrationsDir = "migrations"
	lockName             = "gopass_schema_migrate"
	runTimeout           = 10 * time.Minute

	mysqlErrUnknownDatabase = 1049
	mysqlErrAccessDenied    = 1044
	mysqlErrDBAccessDenied  = 1045
	mysqlErrCreateDenied    = 1227
)

type Migration struct {
	Version int
	Name    string
	Path    string
}

func main() {
	_ = godotenv.Load()

	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	command := strings.ToLower(strings.TrimSpace(os.Args[1]))
	if command != "up" && command != "status" && command != "down" {
		usage()
		os.Exit(1)
	}

	if err := run(command); err != nil {
		log.Fatal(err)
	}
}

func run(command string) error {
	ctx, cancel := context.WithTimeout(context.Background(), runTimeout)
	defer cancel()

	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	migrations, err := loadMigrations()
	if err != nil {
		return err
	}

	switch command {
	case "up":
		// The database named in the DSN may not exist yet on a fresh machine.
		// Create it first, otherwise the very first connection fails with
		// "Unknown database" before any migration gets a chance to run.
		if err := ensureDatabase(ctx, cfg); err != nil {
			return err
		}
	case "status":
		state, err := probeDatabase(ctx, cfg)
		if err != nil {
			return err
		}
		if state == dbDenied {
			return fmt.Errorf("user %q cannot access database %q (it may not exist, or the user lacks privileges)", cfg.User, cfg.DBName)
		}
		if state == dbMissing {
			fmt.Printf("Database %q does not exist yet - all migrations are pending.\n", cfg.DBName)
			for _, m := range migrations {
				fmt.Printf("%04d  %-10s  %s\n", m.Version, "pending", m.Name)
			}
			return nil
		}
	}

	connector, err := mysql.NewConnector(cfg)
	if err != nil {
		return fmt.Errorf("create database connector: %w", err)
	}

	db := sql.OpenDB(connector)
	defer db.Close()

	// One dedicated connection: session state (GET_LOCK, current database)
	// must stay on the same connection for the whole run.
	conn, err := db.Conn(ctx)
	if err != nil {
		return explainConnectError(cfg, err)
	}
	defer conn.Close()

	switch command {
	case "up":
		return migrateUp(ctx, conn, migrations)
	case "status":
		return migrationStatus(ctx, conn, migrations)
	default:
		return migrateDown(ctx, conn, migrations)
	}
}

// loadConfig resolves the DSN and forces the driver options the migrator needs.
func loadConfig() (*mysql.Config, error) {
	dsn := config.DSN()
	if dsn == "" {
		return nil, errors.New("database not configured: set DB_DSN, or DB_NAME (plus DB_HOST/DB_PORT/DB_USER/DB_PASSWORD) in .env")
	}

	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return nil, fmt.Errorf("invalid DB_DSN: %w", err)
	}
	if cfg.DBName == "" {
		return nil, errors.New("DB_DSN must include a database name, e.g. user:pass@tcp(127.0.0.1:3306)/gopass")
	}

	// Migration files contain many statements. Without multiStatements the
	// driver sends them as one statement and MySQL answers with error 1064
	// (syntax error near the second statement). Force it on regardless of
	// what the user's DSN says.
	cfg.MultiStatements = true
	cfg.ParseTime = true

	return cfg, nil
}

func connectorFor(cfg *mysql.Config) (driver.Connector, error) {
	c, err := mysql.NewConnector(cfg)
	if err != nil {
		return nil, fmt.Errorf("create database connector: %w", err)
	}
	return c, nil
}

type dbState int

const (
	dbExists dbState = iota
	dbMissing
	// dbDenied: the server refused access to the named database. MySQL
	// answers 1044 (not 1049) for a database that does not exist when the
	// user has no global privileges, so "denied" can also mean "missing".
	dbDenied
)

func probeDatabase(ctx context.Context, cfg *mysql.Config) (dbState, error) {
	connector, err := connectorFor(cfg)
	if err != nil {
		return dbMissing, err
	}

	db := sql.OpenDB(connector)
	defer db.Close()

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	err = db.PingContext(pingCtx)
	switch {
	case err == nil:
		return dbExists, nil
	case isMySQLError(err, mysqlErrUnknownDatabase):
		return dbMissing, nil
	case isMySQLError(err, mysqlErrAccessDenied):
		return dbDenied, nil
	default:
		return dbMissing, explainConnectError(cfg, err)
	}
}

func ensureDatabase(ctx context.Context, cfg *mysql.Config) error {
	state, err := probeDatabase(ctx, cfg)
	if err != nil {
		return err
	}
	if state == dbExists {
		return nil
	}

	// Connect to the server without selecting a database.
	server := cfg.Clone()
	server.DBName = ""

	connector, err := connectorFor(server)
	if err != nil {
		return err
	}

	db := sql.OpenDB(connector)
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return explainConnectError(server, err)
	}

	fmt.Printf("Database %q does not exist - creating it.\n", cfg.DBName)

	stmt := fmt.Sprintf(
		"CREATE DATABASE IF NOT EXISTS %s CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci",
		quoteIdent(cfg.DBName),
	)
	if _, err := db.ExecContext(ctx, stmt); err != nil {
		if isMySQLError(err, mysqlErrAccessDenied) ||
			isMySQLError(err, mysqlErrDBAccessDenied) ||
			isMySQLError(err, mysqlErrCreateDenied) {
			return fmt.Errorf(
				"database %q does not exist and user %q may not create it: %w\n"+
					"Create it once as an admin, then re-run:\n"+
					"  CREATE DATABASE %s CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;\n"+
					"  GRANT ALL ON %s.* TO '%s'@'%%';",
				cfg.DBName, cfg.User, err, quoteIdent(cfg.DBName), quoteIdent(cfg.DBName), cfg.User,
			)
		}
		return fmt.Errorf("create database %q: %w", cfg.DBName, err)
	}

	return nil
}

func ensureMigrationTable(ctx context.Context, conn *sql.Conn) error {
	const query = `
CREATE TABLE IF NOT EXISTS schema_migrations (
	version BIGINT UNSIGNED NOT NULL,
	name VARCHAR(255) NOT NULL,
	applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	PRIMARY KEY (version)
) ENGINE=InnoDB`

	if _, err := conn.ExecContext(ctx, query); err != nil {
		return fmt.Errorf("create schema_migrations table: %w", err)
	}
	return nil
}

// acquireLock stops two `migrate up` runs (e.g. two deploy targets) from
// applying the same migration at the same time.
func acquireLock(ctx context.Context, conn *sql.Conn) (func(), error) {
	var got sql.NullInt64
	if err := conn.QueryRowContext(ctx, `SELECT GET_LOCK(?, 30)`, lockName).Scan(&got); err != nil {
		return nil, fmt.Errorf("acquire migration lock: %w", err)
	}
	if !got.Valid || got.Int64 != 1 {
		return nil, errors.New("another migration run is in progress (could not get lock within 30s)")
	}
	return func() {
		rctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = conn.ExecContext(rctx, `DO RELEASE_LOCK(?)`, lockName)
	}, nil
}

func migrateUp(ctx context.Context, conn *sql.Conn, migrations []Migration) error {
	release, err := acquireLock(ctx, conn)
	if err != nil {
		return err
	}
	defer release()

	if err := ensureMigrationTable(ctx, conn); err != nil {
		return err
	}

	applied, err := appliedMigrations(ctx, conn)
	if err != nil {
		return err
	}

	appliedCount := 0

	for _, migration := range migrations {
		if applied[migration.Version] {
			continue
		}

		fmt.Printf("Applying migration %04d_%s...\n", migration.Version, migration.Name)

		content, err := os.ReadFile(migration.Path)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", migration.Path, err)
		}

		// Windows editors often save SQL with a UTF-8 BOM, which MySQL
		// rejects as a syntax error at the very first statement.
		content = bytes.TrimPrefix(content, []byte("\xef\xbb\xbf"))

		if len(bytes.TrimSpace(content)) == 0 {
			return fmt.Errorf("migration %s is empty", migration.Path)
		}

		// Not wrapped in a transaction on purpose: MySQL DDL implicitly
		// commits. Migrations are therefore written to be re-runnable
		// (IF NOT EXISTS / guarded ALTERs / INSERT IGNORE).
		if _, err := conn.ExecContext(ctx, string(content)); err != nil {
			return fmt.Errorf(
				"migration %04d_%s failed: %w\n"+
					"MySQL DDL is not transactional, so part of this file may already be applied.\n"+
					"The migration files are re-runnable: fix the cause above and run `migrate up` again",
				migration.Version, migration.Name, err,
			)
		}

		if _, err := conn.ExecContext(
			ctx,
			`INSERT INTO schema_migrations (version, name) VALUES (?, ?)`,
			migration.Version, migration.Name,
		); err != nil {
			return fmt.Errorf("record migration %d: %w", migration.Version, err)
		}

		fmt.Printf("Applied migration %04d_%s\n", migration.Version, migration.Name)
		appliedCount++
	}

	if appliedCount == 0 {
		fmt.Println("No pending migrations.")
	} else {
		fmt.Printf("Applied %d migration(s).\n", appliedCount)
	}

	return nil
}

func migrateDown(ctx context.Context, conn *sql.Conn, migrations []Migration) error {
	if err := ensureMigrationTable(ctx, conn); err != nil {
		return err
	}

	applied, err := appliedMigrations(ctx, conn)
	if err != nil {
		return err
	}

	var latest *Migration
	for i := len(migrations) - 1; i >= 0; i-- {
		if applied[migrations[i].Version] {
			latest = &migrations[i]
			break
		}
	}

	if latest == nil {
		fmt.Println("No applied migrations.")
		return nil
	}

	return fmt.Errorf(
		"cannot automatically roll back %04d_%s: down migrations are not implemented (up-only SQL migrations)",
		latest.Version, latest.Name,
	)
}

func migrationStatus(ctx context.Context, conn *sql.Conn, migrations []Migration) error {
	if err := ensureMigrationTable(ctx, conn); err != nil {
		return err
	}

	applied, err := appliedMigrations(ctx, conn)
	if err != nil {
		return err
	}

	known := make(map[int]bool, len(migrations))

	for _, migration := range migrations {
		known[migration.Version] = true

		status := "pending"
		if applied[migration.Version] {
			status = "applied"
		}

		fmt.Printf("%04d  %-10s  %s\n", migration.Version, status, migration.Name)
	}

	// A version recorded in the database with no file on disk usually means
	// the wrong branch / an old checkout is being run.
	var orphans []int
	for v := range applied {
		if !known[v] {
			orphans = append(orphans, v)
		}
	}
	sort.Ints(orphans)
	for _, v := range orphans {
		fmt.Printf("%04d  %-10s  (recorded in database but no matching file)\n", v, "missing")
	}

	return nil
}

func migrationsDir() string {
	if v := strings.TrimSpace(os.Getenv("MIGRATIONS_DIR")); v != "" {
		return v
	}
	return defaultMigrationsDir
}

func loadMigrations() ([]Migration, error) {
	dir := migrationsDir()

	files, err := filepath.Glob(filepath.Join(dir, "*.sql"))
	if err != nil {
		return nil, fmt.Errorf("find migrations: %w", err)
	}

	if len(files) == 0 {
		abs, _ := filepath.Abs(dir)
		// Previously this silently reported "No pending migrations" when the
		// command was run from the wrong directory.
		return nil, fmt.Errorf(
			"no .sql migration files found in %s - run from the project root or set MIGRATIONS_DIR",
			abs,
		)
	}

	var migrations []Migration

	for _, path := range files {
		filename := filepath.Base(path)

		version, name, err := parseMigrationFilename(filename)
		if err != nil {
			return nil, err
		}

		migrations = append(migrations, Migration{Version: version, Name: name, Path: path})
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	for i := 1; i < len(migrations); i++ {
		if migrations[i-1].Version == migrations[i].Version {
			return nil, fmt.Errorf("duplicate migration version: %d", migrations[i].Version)
		}
	}

	return migrations, nil
}

func parseMigrationFilename(filename string) (int, string, error) {
	parts := strings.SplitN(filename, "_", 2)

	if len(parts) != 2 {
		return 0, "", fmt.Errorf("invalid migration filename %q: expected NNNN_name.sql", filename)
	}

	version, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, "", fmt.Errorf("invalid migration version in %q: %w", filename, err)
	}

	name := strings.TrimSuffix(parts[1], ".sql")
	if name == "" {
		return 0, "", fmt.Errorf("invalid migration filename %q: missing name", filename)
	}

	return version, name, nil
}

func appliedMigrations(ctx context.Context, conn *sql.Conn) (map[int]bool, error) {
	rows, err := conn.QueryContext(ctx, `SELECT version FROM schema_migrations ORDER BY version`)
	if err != nil {
		return nil, fmt.Errorf("load applied migrations: %w", err)
	}
	defer rows.Close()

	applied := make(map[int]bool)

	for rows.Next() {
		var version int
		if err := rows.Scan(&version); err != nil {
			return nil, fmt.Errorf("scan migration version: %w", err)
		}
		applied[version] = true
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate migrations: %w", err)
	}

	return applied, nil
}

func isMySQLError(err error, number uint16) bool {
	var me *mysql.MySQLError
	return errors.As(err, &me) && me.Number == number
}

func explainConnectError(cfg *mysql.Config, err error) error {
	switch {
	case isMySQLError(err, mysqlErrDBAccessDenied):
		return fmt.Errorf("access denied for user %q (check DB_DSN user/password): %w", cfg.User, err)
	case isMySQLError(err, mysqlErrAccessDenied):
		return fmt.Errorf("user %q has no access to database %q: %w", cfg.User, cfg.DBName, err)
	}

	var me *mysql.MySQLError
	if errors.As(err, &me) {
		return fmt.Errorf("database error: %w", err)
	}

	return fmt.Errorf("cannot connect to MySQL at %s (is the server running?): %w", cfg.Addr, err)
}

func quoteIdent(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}

func usage() {
	fmt.Println(`Usage:
  go run ./cmd/migrate up       apply pending migrations (creates the database if missing)
  go run ./cmd/migrate status   show applied / pending migrations
  go run ./cmd/migrate down     not implemented (up-only migrations)

Env: DB_DSN, or DB_NAME + DB_HOST/DB_PORT/DB_USER/DB_PASSWORD; optional MIGRATIONS_DIR.`)
}
