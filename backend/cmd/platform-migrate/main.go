package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/alumasinde/gopass/internal/app/config"
	"github.com/alumasinde/gopass/internal/app/database"
	platformmigrations "github.com/alumasinde/gopass/platform_migrations"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/platform-migrate up|status")
		os.Exit(2)
	}
	cmd := strings.ToLower(strings.TrimSpace(os.Args[1]))
	if cmd != "up" && cmd != "status" {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/platform-migrate up|status")
		os.Exit(2)
	}
	dsn := config.PlatformDSN()
	if dsn == "" {
		log.Fatal("PLATFORM_DB_DSN is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	if cmd == "up" {
		if err = platformmigrations.ApplyEmbedded(ctx, db); err != nil {
			log.Fatal(err)
		}
	}
	report, err := platformmigrations.Status(ctx, db)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Platform database migration status: applied=%d pending=%d drift=%d\n", report.AppliedCount, report.PendingCount, report.DriftCount)
	for _, m := range report.Migrations {
		state := "pending"
		if m.Applied {
			state = "applied"
		}
		fmt.Printf("%04d %-8s %-28s %s\n", m.Version, state, m.Name, m.ChecksumStatus)
	}
	if cmd == "up" && report.PendingCount > 0 {
		log.Fatal("platform migrations remain pending")
	}
}
