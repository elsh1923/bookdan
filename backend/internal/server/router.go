// Package server builds the HTTP router. Routes under /v1 follow the mobile contract
// (see api/openapi.yaml); anything not built yet answers 501 with a stable error code.
package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/bookdan/backend/internal/config"
	"github.com/bookdan/backend/internal/httpx"
	"github.com/bookdan/backend/internal/ops"
)

// Pinger is satisfied by the pgx pool; nil means no database is configured.
type Pinger interface {
	Ping(ctx context.Context) error
}

// New builds the router. mount registers the real endpoints (see internal/api); with nil, every contract route answers 501.
func New(cfg config.Config, db Pinger, mount func(chi.Router), mountRoot ...func(chi.Router)) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, requestLog, recoverAndAlert, securityHeaders(cfg), cors(cfg), rateLimit(cfg))

	for _, m := range mountRoot {
		if m != nil {
			m(r)
		}
	}

	r.Route("/v1", func(r chi.Router) {
		r.Get("/healthz", health(cfg))
		r.Get("/readyz", ready(db))

		if mount != nil {
			mount(r)
		} else {
			for _, route := range []struct{ method, path string }{
				{"POST", "/auth/otp/request"}, {"POST", "/auth/otp/verify"}, {"POST", "/auth/refresh"}, {"POST", "/auth/logout"},
				{"GET", "/search"}, {"GET", "/suggest"}, {"GET", "/properties/{id}"}, {"POST", "/quotes"}, {"POST", "/bookings"}, {"GET", "/bookings"},
				{"GET", "/bookings/{id}"}, {"POST", "/bookings/{id}/pay"}, {"POST", "/bookings/{id}/cancel"}, {"POST", "/bookings/{id}/review"}, {"GET", "/bookings/{id}/voucher"},
				{"GET", "/me/profile"}, {"PUT", "/me/profile"}, {"DELETE", "/me/profile"},
				{"PUT", "/partner/devices/push-token"}, {"POST", "/partner/bookings/{id}/guest/reveal"}, {"GET", "/partner/bookings"},
				{"POST", "/partner/bookings/{id}/accept"}, {"POST", "/partner/bookings/{id}/decline"},
				{"POST", "/partner/bookings/{id}/check-in"}, {"POST", "/partner/bookings/{id}/no-show"}, {"POST", "/partner/checkin/lookup"}, {"GET", "/partner/earnings"},
				{"GET", "/partner/rooms"}, {"GET", "/partner/rooms/blocks"}, {"POST", "/partner/rooms/blocks"}, {"DELETE", "/partner/rooms/blocks/{id}"},
			} {
				r.MethodFunc(route.method, route.path, notImplemented)
			}
		}
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
		if ww.Status() >= 500 {
			errorBurst.note(routeOf(r), r.Method, middleware.GetReqID(r.Context()))
		}
		slog.Info("request",
			"request_id", middleware.GetReqID(r.Context()),
			"method", r.Method,
			"path", r.URL.Path,
			"status", ww.Status(),
			"ms", time.Since(start).Milliseconds(),
		)
	})
}

// routeOf is the matched route pattern ("/v1/bookings/{id}"), so alerts group by endpoint and carry no ids.
func routeOf(r *http.Request) string {
	if rc := chi.RouteContext(r.Context()); rc != nil {
		if p := rc.RoutePattern(); p != "" {
			return p
		}
	}
	return "unmatched"
}

// recoverAndAlert turns a panic into a 500 in the stable error shape, logs it with its stack, and alerts the operators. A client that hung up
// (http.ErrAbortHandler) is not a bug.
func recoverAndAlert(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}
			if rec == http.ErrAbortHandler {
				panic(rec)
			}
			id := middleware.GetReqID(r.Context())
			slog.Error("panic", "request_id", id, "method", r.Method, "route", routeOf(r), "panic", fmt.Sprint(rec), "stack", string(debug.Stack()))
			ops.Report("panic "+r.Method+" "+routeOf(r), "A request panicked", fmt.Sprintf("%s %s\nrequest %s\n%v", r.Method, routeOf(r), id, rec))
			httpx.Error(w, r, http.StatusInternalServerError, httpx.CodeInternal, "Something went wrong", nil)
		}()
		next.ServeHTTP(w, r)
	})
}

// burst counts server errors over a short window: a few are noise, many in minutes mean something is broken.
type burst struct {
	mu    sync.Mutex
	times []time.Time
}

const (
	burstWindow    = 5 * time.Minute
	burstThreshold = 5
)

var errorBurst = &burst{}

func (b *burst) note(route, method, requestID string) {
	b.mu.Lock()
	now := time.Now()
	cut := now.Add(-burstWindow)
	kept := b.times[:0]
	for _, t := range b.times {
		if t.After(cut) {
			kept = append(kept, t)
		}
	}
	b.times = append(kept, now)
	n := len(b.times)
	b.mu.Unlock()
	if n >= burstThreshold {
		ops.Report("5xx-burst", "Many server errors", fmt.Sprintf("%d server errors in the last %d minutes. The latest: %s %s (request %s)", n, int(burstWindow.Minutes()), method, route, requestID))
	}
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

// cors lets the apps' browser preview call the API. Native apps do not use CORS, so it is off in production.
func cors(cfg config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !cfg.IsProduction() {
				w.Header().Set("Access-Control-Allow-Origin", "*")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Idempotency-Key, X-Device-Id, X-App, X-Platform")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				if r.Method == http.MethodOptions {
					w.WriteHeader(http.StatusNoContent)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
