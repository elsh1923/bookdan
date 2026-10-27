package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/bookdan/backend/internal/config"
	"github.com/bookdan/backend/internal/ops"
)

type alertBox struct {
	mu   sync.Mutex
	msgs []string
}

func (a *alertBox) send(_ context.Context, subject, body string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.msgs = append(a.msgs, subject+"\n"+body)
	return nil
}

func (a *alertBox) all() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.msgs...)
}

func TestAPanicBecomesAStableErrorAndAnAlertThatNamesTheRouteNotTheSecrets(t *testing.T) {
	box := &alertBox{}
	ops.Configure(box.send)
	defer ops.Configure()
	h := New(config.Config{Env: "test"}, nil, func(r chi.Router) {
		r.Get("/boom/{id}", func(http.ResponseWriter, *http.Request) { panic("the cache fell over") })
	})
	req := newRequest(http.MethodGet, "/v1/boom/4829-secret-id?token=SECRET123")
	req.Header.Set("Authorization", "Bearer TOPSECRETTOKEN")
	rec := serve(h, req)
	if rec.Code != 500 || !strings.Contains(rec.Body.String(), `"code":"INTERNAL"`) {
		t.Fatalf("a panic is a 500 in the usual shape: %d %s", rec.Code, rec.Body.String())
	}
	ops.Wait()
	msgs := box.all()
	if len(msgs) != 1 || !strings.Contains(msgs[0], "/v1/boom/{id}") || !strings.Contains(msgs[0], "the cache fell over") {
		t.Fatalf("the alert names the route pattern and the cause: %v", msgs)
	}
	for _, secret := range []string{"4829-secret-id", "SECRET123", "TOPSECRETTOKEN"} {
		if strings.Contains(msgs[0], secret) {
			t.Errorf("an alert never carries ids, query values or tokens: %q in %q", secret, msgs[0])
		}
	}
	// The server keeps serving after a panic.
	if rec := serve(h, newRequest(http.MethodGet, "/v1/healthz")); rec.Code != 200 {
		t.Errorf("still up: %d", rec.Code)
	}
}

func TestAFewServerErrorsAreNoiseAndAManyInMinutesAreAnAlert(t *testing.T) {
	box := &alertBox{}
	ops.Configure(box.send)
	defer ops.Configure()
	errorBurst = &burst{}
	h := New(config.Config{Env: "test"}, nil, func(r chi.Router) {
		r.Get("/broken", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusInternalServerError) })
		r.Get("/missing", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotFound) })
	})
	for i := 0; i < burstThreshold-1; i++ {
		serve(h, newRequest(http.MethodGet, "/v1/broken"))
	}
	for i := 0; i < 20; i++ {
		serve(h, newRequest(http.MethodGet, "/v1/missing")) // client errors are not alerts
	}
	ops.Wait()
	for _, m := range box.all() {
		if strings.Contains(m, "Many server errors") {
			t.Fatalf("below the threshold there is no alert: %v", box.all())
		}
	}
	serve(h, newRequest(http.MethodGet, "/v1/broken"))
	ops.Wait()
	var found bool
	for _, m := range box.all() {
		found = found || (strings.Contains(m, "Many server errors") && strings.Contains(m, "/v1/broken"))
	}
	if !found {
		t.Errorf("the burst is reported with the latest route: %v", box.all())
	}
}

func newRequest(method, path string) *http.Request { return httptest.NewRequest(method, path, nil) }

func serve(h http.Handler, r *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}
