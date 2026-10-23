// Command devserver runs the whole backend on your own computer with nothing to install: it starts a private PostgreSQL
// (data kept in ~/.bookdan-dev), applies the migrations, loads the demo hotels, and serves the API on :8080.
//
//	go run ./cmd/devserver
//
// Demo mode is on (every sign-in code is 123456), payments use the fake test page, and the demo hotel owner is
// owner@bookdan.test. This is for development only: it refuses to start if APP_ENV=production.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/bookdan/backend/internal/api"
	"github.com/bookdan/backend/internal/config"
	"github.com/bookdan/backend/internal/db"
	"github.com/bookdan/backend/internal/dbtest"
	"github.com/bookdan/backend/internal/server"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, nil)))
	cfg := config.Load()
	if cfg.IsProduction() {
		slog.Error("devserver is for development only")
		os.Exit(1)
	}
	cfg.DemoMode = true
	cfg.PaymentProvider, cfg.OTPProvider, cfg.PushProvider = "dev", "log", "log"
	// The admin page at /admin: a fixed password and admin email for local use only.
	if cfg.AdminPassword == "" {
		cfg.AdminPassword, cfg.AdminEmails, cfg.AdminOwners = "local-admin-password", []string{"admin@bookdan.test"}, true
	}

	home, _ := os.UserHomeDir()
	slog.Info("starting a private PostgreSQL (the first run downloads it, about 60 MB)")
	url, stopDB, err := dbtest.StartDev(filepath.Join(home, ".bookdan-dev"), 54329)
	if err != nil {
		slog.Error("database", "err", err)
		os.Exit(1)
	}
	defer stopDB()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	pool, err := db.Open(ctx, url)
	if err != nil {
		slog.Error("database", "err", err)
		stopDB()
		os.Exit(1)
	}
	defer pool.Close()
	keys, _ := api.NewKeys(cfg)
	a, err := api.New(ctx, cfg, pool, keys, api.DevProviders(cfg))
	if err != nil {
		slog.Error("api", "err", err)
		stopDB()
		os.Exit(1)
	}
	go a.RunWorkers(ctx)

	srv := &http.Server{Addr: cfg.HTTPAddr, Handler: server.New(cfg, pool, func(r chi.Router) { a.Routes(r) }, a.AdminPage), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		slog.Info("BookDan API is ready", "url", "http://localhost"+cfg.HTTPAddr+"/v1/healthz",
			"guest_sign_in", "any email or +2519… number, code 123456", "hotel_sign_in", "owner@bookdan.test, code 123456", "admin_page", "http://localhost"+cfg.HTTPAddr+"/admin (admin@bookdan.test / local-admin-password)")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server", "err", err)
			cancel()
		}
	}()
	<-ctx.Done()
	shutdown, c2 := context.WithTimeout(context.Background(), 10*time.Second)
	defer c2()
	_ = srv.Shutdown(shutdown)
}
