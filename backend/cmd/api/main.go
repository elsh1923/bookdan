package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/bookdan/backend/internal/api"
	"github.com/bookdan/backend/internal/config"
	"github.com/bookdan/backend/internal/db"
	"github.com/bookdan/backend/internal/ops"
	"github.com/bookdan/backend/internal/server"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		slog.Error("invalid configuration", "err", err)
		os.Exit(1)
	}

	if cfg.DemoMode {
		slog.Warn("DEMO_MODE is on: every sign-in code is 123456")
	}

	// The database is optional in development so the API can start before it is set up;
	// /v1/readyz reports 503 until it is reachable, and the endpoints answer 501 until then. Production requires it.
	var pinger server.Pinger
	var mount, mountRoot func(chi.Router)
	workers := func(context.Context) {}
	if cfg.DatabaseURL != "" {
		pool, err := db.Open(context.Background(), cfg.DatabaseURL)
		if err != nil {
			slog.Error("database", "err", err)
			os.Exit(1)
		}
		defer pool.Close()
		pinger = pool

		keys, err := api.NewKeys(cfg)
		if err != nil {
			slog.Error("keys", "err", err)
			os.Exit(1)
		}
		providers, err := api.ProvidersFor(cfg)
		if err != nil {
			slog.Error("integrations", "err", err)
			os.Exit(1)
		}
		a, err := api.New(context.Background(), cfg, pool, keys, providers)
		if err != nil {
			slog.Error("api", "err", err)
			os.Exit(1)
		}
		mount, mountRoot, workers = a.Routes, a.AdminPage, a.RunWorkers
		// Where the operators hear about a panic, a burst of server errors or a refund that cannot be paid.
		var senders []ops.Sender
		if cfg.AlertWebhookURL != "" {
			senders = append(senders, ops.Webhook(cfg.AlertWebhookURL))
		}
		to := cfg.AlertEmails
		if len(to) == 0 {
			to = cfg.AdminEmails
		}
		if len(to) > 0 && cfg.EmailProvider != "log" && cfg.EmailProvider != "" {
			senders = append(senders, ops.Mail(providers.Mail.SendMail, to))
		}
		ops.Configure(senders...)
		if len(senders) == 0 && cfg.IsProduction() {
			slog.Warn("no alert destination: set ALERT_WEBHOOK_URL (Slack, Discord or Teams) or ALERT_EMAIL, so you hear about problems before your users do")
		}
		if cfg.PaymentProvider == "dev" {
			slog.Warn("PAYMENT_PROVIDER=dev: payments use a fake test page; no real money moves")
		}
	} else {
		slog.Warn("DATABASE_URL is not set; running without a database")
	}

	background, stopWorkers := context.WithCancel(context.Background())
	defer stopWorkers()
	go workers(background)

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           server.New(cfg, pinger, mount, mountRoot),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		slog.Info("api listening", "addr", cfg.HTTPAddr, "env", cfg.Env)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server", "err", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("shutdown", "err", err)
	}
}
