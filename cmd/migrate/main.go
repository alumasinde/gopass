package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"

	_ "github.com/go-sql-driver/mysql"
)

const (
	migrationsDir = "migrations"
	tableName     = "schema_migrations"
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

	db, err := openDB()
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := ensureMigrationTable(ctx, db); err != nil {
		log.Fatal(err)
	}

	command := strings.ToLower(strings.TrimSpace(os.Args[1]))

	switch command {
	case "up":
		if err := migrateUp(ctx, db); err != nil {
			log.Fatal(err)
		}

	case "status":
		if err := migrationStatus(ctx, db); err != nil {
			log.Fatal(err)
		}

	case "down":
		if err := migrateDown(ctx, db); err != nil {
			log.Fatal(err)
		}

	default:
		usage()
		os.Exit(1)
	}
}

func openDB() (*sql.DB, error) {
	dsn := os.Getenv("DB_DSN")

	if dsn == "" {
		host := getenv("DB_HOST", "127.0.0.1")
		port := getenv("DB_PORT", "3306")
		user := getenv("DB_USER", "root")
		password := os.Getenv("DB_PASSWORD")
		name := os.Getenv("DB_NAME")

		if name == "" {
			return nil, errors.New("gopass required")
		}

		dsn = fmt.Sprintf(
			"%s:%s@tcp(%s:%s)/%s?parseTime=true&multiStatements=true",
			user,
			password,
			host,
			port,
			name,
		)
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return db, nil
}

func ensureMigrationTable(ctx context.Context, db *sql.DB) error {
	const query = `
CREATE TABLE IF NOT EXISTS schema_migrations (
	version BIGINT UNSIGNED NOT NULL,
	name VARCHAR(255) NOT NULL,
	applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	PRIMARY KEY (version)
) ENGINE=InnoDB;
`

	if _, err := db.ExecContext(ctx, query); err != nil {
		return fmt.Errorf(
			"create schema_migrations table: %w",
			err,
		)
	}

	return nil
}

func migrateUp(ctx context.Context, db *sql.DB) error {
	migrations, err := loadMigrations()
	if err != nil {
		return err
	}

	applied, err := appliedMigrations(ctx, db)
	if err != nil {
		return err
	}

	appliedCount := 0

	for _, migration := range migrations {
		if applied[migration.Version] {
			continue
		}

		fmt.Printf(
			"Applying migration %04d_%s...\n",
			migration.Version,
			migration.Name,
		)

		content, err := os.ReadFile(migration.Path)
		if err != nil {
			return fmt.Errorf(
				"read migration %s: %w",
				migration.Path,
				err,
			)
		}

		// Do NOT wrap the migration in a transaction.
		//
		// MySQL DDL statements such as CREATE DATABASE,
		// CREATE TABLE and USE can implicitly commit and
		// therefore cannot safely be treated as one transaction.
		if _, err := db.ExecContext(ctx, string(content)); err != nil {
			return fmt.Errorf(
				"migration %04d_%s failed: %w",
				migration.Version,
				migration.Name,
				err,
			)
		}

		// A migration is allowed to change database context,
		// so make sure the tracking table exists again before
		// recording the migration.
		if err := ensureMigrationTable(ctx, db); err != nil {
			return fmt.Errorf(
				"ensure migration tracking table after %04d_%s: %w",
				migration.Version,
				migration.Name,
				err,
			)
		}

		_, err = db.ExecContext(
			ctx,
			`INSERT INTO schema_migrations (version, name)
			 VALUES (?, ?)`,
			migration.Version,
			migration.Name,
		)
		if err != nil {
			return fmt.Errorf(
				"record migration %d: %w",
				migration.Version,
				err,
			)
		}

		fmt.Printf(
			"Applied migration %04d_%s\n",
			migration.Version,
			migration.Name,
		)

		appliedCount++
	}

	if appliedCount == 0 {
		fmt.Println("No pending migrations.")
	} else {
		fmt.Printf("Applied %d migration(s).\n", appliedCount)
	}

	return nil
}

func migrateDown(ctx context.Context, db *sql.DB) error {
	migrations, err := loadMigrations()
	if err != nil {
		return err
	}

	applied, err := appliedMigrations(ctx, db)
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

	fmt.Printf(
		"Cannot automatically roll back migration %04d_%s.\n",
		latest.Version,
		latest.Name,
	)

	fmt.Println(
		"Down migrations are not implemented because the project currently uses up-only SQL migrations.",
	)

	return nil
}

func migrationStatus(ctx context.Context, db *sql.DB) error {
	migrations, err := loadMigrations()
	if err != nil {
		return err
	}

	applied, err := appliedMigrations(ctx, db)
	if err != nil {
		return err
	}

	if len(migrations) == 0 {
		fmt.Println("No migration files found.")
		return nil
	}

	for _, migration := range migrations {
		status := "pending"

		if applied[migration.Version] {
			status = "applied"
		}

		fmt.Printf(
			"%04d  %-10s  %s\n",
			migration.Version,
			status,
			migration.Name,
		)
	}

	return nil
}

func loadMigrations() ([]Migration, error) {
	files, err := filepath.Glob(
		filepath.Join(migrationsDir, "*.sql"),
	)
	if err != nil {
		return nil, fmt.Errorf("find migrations: %w", err)
	}

	var migrations []Migration

	for _, path := range files {
		filename := filepath.Base(path)

		version, name, err := parseMigrationFilename(filename)
		if err != nil {
			return nil, err
		}

		migrations = append(migrations, Migration{
			Version: version,
			Name:    name,
			Path:    path,
		})
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	for i := 1; i < len(migrations); i++ {
		if migrations[i-1].Version == migrations[i].Version {
			return nil, fmt.Errorf(
				"duplicate migration version: %d",
				migrations[i].Version,
			)
		}
	}

	return migrations, nil
}

func parseMigrationFilename(filename string) (int, string, error) {
	parts := strings.SplitN(filename, "_", 2)

	if len(parts) != 2 {
		return 0, "", fmt.Errorf(
			"invalid migration filename %q: expected NNNN_name.sql",
			filename,
		)
	}

	version, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, "", fmt.Errorf(
			"invalid migration version in %q: %w",
			filename,
			err,
		)
	}

	name := strings.TrimSuffix(parts[1], ".sql")

	if name == "" {
		return 0, "", fmt.Errorf(
			"invalid migration filename %q: missing name",
			filename,
		)
	}

	return version, name, nil
}

func appliedMigrations(
	ctx context.Context,
	db *sql.DB,
	) (map[int]bool, error) {
	rows, err := db.QueryContext(
		ctx,
		`SELECT version FROM schema_migrations ORDER BY version`,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"load applied migrations: %w",
			err,
		)
	}
	defer rows.Close()

	applied := make(map[int]bool)

	for rows.Next() {
		var version int

		if err := rows.Scan(&version); err != nil {
			return nil, fmt.Errorf(
				"scan migration version: %w",
				err,
			)
		}

		applied[version] = true
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf(
			"iterate migrations: %w",
			err,
		)
	}

	return applied, nil
}

func getenv(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))

	if value == "" {
		return fallback
	}

	return value
}

func usage() {
	fmt.Println(`Usage:
  go run ./cmd/migrate up
  go run ./cmd/migrate status
  go run ./cmd/migrate down`)
}
