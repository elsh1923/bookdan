// Package api implements the HTTP endpoints of the mobile contract (api/openapi.yaml) on PostgreSQL.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/bookdan/backend/internal/config"
	"github.com/bookdan/backend/internal/httpx"
	"github.com/bookdan/backend/internal/ops"
)

// Business rules that are fixed in code (the money rates live in pricing_config).
const (
	quoteTTL       = 10 * time.Minute // how long a price can be used to place a hold
	holdTTL        = 15 * time.Minute // how long a room is held while the guest pays
	hotelAnswerTTL = 15 * time.Minute // how long the hotel has to answer after the guest has paid
	maxRooms       = 9
	maxNights      = 30
	accessTTL      = 15 * time.Minute
	refreshTTL     = 30 * 24 * time.Hour
)

// Stable error codes (the apps translate these).
const (
	codeInvalidPhone    = "INVALID_PHONE"
	codeInvalidEmail    = "INVALID_EMAIL"
	codeInvalidOTP      = "INVALID_OTP"
	codeTooMany         = "TOO_MANY_REQUESTS"
	codeSendFailed      = "SEND_FAILED"
	codeCancelTooLate   = "CANCEL_TOO_LATE"
	codeReviewNotReady  = "REVIEW_NOT_READY"
	codeAlreadyReviewed = "ALREADY_REVIEWED"
	codeSoldOut         = "SOLD_OUT"
	codeQuoteExpired    = "QUOTE_EXPIRED"
	codeHoldExpired     = "HOLD_EXPIRED"
	codeIdemMismatch    = "IDEMPOTENCY_MISMATCH"
	codeVoucherNotYet   = "VOUCHER_NOT_READY"
	codePaymentFailed   = "PAYMENT_FAILED"
	codeDeadlinePassed  = "DEADLINE_PASSED"
	codeInvalidState    = "INVALID_STATE"
	codeInvalidID       = "INVALID_ID"
	codeGone            = "GONE"
	codeProfileMissing  = "PROFILE_INCOMPLETE"
	codeForbidden       = "FORBIDDEN"
	codeNotStaff        = "NOT_STAFF"
	codePricingPending  = "PRICING_NOT_CONFIRMED"
)

type apiError struct {
	status  int
	code    string
	message string
}

func (e *apiError) Error() string { return e.code + ": " + e.message }

func fail(status int, code, message string) error { return &apiError{status, code, message} }

var (
	errNotFound = fail(http.StatusNotFound, httpx.CodeNotFound, "Not found")
	errAuth     = fail(http.StatusUnauthorized, httpx.CodeUnauthenticated, "Sign in required")
)

type Server struct {
	cfg  config.Config
	pool *pgxpool.Pool
	keys *Keys
	p    Providers
	// postgis decides how coordinates are read (PostGIS columns, or the plain-text columns of the test database).
	postgis bool
	now     func() time.Time
	// adminLimit slows down guessing of the admin password.
	adminLimit loginLimiter
}

// New builds the API. It checks once whether PostGIS is installed.
func New(ctx context.Context, cfg config.Config, pool *pgxpool.Pool, keys *Keys, p Providers) (*Server, error) {
	s := &Server{cfg: cfg, pool: pool, keys: keys, p: p, now: time.Now}
	if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'postgis')`).Scan(&s.postgis); err != nil {
		return nil, err
	}
	return s, nil
}

// Routes registers every /v1 endpoint that this package serves on r.
func (s *Server) Routes(root chi.Router) {
	root.Group(func(r chi.Router) {
		s.adminRoutes(r)

		r.Post("/auth/otp/request", s.h(s.requestOTP))
		r.Post("/auth/otp/verify", s.h(s.verifyOTP))
		r.Post("/auth/refresh", s.h(s.refresh))
		r.Post("/auth/logout", s.h(s.logout))
		r.Get("/keys/voucher", s.h(s.voucherKey))

		r.Get("/search", s.h(s.search))
		r.Get("/suggest", s.h(s.suggest))
		r.Post("/hotel-applications", s.h(s.submitApplication))
		r.Get("/rates", s.h(s.rates))
		r.Get("/properties/{id}", s.h(s.property))

		r.Group(func(r chi.Router) {
			r.Use(s.authenticate("guest"))
			r.Post("/quotes", s.h(s.createQuote))
			r.Post("/bookings", s.h(s.createBooking))
			r.Get("/bookings", s.h(s.listBookings))
			r.Get("/bookings/{id}", s.h(s.getBooking))
			r.Post("/bookings/{id}/pay", s.h(s.payBooking))
			r.Post("/bookings/{id}/cancel", s.h(s.cancelBooking))
			r.Post("/bookings/{id}/review", s.h(s.submitReview))
			r.Post("/bookings/{id}/dispute", s.h(s.raiseDispute))
			r.Post("/bookings/{id}/change-dates", s.h(s.changeDates))
			r.Post("/bookings/{id}/call", s.h(s.logCall))
			r.Get("/bookings/{id}/voucher", s.h(s.voucher))
			r.Put("/me/devices/push-token", s.h(s.registerPushToken("guest")))
			r.Get("/me/profile", s.h(s.getProfile))
			r.Put("/me/profile", s.h(s.putProfile))
			r.Delete("/me/profile", s.h(s.deleteProfile))
			r.Delete("/me/account", s.h(s.deleteAccount))
			r.Get("/me/export", s.h(s.exportAccount))
			r.Post("/support/tickets", s.h(s.createTicket))
			r.Get("/support/tickets", s.h(s.listTickets))
			r.Get("/support/tickets/{id}", s.h(s.getTicket))
			r.Post("/support/tickets/{id}/messages", s.h(s.replyTicket))
			r.Post("/support/tickets/{id}/close", s.h(s.closeTicket))
		})

		r.Route("/partner", func(r chi.Router) {
			r.Use(s.authenticate("partner"))
			r.Put("/devices/push-token", s.h(s.registerPushToken("partner")))
			r.Get("/bookings", s.h(s.partnerBookings))
			r.Post("/bookings/{id}/accept", s.h(s.partnerAccept))
			r.Post("/bookings/{id}/decline", s.h(s.partnerDecline))
			r.Post("/bookings/{id}/cancel", s.h(s.partnerCancel))
			r.Get("/statements", s.h(s.partnerStatements))
			r.Get("/reviews", s.h(s.partnerReviews))
			r.Put("/reviews/{id}/reply", s.h(s.partnerReplyReview))
			r.Delete("/reviews/{id}/reply", s.h(s.partnerDeleteReply))
			r.Post("/support/tickets", s.h(s.createTicket))
			r.Get("/support/tickets", s.h(s.listTickets))
			r.Get("/support/tickets/{id}", s.h(s.getTicket))
			r.Post("/support/tickets/{id}/messages", s.h(s.replyTicket))
			r.Post("/support/tickets/{id}/close", s.h(s.closeTicket))
			r.Post("/bookings/{id}/relocate", s.h(s.partnerRelocate))
			r.Get("/bookings/{id}/dispute", s.h(s.partnerDispute))
			r.Post("/bookings/{id}/dispute/statement", s.h(s.partnerDisputeStatement))
			r.Post("/bookings/{id}/check-in", s.h(s.partnerCheckin))
			r.Post("/bookings/{id}/no-show", s.h(s.partnerNoShow))
			r.Get("/rooms", s.h(s.partnerRoomsChart))
			r.Get("/rooms/blocks", s.h(s.partnerBlocks))
			r.Post("/rooms/blocks", s.h(s.partnerCreateBlock))
			r.Delete("/rooms/blocks/{id}", s.h(s.partnerReleaseBlock))
			r.Post("/checkin/lookup", s.h(s.partnerCheckinLookup))
			r.Post("/checkin/sync", s.h(s.partnerCheckinSync))
			r.Get("/earnings", s.h(s.partnerEarnings))
			r.Post("/bookings/{id}/guest/reveal", s.h(s.partnerReveal))
		})

		// Payment results arrive here, signed by the gateway. The development gateway page is served only outside production.
		if _, ok := s.p.Gateway.(*chapaGateway); ok {
			r.Post("/webhooks/payments/chapa", s.h(s.chapaWebhook))
			r.Get("/payments/chapa/return", s.chapaReturn)
		}
		r.Post("/webhooks/payments/{provider}", s.h(s.paymentWebhook))
		if s.cfg.PaymentProvider == "dev" && !s.cfg.IsProduction() {
			r.Get("/dev/gateway/{id}", s.devGatewayPage)
			r.Post("/dev/gateway/{id}", s.devGatewayComplete)
		}
	})
}

// ---- plumbing

type handler func(w http.ResponseWriter, r *http.Request) error

// h turns a handler that returns an error into an http.HandlerFunc with the standard error body.
func (s *Server) h(fn handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := fn(w, r)
		if err == nil {
			return
		}
		var ae *apiError
		if errors.As(err, &ae) {
			httpx.Error(w, r, ae.status, ae.code, ae.message, nil)
			return
		}
		if r.Context().Err() != nil {
			return // the client went away
		}
		slog.Error("request failed", "request_id", middleware.GetReqID(r.Context()), "path", r.URL.Path, "err", err)
		route := "unmatched"
		if rc := chi.RouteContext(r.Context()); rc != nil && rc.RoutePattern() != "" {
			route = rc.RoutePattern()
		}
		ops.Report("api-500 "+r.Method+" "+route, "An endpoint failed", r.Method+" "+route+"\nrequest "+middleware.GetReqID(r.Context())+"\n"+err.Error())
		httpx.Error(w, r, http.StatusInternalServerError, httpx.CodeInternal, "Something went wrong", nil)
	}
}

type ctxKey int

const claimsKey ctxKey = 1

func claimsFrom(r *http.Request) claims {
	c, _ := r.Context().Value(claimsKey).(claims)
	return c
}

// authenticate requires a valid access token issued to the given app.
func (s *Server) authenticate(app string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return s.h(func(w http.ResponseWriter, r *http.Request) error {
			token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !ok {
				return errAuth
			}
			c, err := s.keys.VerifyJWT(token, s.now())
			if err != nil || c.App != app {
				return errAuth
			}
			// A valid signature is not enough: the account must still be active and the phone not revoked, so signing out, closing an account or
			// suspending a user ends access at once, not when the token expires.
			var live bool
			if err := s.pool.QueryRow(r.Context(), `SELECT u.status = 'active' AND (d.id IS NULL OR d.revoked_at IS NULL) FROM users u
				LEFT JOIN devices d ON d.id = NULLIF($2, '')::uuid WHERE u.id = $1`, c.Sub, c.Dev).Scan(&live); err != nil || !live {
				if err != nil && err != pgx.ErrNoRows {
					return err
				}
				return errAuth
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), claimsKey, c)))
			return nil
		})
	}
}

// decode reads a JSON body (at most 64 KiB) and rejects unknown shapes of input.
func decode(r *http.Request, v any) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, 64<<10))
	if err != nil {
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "Could not read the request")
	}
	if err := json.Unmarshal(body, v); err != nil {
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "The request body is not valid")
	}
	return nil
}

func respond(w http.ResponseWriter, status int, v any) {
	httpx.JSON(w, status, v)
}

// tx runs fn in a transaction, rolling back on any error.
func (s *Server) tx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // a rollback after commit is a no-op
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// actor records who makes a status change; the database writes it to booking_events itself.
func actor(ctx context.Context, tx pgx.Tx, who, reason string) error {
	_, err := tx.Exec(ctx, `SELECT set_config('app.actor', $1, true), set_config('app.reason', $2, true)`, who, reason)
	return err
}

// addisToday is today's date in Africa/Addis_Ababa (UTC+3, no daylight saving).
func (s *Server) addisToday() string { return s.now().UTC().Add(3 * time.Hour).Format("2006-01-02") }

func jsonRaw(b []byte) any { return json.RawMessage(b) }

func pgDate(t time.Time) string { return t.Format("2006-01-02") }

func isDate(v string) bool {
	_, err := time.Parse("2006-01-02", v)
	return err == nil
}

func nightsBetween(in, out string) int {
	a, _ := time.Parse("2006-01-02", in)
	b, _ := time.Parse("2006-01-02", out)
	return int(b.Sub(a).Hours() / 24)
}
