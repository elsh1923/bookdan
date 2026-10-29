package api_test

import (
	"bytes"
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

func activeHotels(t *testing.T, n int) []string {
	t.Helper()
	rows, err := pool.Query(context.Background(), `SELECT id::text FROM properties WHERE status = 'active' ORDER BY name LIMIT $1`, n)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		rows.Scan(&id)
		ids = append(ids, id)
	}
	if len(ids) < n {
		t.Fatalf("need %d active hotels, have %d", n, len(ids))
	}
	return ids
}

func savedIDs(r resp) []string {
	var out []string
	items, _ := r.body["items"].([]any)
	for _, x := range items {
		out = append(out, x.(map[string]any)["property_id"].(string))
	}
	return out
}

func has(list []string, id string) bool {
	for _, x := range list {
		if x == id {
			return true
		}
	}
	return false
}

func TestSavedHotelsAreKeptOnTheServerAndSynced(t *testing.T) {
	g, _ := supportGuest(t, "sv")
	other, _ := supportGuest(t, "sv2")
	ids := activeHotels(t, 3)

	if r := newClient(t, "guest").do("GET", "/me/saved", nil); r.status != 401 {
		t.Errorf("signed out: %d", r.status)
	}
	if r := g.do("GET", "/me/saved", nil); r.status != 200 || len(savedIDs(r)) != 0 {
		t.Fatalf("empty to begin with: %d %s", r.status, r.raw)
	}
	if r := g.do("PUT", "/me/saved/"+ids[0], nil); r.status != 200 {
		t.Fatalf("save: %d %s", r.status, r.raw)
	}
	if r := g.do("PUT", "/me/saved/"+ids[0], nil); r.status != 200 {
		t.Errorf("saving twice is fine: %d", r.status)
	}
	if got := savedIDs(g.do("GET", "/me/saved", nil)); len(got) != 1 || got[0] != ids[0] {
		t.Errorf("one saved hotel: %v", got)
	}
	for name, r := range map[string]resp{
		"unknown": g.do("PUT", "/me/saved/00000000-0000-0000-0000-000000000000", nil),
		"garbled": g.do("PUT", "/me/saved/not-a-uuid", nil),
	} {
		if r.status != 404 {
			t.Errorf("%s: %d %s", name, r.status, r.raw)
		}
	}
	if got := savedIDs(other.do("GET", "/me/saved", nil)); len(got) != 0 {
		t.Errorf("another guest's list is their own: %v", got)
	}

	// Sync: what changed on the phone. A hotel in both lists is removed; hotels that do not exist are ignored; the answer is the whole list.
	r := g.do("POST", "/me/saved/sync", map[string]any{"add": []string{ids[1], ids[2], ids[2], "00000000-0000-0000-0000-000000000000"}, "remove": []string{ids[0], ids[2]}})
	if r.status != 200 {
		t.Fatalf("sync: %d %s", r.status, r.raw)
	}
	got := savedIDs(r)
	if len(got) != 1 || got[0] != ids[1] {
		t.Errorf("after the sync only the second hotel is saved: %v", got)
	}
	for name, body := range map[string]map[string]any{
		"bad id":   {"add": []string{"nope"}},
		"too many": {"add": make([]string, 501)},
	} {
		if x := g.do("POST", "/me/saved/sync", body); x.status != 400 {
			t.Errorf("%s: %d", name, x.status)
		}
	}
	if x := g.do("POST", "/me/saved/sync", map[string]any{}); x.status != 200 || len(savedIDs(x)) != 1 {
		t.Errorf("an empty sync just returns the list: %d %s", x.status, x.raw)
	}
	if x := g.do("DELETE", "/me/saved/"+ids[1], nil); x.status != 200 || len(savedIDs(g.do("GET", "/me/saved", nil))) != 0 {
		t.Errorf("remove: %d", x.status)
	}
	if x := g.do("DELETE", "/me/saved/"+ids[1], nil); x.status != 200 {
		t.Errorf("removing twice is fine: %d", x.status)
	}

	// The list is part of the data export and goes with the account.
	g.do("PUT", "/me/saved/"+ids[2], nil)
	if e := g.do("GET", "/me/export", nil); !strings.Contains(string(e.raw), ids[2]) || !strings.Contains(string(e.raw), "saved_hotels") {
		t.Errorf("the export holds the saved hotels: %s", e.raw)
	}
	var uid string
	pool.QueryRow(context.Background(), `SELECT user_id::text FROM saved_properties WHERE property_id = $1 ORDER BY saved_at DESC LIMIT 1`, ids[2]).Scan(&uid)
	if d := g.do("DELETE", "/me/account", map[string]string{"confirm": "DELETE"}); d.status != 200 {
		t.Fatalf("delete the account: %d %s", d.status, d.raw)
	}
	var left int
	pool.QueryRow(context.Background(), `SELECT count(*) FROM saved_properties WHERE user_id = $1`, uid).Scan(&left)
	if left != 0 {
		t.Errorf("deleting the account deletes the list: %d", left)
	}
}

func TestOneConnectionCannotAskForCodesWithoutEnd(t *testing.T) {
	cfg := config.Config{Env: "test", DemoMode: true, PublicURL: "http://example.test", PaymentProvider: "dev", PaymentWebhookSecret: "whsec", OTPMaxPerIP: 3,
		DemoAccountDomains: []string{"bookdan.test", "example.com", "example.org"}}
	providers := api.DevProviders(cfg)
	providers.OTP, providers.EmailReal = api.RoutedSender(mailer, mailer), true
	a2, err := api.New(context.Background(), cfg, pool, keys, providers)
	if err != nil {
		t.Fatal(err)
	}
	s2 := httptest.NewServer(server.New(cfg, pool, func(r chi.Router) { a2.Routes(r) }))
	defer s2.Close()

	ask := func(email, ip string) int {
		body, _ := json.Marshal(map[string]string{"type": "email", "value": email})
		req, _ := http.NewRequest("POST", s2.URL+"/v1/auth/otp/request", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-App", "guest")
		req.Header.Set("X-Forwarded-For", ip)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res.StatusCode
	}
	for i, want := range []int{200, 200, 200, 429, 429} {
		if got := ask(uniqueEmail("ipl"), "10.77.1.1"); got != want {
			t.Errorf("request %d from one connection: %d, want %d", i+1, got, want)
		}
	}
	// Another connection is not affected, and neither is the limit per address for the same person.
	if got := ask(uniqueEmail("ipl"), "10.77.1.2"); got != 200 {
		t.Errorf("another connection: %d", got)
	}
}
