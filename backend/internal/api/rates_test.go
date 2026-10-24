package api_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

// quoteWith asks for a price with extra fields (meal, refundable, coupon_code).
func quoteWith(c *client, property, room string, in, out, guests, rooms int, extra map[string]any) resp {
	body := map[string]any{"property_id": property, "room_type_id": room, "check_in": day(in), "check_out": day(out), "guests": guests, "rooms": rooms}
	for k, v := range extra {
		body[k] = v
	}
	return c.do("POST", "/quotes", body)
}

// setRoomOptions changes what a room offers (breakfast price per night, non-refundable discount in basis points); nil removes it.
func setRoomOptions(t *testing.T, admin *client, hotelID, roomID string, breakfast *int64, nr *int) {
	t.Helper()
	d := admin.do("GET", "/admin/hotels/"+hotelID, nil)
	for _, r := range d.body["rooms"].([]any) {
		m := r.(map[string]any)
		if m["id"] != roomID {
			continue
		}
		body := map[string]any{"name": m["name"], "capacity": m["capacity"], "total_rooms": m["total_rooms"], "amenities": m["amenities"], "breakfast_santim": breakfast, "nonrefundable_bps": nr}
		if res := admin.do("PUT", "/admin/hotels/"+hotelID+"/rooms/"+roomID, body); res.status != 200 {
			t.Fatalf("set room options: %d %s", res.status, res.raw)
		}
		return
	}
	t.Fatalf("room %s not found", roomID)
}

func roomOptionsOf(t *testing.T, c *client, hotelID, roomID string) (price int64, opts map[string]int64) {
	t.Helper()
	d := c.do("GET", fmt.Sprintf("/properties/%s?check_in=%s&check_out=%s", hotelID, day(3), day(4)), nil)
	for _, r := range d.body["room_types"].([]any) {
		m := r.(map[string]any)
		if m["id"] != roomID {
			continue
		}
		opts = map[string]int64{}
		for _, o := range m["options"].([]any) {
			om := o.(map[string]any)
			opts[fmt.Sprintf("%v/%v", om["meal"], om["refundable"])] = int64(om["price_santim"].(float64))
		}
		return int64(m["price_santim"].(float64)), opts
	}
	t.Fatalf("room %s not on the hotel page", roomID)
	return
}

func ledgerOf(t *testing.T, bookingID, account string) int64 {
	t.Helper()
	var n int64
	if err := pool.QueryRow(context.Background(), `SELECT coalesce(sum(e.amount_santim) FILTER (WHERE e.account_code = $2), 0)::bigint
		FROM ledger_entries e JOIN ledger_transactions t ON t.id = e.transaction_id WHERE t.booking_id = $1`, bookingID, account).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestRoomOptionsAreOfferedPricedAndRefundedAsAgreed(t *testing.T) {
	n := time.Now().UnixNano()
	admin := adminClient(t)
	bf, nr := int64(20000), 1000 // breakfast adds 200 birr a night; non-refundable is 10% cheaper
	setRoomOptions(t, admin, meskel, standard, &bf, &nr)
	defer setRoomOptions(t, admin, meskel, standard, nil, nil)

	guest := newClient(t, "guest")
	guest.signIn("email", uniqueEmail("opt"))
	withProfile(guest)
	hotel := hotelFor(t)

	// 1. The hotel page lists the four options with their prices.
	base, opts := roomOptionsOf(t, guest, meskel, standard)
	cheaper := (base*9000 + 5000) / 10000
	want := map[string]int64{"room_only/true": base, "room_only/false": cheaper, "breakfast/true": base + bf, "breakfast/false": cheaper + bf}
	if len(opts) != 4 {
		t.Fatalf("expected four options, got %v", opts)
	}
	for k, v := range want {
		if opts[k] != v {
			t.Fatalf("option %s costs %d, expected %d (all: %v)", k, opts[k], v, opts)
		}
	}
	// A room that offers nothing extra offers only the plain room.
	if _, o := roomOptionsOf(t, guest, meskel, "b0000000-0000-0000-0000-000000000002"); len(o) != 1 {
		t.Fatalf("a room with no extras has one option: %v", o)
	}

	// 2. The quote prices the chosen option; options a room does not offer are refused.
	q := quoteWith(guest, meskel, standard, 300, 301, 1, 1, map[string]any{"meal": "breakfast", "refundable": false})
	if q.status != 200 || int64(q.num("nightly_santim")) != cheaper+bf || q.str("meal") != "breakfast" || q.body["refundable"] != false {
		t.Fatalf("breakfast, non-refundable quote: %d %s", q.status, q.raw)
	}
	if r := quoteWith(guest, meskel, "b0000000-0000-0000-0000-000000000002", 300, 301, 1, 1, map[string]any{"meal": "breakfast"}); r.status != 409 || r.str("error", "code") != "OPTION_UNAVAILABLE" {
		t.Fatalf("breakfast on a room without it: %d %s", r.status, r.raw)
	}
	if r := quoteWith(guest, meskel, "b0000000-0000-0000-0000-000000000002", 300, 301, 1, 1, map[string]any{"refundable": false}); r.status != 409 {
		t.Fatalf("non-refundable on a room without it: %d", r.status)
	}
	if r := quoteWith(guest, meskel, standard, 300, 301, 1, 1, map[string]any{"meal": "lunch"}); r.status != 400 {
		t.Fatalf("an unknown meal: %d", r.status)
	}
	// Without options the booking is the plain, refundable room, as before.
	if plain := quoteWith(guest, meskel, standard, 300, 301, 1, 1, nil); plain.str("meal") != "room_only" || plain.body["refundable"] != true || int64(plain.num("nightly_santim")) != base {
		t.Fatalf("the default is room only with a refund: %s", plain.raw)
	}

	// 3. A non-refundable booking the hotel has not answered yet is still cancelled with everything back ...
	book := func(in int, key string, extra map[string]any) string {
		qq := quoteWith(guest, meskel, standard, in, in+1, 1, 1, extra)
		b := hold(guest, qq.str("id"), key)
		if b.status != 200 {
			t.Fatalf("hold: %s", b.raw)
		}
		payOnline(t, guest, b.str("id"), "succeeded")
		return b.str("id")
	}
	nrExtra := map[string]any{"refundable": false}
	early := book(302, fmt.Sprintf("opt-1-%d", n), nrExtra)
	b := guest.do("GET", "/bookings/"+early, nil)
	if b.body["refundable"] != false || int64(b.num("refund_if_cancelled_santim")) != int64(b.num("total_santim")) {
		t.Fatalf("before the hotel answers a non-refundable booking still refunds fully: %s", b.raw)
	}
	if c := guest.do("POST", "/bookings/"+early+"/cancel", nil); c.status != 200 || int64(c.num("refund_santim")) != int64(b.num("total_santim")) {
		t.Fatalf("cancel before an answer: %d %s", c.status, c.raw)
	}

	// ... but once confirmed it gets nothing back, and the hotel keeps its share.
	late := book(304, fmt.Sprintf("opt-2-%d", n), nrExtra)
	hotel.do("POST", "/partner/bookings/"+late+"/accept", nil)
	b = guest.do("GET", "/bookings/"+late, nil)
	if b.num("refund_if_cancelled_santim") != 0 || b.body["full_refund_until"] != nil || b.body["can_cancel"] != true {
		t.Fatalf("a confirmed non-refundable booking refunds nothing: %s", b.raw)
	}
	before := ledgerOf(t, late, "property_payable")
	if c := guest.do("POST", "/bookings/"+late+"/cancel", nil); c.status != 200 || c.num("refund_santim") != 0 || c.str("status") != "CANCELLED_REFUNDED" {
		t.Fatalf("cancelling a confirmed non-refundable booking: %d %s", c.status, c.raw)
	}
	if ledgerOf(t, late, "property_payable") != before || before >= 0 {
		t.Fatalf("the hotel must keep its share when nothing is refunded (%d -> %d)", before, ledgerOf(t, late, "property_payable"))
	}

	// 4. The hotel sees what to serve and that it is non-refundable.
	withBreakfast := book(306, fmt.Sprintf("opt-3-%d", n), map[string]any{"meal": "breakfast"})
	hotel.do("POST", "/partner/bookings/"+withBreakfast+"/accept", nil)
	seen := false
	for _, it := range hotel.do("GET", "/partner/bookings", nil).body["items"].([]any) {
		if m := it.(map[string]any); m["id"] == withBreakfast && m["meal"] == "breakfast" && m["refundable"] == true {
			seen = true
		}
	}
	if !seen {
		t.Fatalf("the hotel's list should show breakfast is included")
	}
}

func TestCouponsDiscountTheRoomAndBookDanPaysForThem(t *testing.T) {
	ctx := context.Background()
	n := time.Now().UnixNano()
	suffix := fmt.Sprintf("%d", n%1000000)
	admin := adminClient(t)
	hotel := hotelFor(t)
	pool.Exec(ctx, `UPDATE pricing_config SET commission_bps = 1500, commission_vat_bps = 1500`)
	var vatBP, commBP, commVatBP int64
	pool.QueryRow(ctx, `SELECT vat_bps, commission_bps, commission_vat_bps FROM pricing_config WHERE id = 1`).Scan(&vatBP, &commBP, &commVatBP)
	round := func(v, bp int64) int64 { return (v*bp + 5000) / 10000 }

	create := func(body map[string]any) string {
		r := admin.do("POST", "/admin/coupons", body)
		if r.status != 200 {
			t.Fatalf("create coupon %v: %d %s", body["code"], r.status, r.raw)
		}
		return r.str("id")
	}
	ten := "TEN" + suffix
	create(map[string]any{"code": ten, "kind": "percent", "value": 1000, "per_user_limit": 1, "active": true})

	// Admin rules are checked.
	for name, body := range map[string]map[string]any{
		"short code":     {"code": "AB", "kind": "percent", "value": 1000, "active": true},
		"percent > 100%": {"code": "BIGONE", "kind": "percent", "value": 10001, "active": true},
		"zero":           {"code": "ZEROONE", "kind": "fixed", "value": 0, "active": true},
		"unknown kind":   {"code": "KINDX1", "kind": "free", "value": 5, "active": true},
		"bad date":       {"code": "DATEX1", "kind": "fixed", "value": 5, "ends_on": "tomorrow", "active": true},
	} {
		if r := admin.do("POST", "/admin/coupons", body); r.status != 400 {
			t.Errorf("%s must be refused, got %d", name, r.status)
		}
	}
	if r := admin.do("POST", "/admin/coupons", map[string]any{"code": strings.ToLower(ten), "kind": "percent", "value": 500, "active": true}); r.status != 409 {
		t.Fatalf("a code that exists (any case): %d", r.status)
	}
	if r := newClient(t, "admin").do("GET", "/admin/coupons", nil); r.status != 401 {
		t.Fatalf("coupons need the admin: %d", r.status)
	}

	guest := newClient(t, "guest")
	guest.signIn("email", uniqueEmail("cpn"))
	withProfile(guest)

	// 1. The discount comes off the room charges before VAT; the total and the price without the coupon are both shown.
	base, _ := roomOptionsOf(t, guest, meskel, standard)
	d := round(base, 1000)
	vat := round(base-d, vatBP)
	wantTotal := base - d + vat
	listTotal := base + round(base, vatBP)
	q := quoteWith(guest, meskel, standard, 308, 309, 1, 1, map[string]any{"coupon_code": strings.ToLower(ten)}) // any case works
	if q.status != 200 || int64(q.num("total_santim")) != wantTotal || int64(q.num("discount_santim")) != listTotal-wantTotal ||
		int64(q.num("list_total_santim")) != listTotal || q.str("coupon_code") != ten {
		t.Fatalf("10%% coupon: want total %d, discount %d, list %d: %d %s", wantTotal, listTotal-wantTotal, listTotal, q.status, q.raw)
	}
	// A coupon is for online payment: even a pay-at-hotel hotel does not offer that for this price.
	if r := quoteWith(guest, bole, boleSingle, 308, 309, 1, 1, map[string]any{"coupon_code": ten}); r.status != 200 || r.body["pay_at_hotel"] != false {
		t.Fatalf("a quote with a coupon must not offer pay at hotel: %s", r.raw)
	}
	b := hold(guest, q.str("id"), fmt.Sprintf("cpn-1-%d", n))
	id := b.str("id")
	if int64(b.num("discount_santim")) != listTotal-wantTotal || b.str("coupon_code") != ten {
		t.Fatalf("the booking carries the coupon: %s", b.raw)
	}
	bq := quoteWith(guest, bole, boleSingle, 310, 311, 1, 1, map[string]any{"coupon_code": "TENX" + suffix})
	if bq.status != 400 || bq.str("error", "code") != "COUPON_INVALID" {
		t.Fatalf("an unknown code: %d %s", bq.status, bq.raw)
	}
	// Paying at the hotel is refused for a booking with a coupon (use Bole, which takes pay at hotel, with a coupon booking).
	bb := hold(guest, quoteWith(guest, bole, boleSingle, 312, 313, 1, 1, map[string]any{"coupon_code": "OTHER" + suffix}).str("id"), fmt.Sprintf("cpn-x-%d", n))
	_ = bb
	payOnline(t, guest, id, "succeeded")
	hotel.do("POST", "/partner/bookings/"+id+"/accept", nil)

	// 2. The hotel gets its share of the FULL price; BookDan's discount is a cost; the ledger balances.
	paid := int64(guest.do("GET", "/bookings/"+id, nil).num("total_santim"))
	if paid != wantTotal {
		t.Fatalf("the guest pays %d, not %d", paid, wantTotal)
	}
	full := paid + (listTotal - wantTotal)
	comm := round(full, commBP)
	cvat := round(comm, commVatBP)
	if got := ledgerOf(t, id, "coupon_expense"); got != listTotal-wantTotal {
		t.Fatalf("the coupon cost should be %d, got %d", listTotal-wantTotal, got)
	}
	if got := ledgerOf(t, id, "property_payable"); got != -(full - comm - cvat) {
		t.Fatalf("the hotel is owed its share of the full price %d, ledger says %d", full-comm-cvat, -got)
	}
	if got := ledgerOf(t, id, "gateway_clearing"); got != paid {
		t.Fatalf("the gateway holds what the guest paid (%d), ledger says %d", paid, got)
	}
	// The same room without a coupon pays the hotel exactly the same.
	plain := hold(guest, quoteWith(guest, meskel, standard, 314, 315, 1, 1, nil).str("id"), fmt.Sprintf("cpn-2-%d", n))
	payOnline(t, guest, plain.str("id"), "succeeded")
	hotel.do("POST", "/partner/bookings/"+plain.str("id")+"/accept", nil)
	if ledgerOf(t, plain.str("id"), "property_payable") != ledgerOf(t, id, "property_payable") {
		t.Fatalf("a coupon must not reduce what the hotel receives")
	}
	// The hotel's earnings show the full price and the part BookDan paid.
	var line map[string]any
	for _, h := range hotel.do("GET", "/partner/earnings", nil).body["hotels"].([]any) {
		if h.(map[string]any)["id"] != meskel {
			continue
		}
		for _, it := range h.(map[string]any)["items"].([]any) {
			if m := it.(map[string]any); m["booking_id"] == id {
				line = m
			}
		}
	}
	if line == nil || int64(line["gross_santim"].(float64)) != full || int64(line["discount_santim"].(float64)) != listTotal-wantTotal ||
		int64(line["net_santim"].(float64)) != full-comm-cvat {
		t.Fatalf("the earnings line should show price %d, discount %d: %v", full, listTotal-wantTotal, line)
	}

	// 3. Once used, the coupon is used (one per guest) ... until the booking is cancelled with a refund, which gives the use back.
	if r := quoteWith(guest, meskel, standard, 316, 317, 1, 1, map[string]any{"coupon_code": ten}); r.status != 400 || !strings.Contains(r.str("error", "message"), "already used") {
		t.Fatalf("a second use by the same guest: %d %s", r.status, r.raw)
	}
	if c := guest.do("POST", "/bookings/"+id+"/cancel", nil); c.status != 200 || int64(c.num("refund_santim")) != paid {
		t.Fatalf("cancel with a coupon refunds what was paid: %d %s", c.status, c.raw)
	}
	for _, acct := range []string{"coupon_expense", "property_payable", "commission_revenue"} {
		if got := ledgerOf(t, id, acct); got != 0 {
			t.Fatalf("a full refund undoes the %s entries, %d left", acct, got)
		}
	}
	if r := quoteWith(guest, meskel, standard, 316, 317, 1, 1, map[string]any{"coupon_code": ten}); r.status != 200 {
		t.Fatalf("the use should be given back after a refunded cancellation: %d %s", r.status, r.raw)
	}
}

func TestCouponRulesAreEnforced(t *testing.T) {
	ctx := context.Background()
	n := time.Now().UnixNano()
	suffix := fmt.Sprintf("%d", n%1000000)
	admin := adminClient(t)
	hotel := hotelFor(t)
	create := func(body map[string]any) (string, string) {
		code := body["code"].(string)
		r := admin.do("POST", "/admin/coupons", body)
		if r.status != 200 {
			t.Fatalf("create %s: %d %s", code, r.status, r.raw)
		}
		return r.str("id"), code
	}
	newGuest := func(prefix string) *client {
		g := newClient(t, "guest")
		g.signIn("email", uniqueEmail(prefix))
		withProfile(g)
		return g
	}
	problem := func(g *client, code string, want string) {
		t.Helper()
		r := quoteWith(g, meskel, standard, 318, 319, 1, 1, map[string]any{"coupon_code": code})
		if r.status != 400 || r.str("error", "code") != "COUPON_INVALID" || !strings.Contains(r.str("error", "message"), want) {
			t.Fatalf("%s: wanted a COUPON_INVALID about %q, got %d %s", code, want, r.status, r.raw)
		}
	}
	ok := func(g *client, code string) resp {
		t.Helper()
		r := quoteWith(g, meskel, standard, 318, 319, 1, 1, map[string]any{"coupon_code": code})
		if r.status != 200 {
			t.Fatalf("%s should work: %d %s", code, r.status, r.raw)
		}
		return r
	}
	g := newGuest("rules")
	base, _ := roomOptionsOf(t, g, meskel, standard)

	// A fixed coupon, never more than the room costs; a percent coupon is capped.
	_, fixed := create(map[string]any{"code": "FIXED" + suffix, "kind": "fixed", "value": 50000, "active": true})
	ok(g, fixed)
	_, huge := create(map[string]any{"code": "HUGE" + suffix, "kind": "fixed", "value": 999999999, "active": true})
	if r := ok(g, huge); int64(r.num("total_santim")) != 0 {
		t.Fatalf("a discount cannot go below zero: %s", r.raw)
	}
	_, capped := create(map[string]any{"code": "CAP" + suffix, "kind": "percent", "value": 5000, "max_discount_santim": 10000, "active": true})
	q := ok(g, capped)
	if want := base - 10000 + ((base-10000)*1500+5000)/10000; int64(q.num("total_santim")) != want {
		t.Fatalf("a 50%% coupon capped at 100 birr: want total %d: %s", want, q.raw)
	}

	// Minimum booking, dates, active flag, hotel, uses.
	_, minimum := create(map[string]any{"code": "MIN" + suffix, "kind": "fixed", "value": 10000, "min_subtotal_santim": base + 1, "active": true})
	problem(g, minimum, "larger booking")
	yesterday := time.Now().UTC().Add(3*time.Hour).AddDate(0, 0, -1).Format("2006-01-02")
	tomorrow := time.Now().UTC().Add(3*time.Hour).AddDate(0, 0, 1).Format("2006-01-02")
	_, expired := create(map[string]any{"code": "OLD" + suffix, "kind": "fixed", "value": 10000, "ends_on": yesterday, "active": true})
	problem(g, expired, "expired")
	_, future := create(map[string]any{"code": "SOON" + suffix, "kind": "fixed", "value": 10000, "starts_on": tomorrow, "active": true})
	problem(g, future, "not valid yet")
	_, today := create(map[string]any{"code": "TODAY" + suffix, "kind": "fixed", "value": 10000, "starts_on": yesterday, "ends_on": time.Now().UTC().Add(3 * time.Hour).Format("2006-01-02"), "active": true})
	ok(g, today) // the last day counts
	offID, off := create(map[string]any{"code": "OFF" + suffix, "kind": "fixed", "value": 10000, "active": false})
	problem(g, off, "no longer available")
	_ = offID
	_, otherHotel := create(map[string]any{"code": "BOLE" + suffix, "kind": "fixed", "value": 10000, "property_id": bole, "active": true})
	problem(g, otherHotel, "not valid for this hotel")
	if r := quoteWith(g, bole, boleSingle, 318, 319, 1, 1, map[string]any{"coupon_code": otherHotel}); r.status != 200 {
		t.Fatalf("a hotel's own coupon works there: %d %s", r.status, r.raw)
	}
	problem(g, "NOPE"+suffix, "not valid")
	problem(g, "!!", "not valid")

	// Only so many uses in total: the second guest finds it used up (a use is held from the moment of booking).
	_, once := create(map[string]any{"code": "ONCE" + suffix, "kind": "fixed", "value": 10000, "max_uses": 1, "per_user_limit": 5, "active": true})
	first := newGuest("one")
	second := newGuest("two")
	fb := hold(first, ok(first, once).str("id"), fmt.Sprintf("rules-1-%d", n))
	if fb.status != 200 {
		t.Fatalf("first use: %s", fb.raw)
	}
	problem(second, once, "used up")
	// If that hold runs out the use is free again.
	pool.Exec(ctx, `UPDATE bookings SET hold_expires_at = now() - interval '1 minute' WHERE id = $1`, fb.str("id"))
	ok(second, once)

	// The price a guest saw is checked again when they book: a coupon that ran out in between stops the booking.
	_, race := create(map[string]any{"code": "RACE" + suffix, "kind": "fixed", "value": 10000, "max_uses": 1, "per_user_limit": 5, "active": true})
	a, b := newGuest("raceA"), newGuest("raceB")
	qa, qb := ok(a, race), ok(b, race)
	if h := hold(a, qa.str("id"), fmt.Sprintf("race-a-%d", n)); h.status != 200 {
		t.Fatalf("first booking with the coupon: %s", h.raw)
	}
	if h := hold(b, qb.str("id"), fmt.Sprintf("race-b-%d", n)); h.status != 400 || h.str("error", "code") != "COUPON_INVALID" {
		t.Fatalf("the last use is taken: %d %s", h.status, h.raw)
	}

	// First booking only: fine for a new guest, refused once they have a booking the hotel has taken.
	_, firstOnly := create(map[string]any{"code": "FIRST" + suffix, "kind": "fixed", "value": 10000, "first_booking_only": true, "per_user_limit": 5, "active": true})
	fg := newGuest("first")
	ok(fg, firstOnly)
	pid := paid(t, fg, meskel, standard, 320, 321, fmt.Sprintf("rules-f-%d", n))
	hotel.do("POST", "/partner/bookings/"+pid+"/accept", nil)
	problem(fg, firstOnly, "first booking only")

	// Pay at hotel is refused for a booking with a coupon, whatever the app sends.
	cb := hold(g, quoteWith(g, bole, boleSingle, 322, 323, 1, 1, map[string]any{"coupon_code": otherHotel}).str("id"), fmt.Sprintf("rules-p-%d", n))
	if cb.status != 200 {
		t.Fatalf("hold with a coupon: %s", cb.raw)
	}
	if r := g.do("POST", "/bookings/"+cb.str("id")+"/pay", map[string]string{"method": "pay_at_hotel"}); r.status != 409 || r.str("error", "code") != "COUPON_ONLINE_ONLY" {
		t.Fatalf("pay at hotel with a coupon: %d %s", r.status, r.raw)
	}

	// The admin sees how often each coupon was used and can switch one off.
	list := admin.do("GET", "/admin/coupons", nil)
	var seen bool
	for _, c := range list.body["coupons"].([]any) {
		if m := c.(map[string]any); m["code"] == once && m["uses"].(float64) == 0 {
			seen = true // the only booking with it expired
		}
	}
	if !seen {
		t.Fatalf("the admin list should show usage: %s", list.raw)
	}
	update := map[string]any{"kind": "fixed", "value": 50000, "per_user_limit": 1, "active": false}
	if r := admin.do("PUT", "/admin/coupons/"+create2(t, admin, "SWITCH"+suffix), update); r.status != 200 {
		t.Fatalf("update: %d %s", r.status, r.raw)
	}
	problem(g, "SWITCH"+suffix, "no longer available")
	if r := admin.do("PUT", "/admin/coupons/00000000-0000-0000-0000-000000000000", update); r.status != 404 {
		t.Fatalf("unknown coupon: %d", r.status)
	}
}

func create2(t *testing.T, admin *client, code string) string {
	r := admin.do("POST", "/admin/coupons", map[string]any{"code": code, "kind": "fixed", "value": 10000, "active": true})
	if r.status != 200 {
		t.Fatalf("create %s: %d %s", code, r.status, r.raw)
	}
	return r.str("id")
}
