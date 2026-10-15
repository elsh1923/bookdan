// Command migrate manages the database: `migrate up`, `down`, `status`, and `seed` (demo data, never in production).
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver
	"github.com/pressly/goose/v3"

	"github.com/bookdan/backend/db"
	"github.com/bookdan/backend/internal/config"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: migrate up | down | status | seed")
		os.Exit(2)
	}
	cfg := config.Load()
	if cfg.DatabaseURL == "" {
		log.Fatal("DATABASE_URL is not set (put it in backend/.env)")
	}

	conn, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("open: %v", err)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := conn.PingContext(ctx); err != nil {
		log.Fatalf("cannot reach the database: %v", err)
	}

	goose.SetBaseFS(db.Migrations)
	if err := goose.SetDialect("postgres"); err != nil {
		log.Fatal(err)
	}

	switch cmd := os.Args[1]; cmd {
	case "up":
		err = goose.UpContext(ctx, conn, "migrations")
	case "down":
		// One step back. This destroys data: refuse it where real data lives.
		if cfg.IsProduction() {
			log.Fatal("refusing to run down migrations when APP_ENV=production")
		}
		err = goose.DownContext(ctx, conn, "migrations")
	case "status":
		err = goose.StatusContext(ctx, conn, "migrations")
	case "seed":
		if cfg.IsProduction() {
			log.Fatal("refusing to load demo data when APP_ENV=production")
		}
		_, err = conn.ExecContext(ctx, db.DevSeed)
		if err == nil {
			log.Println("demo data loaded (safe to run again)")
		}
	default:
		log.Fatalf("unknown command %q", cmd)
	}
	if err != nil {
		log.Fatalf("migrate %s: %v", os.Args[1], err)
	}
}
