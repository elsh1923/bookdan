package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bookdan/backend/internal/config"
)

func TestTheLimiterAllowsTheLimitThenRefusesUntilTheMinuteEnds(t *testing.T) {
	l := newLimiter(3)
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	for i := 1; i <= 3; i++ {
		if ok, _ := l.allow("1.2.3.4", now); !ok {
			t.Fatalf("request %d of 3 must be allowed", i)
		}
	}
	ok, after := l.allow("1.2.3.4", now.Add(10*time.Second))
	if ok || after < 1 || after > 60 {
		t.Fatalf("the fourth request in the minute is refused with a wait of 1 to 60 seconds: %v %d", ok, after)
	}
	if ok, _ := l.allow("5.6.7.8", now.Add(10*time.Second)); !ok {
		t.Fatalf("another address has its own allowance")
	}
	if ok, _ := l.allow("1.2.3.4", now.Add(61*time.Second)); !ok {
		t.Fatalf("a new minute starts a new allowance")
	}
}

func TestOnlyReadingAndTakingRoomsAreRateLimited(t *testing.T) {
	cases := []struct {
		method, path, group string
	}{
		{"GET", "/v1/search", "read"}, {"GET", "/v1/suggest", "read"}, {"GET", "/v1/properties/abc", "read"},
		{"POST", "/v1/quotes", "write"}, {"POST", "/v1/bookings", "write"}, {"POST", "/v1/bookings/x/pay", "write"}, {"POST", "/v1/bookings/x/cancel", "write"},
		{"GET", "/v1/bookings", ""}, {"GET", "/v1/healthz", ""}, {"POST", "/v1/webhooks/payments/chapa", ""}, {"POST", "/v1/auth/otp/request", ""},
		{"GET", "/v1/payments/chapa/return", ""},
	}
	for _, c := range cases {
		if g := rateGroup(httptest.NewRequest(c.method, c.path, nil)); g != c.group {
			t.Errorf("%s %s is in group %q, want %q (payment webhooks and the app's own bookings list must never be limited)", c.method, c.path, g, c.group)
		}
	}
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
}

func TestTheRateLimitAnswers429WithRetryAfterAndIsOffWhenNotConfigured(t *testing.T) {
	limited := rateLimit(config.Config{RateReadPerMin: 5})(okHandler())
	var last *httptest.ResponseRecorder
	got429 := 0
	for i := 0; i < 8; i++ {
		last = httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/v1/search", nil)
		req.RemoteAddr = "9.9.9.9:1234"
		limited.ServeHTTP(last, req)
		if last.Code == http.StatusTooManyRequests {
			got429++
		}
	}
	if got429 != 3 || last.Header().Get("Retry-After") == "" {
		t.Fatalf("5 allowed, 3 refused, with Retry-After: got %d refusals, header %q", got429, last.Header().Get("Retry-After"))
	}

	off := rateLimit(config.Config{})(okHandler()) // development and tests: no limit
	for i := 0; i < 2000; i++ {
		rec := httptest.NewRecorder()
		off.ServeHTTP(rec, httptest.NewRequest("GET", "/v1/search", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("with no limit configured nothing may be refused")
		}
	}
}

func TestProductionTurnsTheRateLimitOnWithSensibleDefaults(t *testing.T) {
	prod := rateLimit(config.Config{Env: "production"})(okHandler())
	refused := 0
	for i := 0; i < 80; i++ { // the default for taking rooms or money is 60 a minute
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/v1/quotes", nil)
		req.RemoteAddr = "8.8.8.8:1"
		prod.ServeHTTP(rec, req)
		if rec.Code == http.StatusTooManyRequests {
			refused++
		}
	}
	if refused != 20 {
		t.Fatalf("production defaults to 60 a minute for bookings: %d of 80 were refused, want 20", refused)
	}
}

func TestSecurityHeadersAreSentAndHSTSOnlyOverHTTPS(t *testing.T) {
	for _, c := range []struct {
		publicURL string
		hsts      bool
	}{{"http://localhost:8080", false}, {"https://api.bookdan.example", true}} {
		rec := httptest.NewRecorder()
		securityHeaders(config.Config{PublicURL: c.publicURL})(okHandler()).ServeHTTP(rec, httptest.NewRequest("GET", "/v1/healthz", nil))
		if rec.Header().Get("X-Content-Type-Options") != "nosniff" || rec.Header().Get("Referrer-Policy") == "" {
			t.Errorf("%s: nosniff and a referrer policy are always sent", c.publicURL)
		}
		if got := rec.Header().Get("Strict-Transport-Security") != ""; got != c.hsts {
			t.Errorf("%s: HSTS sent=%v, want %v", c.publicURL, got, c.hsts)
		}
	}
}
