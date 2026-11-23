package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResolveMapLink(t *testing.T) {
	old := mapLinkNext
	defer func() { mapLinkNext = old }()
	var contacted []string
	mapLinkNext = func(_ context.Context, u string) (string, string, error) {
		contacted = append(contacted, u)
		switch u {
		case "https://maps.app.goo.gl/abc":
			return "https://www.google.com/maps/place/Bole/@9.0227,38.7468,17z/data=!3m1", "", nil
		case "https://goo.gl/maps/loop":
			return "/maps/loop2", "", nil
		case "https://www.google.com/maps/loop2":
			return "https://goo.gl/maps/loop", "", nil
		}
		return "", "", nil
	}
	s := &Server{}
	run := func(body string) (int, map[string]float64) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/v1/admin/geo/resolve", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		s.h(s.adminResolveMapLink).ServeHTTP(rec, req)
		var out map[string]float64
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out
	}
	if code, out := run(`{"url":"https://maps.app.goo.gl/abc"}`); code != http.StatusOK || out["lat"] != 9.0227 || out["lng"] != 38.7468 {
		t.Fatalf("short link: %d %v", code, out)
	}
	if code, out := run(`{"url":"https://www.google.com/maps?q=9.01,38.76"}`); code != http.StatusOK || out["lat"] != 9.01 {
		t.Fatalf("long link: %d %v", code, out)
	}
	// The pin of the place (!3d!4d) beats the point the map happened to be centred on (@).
	if code, out := run(`{"url":"https://www.google.com/maps/place/Sky/@8.9882,38.7870,17z/data=!3d8.990038!4d38.788906"}`); code != http.StatusOK || out["lat"] != 8.990038 || out["lng"] != 38.788906 {
		t.Fatalf("place pin: %d %v", code, out)
	}
	contacted = nil
	for _, bad := range []string{`{"url":"http://maps.app.goo.gl/abc"}`, `{"url":"https://evil.example.com/maps/x"}`, `{"url":"https://169.254.169.254/latest"}`, `{"url":"https://maps.app.goo.gl.evil.com/x"}`} {
		if code, _ := run(bad); code == http.StatusOK {
			t.Errorf("%s accepted", bad)
		}
	}
	if len(contacted) != 0 {
		t.Errorf("contacted hosts it should not have: %v", contacted)
	}
	if code, _ := run(`{"url":"https://goo.gl/maps/loop"}`); code != http.StatusUnprocessableEntity {
		t.Errorf("redirect loop: %d", code)
	}
}
