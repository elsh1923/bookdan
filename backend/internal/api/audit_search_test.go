//go:build audit

package api_test

// Pre-release audit, scenario group S1 (search). Run with: go test -tags audit ./internal/api -run Audit_S1
// A failing test here is a finding. Each test states the rule it checks, in the words of the audit brief.

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/bookdan/backend/internal/api"
	"github.com/bookdan/backend/internal/config"
	"github.com/bookdan/backend/internal/server"
)

const boleTwin = "b0000000-0000-0000-0000-000000000005" // Bole Garden Guesthouse: 3 Twin Rooms (capacity 2); 4 Single Rooms (capacity 1) are boleSingle

// fillBoleNight takes every room of the Bole Garden Guesthouse for one night (4 singles and 3 twins), so the hotel has nothing to sell that night.
func fillBoleNight(t *testing.T, c *client, night int) {
	t.Helper()
	n := time.Now().UnixNano()
	for _, h := range []struct {
		room  string
		rooms int
	}{{boleSingle, 4}, {boleTwin, 3}} {
		q := quote(c, bole, h.room, night, night+1, h.rooms, h.rooms)
		if q.status != 200 {
			t.Fatalf("setup: quote %s for night %d: %d %s", h.room, night, q.status, q.raw)
		}
		if b := hold(c, q.str("id"), fmt.Sprintf("aud-fill-%s-%d-%d", h.room[len(h.room)-2:], night, n)); b.status != 200 {
			t.Fatalf("setup: hold %s for night %d: %d %s", h.room, night, b.status, b.raw)
		}
	}
}

// auditServer starts a second API server on the same database with a changed configuration (rate limits, an https public address),
// and points the test client at it until the returned function is called.
func auditServer(t *testing.T, change func(*config.Config), mods ...func(*api.Providers)) (restore func()) {
	t.Helper()
	cfg := config.Config{Env: "test", DemoMode: true, IDRetentionDays: 30, PublicURL: "http://example.test", PaymentProvider: "dev", PaymentWebhookSecret: "whsec",
		AdminPassword: "correct-horse-battery-staple", AdminEmails: []string{"admin@bookdan.test"}, AdminOwners: true,
		DemoAccountDomains: []string{"bookdan.test", "example.com", "example.org"}}
	change(&cfg)
	providers := api.DevProviders(cfg)
	providers.Push, providers.Mail = push, mails
	providers.OTP, providers.EmailReal = api.RoutedSender(mailer, mailer), true
	for _, m := range mods {
		m(&providers)
	}
	app2, err := api.New(context.Background(), cfg, pool, keys, providers)
	if err != nil {
		t.Fatal(err)
	}
	srv2 := httptest.NewServer(server.New(cfg, pool, func(r chi.Router) { app2.Routes(r) }, app2.AdminPage))
	main := srv
	srv = srv2
	return func() { srv = main; srv2.Close() }
}

func auditGuest(t *testing.T) *client {
	t.Helper()
	c := newClient(t, "guest")
	c.signIn("email", uniqueEmail("audit"))
	withProfile(c)
	return c
}

// searchBole searches for the Bole hotel by name for nights [in, out) and returns the response.
func searchBole(c *client, in, out, guests int) resp {
	return c.do("GET", fmt.Sprintf("/search?q=bole&check_in=%s&check_out=%s&guests=%d&rooms=1", day(in), day(out), guests), nil)
}

// itemFor returns the hotel with this id from a search response, or nil when it is not listed.
func itemFor(r resp, id string) map[string]any {
	items, _ := r.body["items"].([]any)
	for _, it := range items {
		if m, _ := it.(map[string]any); m["id"] == id {
			return m
		}
	}
	return nil
}

func TestAudit_S1_01_RoomFreeEveryNightHotelIsReturned(t *testing.T) {
	c := auditGuest(t)
	r := searchBole(c, 70, 72, 1)
	if it := itemFor(r, bole); r.status != 200 || it == nil || it["available"] != true {
		t.Fatalf("S1.1: a hotel with a room free on every night must be returned as available: %d %v", r.status, it)
	}
}

// S1.2 as decided by the owner: a hotel free on only some nights is NOT bookable for the stay, but stays in the list marked as sold out
// (the brief's original rule was "excluded").
func TestAudit_S1_02_RoomFreeOnSomeNightsOnlyHotelIsListedAsSoldOut(t *testing.T) {
	c := auditGuest(t)
	fillBoleNight(t, c, 81) // the middle night of a three-night stay
	r := searchBole(c, 80, 83, 1)
	it := itemFor(r, bole)
	if it == nil || it["available"] != false {
		t.Fatalf("S1.2: the hotel must be listed and marked unavailable for the stay: %v", it)
	}
}

func TestAudit_S1_03_RoomBookedOnlyOnCheckOutDateHotelIsIncluded(t *testing.T) {
	c := auditGuest(t)
	fillBoleNight(t, c, 92) // the check-out day of a stay on nights 90 and 91
	r := searchBole(c, 90, 92, 1)
	if it := itemFor(r, bole); it == nil || it["available"] != true {
		t.Fatalf("S1.3: a room taken only on the check-out date must not block the stay: %v", it)
	}
}

func TestAudit_S1_04_RoomBookedTheNightBeforeCheckInHotelIsIncluded(t *testing.T) {
	c := auditGuest(t)
	fillBoleNight(t, c, 99) // the night before check-in
	r := searchBole(c, 100, 102, 1)
	if it := itemFor(r, bole); it == nil || it["available"] != true {
		t.Fatalf("S1.4: a room taken the night before check-in must not block the stay: %v", it)
	}
}

// S1.5 as decided: inventory rows are created on demand (a night with no row has every room free). The brief's rule, "treat a missing row as
// unavailable", would make every date without a pre-made row unsellable. What must hold is that this can never oversell, so the test
// deletes the rows for a night, takes every room, and checks that one more booking is refused.
func TestAudit_S1_05_NightsWithNoInventoryRowStillCannotBeOversold(t *testing.T) {
	c := auditGuest(t)
	if _, err := pool.Exec(context.Background(), `DELETE FROM inventory WHERE night = $1::date AND room_type_id IN ($2::uuid, $3::uuid)`, day(110), boleSingle, boleTwin); err != nil {
		t.Fatalf("setup: %v", err)
	}
	fillBoleNight(t, c, 110) // creates the rows and takes every room
	if q := quote(c, bole, boleSingle, 110, 111, 1, 1); q.status != 409 || q.str("error", "code") != "SOLD_OUT" {
		t.Fatalf("S1.5: after every room is taken the next quote must be SOLD_OUT: %d %s", q.status, q.raw)
	}
	var over int
	pool.QueryRow(context.Background(), `SELECT count(*) FROM inventory WHERE night = $1::date AND held + booked > capacity`, day(110)).Scan(&over)
	if over != 0 {
		t.Fatalf("S1.5: %d inventory rows are oversold", over)
	}
}

func TestAudit_S1_06_CheckInAfterCheckOutIs400(t *testing.T) {
	c := auditGuest(t)
	if r := c.do("GET", fmt.Sprintf("/search?q=bole&check_in=%s&check_out=%s&guests=1&rooms=1", day(125), day(120)), nil); r.status != 400 {
		t.Fatalf("S1.6: %d", r.status)
	}
}

func TestAudit_S1_07_CheckInEqualsCheckOutIs400(t *testing.T) {
	c := auditGuest(t)
	if r := c.do("GET", fmt.Sprintf("/search?q=bole&check_in=%s&check_out=%s&guests=1&rooms=1", day(125), day(125)), nil); r.status != 400 {
		t.Fatalf("S1.7: %d", r.status)
	}
}

func TestAudit_S1_08_CheckInInThePastIs400(t *testing.T) {
	c := auditGuest(t)
	if r := c.do("GET", fmt.Sprintf("/search?q=bole&check_in=%s&check_out=%s&guests=1&rooms=1", day(-5), day(-3)), nil); r.status != 400 {
		t.Errorf("S1.8: a stay in the past must be refused; got %d", r.status)
	}
}

func TestAudit_S1_09_StayLongerThanTheMaximumIs400(t *testing.T) {
	c := auditGuest(t)
	if r := c.do("GET", fmt.Sprintf("/search?q=bole&check_in=%s&check_out=%s&guests=1&rooms=1", day(130), day(161)), nil); r.status != 400 {
		t.Fatalf("S1.9: 31 nights must be refused (maximum 30): %d", r.status)
	}
	if r := c.do("GET", fmt.Sprintf("/search?q=bole&check_in=%s&check_out=%s&guests=1&rooms=1", day(130), day(160)), nil); r.status != 200 {
		t.Fatalf("S1.9: 30 nights is the largest stay and must work: %d", r.status)
	}
}

func TestAudit_S1_10_DatesTooFarInTheFutureAreRejectedOrEmpty(t *testing.T) {
	c := auditGuest(t)
	r := c.do("GET", fmt.Sprintf("/search?q=bole&check_in=%s&check_out=%s&guests=1&rooms=1", day(900), day(902)), nil)
	items, _ := r.body["items"].([]any)
	if r.status == 200 && len(items) > 0 {
		t.Errorf("S1.10: two and a half years ahead (no rates exist) must be rejected or return an empty result with a message; got %d hotels, no message", len(items))
	}
}

// S1.11 changed: searches are limited to 365 days ahead, so 29 February is out of reach for a real search. The night count across a leap day
// is now checked directly in TestNightCountsAcrossLeapDaysAndYearEnds (alternatives_test.go), and the horizon itself here.
func TestAudit_S1_11_SearchesBeyondTheBookingHorizonAreRefused(t *testing.T) {
	c := auditGuest(t)
	if r := c.do("GET", "/search?q=bole&check_in=2028-02-27&check_out=2028-03-02&guests=1&rooms=1", nil); r.status != 400 {
		t.Fatalf("S1.11: a stay beyond 365 days ahead must be refused: %d", r.status)
	}
}

func TestAudit_S1_12_GuestsAboveTheLargestRoomHaveNothingToBook(t *testing.T) {
	c := auditGuest(t)
	r := searchBole(c, 140, 142, 30)
	for _, it := range r.body["items"].([]any) {
		if m := it.(map[string]any); m["id"] == bole && m["available"] == true {
			t.Fatalf("S1.12: 30 guests in one room is not bookable")
		}
	}
}

func TestAudit_S1_13_NoMatchIs200WithAnEmptyList(t *testing.T) {
	c := auditGuest(t)
	r := c.do("GET", fmt.Sprintf("/search?q=zzzz-no-such-place&check_in=%s&check_out=%s&guests=1&rooms=1", day(140), day(142)), nil)
	if items, _ := r.body["items"].([]any); r.status != 200 || len(items) != 0 {
		t.Fatalf("S1.13: %d %s", r.status, r.raw)
	}
}

func TestAudit_S1_15_HostileInputNeverBreaksTheSearch(t *testing.T) {
	c := auditGuest(t)
	inputs := map[string]string{
		"sql":   "'; DROP TABLE properties; --",
		"sql2":  "bole' OR '1'='1",
		"emoji": "🏨🌍🎉",
		"long":  strings.Repeat("a", 10_000),
		"nul":   "bole%00",
		"like":  "100%_",
	}
	for name, q := range inputs {
		r := c.do("GET", fmt.Sprintf("/search?q=%s&check_in=%s&check_out=%s&guests=1&rooms=1", url.QueryEscape(q), day(140), day(142)), nil)
		if r.status >= 500 {
			t.Errorf("S1.15 %s: server error %d %s", name, r.status, r.raw)
		}
	}
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM properties`).Scan(&n); err != nil || n == 0 {
		t.Fatalf("S1.15: the properties table must still exist after hostile input: %v", err)
	}
}

func TestAudit_S1_17_EverySortOptionReturnsTheRightOrder(t *testing.T) {
	c := auditGuest(t)
	get := func(sortBy string) []map[string]any {
		r := c.do("GET", fmt.Sprintf("/search?check_in=%s&check_out=%s&guests=1&rooms=1&sort=%s", day(145), day(146), sortBy), nil)
		if r.status != 200 {
			t.Fatalf("S1.17 %s: %d %s", sortBy, r.status, r.raw)
		}
		var out []map[string]any
		for _, it := range r.body["items"].([]any) {
			if m := it.(map[string]any); m["available"] == true {
				out = append(out, m)
			}
		}
		return out
	}
	price := func(m map[string]any) float64 { return m["from_price_santim"].(float64) }
	asc, desc := get("price_asc"), get("price_desc")
	if len(asc) < 2 || len(desc) < 2 {
		t.Fatalf("S1.17: not enough hotels to check the order: %d %d", len(asc), len(desc))
	}
	if !sort.SliceIsSorted(asc, func(i, j int) bool { return price(asc[i]) < price(asc[j]) }) {
		t.Errorf("S1.17: price_asc is not ascending")
	}
	if !sort.SliceIsSorted(desc, func(i, j int) bool { return price(desc[i]) > price(desc[j]) }) {
		t.Errorf("S1.17: price_desc is not descending")
	}
	rated := get("rating")
	last := 6.0
	for _, m := range rated {
		rv, _ := m["rating"].(float64) // null rating reads as 0
		if rv > last {
			t.Errorf("S1.17/S1.22: rating sort must be descending with unrated hotels last: %v after %v", rv, last)
		}
		last = rv
	}
}

func TestAudit_S1_18_SortTiesHaveADeterministicOrder(t *testing.T) {
	c := auditGuest(t)
	var first []string
	for i := 0; i < 5; i++ {
		r := c.do("GET", fmt.Sprintf("/search?check_in=%s&check_out=%s&guests=1&rooms=1&sort=price_asc", day(145), day(146)), nil)
		var ids []string
		for _, it := range r.body["items"].([]any) {
			ids = append(ids, it.(map[string]any)["id"].(string))
		}
		if i == 0 {
			first = ids
		} else if strings.Join(ids, ",") != strings.Join(first, ",") {
			t.Fatalf("S1.18: the order changed between identical searches")
		}
	}
}

func TestAudit_S1_19_DistanceSortWithoutCoordinatesIs400(t *testing.T) {
	c := auditGuest(t)
	if r := c.do("GET", fmt.Sprintf("/search?q=bole&check_in=%s&check_out=%s&guests=1&rooms=1&sort=distance", day(145), day(146)), nil); r.status != 400 {
		t.Fatalf("S1.19: %d", r.status)
	}
}

func TestAudit_S1_21_PagingParametersAreClamped(t *testing.T) {
	c := auditGuest(t)
	base := fmt.Sprintf("/search?check_in=%s&check_out=%s&guests=1&rooms=1", day(145), day(146))
	for _, cursor := range []string{"-5", "0", "999999", "abc", "9223372036854775807"} {
		r := c.do("GET", base+"&cursor="+cursor, nil)
		if r.status != 200 {
			t.Errorf("S1.21: cursor %q gave %d %s", cursor, r.status, r.raw)
		}
	}
}

func TestAudit_S1_24_HiddenHotelsAreNeverReturned(t *testing.T) {
	c := auditGuest(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `UPDATE properties SET status = 'hidden' WHERE id = $1`, bole); err != nil {
		t.Fatalf("setup: %v", err)
	}
	defer pool.Exec(ctx, `UPDATE properties SET status = 'active' WHERE id = $1`, bole)
	if r := searchBole(c, 150, 152, 1); itemFor(r, bole) != nil {
		t.Fatalf("S1.24: a hidden hotel must never appear in search")
	}
	if r := c.do("GET", fmt.Sprintf("/properties/%s?check_in=%s&check_out=%s", bole, day(150), day(152)), nil); r.status != 404 {
		t.Fatalf("S1.24: a hidden hotel's page must not open: %d", r.status)
	}
}

func TestAudit_S1_27_TheSameSearchTwiceGivesTheSameResults(t *testing.T) {
	c := auditGuest(t)
	a, b := searchBole(c, 155, 157, 1), searchBole(c, 155, 157, 1)
	if string(fmt.Sprint(a.body["items"])) != string(fmt.Sprint(b.body["items"])) {
		t.Fatalf("S1.27: two identical searches differ")
	}
}

// S1.28: the limit is configured (production turns it on by default), so this runs on a server started with a limit of 100 searches a minute.
func TestAudit_S1_28_BotLikeRequestRateGets429(t *testing.T) {
	defer auditServer(t, func(c *config.Config) { c.RateReadPerMin = 100 })()
	c := auditGuest(t)
	ok, limited := 0, 0
	for i := 0; i < 300; i++ {
		switch searchBole(c, 155, 157, 1).status {
		case http.StatusOK:
			ok++
		case http.StatusTooManyRequests:
			limited++
		}
	}
	if limited < 150 || ok < 90 {
		t.Errorf("S1.28: with a limit of 100 a minute, 300 searches should mostly be refused: answered=%d refused=%d", ok, limited)
	}
}

func TestAudit_S1_29_PlaceOnlySearchReturnsFromPricesWithoutAvailability(t *testing.T) {
	c := auditGuest(t)
	r := c.do("GET", "/search?q=bole", nil)
	if r.status != 200 {
		t.Errorf("S1.29: a search with a place and no dates should return hotels with a \"from\" price; got %d %s", r.status, r.raw)
	}
}

func TestAudit_S1_30_GuestAndRoomCountsAreValidated(t *testing.T) {
	c := auditGuest(t)
	for _, q := range []string{"guests=0&rooms=1", "guests=-2&rooms=1", "guests=1&rooms=0", "guests=1&rooms=-1", "guests=1&rooms=10", "guests=41&rooms=1"} {
		if r := c.do("GET", fmt.Sprintf("/search?q=bole&check_in=%s&check_out=%s&%s", day(155), day(156), q), nil); r.status != 400 {
			t.Errorf("S1.30: %s gave %d, want 400", q, r.status)
		}
	}
}

func TestAudit_S1_26_LoadOneThousandConcurrentSearches(t *testing.T) {
	c := auditGuest(t)
	const total, workers = 1000, 100
	var mu sync.Mutex
	var lat []time.Duration
	failed := 0
	jobs := make(chan int, total)
	for i := 0; i < total; i++ {
		jobs <- i
	}
	close(jobs)
	start := time.Now()
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range jobs {
				t0 := time.Now()
				req, _ := http.NewRequest("GET", fmt.Sprintf("%s/v1/search?check_in=%s&check_out=%s&guests=2&rooms=1", srv.URL, day(160), day(162)), nil)
				req.Header.Set("X-App", "guest")
				req.Header.Set("X-Device-Id", c.deviceID)
				res, err := http.DefaultClient.Do(req)
				d := time.Since(t0)
				mu.Lock()
				lat = append(lat, d)
				if err != nil || res.StatusCode != 200 {
					failed++
				}
				mu.Unlock()
				if err == nil {
					res.Body.Close()
				}
			}
		}()
	}
	wg.Wait()
	sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })
	p := func(q float64) time.Duration { return lat[int(float64(len(lat)-1)*q)] }
	t.Logf("S1.26 LOAD (embedded local PostgreSQL, one machine, %d workers): %d searches in %s; p50=%s p95=%s p99=%s; failed=%d",
		workers, total, time.Since(start).Round(time.Millisecond), p(.50), p(.95), p(.99), failed)
	if failed > 0 {
		t.Errorf("S1.26: %d of %d searches failed under load", failed, total)
	}
}
