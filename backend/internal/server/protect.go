package server

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bookdan/backend/internal/config"
	"github.com/bookdan/backend/internal/httpx"
)

// securityHeaders adds the headers every API answer should carry. Strict-Transport-Security is sent only when the public address is
// https (sending it for plain http in development would be wrong, and browsers ignore it there).
func securityHeaders(cfg config.Config) func(http.Handler) http.Handler {
	https := strings.HasPrefix(cfg.PublicURL, "https://")
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("Referrer-Policy", "no-referrer")
			if https {
				h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}
			next.ServeHTTP(w, r)
		})
	}
}

// A request rate limit per client address, in two groups: reading hotels (search, suggestions, hotel pages) and taking rooms or money
// (quotes, bookings, payments, cancellations). It is a fixed one-minute window held in memory, which is enough to stop a script from
// hammering one server; a limit shared by several servers, or one at the edge, is the next step. A limit of 0 turns a group off.
//
// The client address comes from the proxy headers (chi's RealIP), so it is only as honest as the proxy in front of the server.

type limiter struct {
	mu      sync.Mutex
	perMin  int
	window  time.Time
	counts  map[string]int
	started time.Time
}

func newLimiter(perMin int) *limiter {
	return &limiter{perMin: perMin, counts: map[string]int{}}
}

// allow reports whether key may make another request in the current minute. now is a parameter so tests do not wait.
func (l *limiter) allow(key string, now time.Time) (ok bool, retryAfter int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if now.Sub(l.window) >= time.Minute {
		l.window, l.counts = now, map[string]int{}
	}
	l.counts[key]++
	if l.counts[key] > l.perMin {
		left := int(time.Minute.Seconds() - now.Sub(l.window).Seconds())
		if left < 1 {
			left = 1
		}
		return false, left
	}
	return true, 0
}

// rateGroup says which limit a request counts against ("" = none).
func rateGroup(r *http.Request) string {
	p := strings.TrimPrefix(r.URL.Path, "/v1")
	switch {
	case p == "/search" || p == "/suggest" || strings.HasPrefix(p, "/properties/"):
		return "read"
	case r.Method == http.MethodPost && (p == "/quotes" || p == "/bookings" || strings.HasPrefix(p, "/bookings/")):
		return "write"
	}
	return ""
}

func rateLimit(cfg config.Config) func(http.Handler) http.Handler {
	read, write := cfg.RateReadPerMin, cfg.RateWritePerMin
	if cfg.IsProduction() {
		if read == 0 {
			read = 600 // 10 a second: far above a person, well below a script
		}
		if write == 0 {
			write = 60
		}
	}
	limits := map[string]*limiter{}
	if read > 0 {
		limits["read"] = newLimiter(read)
	}
	if write > 0 {
		limits["write"] = newLimiter(write)
	}
	return func(next http.Handler) http.Handler {
		if len(limits) == 0 {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if l := limits[rateGroup(r)]; l != nil {
				host, _, err := net.SplitHostPort(r.RemoteAddr)
				if err != nil {
					host = r.RemoteAddr
				}
				if ok, after := l.allow(host, time.Now()); !ok {
					w.Header().Set("Retry-After", strconv.Itoa(after))
					httpx.Error(w, r, http.StatusTooManyRequests, "TOO_MANY_REQUESTS", "Too many requests. Try again in a minute.", nil)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
