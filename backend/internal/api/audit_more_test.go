//go:build audit

package api_test

// Pre-release audit, second batch: scenarios that were NOT TESTED in Phase A and can be tested without an outside service.
// Run with: go test -tags audit ./internal/api -run Audit_
// A failing test here is a finding.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bookdan/backend/internal/api"
	"github.com/bookdan/backend/internal/config"
)

// rawDo makes one request with plain net/http, so it can run in a goroutine.
func rawDo(app, token, deviceID, method, path string, body any) (int, map[string]any) {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, srv.URL+"/v1"+path, rd)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-App", app)
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

// ledgerRefunds counts the refunds owed (written when the booking ends) and refundsPaid those the gateway has accepted.
func ledgerRefunds(id string) int {
	var n int
	pool.QueryRow(context.Background(), `SELECT count(*) FROM ledger_transactions WHERE booking_id = $1 AND kind = 'refund' AND memo NOT LIKE 'refund paid%'`, id).Scan(&n)
	return n
}

func takenOn(t *testing.T, room string, night int) int {
	t.Helper()
	h, b := heldOn(t, room, night)
	return h + b
}

// ---- S2 and S1: hotel pages and names

func TestAudit_S2_04_AHotelWithNoSellableRoomTypeStillOpensAndIsListed(t *testing.T) {
	c := auditGuest(t)
	piassa := "a0000000-0000-0000-0000-000000000003" // its only room type has 0 rooms
	r := c.do("GET", fmt.Sprintf("/properties/%s?check_in=%s&check_out=%s", piassa, day(240), day(242)), nil)
	if r.status != 200 {
		t.Fatalf("S2.4: a hotel with no sellable room must still open: %d %s", r.status, r.raw)
	}
	rooms, _ := r.body["room_types"].([]any)
	for _, rt := range rooms {
		if m := rt.(map[string]any); m["rooms_left"].(float64) > 0 {
			t.Fatalf("S2.4: a room type with 0 rooms must show none left: %v", m)
		}
	}
	list := c.do("GET", fmt.Sprintf("/search?q=piassa&check_in=%s&check_out=%s&guests=1&rooms=1", day(240), day(242)), nil)
	if it := itemFor(list, piassa); list.status != 200 || it == nil || it["available"] != false {
		t.Fatalf("S2.4: the hotel is listed as unavailable: %d %v", list.status, it)
	}
}

func TestAudit_S1_16_NamesInAnyScriptAreFoundCaseInsensitively(t *testing.T) {
	c := auditGuest(t)
	ctx := context.Background()
	var enc string
	pool.QueryRow(ctx, `SELECT pg_encoding_to_char(encoding) FROM pg_database WHERE datname = current_database()`).Scan(&enc)
	if enc != "UTF8" {
		t.Skipf("S1.16 cannot be checked here: the embedded test database is %s, not UTF8 (production Supabase is UTF8); check on a UTF8 database", enc)
	}
	defer pool.Exec(ctx, `UPDATE properties SET name = 'Bole Garden Guesthouse' WHERE id = $1`, bole)
	for _, tc := range []struct{ name, query string }{
		{"ቦሌ የእንግዳ ማረፊያ", "እንግዳ"},
		{"ボレ ガーデン ホテル", "ガーデン"},
		{"فندق بولي الحديقة", "بولي"},
		{"Café Ünïcode Résidence", "ÜNÏCODE"},
	} {
		if _, err := pool.Exec(ctx, `UPDATE properties SET name = $2 WHERE id = $1`, bole, tc.name); err != nil {
			t.Fatalf("setup: %v", err)
		}
		r := c.do("GET", fmt.Sprintf("/search?q=%s&check_in=%s&check_out=%s&guests=1&rooms=1", urlEscape(tc.query), day(245), day(246)), nil)
		if itemFor(r, bole) == nil {
			t.Errorf("S1.16: searching %q must find the hotel named %q", tc.query, tc.name)
		}
	}
}

func urlEscape(s string) string {
	return strings.NewReplacer(" ", "%20").Replace(strings.ReplaceAll(fmt.Sprintf("%s", urlQuery(s)), "+", "%20"))
}

func urlQuery(s string) string {
	var b strings.Builder
	for _, c := range []byte(s) {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.' {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

// ---- S3: promo codes

func TestAudit_S3_06_ACouponCodeIsAcceptedInAnyLetterCase(t *testing.T) {
	admin := adminClient(t)
	code := fmt.Sprintf("CASE%d", time.Now().UnixNano()%1000000)
	if r := admin.do("POST", "/admin/coupons", map[string]any{"code": code, "kind": "percent", "value": 1000, "per_user_limit": 5, "active": true}); r.status != 200 {
		t.Fatalf("setup: %d %s", r.status, r.raw)
	}
	c := auditGuest(t)
	for _, typed := range []string{strings.ToLower(code), strings.ToUpper(code), " " + code + " "} {
		q := quoteWith(c, meskel, standard, 295, 296, 1, 1, map[string]any{"coupon_code": typed})
		if q.status != 200 || q.num("discount_santim") <= 0 {
			t.Errorf("S3.6: the code typed as %q must work: %d %s", typed, q.status, q.raw)
		}
	}
}

// ---- S5: confirm and cancel racing

func TestAudit_S5_05_ConfirmingAndCancellingTogetherNeverLeaveAHalfState(t *testing.T) {
	hotel := hotelFor(t)
	for i := 0; i < 6; i++ {
		g := auditGuest(t)
		night := 270 + i
		before := takenOn(t, standard, night)
		id := hold(g, quote(g, meskel, standard, night, night+1, 1, 1).str("id"), fmt.Sprintf("aud-s5-5-%d-%d", i, time.Now().UnixNano())).str("id")
		payOnline(t, g, id, "succeeded")
		var wg sync.WaitGroup
		wg.Add(2)
		start := make(chan struct{})
		go func() {
			defer wg.Done()
			<-start
			rawDo("partner", hotel.token, hotel.deviceID, "POST", "/partner/bookings/"+id+"/accept", nil)
		}()
		go func() {
			defer wg.Done()
			<-start
			rawDo("guest", g.token, g.deviceID, "POST", "/bookings/"+id+"/cancel", nil)
		}()
		close(start)
		wg.Wait()
		final := g.do("GET", "/bookings/"+id, nil).str("status")
		after := takenOn(t, standard, night)
		refunds := ledgerRefunds(id)
		switch final {
		case "CONFIRMED":
			if after != before+1 || refunds != 0 {
				t.Errorf("S5.5 run %d: confirmed means one room taken and no refund: taken %d->%d refunds %d", i, before, after, refunds)
			}
		case "CANCELLED_REFUNDED":
			if after != before || refunds != 1 {
				t.Errorf("S5.5 run %d: cancelled means the room is back exactly once and one refund: taken %d->%d refunds %d", i, before, after, refunds)
			}
		default:
			t.Errorf("S5.5 run %d: the only possible outcomes are CONFIRMED or CANCELLED_REFUNDED, got %s", i, final)
		}
	}
}

func TestAudit_S5_05b_TwentyCancelsAtOnceRefundOnce(t *testing.T) {
	hotel := hotelFor(t)
	g := auditGuest(t)
	id, _, _, _ := confirmedBooking(t, g, hotel, 277, 279, fmt.Sprintf("aud-s5-5b-%d", time.Now().UnixNano()))
	var wg sync.WaitGroup
	start := make(chan struct{})
	ok := 0
	var mu sync.Mutex
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if status, _ := rawDo("guest", g.token, g.deviceID, "POST", "/bookings/"+id+"/cancel", nil); status == 200 {
				mu.Lock()
				ok++
				mu.Unlock()
			}
		}()
	}
	close(start)
	wg.Wait()
	if refunds := ledgerRefunds(id); refunds != 1 {
		t.Fatalf("S5.5: twenty simultaneous cancels must refund exactly once, got %d refunds (%d answered 200)", refunds, ok)
	}
}

// ---- S6: payment results out of order, and a refund that fails

func TestAudit_S6_06_AGatewayResultThatContradictsAnEarlierOneIsHandledSafely(t *testing.T) {
	post := func(url, outcome string) {
		res, err := http.PostForm(url, map[string][]string{"outcome": {outcome}})
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
	}
	// Succeeded first, then a late "failed": the money was received, so the booking must stay paid.
	g := auditGuest(t)
	id := hold(g, quote(g, bole, boleSingle, 247, 248, 1, 1).str("id"), fmt.Sprintf("aud-s6-6a-%d", time.Now().UnixNano())).str("id")
	r := g.do("POST", "/bookings/"+id+"/pay", map[string]string{"method": "telebirr"})
	url := strings.Replace(r.str("session", "payment_url"), "http://example.test", srv.URL, 1)
	post(url, "succeeded")
	post(url, "failed")
	if s := g.do("GET", "/bookings/"+id, nil).str("status"); s != "PAID_AWAITING_HOTEL" || paymentStatus(t, id) != "succeeded" {
		t.Errorf("S6.6: a late failure must not undo a payment that succeeded: booking %s, payment %s", s, paymentStatus(t, id))
	}

	// Failed first, then "succeeded": money arrived after we recorded a failure. It must not be lost: the booking is paid, or the money is refunded.
	g2 := auditGuest(t)
	id2 := hold(g2, quote(g2, bole, boleSingle, 249, 250, 1, 1).str("id"), fmt.Sprintf("aud-s6-6b-%d", time.Now().UnixNano())).str("id")
	r2 := g2.do("POST", "/bookings/"+id2+"/pay", map[string]string{"method": "telebirr"})
	url2 := strings.Replace(r2.str("session", "payment_url"), "http://example.test", srv.URL, 1)
	post(url2, "failed")
	post(url2, "succeeded")
	s2, p2 := g2.do("GET", "/bookings/"+id2, nil).str("status"), paymentStatus(t, id2)
	paidAndKept := s2 == "PAID_AWAITING_HOTEL" && p2 == "succeeded"
	refunded := p2 == "refunded"
	if !paidAndKept && !refunded {
		t.Errorf("S6.6: the gateway reported success after an earlier failure, but the payment is %q and the booking %s: money received is neither kept as a paid booking nor refunded", p2, s2)
	}
}

func chapaAuditServer(t *testing.T) (*fakeChapa, func()) {
	t.Helper()
	fake := &fakeChapa{}
	chapaSrv := httptest.NewServer(fake.handler())
	const priv = "CHAPA_TEST_PRIV_audit"
	gw, err := api.NewChapaGateway(priv, "whsec-audit", "http://example.test", chapaSrv.URL)
	if err != nil {
		t.Fatal(err)
	}
	restore := auditServer(t, func(c *config.Config) { c.PaymentProvider, c.ChapaSecretKey = "chapa", priv }, func(p *api.Providers) { p.Gateway = gw })
	return fake, func() { restore(); chapaSrv.Close() }
}

func TestAudit_S6_10_AFailedRefundIsRetriedUntilItGoesThrough(t *testing.T) {
	fake, done := chapaAuditServer(t)
	defer done()
	hotel := hotelFor(t)
	g := auditGuest(t)
	id := hold(g, quote(g, meskel, standard, 281, 282, 1, 1).str("id"), fmt.Sprintf("aud-s6-10-%d", time.Now().UnixNano())).str("id")
	if r := g.do("POST", "/bookings/"+id+"/pay", map[string]string{"method": "chapa"}); r.status != 200 {
		t.Fatalf("setup: pay %d %s", r.status, r.raw)
	}
	fake.mu.Lock()
	ref, _ := fake.hosted["merchant_reference"].(string)
	fake.mu.Unlock()
	b := g.do("GET", "/bookings/"+id, nil)
	amount := int64(b.num("deposit_santim"))
	if amount == 0 {
		amount = int64(b.num("total_santim"))
	}
	fake.answer("SUCCESS", amount, "ETB")
	if res, err := http.Get(srv.URL + "/v1/payments/chapa/return?tx_ref=" + ref); err != nil {
		t.Fatal(err)
	} else {
		res.Body.Close()
	}
	if s := g.do("GET", "/bookings/"+id, nil).str("status"); s != "PAID_AWAITING_HOTEL" {
		t.Fatalf("setup: paid booking expected, got %s", s)
	}

	fake.mu.Lock()
	fake.failRefunds = true
	fake.mu.Unlock()
	if r := hotel.do("POST", "/partner/bookings/"+id+"/decline", map[string]string{"reason": "fully_booked"}); r.status != 200 {
		t.Fatalf("decline: %d %s", r.status, r.raw)
	}
	if s := g.do("GET", "/bookings/"+id, nil).str("status"); s != "DECLINED_REFUNDED" {
		t.Fatalf("the hotel declined: %s", s)
	}
	if p := paymentStatus(t, id); p != "succeeded" {
		t.Fatalf("S6.10: while the gateway refuses the refund the payment must stay owed, not be marked refunded: %q", p)
	}
	if refundsPaid(id) != 0 {
		t.Fatalf("S6.10: no refund may be recorded as paid before the gateway accepts it")
	}

	fake.mu.Lock()
	fake.failRefunds = false
	fake.mu.Unlock()
	hotel.do("GET", "/partner/bookings", nil) // the background sweep retries
	if p := paymentStatus(t, id); p != "refunded" || refundsPaid(id) != 1 {
		t.Errorf("S6.10: once the gateway recovers the refund goes through exactly once: payment %q, refunds paid %d", p, refundsPaid(id))
	}
	fake.mu.Lock()
	tries, sent := fake.refundTries, len(fake.refunds)
	fake.mu.Unlock()
	if tries < 1 || sent != 1 {
		t.Errorf("S6.10: at least one failed try, then exactly one accepted refund: tries=%d accepted=%d", tries, sent)
	}
}

// ---- S9: hiding a hotel that has bookings to come

func TestAudit_S9_03_HidingAHotelLeavesItsFutureBookingsWorkingAndTellsTheAdmin(t *testing.T) {
	admin := adminClient(t)
	hotel := hotelFor(t)
	g := auditGuest(t)
	id, _, _, _ := confirmedBooking(t, g, hotel, 283, 285, fmt.Sprintf("aud-s9-3-%d", time.Now().UnixNano()))

	got := admin.do("GET", "/admin/hotels/"+meskel, nil)
	if got.status != 200 {
		t.Fatalf("setup: %d %s", got.status, got.raw)
	}
	body := got.body
	body["status"] = "hidden"
	defer pool.Exec(context.Background(), `UPDATE properties SET status = 'active' WHERE id = $1`, meskel)
	put := admin.do("PUT", "/admin/hotels/"+meskel, body)
	if put.status >= 500 {
		t.Fatalf("hiding must not fail: %d %s", put.status, put.raw)
	}
	if put.status == 400 {
		t.Fatalf("setup: the update body was refused: %s", put.raw)
	}
	// The guest's booking and voucher still work.
	if r := g.do("GET", "/bookings/"+id, nil); r.status != 200 {
		t.Errorf("S9.3: the booking must stay readable: %d", r.status)
	}
	if r := g.do("GET", "/bookings/"+id+"/voucher", nil); r.status != 200 {
		t.Errorf("S9.3: the voucher must keep working: %d", r.status)
	}
	// The admin is told about the bookings that are affected, and nothing is hidden until they confirm.
	if put.status != 409 || put.str("error", "code") != "HAS_UPCOMING_BOOKINGS" {
		t.Fatalf("S9.3: hiding a hotel with bookings to come must be refused until confirmed: %d %s", put.status, put.raw)
	}
	var st string
	pool.QueryRow(context.Background(), `SELECT status FROM properties WHERE id = $1`, meskel).Scan(&st)
	if st != "active" {
		t.Errorf("S9.3: the hotel must stay listed until the admin confirms: %s", st)
	}
	body["confirm_hide"] = true
	if r := admin.do("PUT", "/admin/hotels/"+meskel, body); r.status != 200 {
		t.Fatalf("S9.3: a confirmed hide goes through: %d %s", r.status, r.raw)
	}
	if r := g.do("GET", "/bookings/"+id+"/voucher", nil); r.status != 200 {
		t.Errorf("S9.3: the voucher keeps working after the hotel is hidden: %d", r.status)
	}
}

// ---- S10: notifications

type failingMailer struct{}

func (failingMailer) SendMail(context.Context, string, string, string, string) error {
	return errors.New("mail server is down")
}

func countSubject(email, contains string) int {
	n := 0
	for _, m := range mails.to(email) {
		if strings.Contains(m.subject, contains) {
			n++
		}
	}
	return n
}

func realGuest(t *testing.T, prefix string) (*client, string) {
	t.Helper()
	email := realAddress(prefix)
	g := newClient(t, "guest")
	signed := signInWithEmailedCode(g, "/auth/otp/request", "/auth/otp/verify", email)
	if signed.status != 200 {
		t.Fatalf("sign in: %d %s", signed.status, signed.raw)
	}
	g.token = signed.str("token")
	withProfile(g)
	return g, email
}

func TestAudit_S10_01_EachEmailIsSentExactlyOnceEvenWhenTheActionIsRepeated(t *testing.T) {
	hotel := hotelFor(t)
	g, email := realGuest(t, "once")
	id := hold(g, quote(g, meskel, standard, 287, 288, 1, 1).str("id"), fmt.Sprintf("aud-s10-1-%d", time.Now().UnixNano())).str("id")
	payOnline(t, g, id, "succeeded")
	if n := countSubject(email, "Your booking request"); n != 1 {
		t.Errorf("S10.1: the request email is sent once: %d", n)
	}
	hotel.do("POST", "/partner/bookings/"+id+"/accept", nil)
	hotel.do("POST", "/partner/bookings/"+id+"/accept", nil) // repeated
	hotel.do("GET", "/partner/bookings", nil)                // the sweep runs
	if n := countSubject(email, "Booking confirmed"); n != 1 {
		t.Errorf("S10.1: the confirmation email is sent once even if the hotel accepts twice: %d", n)
	}
	g.do("POST", "/bookings/"+id+"/cancel", nil)
	g.do("POST", "/bookings/"+id+"/cancel", nil) // repeated
	if n := countSubject(email, "cancel"); n > 1 {
		t.Errorf("S10.1: the cancellation email is sent at most once: %d", n)
	}
}

func TestAudit_S10_02_AFailedEmailNeverUndoesTheBooking(t *testing.T) {
	defer auditServer(t, func(*config.Config) {}, func(p *api.Providers) { p.Mail = failingMailer{} })()
	hotel := hotelFor(t)
	g, _ := realGuest(t, "mailfail")
	id := hold(g, quote(g, meskel, standard, 289, 290, 1, 1).str("id"), fmt.Sprintf("aud-s10-2-%d", time.Now().UnixNano())).str("id")
	payOnline(t, g, id, "succeeded")
	if r := hotel.do("POST", "/partner/bookings/"+id+"/accept", nil); r.status != 200 {
		t.Fatalf("S10.2: accepting must work when email is down: %d %s", r.status, r.raw)
	}
	if s := g.do("GET", "/bookings/"+id, nil).str("status"); s != "CONFIRMED" {
		t.Fatalf("S10.2: the booking stays confirmed when its email cannot be sent: %s", s)
	}
	if r := g.do("GET", "/bookings/"+id+"/voucher", nil); r.status != 200 {
		t.Fatalf("S10.2: the voucher is available: %d", r.status)
	}
}

// ---- S11: reviews

func TestAudit_S11_01_ReviewRatingBoundsAndOwnership(t *testing.T) {
	hotel := hotelFor(t)
	g := auditGuest(t)
	other := auditGuest(t)
	id, _, _, _ := confirmedBooking(t, g, hotel, 291, 293, fmt.Sprintf("aud-s11-%d", time.Now().UnixNano()))
	if r := g.do("POST", "/bookings/"+id+"/review", map[string]any{"rating": 5, "text": "early"}); r.status < 400 {
		t.Errorf("S11: a review before the stay is over must be refused: %d", r.status)
	}
	pool.Exec(context.Background(), fmt.Sprintf(`UPDATE bookings SET check_in = %s - 6, check_out = %s - 4 WHERE id = $1`, addisToday, addisToday), id)
	app.RunMaintenance(context.Background())
	for _, rating := range []int{0, 6, -1, 100} {
		if r := g.do("POST", "/bookings/"+id+"/review", map[string]any{"rating": rating, "text": "x"}); r.status != 400 {
			t.Errorf("S11: rating %d must be refused with 400, got %d", rating, r.status)
		}
	}
	if r := g.do("POST", "/bookings/"+id+"/review", map[string]any{"rating": 4, "text": strings.Repeat("a", 1001)}); r.status != 400 {
		t.Errorf("S11: a 1001-character review must be refused: %d", r.status)
	}
	if r := other.do("POST", "/bookings/"+id+"/review", map[string]any{"rating": 5, "text": "not mine"}); r.status != 404 {
		t.Errorf("S11: another guest cannot review this stay: %d", r.status)
	}
	if r := g.do("POST", "/bookings/"+id+"/review", map[string]any{"rating": 4, "text": "Clean rooms <script>alert(1)</script>"}); r.status != 200 {
		t.Fatalf("S11: a valid review is accepted: %d %s", r.status, r.raw)
	}
	if r := g.do("POST", "/bookings/"+id+"/review", map[string]any{"rating": 5, "text": "again"}); r.status < 400 {
		t.Errorf("S11: one review per stay: %d", r.status)
	}
}

// ---- S15: a booking spike

func TestAudit_S15_04_SixtyGuestsBookAtTheSameInstant(t *testing.T) {
	type guest struct{ token, device, quote string }
	const n = 60
	guests := make([]guest, n)
	for i := range guests {
		c := auditGuest(t)
		night := 352 + i/5 // the Standard Double has 5 rooms a night, so each night takes exactly 5 guests
		q := quote(c, meskel, standard, night, night+1, 1, 1)
		if q.status != 200 {
			t.Fatalf("setup: quote %d: %d %s", i, q.status, q.raw)
		}
		guests[i] = guest{c.token, c.deviceID, q.str("id")}
	}
	var mu sync.Mutex
	var lat []time.Duration
	ok, other := 0, 0
	start := make(chan struct{})
	var wg sync.WaitGroup
	t0 := time.Now()
	for i, g := range guests {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			s := time.Now()
			status, _ := rawHold(g.token, g.device, g.quote, fmt.Sprintf("aud-s15-%d-%d", i, time.Now().UnixNano()))
			d := time.Since(s)
			mu.Lock()
			lat = append(lat, d)
			if status == 200 {
				ok++
			} else {
				other++
			}
			mu.Unlock()
		}()
	}
	close(start)
	wg.Wait()
	sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })
	p := func(q float64) time.Duration { return lat[int(float64(len(lat)-1)*q)] }
	t.Logf("S15.4 SPIKE (embedded local PostgreSQL, one machine): %d simultaneous bookings in %s; p50=%s p95=%s p99=%s; ok=%d failed=%d",
		n, time.Since(t0).Round(time.Millisecond), p(.50), p(.95), p(.99), ok, other)
	if ok != n {
		t.Errorf("S15.4: every guest had a free room, all %d should succeed: ok=%d failed=%d", n, ok, other)
	}
	for night := 352; night < 352+n/5; night++ {
		if h, b := heldOn(t, standard, night); h+b != 5 {
			t.Errorf("S15.4: night %d should have exactly 5 rooms taken: %d", night, h+b)
		}
	}
}

// ---- S17: logs

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}

func TestAudit_S17_01_LogsHoldNoTokensAndNoQueryValues(t *testing.T) {
	g := auditGuest(t)
	var logs lockedBuffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	defer slog.SetDefault(old)

	g.do("GET", "/search?q=bole&secret_param=TOPSECRETVALUE123&check_in="+day(250)+"&check_out="+day(251)+"&guests=1&rooms=1", nil)
	g.do("GET", "/bookings/not-a-uuid", nil)                      // an error path
	g.do("POST", "/bookings", map[string]string{"quote_id": "x"}) // a bad body
	out := logs.String()
	if !strings.Contains(out, `"path"`) || !strings.Contains(out, "/v1/search") {
		t.Fatalf("S17.1: requests are logged with their path: %s", out)
	}
	for _, secret := range []string{g.token, "TOPSECRETVALUE123", "Bearer "} {
		if strings.Contains(out, secret) {
			t.Errorf("S17.1: the logs must not contain %q", secret[:min(len(secret), 12)])
		}
	}
	if !strings.Contains(out, `"request_id"`) {
		t.Errorf("S17.1: every request log carries a request id")
	}
}
