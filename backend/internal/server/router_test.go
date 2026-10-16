package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bookdan/backend/internal/config"
)

type fakeDB struct{ err error }

func (f fakeDB) Ping(context.Context) error { return f.err }

func get(t *testing.T, h http.Handler, path string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("%s: body is not JSON: %v", path, err)
	}
	return rec, body
}

func TestHealthz_IsLivenessOnly(t *testing.T) {
	h := New(config.Config{Env: "test"}, nil, nil) // no database at all
	rec, body := get(t, h, "/v1/healthz")
	if rec.Code != http.StatusOK || body["status"] != "ok" {
		t.Fatalf("got %d %v", rec.Code, body)
	}
	if _, ok := body["server_time"]; !ok {
		t.Fatal("every response must carry server_time")
	}
}

func TestReadyz(t *testing.T) {
	cases := []struct {
		name string
		db   Pinger
		want int
	}{
		{"no database configured", nil, http.StatusServiceUnavailable},
		{"database down", fakeDB{err: errors.New("boom")}, http.StatusServiceUnavailable},
		{"database up", fakeDB{}, http.StatusOK},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec, _ := get(t, New(config.Config{}, c.db, nil), "/v1/readyz")
			if rec.Code != c.want {
				t.Fatalf("got %d, want %d", rec.Code, c.want)
			}
		})
	}
}

func TestUnbuiltRoutes_UseTheStandardErrorShape(t *testing.T) {
	h := New(config.Config{}, nil, nil)
	for _, path := range []string{"/v1/search", "/v1/bookings", "/v1/partner/rooms"} {
		rec, body := get(t, h, path)
		if rec.Code != http.StatusNotImplemented {
			t.Fatalf("%s: got %d", path, rec.Code)
		}
		e, ok := body["error"].(map[string]any)
		if !ok || e["code"] != "NOT_IMPLEMENTED" || e["request_id"] == "" {
			t.Fatalf("%s: bad error body %v", path, body)
		}
		if _, ok := body["server_time"]; !ok {
			t.Fatalf("%s: errors must carry server_time too", path)
		}
	}
}
