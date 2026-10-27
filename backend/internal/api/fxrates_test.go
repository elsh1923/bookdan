package api_test

import (
	"context"
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
	if r := admin.do("PUT", "/admin/rates/GBP", map[string]any{"birr_per_unit": 190}); r.status != 404 {
		t.Errorf("only USD and EUR: %d", r.status)
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
