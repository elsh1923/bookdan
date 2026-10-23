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
		fmt.Fprintln(os.Stderr, "usage: migrate up | down | status | seed | reset [--yes]")
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
	case "reset":
		err = reset(ctx, conn, cfg, len(os.Args) > 2 && os.Args[2] == "--yes")
	default:
		log.Fatalf("unknown command %q", cmd)
	}
	if err != nil {
		log.Fatalf("migrate %s: %v", os.Args[1], err)
	}
}

// reset puts a DEMO database back to a clean start: every table is rebuilt and the demo hotels and demo owner are loaded again.
// The database refuses to delete money and booking history one row at a time (that is the point of it), so wiping test
// bookings means rebuilding. Without --yes it only shows what would be lost.
func reset(ctx context.Context, conn *sql.DB, cfg config.Config, confirmed bool) error {
	if cfg.IsProduction() {
		return fmt.Errorf("refusing to reset when APP_ENV=production")
	}
	counts := []struct{ label, query string }{
		{"users", "SELECT count(*) FROM app.users"},
		{"saved guest profiles (with encrypted IDs)", "SELECT count(*) FROM app.guest_profiles"},
		{"bookings", "SELECT count(*) FROM app.bookings"},
		{"payments", "SELECT count(*) FROM app.payments"},
		{"ledger transactions", "SELECT count(*) FROM app.ledger_transactions"},
		{"phones registered for alerts", "SELECT count(*) FROM app.devices"},
		{"audit log entries", "SELECT count(*) FROM app.audit_log"},
	}
	fmt.Println("This will DELETE everything in the database and reload the demo hotels:")
	for _, c := range counts {
		var n int
		if err := conn.QueryRowContext(ctx, c.query).Scan(&n); err != nil {
			return fmt.Errorf("count %s: %w", c.label, err)
		}
		fmt.Printf("  %-44s %d\n", c.label, n)
	}
	// Money that really moved must never be wiped by a convenience command.
	var real int
	if err := conn.QueryRowContext(ctx, "SELECT count(*) FROM app.payments WHERE provider <> 'mock'").Scan(&real); err != nil {
		return err
	}
	if real > 0 {
		return fmt.Errorf("refusing to reset: %d payment(s) came from a real gateway", real)
	}
	if !confirmed {
		fmt.Println("\nNothing was changed. To do it, run: go run ./cmd/migrate reset --yes")
		return nil
	}
	if _, err := conn.ExecContext(ctx, "DROP SCHEMA IF EXISTS app CASCADE; DROP TABLE IF EXISTS public.goose_db_version"); err != nil {
		return fmt.Errorf("drop: %w", err)
	}
	if err := goose.UpContext(ctx, conn, "migrations"); err != nil {
		return fmt.Errorf("rebuild: %w", err)
	}
	if _, err := conn.ExecContext(ctx, db.DevSeed); err != nil {
		return fmt.Errorf("demo data: %w", err)
	}
	fmt.Println("\nDone: a clean database with the 8 demo hotels and the demo owner (owner@bookdan.test).")
	fmt.Println("Phones will need to sign in again. If the live server acts oddly afterwards, restart it on Render.")
	return nil
}
