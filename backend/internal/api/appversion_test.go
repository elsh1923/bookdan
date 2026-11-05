package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/bookdan/backend/internal/api"
	"github.com/bookdan/backend/internal/config"
	"github.com/bookdan/backend/internal/server"
)

func TestAnAppOlderThanTheMinimumVersionIsToldToUpdate(t *testing.T) {
	cfg := config.Config{Env: "test", DemoMode: true, PublicURL: "http://example.test", PaymentProvider: "dev", PaymentWebhookSecret: "whsec",
		MinAppVersionGuest: "1.2.0", MinAppVersionPartner: "not-a-version", GuestAppURL: "https://example.test/guest.apk", PartnerAppURL: "https://example.test/partner.apk",
		DemoAccountDomains: []string{"bookdan.test", "example.com", "example.org"}}
	a2, err := api.New(context.Background(), cfg, pool, keys, api.DevProviders(cfg))
	if err != nil {
		t.Fatal(err)
	}
	s2 := httptest.NewServer(server.New(cfg, pool, func(r chi.Router) { a2.Routes(r) }))
	defer s2.Close()

	get := func(path, app, version string) (int, map[string]any) {
		req, _ := http.NewRequest("GET", s2.URL+"/v1"+path, nil)
		if app != "" {
			req.Header.Set("X-App", app)
		}
		if version != "" {
			req.Header.Set("X-App-Version", version)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var body map[string]any
		_ = json.NewDecoder(res.Body).Decode(&body)
		return res.StatusCode, body
	}

	// Old builds are refused with a stable code; the same or newer, or a build that sends no version, are served.
	for _, c := range []struct {
		version string
		want    int
	}{{"1.1.9", 426}, {"0.2.0", 426}, {"1.2.0", 200}, {"1.10.0", 200}, {"2.0.0-beta.1", 200}, {"", 200}, {"garbage", 200}} {
		status, body := get("/search?q=addis", "guest", c.version)
		if status != c.want {
			t.Errorf("guest %q: %d, want %d (%v)", c.version, status, c.want, body)
		}
		if c.want == 426 {
			if e, _ := body["error"].(map[string]any); e == nil || e["code"] != "UPGRADE_REQUIRED" {
				t.Errorf("guest %q: the refusal needs the stable code: %v", c.version, body)
			}
		}
	}
	// A setting that is not a version means no minimum (and the partner app is served).
	if status, _ := get("/search?q=addis", "partner", "0.0.1"); status != 200 {
		t.Errorf("an invalid minimum must not lock anyone out: %d", status)
	}

	// The config endpoint is never gated, and tells the app what to do.
	status, body := get("/app-config?app=guest", "guest", "0.1.0")
	if status != 200 || body["min_version"] != "1.2.0" || body["download_url"] != "https://example.test/guest.apk" {
		t.Errorf("app-config for guest: %d %v", status, body)
	}
	if status, body := get("/app-config?app=partner", "", ""); status != 200 || body["min_version"] != "" {
		t.Errorf("an invalid minimum is reported as none: %d %v", status, body)
	}
	if status, _ := get("/app-config?app=unknown", "", ""); status != 400 {
		t.Errorf("an unknown app: %d", status)
	}
	if !strings.Contains(s2.URL, "http") {
		t.Fatal("unreachable")
	}
}

func TestWithoutAMinimumEveryVersionIsServed(t *testing.T) {
	c := newClient(t, "guest")
	if r := c.do("GET", "/search?q=addis", nil, "X-App-Version", "0.0.1"); r.status != 200 {
		t.Fatalf("no minimum is set: %d %s", r.status, r.raw)
	}
	if r := c.do("GET", "/app-config?app=guest", nil); r.status != 200 || r.str("min_version") != "" {
		t.Fatalf("app-config without a minimum: %d %s", r.status, r.raw)
	}
}
