//go:build audit

package api_test

// Pre-release audit, groups S4 (booking creation), S5 (concurrency), S6 (late payment), S7 (cancellation), S8 (authorization),
// S13 (database guarantees) and S14 (security). Run with: go test -tags audit ./internal/api -run Audit_S
// A failing test here is a finding.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bookdan/backend/internal/config"
)

// rawHold makes a hold with plain net/http, so it can run in a goroutine (the test client calls t.Fatal, which a goroutine must not).
func rawHold(token, deviceID, quoteID, key string) (int, map[string]any) {
	b, _ := json.Marshal(map[string]string{"quote_id": quoteID, "guest_name": "Hanna Tesfaye", "idempotency_key": key})
	req, _ := http.NewRequest("POST", srv.URL+"/v1/bookings", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-App", "guest")
	req.Header.Set("X-Device-Id", deviceID)
	req.Header.Set("Authorization", "Bearer "+token)
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

func heldOn(t *testing.T, room string, night int) (held, booked int) {
	t.Helper()
	if err := pool.QueryRow(context.Background(), `SELECT coalesce(max(held), 0), coalesce(max(booked), 0) FROM inventory WHERE room_type_id = $1 AND night = $2::date`, room, day(night)).Scan(&held, &booked); err != nil {
		t.Fatal(err)
	}
	return
}

// ---- S4: booking creation

func TestAudit_S4_01_AHoldTakesOnlyTheNightsStayedNotTheCheckOutNight(t *testing.T) {
	c := auditGuest(t)
	h0, _ := heldOn(t, boleTwin, 170)
	h1, _ := heldOn(t, boleTwin, 171)
	hOut, _ := heldOn(t, boleTwin, 172)
	b := hold(c, quote(c, bole, boleTwin, 170, 172, 2, 1).str("id"), fmt.Sprintf("aud-s4-1-%d", time.Now().UnixNano()))
	if b.status != 200 {
		t.Fatalf("S4.1: %d %s", b.status, b.raw)
	}
	a0, _ := heldOn(t, boleTwin, 170)
	a1, _ := heldOn(t, boleTwin, 171)
	aOut, _ := heldOn(t, boleTwin, 172)
	if a0 != h0+1 || a1 != h1+1 {
		t.Errorf("S4.1: both nights stayed must lose one room: night1 %d->%d night2 %d->%d", h0, a0, h1, a1)
	}
	if aOut != hOut {
		t.Errorf("S4.1: the check-out night must not lose a room: %d -> %d", hOut, aOut)
	}
}

func TestAudit_S4_02_ARoomThatBecameUnavailableGivesACleanConflict(t *testing.T) {
	a, b := auditGuest(t), auditGuest(t)
	qb := quote(b, meskel, family, 180, 181, 1, 1) // the Family Suite has one room: b is quoted first
	if qb.status != 200 {
		t.Fatalf("setup: %d %s", qb.status, qb.raw)
	}
	if r := hold(a, quote(a, meskel, family, 180, 181, 1, 1).str("id"), fmt.Sprintf("aud-s4-2a-%d", time.Now().UnixNano())); r.status != 200 {
		t.Fatalf("setup: %d %s", r.status, r.raw)
	}
	r := hold(b, qb.str("id"), fmt.Sprintf("aud-s4-2b-%d", time.Now().UnixNano()))
	if r.status != 409 || r.str("error", "code") != "SOLD_OUT" {
		t.Errorf("S4.2: the second guest must get 409 SOLD_OUT, got %d %s", r.status, r.raw)
	}
}

func TestAudit_S4_03_GuestNamesAreValidatedAndAnyScriptIsAccepted(t *testing.T) {
	c := auditGuest(t)
	i := 0
	try := func(name string) int {
		i++
		q := quote(c, bole, boleSingle, 185+i, 186+i, 1, 1)
		b, _ := json.Marshal(map[string]string{"quote_id": q.str("id"), "guest_name": name, "idempotency_key": fmt.Sprintf("aud-s4-3-%d-%d", i, time.Now().UnixNano())})
		return c.do("POST", "/bookings", json.RawMessage(b)).status
	}
	for _, bad := range []string{"", " ", "A"} {
		if s := try(bad); s != 400 {
			t.Errorf("S4.3: name %q should be refused with 400, got %d", bad, s)
		}
	}
	for _, good := range []string{"Seán O'Brien-Smith", "José Ñandú", "山田 太郎", "Αλέξανδρος Παπαδόπουλος", "عبد الله", "አበበ በቀለ"} {
		if s := try(good); s != 200 {
			t.Errorf("S4.3: name %q must be accepted, got %d", good, s)
		}
	}
}

func TestAudit_S4_04_MoreGuestsThanTheRoomHoldsIsRefused(t *testing.T) {
	c := auditGuest(t)
	if q := quote(c, bole, boleSingle, 195, 196, 2, 1); q.status < 400 {
		t.Fatalf("S4.4: two guests in a room for one: %d", q.status)
	}
}

func TestAudit_S4_05_PastZeroAndTooLongStaysAreRefused(t *testing.T) {
	c := auditGuest(t)
	if q := quote(c, bole, boleSingle, -3, -2, 1, 1); q.status < 400 {
		t.Errorf("S4.5: a quote for past dates must be refused; got %d", q.status)
	}
	if q := quote(c, bole, boleSingle, 120, 120, 1, 1); q.status != 400 {
		t.Errorf("S4.5: a stay of zero nights must be 400; got %d", q.status)
	}
	if q := quote(c, bole, boleSingle, 120, 151, 1, 1); q.status != 400 {
		t.Errorf("S4.5: a stay of 31 nights must be 400; got %d", q.status)
	}
}

func TestAudit_S4_06_BookingForSomeoneElseValidatesThePhone(t *testing.T) {
	c := auditGuest(t)
	q := quote(c, bole, boleSingle, 188, 189, 1, 1)
	b, _ := json.Marshal(map[string]any{"quote_id": q.str("id"), "guest_name": "Meron Alem", "idempotency_key": fmt.Sprintf("aud-s4-6-%d", time.Now().UnixNano()), "booked_for_other": true, "guest_phone": "not a phone number!!"})
	if r := c.do("POST", "/bookings", json.RawMessage(b)); r.status != 400 {
		t.Errorf("S4.6: an invalid phone number must be refused; got %d", r.status)
	}
}

func TestAudit_S4_07_BookingReferencesAreUniqueAndWellFormed(t *testing.T) {
	c := auditGuest(t)
	re := regexp.MustCompile(`^BD-[A-Z0-9]{6}$`)
	seen := map[string]bool{}
	for i := 0; i < 8; i++ {
		b := hold(c, quote(c, bole, boleSingle, 190+i, 191+i, 1, 1).str("id"), fmt.Sprintf("aud-s4-7-%d-%d", i, time.Now().UnixNano()))
		ref := b.str("reference")
		if !re.MatchString(ref) || seen[ref] {
			t.Fatalf("S4.7: reference %q must match BD-XXXXXX and be unique", ref)
		}
		seen[ref] = true
	}
}

// ---- S5: concurrency and double booking

func TestAudit_S5_01_FiftyGuestsRaceForTheLastRoomExactlyOneWins(t *testing.T) {
	type guest struct {
		token, device, quote string
	}
	const n = 50
	guests := make([]guest, n)
	for i := range guests {
		c := auditGuest(t)
		q := quote(c, meskel, family, 196, 197, 1, 1)
		if q.status != 200 {
			t.Fatalf("setup: quote %d: %d %s", i, q.status, q.raw)
		}
		guests[i] = guest{c.token, c.deviceID, q.str("id")}
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	wins, sold, other := 0, 0, 0
	start := make(chan struct{})
	for i, g := range guests {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			status, body := rawHold(g.token, g.device, g.quote, fmt.Sprintf("aud-s5-1-%d-%d", i, time.Now().UnixNano()))
			mu.Lock()
			defer mu.Unlock()
			switch {
			case status == 200:
				wins++
			case status == 409:
				if e, _ := body["error"].(map[string]any); e != nil && e["code"] == "SOLD_OUT" {
					sold++
				} else {
					other++
				}
			default:
				other++
			}
		}()
	}
	close(start)
	wg.Wait()
	if wins != 1 || sold != n-1 || other != 0 {
		t.Fatalf("S5.1: 1 booking and %d SOLD_OUT expected; got wins=%d sold_out=%d other=%d", n-1, wins, sold, other)
	}
	if h, b := heldOn(t, family, 196); h+b != 1 {
		t.Fatalf("S5.1: the room count must be exactly 1 taken: held=%d booked=%d", h, b)
	}
}

func TestAudit_S5_03_DoubleTappingBookCreatesOneBooking(t *testing.T) {
	c := auditGuest(t)
	q := quote(c, bole, boleSingle, 199, 200, 1, 1)
	key := fmt.Sprintf("aud-s5-3-%d", time.Now().UnixNano())
	ids := make(chan string, 10)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if status, body := rawHold(c.token, c.deviceID, q.str("id"), key); status == 200 {
				ids <- fmt.Sprint(body["id"])
			}
		}()
	}
	wg.Wait()
	close(ids)
	seen := map[string]int{}
	for id := range ids {
		seen[id]++
	}
	var rows int
	pool.QueryRow(context.Background(), `SELECT count(*) FROM bookings WHERE quote_id = $1`, q.str("id")).Scan(&rows)
	if len(seen) != 1 || rows != 1 {
		t.Fatalf("S5.3: ten simultaneous taps must give one booking: distinct ids=%v rows=%d", seen, rows)
	}
}

// ---- S6: a payment that succeeds after the hold ended

func TestAudit_S6_01_PaymentAfterTheHoldEndedIsRefundedNotKept(t *testing.T) {
	c := auditGuest(t)
	hotel := hotelFor(t)
	b := hold(c, quote(c, bole, boleSingle, 201, 202, 1, 1).str("id"), fmt.Sprintf("aud-s6-1-%d", time.Now().UnixNano()))
	id := b.str("id")
	r := c.do("POST", "/bookings/"+id+"/pay", map[string]string{"method": "telebirr"})
	if r.status != 200 {
		t.Fatalf("setup: pay %d %s", r.status, r.raw)
	}
	pageURL := strings.Replace(r.str("session", "payment_url"), "http://example.test", srv.URL, 1)
	if _, err := pool.Exec(context.Background(), `UPDATE bookings SET hold_expires_at = now() - interval '1 minute' WHERE id = $1`, id); err != nil {
		t.Fatalf("setup: %v", err)
	}
	hotel.do("GET", "/partner/bookings", nil) // the sweeper runs and ends the hold
	if s := c.do("GET", "/bookings/"+id, nil).str("status"); s != "EXPIRED" {
		t.Fatalf("setup: the hold should have expired, status %s", s)
	}
	res, err := http.PostForm(pageURL, url.Values{"outcome": {"succeeded"}}) // the gateway reports success after the room was released
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	var payStatus string
	pool.QueryRow(context.Background(), `SELECT status FROM payments WHERE booking_id = $1`, id).Scan(&payStatus)
	if s := c.do("GET", "/bookings/"+id, nil).str("status"); s == "PAID_AWAITING_HOTEL" || s == "CONFIRMED" {
		t.Errorf("S6: a payment that arrives after the hold ended must never confirm a booking without a room: %s", s)
	}
	if payStatus != "refunded" {
		t.Errorf("S6: the late payment must be refunded; payment status is %q", payStatus)
	}
}

func TestAudit_S6_02_NoCardDataColumnsExist(t *testing.T) {
	rows, err := pool.Query(context.Background(), `SELECT table_name || '.' || column_name FROM information_schema.columns
		WHERE table_schema = 'app' AND column_name ~* '(card|cvv|cvc|pan\b|expiry)'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var col string
		rows.Scan(&col)
		t.Errorf("S6: a column that could hold card data: %s", col)
	}
}

// ---- S7: cancellation

func TestAudit_S7_01_CancellingTwiceAndCancellingAnotherGuestsBookingAreRefused(t *testing.T) {
	a, other := auditGuest(t), auditGuest(t)
	hotel := hotelFor(t)
	id, _, _, _ := confirmedBooking(t, a, hotel, 205, 207, fmt.Sprintf("aud-s7-1-%d", time.Now().UnixNano()))
	if r := other.do("POST", "/bookings/"+id+"/cancel", nil); r.status != 404 {
		t.Errorf("S7: another guest cancelling this booking must get 404; got %d", r.status)
	}
	if r := a.do("POST", "/bookings/"+id+"/cancel", nil); r.status != 200 {
		t.Fatalf("setup: first cancel %d %s", r.status, r.raw)
	}
	var refunds0 int
	pool.QueryRow(context.Background(), `SELECT count(*) FROM ledger_transactions WHERE booking_id = $1 AND kind = 'refund'`, id).Scan(&refunds0)
	if r := a.do("POST", "/bookings/"+id+"/cancel", nil); r.status < 400 {
		t.Errorf("S7: cancelling a cancelled booking again must be an error; got %d", r.status)
	}
	var refunds1 int
	pool.QueryRow(context.Background(), `SELECT count(*) FROM ledger_transactions WHERE booking_id = $1 AND kind = 'refund'`, id).Scan(&refunds1)
	if refunds1 != refunds0 {
		t.Errorf("S7: a second cancel must never refund twice: %d -> %d", refunds0, refunds1)
	}
}

func TestAudit_S7_02_ACompletedStayCannotBeCancelled(t *testing.T) {
	a := auditGuest(t)
	hotel := hotelFor(t)
	id, _, _, _ := confirmedBooking(t, a, hotel, 210, 212, fmt.Sprintf("aud-s7-2-%d", time.Now().UnixNano()))
	pool.Exec(context.Background(), fmt.Sprintf(`UPDATE bookings SET check_in = %s - 6, check_out = %s - 4 WHERE id = $1`, addisToday, addisToday), id)
	app.RunMaintenance(context.Background())
	if s := a.do("GET", "/bookings/"+id, nil).str("status"); s != "COMPLETED" {
		t.Fatalf("setup: the stay should be completed: %s", s)
	}
	if r := a.do("POST", "/bookings/"+id+"/cancel", nil); r.status < 400 {
		t.Errorf("S7: a completed stay cannot be cancelled; got %d", r.status)
	}
}

// ---- S8: authorization

func TestAudit_S8_01_AGuestCannotTouchAnotherGuestsBookingOnAnyEndpoint(t *testing.T) {
	a, other := auditGuest(t), auditGuest(t)
	b := hold(a, quote(a, bole, boleSingle, 215, 216, 1, 1).str("id"), fmt.Sprintf("aud-s8-1-%d", time.Now().UnixNano()))
	id := b.str("id")
	for _, ep := range []struct{ method, path string }{
		{"GET", "/bookings/" + id}, {"POST", "/bookings/" + id + "/pay"}, {"POST", "/bookings/" + id + "/cancel"},
		{"POST", "/bookings/" + id + "/review"}, {"GET", "/bookings/" + id + "/voucher"},
	} {
		var body any
		if ep.method == "POST" {
			body = map[string]any{"method": "telebirr", "rating": 5}
		}
		if r := other.do(ep.method, ep.path, body); r.status != 404 {
			t.Errorf("S8 IDOR: %s %s by another guest gave %d, want 404", ep.method, ep.path, r.status)
		}
	}
	if list := other.do("GET", "/bookings", nil); strings.Contains(string(list.raw), id) {
		t.Errorf("S8 IDOR: another guest's list contains this booking")
	}
}

func TestAudit_S8_02_EveryRoleOnlyReachesItsOwnEndpoints(t *testing.T) {
	guest := auditGuest(t)
	hotel := hotelFor(t)
	for _, ep := range []struct{ method, path string }{{"GET", "/partner/bookings"}, {"GET", "/partner/earnings"}, {"GET", "/partner/rooms"}, {"GET", "/admin/hotels"}} {
		if r := guest.do(ep.method, ep.path, nil); r.status != 401 && r.status != 403 {
			t.Errorf("S8: a guest token on %s %s gave %d", ep.method, ep.path, r.status)
		}
	}
	for _, ep := range []struct{ method, path string }{{"GET", "/bookings"}, {"GET", "/me/profile"}, {"GET", "/admin/hotels"}, {"GET", "/admin/payouts"}} {
		if r := hotel.do(ep.method, ep.path, nil); r.status != 401 && r.status != 403 {
			t.Errorf("S8: a hotel-staff token on %s %s gave %d", ep.method, ep.path, r.status)
		}
	}
	anon := newClient(t, "guest")
	for _, path := range []string{"/bookings", "/me/profile", "/partner/bookings", "/admin/hotels"} {
		if r := anon.do("GET", path, nil); r.status != 401 {
			t.Errorf("S8: no token on GET %s gave %d, want 401", path, r.status)
		}
	}
}

// ---- S13: database guarantees

func TestAudit_S13_01_TheDatabaseRefusesBadRowsOnItsOwn(t *testing.T) {
	ctx := context.Background()
	// A hold creates the inventory row for the night, so the statements below change a real row.
	c := auditGuest(t)
	if b := hold(c, quote(c, bole, boleSingle, 220, 221, 1, 1).str("id"), fmt.Sprintf("aud-s13-%d", time.Now().UnixNano())); b.status != 200 {
		t.Fatalf("setup: %d %s", b.status, b.raw)
	}
	// A second booking, so the "duplicate code" case below involves two different rows.
	if b := hold(c, quote(c, bole, boleSingle, 222, 223, 1, 1).str("id"), fmt.Sprintf("aud-s13b-%d", time.Now().UnixNano())); b.status != 200 {
		t.Fatalf("setup: %d %s", b.status, b.raw)
	}
	cases := map[string]string{
		"inventory held negative":             `UPDATE inventory SET held = -1 WHERE room_type_id = '` + boleSingle + `' AND night = '` + day(220) + `'`,
		"inventory more than capacity":        `UPDATE inventory SET booked = capacity + 1 WHERE room_type_id = '` + boleSingle + `' AND night = '` + day(220) + `'`,
		"room block with check_out<=check_in": `INSERT INTO room_blocks (property_id, room_type_id, check_in, check_out, rooms, kind, guest_name) VALUES ('` + bole + `', '` + boleSingle + `', '` + day(220) + `', '` + day(220) + `', 1, 'walk_in', 'Test Guest')`,
		"booking status outside the list":     `UPDATE bookings SET status = 'MAYBE' WHERE id = (SELECT id FROM bookings LIMIT 1)`,
		"booking with no guests":              `UPDATE bookings SET guests = 0 WHERE id = (SELECT id FROM bookings LIMIT 1)`,
		"duplicate code":                      `UPDATE bookings SET code = (SELECT code FROM bookings ORDER BY created_at LIMIT 1) WHERE id = (SELECT id FROM bookings ORDER BY created_at DESC LIMIT 1)`,
	}
	for name, stmt := range cases {
		if _, err := pool.Exec(ctx, stmt); err == nil {
			t.Errorf("S13: the database accepted %s", name)
		}
	}
}

// ---- S14: security

func TestAudit_S14_01_ResponsesCarrySecurityHeadersAndLeakNothing(t *testing.T) {
	res, err := http.Get(srv.URL + "/v1/healthz")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("S14: X-Content-Type-Options: nosniff is missing on API responses")
	}
	// HSTS is sent when the public address is https (as in production).
	restore := auditServer(t, func(c *config.Config) { c.PublicURL = "https://api.bookdan.example" })
	res2, err := http.Get(srv.URL + "/v1/healthz")
	restore()
	if err != nil {
		t.Fatal(err)
	}
	res2.Body.Close()
	if res2.Header.Get("Strict-Transport-Security") == "" {
		t.Errorf("S14: Strict-Transport-Security is missing when the public address is https")
	}
	if res.Header.Get("Server") != "" || res.Header.Get("X-Powered-By") != "" {
		t.Errorf("S14: server version headers leak: %q %q", res.Header.Get("Server"), res.Header.Get("X-Powered-By"))
	}
	c := auditGuest(t)
	for _, path := range []string{"/bookings/not-a-uuid", "/properties/not-a-uuid?check_in=" + day(5) + "&check_out=" + day(6), "/bookings/00000000-0000-0000-0000-000000000000"} {
		raw := string(c.do("GET", path, nil).raw)
		for _, leak := range []string{"SQLSTATE", "pgx", "goroutine", "panic", "\n\t/", ".go:", "relation \""} {
			if strings.Contains(raw, leak) {
				t.Errorf("S14: %s leaks %q: %s", path, leak, raw)
			}
		}
	}
}

// S14.2: taking rooms or money is limited per address. The limit is configured (production turns it on at 60 a minute), so this runs on a
// server started with 50 a minute.
func TestAudit_S14_02_BookingCreationIsRateLimited(t *testing.T) {
	defer auditServer(t, func(c *config.Config) { c.RateWritePerMin = 50 })()
	c := auditGuest(t)
	ok, limited := 0, 0
	for i := 0; i < 200; i++ {
		switch quote(c, bole, boleSingle, 225, 226, 1, 1).status {
		case http.StatusTooManyRequests:
			limited++
		default:
			ok++
		}
	}
	if limited < 100 || ok > 60 {
		t.Errorf("S14.2: with a limit of 50 a minute, 200 quotes should mostly be refused: answered=%d refused=%d", ok, limited)
	}
}
