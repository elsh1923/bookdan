package api_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"

	"github.com/go-chi/chi/v5"

	"context"
	"github.com/bookdan/backend/internal/api"
	"github.com/bookdan/backend/internal/config"
	"github.com/bookdan/backend/internal/server"
	"strings"
	"testing"
)

func rateOf(r resp, cur string) (rate float64, stale bool, found bool) {
	list, _ := r.body["rates"].([]any)
	for _, x := range list {
		m := x.(map[string]any)
		if m["currency"] == cur {
			return m["birr_per_unit"].(float64), m["stale"].(bool), true
		}
	}
	return 0, false, false
}

func TestDisplayRatesAreSetByAnAdminAndReadByEveryone(t *testing.T) {
	ctx := context.Background()
	pool.Exec(ctx, `DELETE FROM fx_rates`)
	defer pool.Exec(ctx, `DELETE FROM fx_rates`)
	admin := adminClient(t)
	guest := newClient(t, "guest")

	// Nothing is set: the public list is empty, so the apps show ETB only.
	if r := newClient(t, "guest").do("GET", "/rates", nil); r.status != 200 || r.str("base") != "ETB" || len(r.body["rates"].([]any)) != 0 {
		t.Fatalf("no rates yet: %d %s", r.status, r.raw)
	}

	if r := admin.do("PUT", "/admin/rates/USD", map[string]any{"birr_per_unit": 148.5}); r.status != 200 {
		t.Fatalf("set: %d %s", r.status, r.raw)
	}
	if r := admin.do("PUT", "/admin/rates/eur", map[string]any{"birr_per_unit": 160.25}); r.status != 200 {
		t.Fatalf("lower-case code: %d %s", r.status, r.raw)
	}
	pub := guest.do("GET", "/rates", nil)
	if usd, stale, ok := rateOf(pub, "USD"); !ok || usd != 148.5 || stale {
		t.Errorf("USD is public and fresh: %v %v %v %s", usd, stale, ok, pub.raw)
	}
	if eur, _, ok := rateOf(pub, "EUR"); !ok || eur != 160.25 {
		t.Errorf("EUR: %v %v", eur, ok)
	}
	if strings.Contains(string(pub.raw), "updated_by") || strings.Contains(string(pub.raw), "admin@") {
		t.Errorf("who set a rate is not public: %s", pub.raw)
	}
	if a := admin.do("GET", "/admin/rates", nil); a.status != 200 || !strings.Contains(string(a.raw), "updated_by") {
		t.Errorf("the admin sees who set it: %s", a.raw)
	}
}

func TestOnlyAdminsSetRatesAndBadRatesAreRefused(t *testing.T) {
	ctx := context.Background()
	pool.Exec(ctx, `DELETE FROM fx_rates`)
	defer pool.Exec(ctx, `DELETE FROM fx_rates`)
	admin := adminClient(t)
	guest := newClient(t, "guest")
	guest.signIn("email", uniqueEmail("fx"))

	if r := guest.do("PUT", "/admin/rates/USD", map[string]any{"birr_per_unit": 150}); r.status != 401 && r.status != 403 {
		t.Errorf("a guest cannot set rates: %d", r.status)
	}
	if r := newClient(t, "guest").do("PUT", "/admin/rates/USD", map[string]any{"birr_per_unit": 150}); r.status != 401 {
		t.Errorf("signed out: %d", r.status)
	}
	for name, body := range map[string]any{"zero": map[string]any{"birr_per_unit": 0}, "negative": map[string]any{"birr_per_unit": -5}, "huge": map[string]any{"birr_per_unit": 99999}, "text": map[string]any{"birr_per_unit": "a lot"}, "missing": map[string]any{}} {
		if r := admin.do("PUT", "/admin/rates/USD", body); r.status != 400 {
			t.Errorf("%s must be refused: %d %s", name, r.status, r.raw)
		}
	}
	if r := admin.do("PUT", "/admin/rates/XYZ", map[string]any{"birr_per_unit": 190}); r.status != 404 {
		t.Errorf("only the display currencies: %d", r.status)
	}
	if r := admin.do("PUT", "/admin/rates/ETB", map[string]any{"birr_per_unit": 1}); r.status != 404 {
		t.Errorf("ETB is the base, it has no rate: %d", r.status)
	}
}

func TestABigJumpInARateMustBeConfirmedAndAnOldRateIsMarkedStale(t *testing.T) {
	ctx := context.Background()
	pool.Exec(ctx, `DELETE FROM fx_rates`)
	defer pool.Exec(ctx, `DELETE FROM fx_rates`)
	admin := adminClient(t)

	admin.do("PUT", "/admin/rates/USD", map[string]any{"birr_per_unit": 150})
	if r := admin.do("PUT", "/admin/rates/USD", map[string]any{"birr_per_unit": 155}); r.status != 200 {
		t.Errorf("a small change goes through: %d", r.status)
	}
	if r := admin.do("PUT", "/admin/rates/USD", map[string]any{"birr_per_unit": 1550}); r.status != 409 || r.str("error", "code") != "RATE_JUMP" {
		t.Errorf("a typo of a zero too many is caught: %d %s", r.status, r.raw)
	}
	if usd, _, _ := rateOf(admin.do("GET", "/rates", nil), "USD"); usd != 155 {
		t.Errorf("a refused change leaves the rate: %v", usd)
	}
	if r := admin.do("PUT", "/admin/rates/USD", map[string]any{"birr_per_unit": 220, "confirm": true}); r.status != 200 {
		t.Errorf("a confirmed jump goes through: %d %s", r.status, r.raw)
	}

	pool.Exec(ctx, `UPDATE fx_rates SET updated_at = now() - interval '40 days'`)
	if _, stale, _ := rateOf(admin.do("GET", "/rates", nil), "USD"); !stale {
		t.Errorf("a rate not updated for 40 days is stale")
	}
}

// ---- more currencies, and the exchange-rate feed

func feedServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
}

// serverWithFeed is a second API server on the same database whose exchange-rate feed is the given address.
func serverWithFeed(t *testing.T, feedURL string) (*httptest.Server, *api.Server) {
	t.Helper()
	cfg := config.Config{Env: "test", DemoMode: true, PublicURL: "http://example.test", PaymentProvider: "dev", PaymentWebhookSecret: "whsec", FXFeedURL: feedURL,
		AdminPassword: "correct-horse-battery-staple", AdminEmails: []string{"admin@bookdan.test"}, AdminOwners: true,
		DemoAccountDomains: []string{"bookdan.test", "example.com", "example.org"}}
	providers := api.DevProviders(cfg)
	a2, err := api.New(context.Background(), cfg, pool, keys, providers)
	if err != nil {
		t.Fatal(err)
	}
	s2 := httptest.NewServer(server.New(cfg, pool, func(r chi.Router) { a2.Routes(r) }))
	t.Cleanup(s2.Close)
	return s2, a2
}

func doOn(base string, token, method, path string, body any) (int, map[string]any) {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, base+"/v1"+path, rd)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-App", "admin")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, nil
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return res.StatusCode, out
}

func TestManyDisplayCurrenciesAndTheFeedFillsThemWithoutOverwritingTypedRates(t *testing.T) {
	ctx := context.Background()
	pool.Exec(ctx, `DELETE FROM fx_rates`)
	defer pool.Exec(ctx, `DELETE FROM fx_rates`)
	admin := adminClient(t)

	// A currency worth less than a birr (the yen) and a Gulf currency can be typed by hand; a made-up one cannot.
	if r := admin.do("PUT", "/admin/rates/JPY", map[string]any{"birr_per_unit": 1.03}); r.status != 200 {
		t.Fatalf("a rate below one birr: %d %s", r.status, r.raw)
	}
	if r := admin.do("PUT", "/admin/rates/AED", map[string]any{"birr_per_unit": 0.001}); r.status != 400 {
		t.Errorf("below the lowest rate: %d", r.status)
	}
	if r := admin.do("PUT", "/admin/rates/XYZ", map[string]any{"birr_per_unit": 5}); r.status != 404 {
		t.Errorf("not a display currency: %d", r.status)
	}
	if a := admin.do("GET", "/admin/rates", nil); a.status != 200 || len(a.body["supported"].([]any)) < 25 || a.body["feed"] != false {
		t.Errorf("the admin sees every supported currency and whether the feed is on: %s", a.raw)
	}
	pool.Exec(ctx, `DELETE FROM fx_rates`)

	// The feed: how much of each currency one birr buys.
	feed := feedServer(t, 200, `{"result":"success","rates":{"ETB":1,"USD":0.0065,"EUR":0.0055,"JPY":0.97,"AED":0.0227,"XYZ":2,"KWD":0}}`)
	defer feed.Close()
	s2, a2 := serverWithFeed(t, feed.URL)
	tok := admin.token

	// A rate typed by an admin is never overwritten by the feed.
	if r := admin.do("PUT", "/admin/rates/USD", map[string]any{"birr_per_unit": 150}); r.status != 200 {
		t.Fatalf("typed rate: %d", r.status)
	}
	status, out := doOn(s2.URL, tok, "POST", "/admin/rates/refresh", nil)
	if status != 200 {
		t.Fatalf("refresh: %d %v", status, out)
	}
	written := fmt.Sprint(out["written"])
	if !strings.Contains(written, "EUR") || !strings.Contains(written, "JPY") || !strings.Contains(written, "AED") || strings.Contains(written, "USD") || strings.Contains(written, "XYZ") || strings.Contains(written, "KWD") {
		t.Errorf("the feed writes what nobody typed, and nothing it cannot use: %v", out)
	}
	if !strings.Contains(fmt.Sprint(out["left_alone"]), "USD") {
		t.Errorf("the typed rate is left alone: %v", out)
	}
	pub := admin.do("GET", "/rates", nil)
	if usd, _, _ := rateOf(pub, "USD"); usd != 150 {
		t.Errorf("USD stays as typed: %v", usd)
	}
	if jpy, _, ok := rateOf(pub, "JPY"); !ok || jpy < 1.03 || jpy > 1.032 {
		t.Errorf("one yen is about 1.03 birr: %v %v", jpy, ok)
	}
	if aed, _, ok := rateOf(pub, "AED"); !ok || aed < 44.05 || aed > 44.06 {
		t.Errorf("one dirham is about 44.05 birr: %v %v", aed, ok)
	}
	if _, _, ok := rateOf(pub, "KWD"); ok {
		t.Errorf("a currency the feed gives no usable rate for is not invented")
	}
	var by string
	pool.QueryRow(ctx, `SELECT updated_by FROM fx_rates WHERE currency = 'EUR'`).Scan(&by)
	if !strings.HasPrefix(by, "feed:") {
		t.Errorf("a feed rate says where it came from: %q", by)
	}

	// force replaces typed rates too; the daily job refreshes stale ones and the feed's own, but not a typed one.
	if status, out := doOn(s2.URL, tok, "POST", "/admin/rates/refresh", map[string]any{"force": true}); status != 200 || !strings.Contains(fmt.Sprint(out["written"]), "USD") {
		t.Errorf("force replaces typed rates: %d %v", status, out)
	}
	admin.do("PUT", "/admin/rates/USD", map[string]any{"birr_per_unit": 150, "confirm": true})
	pool.Exec(ctx, `UPDATE fx_rates SET updated_at = now() - interval '40 days' WHERE currency = 'EUR'`)
	a2.RunMaintenance(ctx)
	pub = admin.do("GET", "/rates", nil)
	if _, stale, _ := rateOf(pub, "EUR"); stale {
		t.Errorf("the daily job refreshes a stale feed rate")
	}
	if usd, _, _ := rateOf(pub, "USD"); usd != 150 {
		t.Errorf("the daily job leaves a typed rate: %v", usd)
	}

	// Only finance and super admins; a guest is refused; a feed that is down gives a clear answer; no feed configured says so.
	g := newClient(t, "guest")
	g.signIn("email", uniqueEmail("fxf"))
	if r := g.do("POST", "/admin/rates/refresh", nil); r.status != 401 && r.status != 403 {
		t.Errorf("a guest cannot refresh: %d", r.status)
	}
	down := feedServer(t, 500, `oops`)
	defer down.Close()
	s3, _ := serverWithFeed(t, down.URL)
	if status, out := doOn(s3.URL, tok, "POST", "/admin/rates/refresh", nil); status != 502 || fmt.Sprint(out["error"]) == "" {
		t.Errorf("a feed that is down: %d %v", status, out)
	}
	if r := admin.do("POST", "/admin/rates/refresh", nil); r.status != 409 || r.str("error", "code") != "FEED_OFF" {
		t.Errorf("no feed configured: %d %s", r.status, r.raw)
	}
}
