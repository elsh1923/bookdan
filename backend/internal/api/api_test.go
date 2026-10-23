package api_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/bookdan/backend/internal/api"
	"github.com/bookdan/backend/internal/config"
	"github.com/bookdan/backend/internal/db"
	"github.com/bookdan/backend/internal/dbtest"
	"github.com/bookdan/backend/internal/server"
)

var (
	pool *pgxpool.Pool
	srv  *httptest.Server
	app  *api.Server
	keys *api.Keys
	push = &recorder{}
)

const (
	meskel     = "a0000000-0000-0000-0000-000000000001" // Meskel Square Palace Hotel, pay online only
	standard   = "b0000000-0000-0000-0000-000000000001" // 5 Standard Doubles
	family     = "b0000000-0000-0000-0000-000000000003" // 1 Family Suite
	bole       = "a0000000-0000-0000-0000-000000000002" // Bole Garden, takes pay at hotel
	boleSingle = "b0000000-0000-0000-0000-000000000004"
)

func TestMain(m *testing.M) {
	conn, stop, err := dbtest.Start()
	if err != nil {
		fmt.Println("test database:", err)
		os.Exit(1)
	}
	if _, err := conn.Exec(`SET search_path = app, extensions, public; ` + dbtest.SeedSQL()); err != nil {
		fmt.Println("seed:", err)
		stop()
		os.Exit(1)
	}
	ctx := context.Background()
	if pool, err = db.Open(ctx, dbtest.URL); err != nil {
		fmt.Println("pool:", err)
		stop()
		os.Exit(1)
	}
	cfg := config.Config{Env: "test", DemoMode: true, PublicURL: "http://example.test", PaymentProvider: "dev", PaymentWebhookSecret: "whsec"}
	keys, _ = api.NewKeys(cfg)
	providers := api.DevProviders(cfg)
	providers.Push = push // records every alert so tests can read them
	app, err = api.New(ctx, cfg, pool, keys, providers)
	if err != nil {
		fmt.Println("api:", err)
		stop()
		os.Exit(1)
	}
	srv = httptest.NewServer(server.New(cfg, pool, func(r chi.Router) { app.Routes(r) }))
	code := m.Run()
	srv.Close()
	pool.Close()
	stop()
	os.Exit(code)
}

// ---- a tiny client

type client struct {
	t        *testing.T
	token    string
	refresh  string
	appName  string
	deviceID string
}

func newClient(t *testing.T, appName string) *client {
	return &client{t: t, appName: appName, deviceID: fmt.Sprintf("device-%s-%d", appName, time.Now().UnixNano())}
}

type resp struct {
	status int
	body   map[string]any
	raw    []byte
}

func (r resp) str(path ...string) string {
	var cur any = r.body
	for _, p := range path {
		m, _ := cur.(map[string]any)
		cur = m[p]
	}
	s, _ := cur.(string)
	return s
}

func (r resp) num(key string) float64 { f, _ := r.body[key].(float64); return f }

func (c *client) do(method, path string, body any, headers ...string) resp {
	c.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, srv.URL+"/v1"+path, rd)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-App", c.appName)
	req.Header.Set("X-Device-Id", c.deviceID)
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	out := resp{status: res.StatusCode, raw: raw}
	_ = json.Unmarshal(raw, &out.body)
	return out
}

func (c *client) signIn(typ, value string) {
	c.t.Helper()
	if r := c.do("POST", "/auth/otp/request", map[string]string{"type": typ, "value": value}); r.status != 200 {
		c.t.Fatalf("otp request: %d %s", r.status, r.raw)
	}
	r := c.do("POST", "/auth/otp/verify", map[string]string{"type": typ, "value": value, "code": "123456"})
	if r.status != 200 {
		c.t.Fatalf("otp verify: %d %s", r.status, r.raw)
	}
	c.token, c.refresh = r.str("token"), r.str("refresh_token")
}

func uniqueEmail(prefix string) string {
	return fmt.Sprintf("%s%d@example.com", prefix, time.Now().UnixNano())
}

func day(offset int) string {
	return time.Now().UTC().Add(3*time.Hour).AddDate(0, 0, offset).Format("2006-01-02")
}

func withProfile(c *client) {
	c.t.Helper()
	r := c.do("PUT", "/me/profile", map[string]string{"full_name": "Hanna Tesfaye", "id_type": "national_id", "id_number": "4829 1730 5561", "nationality": "Ethiopian"})
	if r.status != 200 {
		c.t.Fatalf("profile: %d %s", r.status, r.raw)
	}
}

func quote(c *client, property, room string, in, out, guests, rooms int) resp {
	return c.do("POST", "/quotes", map[string]any{"property_id": property, "room_type_id": room, "check_in": day(in), "check_out": day(out), "guests": guests, "rooms": rooms})
}

func hold(c *client, quoteID, key string) resp {
	return c.do("POST", "/bookings", map[string]string{"quote_id": quoteID, "guest_name": "Hanna Tesfaye", "idempotency_key": key})
}

// payOnline holds and starts an online payment, then plays the gateway page's result.
func payOnline(t *testing.T, c *client, bookingID string, outcome string) {
	t.Helper()
	r := c.do("POST", "/bookings/"+bookingID+"/pay", map[string]string{"method": "telebirr"})
	if r.status != 200 || r.str("session", "payment_url") == "" {
		t.Fatalf("pay: %d %s", r.status, r.raw)
	}
	res, err := http.PostForm(strings.Replace(r.str("session", "payment_url"), "http://example.test", srv.URL, 1), url.Values{"outcome": {outcome}})
	if err != nil {
		t.Fatal(err)
	}
	if res != nil {
		res.Body.Close()
	}
}

// hotelFor signs in as the demo owner (the staff member of Meskel Square Palace Hotel).
func hotelFor(t *testing.T) *client {
	// Many tests sign the owner in; start each from a clean slate so the sign-in limit (5 codes / 10 min) does not trip.
	pool.Exec(context.Background(), `DELETE FROM otp_challenges WHERE identifier = 'owner@bookdan.test'`)
	c := newClient(t, "partner")
	c.signIn("email", "owner@bookdan.test")
	return c
}

func ledgerBalance(t *testing.T, account string) int64 {
	var n int64
	if err := pool.QueryRow(context.Background(), `SELECT coalesce(sum(balance_santim), 0) FROM ledger_balances WHERE account_code = $1`, account).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func heldBooked(t *testing.T, room string, night string) (held, booked int) {
	if err := pool.QueryRow(context.Background(), `SELECT held, booked FROM inventory WHERE room_type_id = $1 AND night = $2::date`, room, night).Scan(&held, &booked); err != nil {
		t.Fatal(err)
	}
	return
}

// ---- sign-in

func TestSignInRefreshAndReuseDetection(t *testing.T) {
	c := newClient(t, "guest")
	email := uniqueEmail("a")
	c.signIn("email", email)
	if r := c.do("GET", "/bookings", nil); r.status != 200 {
		t.Fatalf("a signed-in guest should list bookings: %d %s", r.status, r.raw)
	}
	old := c.refresh
	r := c.do("POST", "/auth/refresh", map[string]string{"refresh_token": old})
	if r.status != 200 || r.str("refresh_token") == old {
		t.Fatalf("refresh should rotate the token: %d %s", r.status, r.raw)
	}
	newRefresh := r.str("refresh_token")
	// Presenting the old (already used) token again means a copy leaked: it fails and the whole session ends.
	if r := c.do("POST", "/auth/refresh", map[string]string{"refresh_token": old}); r.status != 401 {
		t.Fatalf("reused refresh token must fail, got %d", r.status)
	}
	if r := c.do("POST", "/auth/refresh", map[string]string{"refresh_token": newRefresh}); r.status != 401 {
		t.Fatalf("the family must be revoked after reuse, got %d", r.status)
	}
}

func TestWrongCodesLockTheChallengeAndBadInputIsRefused(t *testing.T) {
	c := newClient(t, "guest")
	email := uniqueEmail("b")
	c.do("POST", "/auth/otp/request", map[string]string{"type": "email", "value": email})
	for i := 0; i < 5; i++ {
		if r := c.do("POST", "/auth/otp/verify", map[string]string{"type": "email", "value": email, "code": "000000"}); r.status != 401 {
			t.Fatalf("wrong code attempt %d: %d", i, r.status)
		}
	}
	// Even the right code no longer works: the challenge is spent after five wrong tries.
	if r := c.do("POST", "/auth/otp/verify", map[string]string{"type": "email", "value": email, "code": "123456"}); r.status != 401 {
		t.Fatalf("a locked challenge must refuse the right code too, got %d", r.status)
	}
	if r := c.do("POST", "/auth/otp/request", map[string]string{"type": "phone", "value": "0911223344"}); r.str("error", "code") != "INVALID_PHONE" {
		t.Fatalf("a local-format number must be refused: %s", r.raw)
	}
	if r := c.do("POST", "/auth/otp/request", map[string]string{"type": "email", "value": "nope"}); r.str("error", "code") != "INVALID_EMAIL" {
		t.Fatalf("a bad email must be refused: %s", r.raw)
	}
}

func TestOnlyHotelStaffCanUseThePartnerApp(t *testing.T) {
	guest := newClient(t, "partner")
	email := uniqueEmail("c")
	guest.do("POST", "/auth/otp/request", map[string]string{"type": "email", "value": email})
	if r := guest.do("POST", "/auth/otp/verify", map[string]string{"type": "email", "value": email, "code": "123456"}); r.status != 403 || r.str("error", "code") != "NOT_STAFF" {
		t.Fatalf("a stranger must not get into the partner app: %d %s", r.status, r.raw)
	}
	// A guest token cannot call partner routes, and a partner token cannot call guest routes.
	g := newClient(t, "guest")
	g.signIn("email", uniqueEmail("d"))
	if r := g.do("GET", "/partner/bookings", nil); r.status != 401 {
		t.Fatalf("guest token on a partner route: %d", r.status)
	}
	h := hotelFor(t)
	if r := h.do("GET", "/bookings", nil); r.status != 401 {
		t.Fatalf("partner token on a guest route: %d", r.status)
	}
	if r := newClient(t, "guest").do("GET", "/bookings", nil); r.status != 401 {
		t.Fatalf("no token: %d", r.status)
	}
}

// ---- catalogue

func TestSearchAndPropertyDetail(t *testing.T) {
	c := newClient(t, "guest")
	r := c.do("GET", fmt.Sprintf("/search?q=bole&check_in=%s&check_out=%s&guests=1", day(3), day(4)), nil)
	items, _ := r.body["items"].([]any)
	if r.status != 200 || len(items) == 0 {
		t.Fatalf("search: %d %s", r.status, r.raw)
	}
	first := items[0].(map[string]any)
	if first["name"] != "Bole Garden Guesthouse" || first["from_price_santim"].(float64) != 140000 || first["available"] != true {
		t.Fatalf("unexpected first result: %v", first)
	}
	if first["lat"].(float64) == 0 || first["lng"].(float64) == 0 {
		t.Fatalf("coordinates are missing: %v", first)
	}

	// Filters: only lodges.
	r = c.do("GET", fmt.Sprintf("/search?types=lodge&check_in=%s&check_out=%s&guests=1", day(3), day(4)), nil)
	for _, it := range r.body["items"].([]any) {
		if it.(map[string]any)["type"] != "lodge" {
			t.Fatalf("filter let a non-lodge through: %v", it)
		}
	}
	// A party of 9 in one room fits nowhere, so every hotel is shown as not available.
	r = c.do("GET", fmt.Sprintf("/search?check_in=%s&check_out=%s&guests=9&rooms=1", day(3), day(4)), nil)
	for _, it := range r.body["items"].([]any) {
		if it.(map[string]any)["available"] == true {
			t.Fatalf("nobody should be available for 9 guests in one room: %v", it)
		}
	}
	if r := c.do("GET", "/search?check_in=2026-13-01&check_out=2026-13-02", nil); r.status != 400 {
		t.Fatalf("bad dates: %d", r.status)
	}

	d := c.do("GET", fmt.Sprintf("/properties/%s?check_in=%s&check_out=%s", meskel, day(3), day(4)), nil)
	rooms, _ := d.body["room_types"].([]any)
	if d.status != 200 || len(rooms) != 3 {
		t.Fatalf("property: %d %s", d.status, d.raw)
	}
	if cheapest := rooms[0].(map[string]any); cheapest["name"] != "Standard Double" || cheapest["rooms_left"].(float64) != 5 {
		t.Fatalf("rooms_left should count the free rooms: %v", cheapest)
	}
	if r := c.do("GET", fmt.Sprintf("/properties/not-a-uuid?check_in=%s&check_out=%s", day(3), day(4)), nil); r.status != 404 {
		t.Fatalf("unknown property: %d", r.status)
	}
}

// ---- the booking journey, start to finish

func TestFullBookingJourney(t *testing.T) {
	guest := newClient(t, "guest")
	guest.signIn("email", uniqueEmail("g"))
	hotel := hotelFor(t)

	q := quote(guest, meskel, standard, 10, 12, 2, 1)
	if q.status != 200 || q.num("total_santim") != 2*320000*115/100 || q.num("rooms") != 1 {
		t.Fatalf("quote: %d %s", q.status, q.raw)
	}
	// The hotel needs the guest's ID, so the hold is refused until the guest has saved one.
	if r := hold(guest, q.str("id"), "key-journey-1"); r.str("error", "code") != "PROFILE_INCOMPLETE" {
		t.Fatalf("hold without a profile: %s", r.raw)
	}
	withProfile(guest)
	b := hold(guest, q.str("id"), "key-journey-1")
	if b.status != 200 || b.str("status") != "HELD" || b.str("hold_expires_at") == "" || b.str("reference") == "" {
		t.Fatalf("hold: %d %s", b.status, b.raw)
	}
	id := b.str("id")

	// Retrying the same attempt returns the same booking; reusing the key for another request is an error.
	if again := hold(guest, q.str("id"), "key-journey-1"); again.str("id") != id {
		t.Fatalf("a retry must return the same booking: %s", again.raw)
	}
	other := quote(guest, meskel, standard, 20, 21, 1, 1)
	if r := hold(guest, other.str("id"), "key-journey-1"); r.str("error", "code") != "IDEMPOTENCY_MISMATCH" {
		t.Fatalf("same key, different request: %s", r.raw)
	}
	if held, _ := heldBooked(t, standard, day(10)); held < 1 {
		t.Fatalf("the room should be held on every night")
	}
	// The hotel sees nothing yet: the guest has not paid.
	if list := hotel.do("GET", "/partner/bookings", nil); strings.Contains(string(list.raw), b.str("reference")) {
		t.Fatalf("a held booking must be invisible to the hotel")
	}

	// A failed payment leaves the hold in place so the guest can try again.
	payOnline(t, guest, id, "failed")
	if r := guest.do("GET", "/bookings/"+id, nil); r.str("status") != "HELD" || r.str("payment") != "failed" {
		t.Fatalf("after a failed payment: %s", r.raw)
	}
	payOnline(t, guest, id, "succeeded")
	if r := guest.do("GET", "/bookings/"+id, nil); r.str("status") != "PAID_AWAITING_HOTEL" || r.str("payment_method") != "telebirr" {
		t.Fatalf("after payment: %s", r.raw)
	}
	if r := guest.do("GET", "/bookings/"+id+"/voucher", nil); r.str("error", "code") != "VOUCHER_NOT_READY" {
		t.Fatalf("the voucher must wait for the hotel: %s", r.raw)
	}
	if bal := ledgerBalance(t, "guest_deposits_held"); bal >= 0 {
		t.Fatalf("the guest's deposit should be held as a liability, balance %d", bal)
	}

	// The hotel sees the request with the guest's profile (masked), reveals the ID (audited), and accepts.
	list := hotel.do("GET", "/partner/bookings", nil)
	var mine map[string]any
	for _, it := range list.body["items"].([]any) {
		if it.(map[string]any)["id"] == id {
			mine = it.(map[string]any)
		}
	}
	if mine == nil || mine["status"] != "PAID_AWAITING_HOTEL" || mine["hotel_deadline_at"] == nil {
		t.Fatalf("the hotel should see the paid request: %s", list.raw)
	}
	g := mine["guest"].(map[string]any)
	if g["id_last4"] != "5561" || g["nationality"] != "Ethiopian" || strings.Contains(string(list.raw), "482917305561") {
		t.Fatalf("guest profile is wrong or leaks the full ID: %v", g)
	}
	if r := hotel.do("POST", "/partner/bookings/"+id+"/guest/reveal", nil); r.str("id_number") != "482917305561" {
		t.Fatalf("reveal: %s", r.raw)
	}
	var audits int
	pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_log WHERE action = 'guest_id_revealed' AND details->>'booking_id' = $1`, id).Scan(&audits)
	if audits != 1 {
		t.Fatalf("every reveal must be audited, found %d", audits)
	}
	if r := hotel.do("POST", "/partner/bookings/"+id+"/accept", nil); r.status != 200 || r.str("status") != "CONFIRMED" {
		t.Fatalf("accept: %d %s", r.status, r.raw)
	}
	if held, booked := heldBooked(t, standard, day(10)); booked < 1 || held < 0 {
		t.Fatalf("the room must move from held to booked (held=%d booked=%d)", held, booked)
	}
	if r := hotel.do("POST", "/partner/bookings/"+id+"/accept", nil); r.status != 409 {
		t.Fatalf("a booking cannot be answered twice: %d", r.status)
	}

	// The voucher: a QR token the partner app can check offline with the server's public key.
	v := guest.do("GET", "/bookings/"+id+"/voucher", nil)
	if v.status != 200 || len(v.str("pin")) != 4 || v.num("balance_due_santim") != 0 {
		t.Fatalf("voucher: %d %s", v.status, v.raw)
	}
	if v2 := guest.do("GET", "/bookings/"+id+"/voucher", nil); v2.str("pin") != v.str("pin") {
		t.Fatalf("the PIN must be the same every time the guest opens the voucher")
	}
	parts := strings.Split(v.str("token"), ".")
	payload, _ := base64.RawURLEncoding.DecodeString(parts[0])
	sig, _ := base64.RawURLEncoding.DecodeString(parts[1])
	pub := guest.do("GET", "/keys/voucher", nil)
	pk, _ := base64.RawURLEncoding.DecodeString(pub.str("public_key"))
	if !ed25519.Verify(pk, payload, sig) {
		t.Fatalf("the voucher signature does not verify with the published key")
	}
	ed := ed25519.PublicKey(pk)
	payload[0] ^= 1 // any change breaks the signature
	if ed25519.Verify(ed, payload, sig) {
		t.Fatalf("a tampered voucher must not verify")
	}
	var pl struct {
		Code string `json:"code"`
	}
	json.Unmarshal(mustDecode(parts[0]), &pl)
	if pl.Code != b.str("reference") {
		t.Fatalf("the voucher must carry the booking reference")
	}
	// Money: the hotel's share is owed to it, and no money is owed back.
	if bal := ledgerBalance(t, "property_payable"); bal >= 0 {
		t.Fatalf("the hotel should be owed its share, balance %d", bal)
	}
	if bal := ledgerBalance(t, "refunds_payable"); bal != 0 {
		t.Fatalf("no refunds should be owed, balance %d", bal)
	}
}

func mustDecode(s string) []byte {
	b, _ := base64.RawURLEncoding.DecodeString(s)
	return b
}

func TestDeclineReleasesRoomsAndRefundsTheGuest(t *testing.T) {
	guest := newClient(t, "guest")
	guest.signIn("email", uniqueEmail("h"))
	withProfile(guest)
	hotel := hotelFor(t)
	q := quote(guest, meskel, family, 30, 31, 3, 1)
	b := hold(guest, q.str("id"), "key-decline-1")
	payOnline(t, guest, b.str("id"), "succeeded")
	// The only Family Suite is taken for that night.
	if r := quote(guest, meskel, family, 30, 31, 3, 1); r.str("error", "code") != "SOLD_OUT" {
		t.Fatalf("the suite should be sold out: %s", r.raw)
	}
	before := ledgerBalance(t, "gateway_clearing")
	if r := hotel.do("POST", "/partner/bookings/"+b.str("id")+"/decline", map[string]string{"reason": "bogus"}); r.status != 400 {
		t.Fatalf("a reason from the list is required: %d", r.status)
	}
	r := hotel.do("POST", "/partner/bookings/"+b.str("id")+"/decline", map[string]string{"reason": "fully_booked"})
	if r.status != 200 || r.str("status") != "DECLINED_REFUNDED" {
		t.Fatalf("decline: %d %s", r.status, r.raw)
	}
	if again := quote(guest, meskel, family, 30, 31, 3, 1); again.status != 200 {
		t.Fatalf("the suite should be free again after the decline: %s", again.raw)
	}
	// The refund was paid back through the (stand-in) gateway: nothing is owed and the clearing account is back down.
	if bal := ledgerBalance(t, "refunds_payable"); bal != 0 {
		t.Fatalf("refunds_payable should be settled, balance %d", bal)
	}
	var status string
	pool.QueryRow(context.Background(), `SELECT status FROM payments WHERE booking_id = $1`, b.str("id")).Scan(&status)
	if status != "refunded" {
		t.Fatalf("the payment should be marked refunded, got %s", status)
	}
	if after := ledgerBalance(t, "gateway_clearing"); after >= before {
		t.Fatalf("money left the clearing account for the refund: before %d after %d", before, after)
	}
	if g := guest.do("GET", "/bookings/"+b.str("id"), nil); g.str("status") != "DECLINED_REFUNDED" {
		t.Fatalf("the guest should see the refund: %s", g.raw)
	}
}

func TestPayAtHotelSkipsThePaymentPage(t *testing.T) {
	guest := newClient(t, "guest")
	guest.signIn("email", uniqueEmail("i"))
	withProfile(guest)
	// Meskel does not take pay-at-hotel; Bole does.
	q1 := quote(guest, meskel, standard, 40, 41, 1, 1)
	b1 := hold(guest, q1.str("id"), "key-pah-1")
	if r := guest.do("POST", "/bookings/"+b1.str("id")+"/pay", map[string]string{"method": "pay_at_hotel"}); r.str("error", "code") != "PAYMENT_FAILED" {
		t.Fatalf("a hotel that does not allow it: %s", r.raw)
	}
	q2 := quote(guest, bole, boleSingle, 40, 41, 1, 1)
	b2 := hold(guest, q2.str("id"), "key-pah-2")
	r := guest.do("POST", "/bookings/"+b2.str("id")+"/pay", map[string]string{"method": "pay_at_hotel"})
	if r.status != 200 || r.str("booking", "status") != "PAID_AWAITING_HOTEL" || r.body["session"] != nil {
		t.Fatalf("pay at hotel: %d %s", r.status, r.raw)
	}
}

func TestMoreThanOneRoom(t *testing.T) {
	guest := newClient(t, "guest")
	guest.signIn("email", uniqueEmail("j"))
	withProfile(guest)
	q := quote(guest, meskel, standard, 50, 52, 4, 3) // 3 rooms for 2 nights
	if q.status != 200 || q.num("rooms") != 3 || q.num("total_santim") != 3*2*320000*115/100 {
		t.Fatalf("a 3-room quote: %d %s", q.status, q.raw)
	}
	if r := quote(guest, meskel, standard, 50, 52, 2, 3); r.status != 400 {
		t.Fatalf("each room needs a guest: %d", r.status)
	}
	if r := quote(guest, meskel, standard, 50, 52, 7, 3); r.status != 409 {
		t.Fatalf("3 doubles do not sleep 7: %d", r.status)
	}
	b := hold(guest, q.str("id"), "key-rooms-1")
	if b.status != 200 || b.num("rooms") != 3 {
		t.Fatalf("hold: %s", b.raw)
	}
	for _, night := range []string{day(50), day(51)} {
		if held, _ := heldBooked(t, standard, night); held != 3 {
			t.Fatalf("3 rooms should be held on %s, got %d", night, held)
		}
	}
	// 5 rooms exist, 3 are held: asking for 3 more must fail whole, taking nothing.
	if r := quote(guest, meskel, standard, 50, 52, 3, 3); r.str("error", "code") != "SOLD_OUT" {
		t.Fatalf("only 2 are left: %s", r.raw)
	}
	if r := quote(guest, meskel, standard, 50, 52, 2, 2); r.status != 200 {
		t.Fatalf("2 are left, so 2 can still be booked: %s", r.raw)
	}
}

func TestOverlappingHoldsNeverOversell(t *testing.T) {
	// 12 guests race for the 1 Family Suite on the same night: exactly one wins.
	var quotes [12]string
	var clients [12]*client
	for i := range clients {
		c := newClient(t, "guest")
		c.signIn("email", uniqueEmail(fmt.Sprintf("race%d", i)))
		withProfile(c)
		clients[i] = c
		quotes[i] = quote(c, meskel, family, 60, 61, 2, 1).str("id")
	}
	var wins int
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := range clients {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if r := hold(clients[i], quotes[i], fmt.Sprintf("key-race-%d-%d", i, time.Now().UnixNano())); r.status == 200 {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if wins != 1 {
		t.Fatalf("exactly one guest should get the only suite, %d did", wins)
	}
	if held, _ := heldBooked(t, family, day(60)); held != 1 {
		t.Fatalf("held = %d, want 1", held)
	}
}

// ---- time

func TestExpiredHoldsAndMissedDeadlinesAreClosedByTheSweeper(t *testing.T) {
	guest := newClient(t, "guest")
	guest.signIn("email", uniqueEmail("k"))
	withProfile(guest)
	hotel := hotelFor(t)

	// 1. A hold that runs out is released, and a late payment is refunded rather than lost.
	q := quote(guest, meskel, family, 70, 71, 2, 1)
	b := hold(guest, q.str("id"), "key-time-1")
	pool.Exec(context.Background(), `UPDATE bookings SET hold_expires_at = now() - interval '1 minute' WHERE id = $1`, b.str("id"))
	if r := guest.do("GET", "/bookings/"+b.str("id"), nil); r.str("status") != "EXPIRED" {
		t.Fatalf("an out-of-time hold reads EXPIRED at once: %s", r.raw)
	}
	if r := guest.do("POST", "/bookings/"+b.str("id")+"/pay", map[string]string{"method": "telebirr"}); r.str("error", "code") != "HOLD_EXPIRED" {
		t.Fatalf("paying an expired hold: %s", r.raw)
	}
	app.Sweep(context.Background())
	if held, _ := heldBooked(t, family, day(70)); held != 0 {
		t.Fatalf("the sweeper must release the room, held=%d", held)
	}

	// 2. A hotel that does not answer in time: the booking closes and the guest is refunded.
	q = quote(guest, meskel, family, 71, 72, 2, 1)
	b = hold(guest, q.str("id"), "key-time-2")
	payOnline(t, guest, b.str("id"), "succeeded")
	pool.Exec(context.Background(), `UPDATE bookings SET hotel_deadline_at = now() - interval '1 minute' WHERE id = $1`, b.str("id"))
	if r := hotel.do("POST", "/partner/bookings/"+b.str("id")+"/accept", nil); r.status != 409 || r.str("error", "code") != "DEADLINE_PASSED" {
		t.Fatalf("accepting after the deadline: %d %s", r.status, r.raw)
	}
	r := guest.do("GET", "/bookings/"+b.str("id"), nil)
	if r.str("status") != "DECLINED_REFUNDED" {
		t.Fatalf("a missed deadline refunds the guest: %s", r.raw)
	}
	var reason, payStatus string
	pool.QueryRow(context.Background(), `SELECT decline_reason FROM bookings WHERE id = $1`, b.str("id")).Scan(&reason)
	pool.QueryRow(context.Background(), `SELECT status FROM payments WHERE booking_id = $1`, b.str("id")).Scan(&payStatus)
	if reason != "no_response" || payStatus != "refunded" {
		t.Fatalf("reason=%s payment=%s", reason, payStatus)
	}
	if held, _ := heldBooked(t, family, day(71)); held != 0 {
		t.Fatalf("the room must be free again, held=%d", held)
	}
}

// ---- profile and payments safety

func TestProfileNeverReturnsTheFullIDAndCanBeRemoved(t *testing.T) {
	c := newClient(t, "guest")
	c.signIn("email", uniqueEmail("l"))
	if r := c.do("GET", "/me/profile", nil); r.status != 200 || r.body["profile"] != nil {
		t.Fatalf("no profile yet: %s", r.raw)
	}
	if r := c.do("PUT", "/me/profile", map[string]string{"full_name": "H", "id_type": "national_id", "id_number": "123", "nationality": "E"}); r.status != 400 {
		t.Fatalf("bad profile: %d", r.status)
	}
	if r := c.do("PUT", "/me/profile", map[string]string{"full_name": "Hanna Tesfaye", "id_type": "national_id", "id_number": "12", "nationality": "Ethiopian"}); r.str("error", "code") != "INVALID_ID" {
		t.Fatalf("bad id: %s", r.raw)
	}
	withProfile(c)
	r := c.do("GET", "/me/profile", nil)
	if r.str("profile", "id_last4") != "5561" || strings.Contains(string(r.raw), "482917305561") {
		t.Fatalf("only the last four may come back: %s", r.raw)
	}
	// In the database the number is ciphertext, never plain text.
	var enc []byte
	pool.QueryRow(context.Background(), `SELECT id_number_enc FROM guest_profiles WHERE id_last4 = '5561' ORDER BY updated_at DESC LIMIT 1`).Scan(&enc)
	if len(enc) == 0 {
		t.Fatalf("the encrypted number should be stored")
	}
	if bytes.Contains(enc, []byte("482917305561")) {
		t.Fatalf("the ID number is stored unencrypted")
	}
	if r := c.do("DELETE", "/me/profile", nil); r.status != 200 {
		t.Fatalf("delete: %d", r.status)
	}
	if r := c.do("GET", "/me/profile", nil); r.body["profile"] != nil {
		t.Fatalf("profile should be gone: %s", r.raw)
	}
}

func TestPaymentWebhookNeedsASignatureAndDuplicatesChangeNothing(t *testing.T) {
	guest := newClient(t, "guest")
	guest.signIn("email", uniqueEmail("m"))
	withProfile(guest)
	q := quote(guest, meskel, standard, 80, 81, 1, 1)
	b := hold(guest, q.str("id"), "key-hook-1")
	pay := guest.do("POST", "/bookings/"+b.str("id")+"/pay", map[string]string{"method": "chapa"})
	// Asking again while the payment is pending returns the same page: never a second charge.
	again := guest.do("POST", "/bookings/"+b.str("id")+"/pay", map[string]string{"method": "chapa"})
	if pay.str("session", "id") == "" || again.str("session", "id") != pay.str("session", "id") {
		t.Fatalf("a pending payment must be reused: %s / %s", pay.raw, again.raw)
	}
	ref := "dev_" + pay.str("session", "id")
	body := []byte(fmt.Sprintf(`{"event_id":"evt-1","payment_ref":%q,"status":"succeeded"}`, ref))
	post := func(sig string) int {
		req, _ := http.NewRequest("POST", srv.URL+"/v1/webhooks/payments/mock", bytes.NewReader(body))
		req.Header.Set("X-Signature", sig)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res.StatusCode
	}
	if code := post("deadbeef"); code != 401 {
		t.Fatalf("an unsigned callback must be refused, got %d", code)
	}
	sig := sign("whsec", body)
	if code := post(sig); code != 200 {
		t.Fatalf("a signed callback: %d", code)
	}
	if code := post(sig); code != 200 {
		t.Fatalf("a repeated callback is acknowledged: %d", code)
	}
	var n int
	pool.QueryRow(context.Background(), `SELECT count(*) FROM ledger_transactions WHERE payment_id = $1 AND kind = 'deposit_received'`, pay.str("session", "id")).Scan(&n)
	if n != 1 {
		t.Fatalf("the deposit must be recorded exactly once, found %d", n)
	}
	if r := guest.do("GET", "/bookings/"+b.str("id"), nil); r.str("status") != "PAID_AWAITING_HOTEL" {
		t.Fatalf("status after webhook: %s", r.raw)
	}
}

func TestOneGuestCannotSeeAnothersBooking(t *testing.T) {
	a := newClient(t, "guest")
	a.signIn("email", uniqueEmail("n"))
	withProfile(a)
	q := quote(a, meskel, standard, 90, 91, 1, 1)
	b := hold(a, q.str("id"), "key-priv-1")
	other := newClient(t, "guest")
	other.signIn("email", uniqueEmail("o"))
	if r := other.do("GET", "/bookings/"+b.str("id"), nil); r.status != 404 {
		t.Fatalf("someone else's booking: %d", r.status)
	}
	if r := other.do("POST", "/bookings/"+b.str("id")+"/pay", map[string]string{"method": "telebirr"}); r.status != 404 {
		t.Fatalf("paying someone else's booking: %d", r.status)
	}
	if r := hold(other, q.str("id"), "key-priv-2"); r.status != 404 {
		t.Fatalf("using someone else's quote: %d", r.status)
	}
}

func sign(secret string, body []byte) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write(body)
	return hex.EncodeToString(m.Sum(nil))
}
