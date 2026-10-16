// Package server builds the HTTP router. Routes under /v1 follow the mobile contract
// (see api/openapi.yaml); anything not built yet answers 501 with a stable error code.
package server

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/bookdan/backend/internal/config"
	"github.com/bookdan/backend/internal/httpx"
)

// Pinger is satisfied by the pgx pool; nil means no database is configured.
type Pinger interface {
	Ping(ctx context.Context) error
}

func New(cfg config.Config, db Pinger) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, requestLog, middleware.Recoverer)

	r.Route("/v1", func(r chi.Router) {
		r.Get("/healthz", health(cfg))
		r.Get("/readyz", ready(db))

		// Guest app
		r.Post("/auth/otp/request", notImplemented)
		r.Post("/auth/otp/verify", notImplemented)
		r.Get("/search", notImplemented)
		r.Get("/properties/{id}", notImplemented)
		r.Post("/quotes", notImplemented)
		r.Post("/bookings", notImplemented)
		r.Get("/bookings", notImplemented)
		r.Get("/bookings/{id}", notImplemented)
		r.Post("/bookings/{id}/pay", notImplemented)
		r.Get("/bookings/{id}/voucher", notImplemented)
		r.Get("/me/profile", notImplemented)
		r.Put("/me/profile", notImplemented)
		r.Delete("/me/profile", notImplemented)

		// Partner app
		r.Route("/partner", func(r chi.Router) {
			r.Put("/devices/push-token", notImplemented)
			r.Post("/bookings/{id}/guest/reveal", notImplemented)
			r.Get("/bookings", notImplemented)
			r.Post("/bookings/{id}/accept", notImplemented)
			r.Post("/bookings/{id}/decline", notImplemented)
			r.Get("/rooms", notImplemented)
			r.Post("/sync/push", notImplemented)
			r.Get("/sync/pull", notImplemented)
			r.Post("/checkin", notImplemented)
			r.Get("/statements", notImplemented)
		})
	})
	return r
}

// requestLog records method, path, status and duration. It deliberately logs no headers, query strings or
// bodies, so tokens, phone numbers, OTPs and PINs can never reach the logs.
func requestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		slog.Info("request",
			"request_id", middleware.GetReqID(r.Context()),
			"method", r.Method,
			"path", r.URL.Path,
			"status", ww.Status(),
			"ms", time.Since(start).Milliseconds(),
		)
	})
}

// health is liveness: the process is up. It never touches the database.
func health(cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		httpx.JSON(w, http.StatusOK, struct {
			httpx.Meta
			Status string `json:"status"`
			Env    string `json:"env"`
		}{httpx.NowMeta(), "ok", cfg.Env})
	}
}

// ready is readiness: the service can do useful work, so the database must answer.
func ready(db Pinger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if db == nil {
			httpx.Error(w, r, http.StatusServiceUnavailable, httpx.CodeUnavailable, "Database is not configured", nil)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := db.Ping(ctx); err != nil {
			slog.Error("readiness: database ping failed", "err", err)
			httpx.Error(w, r, http.StatusServiceUnavailable, httpx.CodeUnavailable, "Database is not reachable", nil)
			return
		}
		httpx.JSON(w, http.StatusOK, struct {
			httpx.Meta
			Status string `json:"status"`
		}{httpx.NowMeta(), "ready"})
	}
}

func notImplemented(w http.ResponseWriter, r *http.Request) {
	httpx.Error(w, r, http.StatusNotImplemented, httpx.CodeNotImplemented, "This endpoint is not built yet", nil)
}
