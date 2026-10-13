package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/alumasinde/gopass/internal/app/config"
	"github.com/alumasinde/gopass/internal/app/database"
	"github.com/alumasinde/gopass/internal/platform/tenancy"
	"github.com/alumasinde/gopass/migrations"
	"github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
	if len(os.Args) > 1 && os.Args[1] != "up" {
		log.Fatal("usage: go run ./cmd/tenant-migrate up")
	}
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	platformDSN, err := mysql.ParseDSN(cfg.PlatformDBDSN)
	if err != nil {
		return fmt.Errorf("invalid platform DSN: %w", err)
	}
	sharedDSN, err := mysql.ParseDSN(cfg.DBDSN)
	if err != nil {
		return fmt.Errorf("invalid shared tenant DSN: %w", err)
	}
	if strings.EqualFold(platformDSN.DBName, sharedDSN.DBName) && strings.EqualFold(platformDSN.Addr, sharedDSN.Addr) && platformDSN.Net == sharedDSN.Net {
		return fmt.Errorf("refusing to migrate tenant schema because PLATFORM_DB_DSN and DB_DSN point to the same database; complete platform database separation/import first")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	platformDB, err := sql.Open("mysql", cfg.PlatformDBDSN)
	if err != nil {
		return fmt.Errorf("open platform database: %w", err)
	}
	defer platformDB.Close()
	sharedDB, err := sql.Open("mysql", cfg.DBDSN)
	if err != nil {
		return fmt.Errorf("open shared tenant database: %w", err)
	}
	defer sharedDB.Close()
	if err = platformDB.PingContext(ctx); err != nil {
		return fmt.Errorf("platform database unavailable: %w", err)
	}
	if err = sharedDB.PingContext(ctx); err != nil {
		return fmt.Errorf("shared tenant database unavailable: %w", err)
	}
	// Always migrate the shared tenant schema, including when no shared-mode
	// tenant is currently active. The runner uses the same advisory lock as the
	// regular CLI and is safe to rerun after a partial MySQL DDL migration.
	if err = migrations.ApplyEmbedded(ctx, sharedDB); err != nil {
		return fmt.Errorf("shared tenant database migration failed: %w", err)
	}
	router := database.NewRouter(platformDB, sharedDB)
	defer router.Close()
	ids, err := router.TenantIDs(ctx)
	if err != nil {
		return fmt.Errorf("list active tenants: %w", err)
	}
	for _, id := range ids {
		db, dbErr := router.DatabaseForTenant(tenancy.With(ctx, id), id)
		if dbErr != nil {
			return fmt.Errorf("resolve tenant %d database: %w", id, dbErr)
		}
		if db == sharedDB {
			continue
		}
		if err = migrations.ApplyEmbedded(ctx, db); err != nil {
			return fmt.Errorf("tenant %d migration failed: %w", id, err)
		}
		fmt.Printf("tenant %d: migrations applied\n", id)
	}
	fmt.Printf("shared tenant database: migrations applied; dedicated tenants checked: %d\n", len(ids))
	return nil
}
