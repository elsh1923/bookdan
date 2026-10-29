package api_test

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

// completeAt settles a payment on the fake gateway page, the way the guest's Approve or Decline button does.
func completeAt(t *testing.T, paymentURL, outcome string) {
	t.Helper()
	res, err := http.PostForm(strings.Replace(paymentURL, "http://example.test", srv.URL, 1), url.Values{"outcome": {outcome}})
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
}

func quoteChange(g *client, id string, in, out int) resp {
	return g.do("POST", "/bookings/"+id+"/change-dates/quote", map[string]string{"check_in": day(in), "check_out": day(out)})
}

func confirmChange(g *client, id string, in, out int, total float64) resp {
	return g.do("POST", "/bookings/"+id+"/change-dates", map[string]any{"check_in": day(in), "check_out": day(out), "confirm_total_santim": int64(total)})
}

func pendingOf(r resp) map[string]any {
	m, _ := r.body["pending_date_change"].(map[string]any)
	return m
}

func ledgerSum(t *testing.T, id string, accounts ...string) int64 {
	t.Helper()
	net := ledgerNetFor(t, id)
	var n int64
	for _, a := range accounts {
		n += net[a]
	}
	return n
}

func TestAGuestMovesToDearerDatesByPayingTheDifference(t *testing.T) {
	n := time.Now().UnixNano()
	g, email := realGuest(t, "dcp")
	hotel := hotelFor(t)
	id, _, _, _ := confirmedBooking(t, g, hotel, 210, 212, fmt.Sprintf("dcp-1-%d", n)) // two nights
	b0 := g.do("GET", "/bookings/"+id, nil)
	oldTotal := b0.num("total_santim")
	paidBefore := ledgerBalance(t, "gateway_clearing")

	// The quote changes nothing: it says what the move would cost.
	q := quoteChange(g, id, 214, 217) // three nights
	if q.status != 200 || q.str("kind") != "pay" || q.num("pay_now_santim") <= 0 || q.num("refund_now_santim") != 0 || q.num("old_total_santim") != oldTotal || q.num("new_total_santim") <= oldTotal {
		t.Fatalf("quote: %d %s", q.status, q.raw)
	}
	newTotal := q.num("new_total_santim")
	if q.num("pay_now_santim") != newTotal-oldTotal || q.num("difference_santim") != newTotal-oldTotal {
		t.Errorf("the guest pays exactly the difference: %s", q.raw)
	}
	if bookedNight(t, standard, 214) != 0 || g.do("GET", "/bookings/"+id, nil).str("check_in") != day(210) {
		t.Errorf("a quote holds nothing and moves nothing")
	}

	// The new price has to be confirmed (an older app, or a price that changed meanwhile, is refused).
	if r := changeDates(g, id, 214, 217); r.status != 409 || r.str("error", "code") != "PRICE_DIFFERS" {
		t.Errorf("no confirmation: %d %s", r.status, r.raw)
	}
	if r := confirmChange(g, id, 214, 217, newTotal-1); r.status != 409 || r.str("error", "code") != "PRICE_DIFFERS" {
		t.Errorf("wrong confirmation: %d %s", r.status, r.raw)
	}
	r := confirmChange(g, id, 214, 217, newTotal)
	if r.status != 200 || r.str("check_in") != day(210) || pendingOf(r) == nil {
		t.Fatalf("request: %d %s", r.status, r.raw)
	}
	pend := pendingOf(r)
	if pend["check_in"] != day(214) || pend["check_out"] != day(217) || pend["pay_now_santim"].(float64) != newTotal-oldTotal || pend["new_total_santim"].(float64) != newTotal {
		t.Errorf("the booking shows what is waiting: %v", pend)
	}
	payment, _ := pend["payment"].(map[string]any)
	if payment == nil || payment["payment_url"] == "" || payment["status"] != "pending" {
		t.Fatalf("a payment page for the difference: %v", pend)
	}
	if r.body["can_change_dates"] != false {
		t.Errorf("no second change while one waits")
	}
	// The new nights are held, the old ones are still booked, the hotel still sees the old dates.
	if h, b := heldBooked(t, standard, day(214)); h != 1 || b != 0 {
		t.Errorf("the new night is held: held %d booked %d", h, b)
	}
	if h, b := heldBooked(t, standard, day(210)); h != 0 || b != 1 {
		t.Errorf("the old night is still booked: held %d booked %d", h, b)
	}
	if list := hotel.do("GET", "/partner/bookings", nil); !strings.Contains(string(list.raw), day(210)) {
		t.Errorf("the hotel still sees the old dates")
	}
	if again := confirmChange(g, id, 220, 223, newTotal); again.status != 409 || again.str("error", "code") != "CHANGE_PENDING" {
		t.Errorf("one change at a time: %d %s", again.status, again.raw)
	}

	// The guest pays the difference: the booking moves and is repriced.
	mailsBefore := len(mails.to(email))
	completeAt(t, payment["payment_url"].(string), "succeeded")
	b := g.do("GET", "/bookings/"+id, nil)
	if b.str("check_in") != day(214) || b.str("check_out") != day(217) || b.num("total_santim") != newTotal || pendingOf(b) != nil || b.str("status") != "CONFIRMED" {
		t.Fatalf("after paying: %s", b.raw)
	}
	if b.str("original_check_in") != day(210) || b.body["can_change_dates"] != false {
		t.Errorf("the first dates are kept and it cannot be moved again: %s", b.raw)
	}
	if h, bk := heldBooked(t, standard, day(214)); h != 0 || bk != 1 {
		t.Errorf("the new night is now booked: held %d booked %d", h, bk)
	}
	if _, bk := heldBooked(t, standard, day(210)); bk != 0 {
		t.Errorf("the old night is free again")
	}
	if got := ledgerBalance(t, "gateway_clearing") - paidBefore; got != int64(newTotal-oldTotal) {
		t.Errorf("BookDan received exactly the difference: %d vs %v", got, newTotal-oldTotal)
	}
	// The hotel's share, the commission and the VAT on it add up to everything the guest paid, and the money held for the guest is all accounted for.
	if s := ledgerSum(t, id, "property_payable", "commission_revenue", "vat_on_commission_payable"); s != -int64(newTotal) {
		t.Errorf("the hotel's share, the commission and its VAT make up the new price: %d vs %v", s, -newTotal)
	}
	if held := ledgerSum(t, id, "guest_deposits_held"); held != 0 {
		t.Errorf("no money is left unassigned: %d", held)
	}
	if list := hotel.do("GET", "/partner/bookings", nil); !strings.Contains(string(list.raw), day(214)) {
		t.Errorf("the hotel now sees the new dates")
	}
	if len(mails.to(email)) != mailsBefore+1 || countSubject(email, "Dates changed") < 1 {
		t.Errorf("the guest is emailed once the booking has moved")
	}

	// Cancelling later returns everything the guest paid, both payments, and the hotel's money goes back with it.
	c := g.do("POST", "/bookings/"+id+"/cancel", nil)
	if c.status != 200 || c.num("refund_santim") != newTotal {
		t.Fatalf("cancel: %d %s", c.status, c.raw)
	}
	for account, net := range ledgerNetFor(t, id) {
		if net != 0 {
			t.Errorf("everything nets to zero after the refund: %s = %d", account, net)
		}
	}
	var refunded int
	pool.QueryRow(context.Background(), `SELECT count(*) FROM payments WHERE booking_id = $1 AND status = 'refunded'`, id).Scan(&refunded)
	if refunded != 2 {
		t.Errorf("both payments were refunded: %d", refunded)
	}
	if _, bk := heldBooked(t, standard, day(214)); bk != 0 {
		t.Errorf("the nights are free again")
	}
}

func TestAWaitingChangeCanBeCancelledOrRunsOutAndLateMoneyIsRefunded(t *testing.T) {
	n := time.Now().UnixNano()
	g, _ := supportGuest(t, "dcx")
	hotel := hotelFor(t)
	id, _, _, _ := confirmedBooking(t, g, hotel, 222, 224, fmt.Sprintf("dcx-1-%d", n))
	total := g.do("GET", "/bookings/"+id, nil).num("total_santim")
	newTotal := quoteChange(g, id, 226, 229).num("new_total_santim")

	// Cancelling the waiting change releases the nights and the booking is as it was.
	r := confirmChange(g, id, 226, 229, newTotal)
	if r.status != 200 || pendingOf(r) == nil {
		t.Fatalf("request: %d %s", r.status, r.raw)
	}
	if c := g.do("POST", "/bookings/"+id+"/change-dates/cancel", nil); c.status != 200 || pendingOf(c) != nil || c.body["can_change_dates"] != true || c.num("total_santim") != total {
		t.Fatalf("cancel the change: %d %s", c.status, c.raw)
	}
	if h, _ := heldBooked(t, standard, day(226)); h != 0 {
		t.Errorf("the held night is released: %d", h)
	}
	if c := g.do("POST", "/bookings/"+id+"/change-dates/cancel", nil); c.status != 409 {
		t.Errorf("nothing to cancel twice: %d", c.status)
	}

	// A request that is not paid in time ends by itself; the booking is untouched.
	r = confirmChange(g, id, 226, 229, newTotal)
	pend := pendingOf(r)
	if r.status != 200 || pend == nil {
		t.Fatalf("second request: %d %s", r.status, r.raw)
	}
	payURL := pend["payment"].(map[string]any)["payment_url"].(string)
	pool.Exec(context.Background(), `UPDATE date_change_requests SET expires_at = now() - interval '1 minute' WHERE booking_id = $1 AND status = 'pending'`, id)
	app.Sweep(context.Background())
	if h, _ := heldBooked(t, standard, day(226)); h != 0 {
		t.Errorf("the held night is released when the request runs out: %d", h)
	}
	b := g.do("GET", "/bookings/"+id, nil)
	if pendingOf(b) != nil || b.str("check_in") != day(222) || b.num("total_santim") != total || b.body["can_change_dates"] != true {
		t.Fatalf("the booking is as it was: %s", b.raw)
	}

	// Money that still arrives afterwards is refunded, and nothing moves.
	paidBefore := ledgerBalance(t, "gateway_clearing")
	completeAt(t, payURL, "succeeded")
	b = g.do("GET", "/bookings/"+id, nil)
	if b.str("check_in") != day(222) || b.num("total_santim") != total {
		t.Errorf("a late payment does not move the booking: %s", b.raw)
	}
	var late string
	pool.QueryRow(context.Background(), `SELECT status FROM payments WHERE booking_id = $1 AND kind = 'date_change' ORDER BY created_at DESC LIMIT 1`, id).Scan(&late)
	if late != "refunded" {
		t.Errorf("the late payment is refunded: %s", late)
	}
	if got := ledgerBalance(t, "gateway_clearing"); got != paidBefore {
		t.Errorf("BookDan keeps none of it: %d vs %d", got, paidBefore)
	}

	// Cancelling the booking while a change waits releases the held nights too.
	r = confirmChange(g, id, 226, 229, newTotal)
	if r.status != 200 || pendingOf(r) == nil {
		t.Fatalf("third request: %d %s", r.status, r.raw)
	}
	if c := g.do("POST", "/bookings/"+id+"/cancel", nil); c.status != 200 {
		t.Fatalf("cancel the booking: %d %s", c.status, c.raw)
	}
	if h, _ := heldBooked(t, standard, day(226)); h != 0 {
		t.Errorf("cancelling the booking releases the held night: %d", h)
	}
}

func TestAFailedPaymentOfTheDifferenceCanBeTriedAgain(t *testing.T) {
	n := time.Now().UnixNano()
	g, _ := supportGuest(t, "dcr")
	hotel := hotelFor(t)
	id, _, _, _ := confirmedBooking(t, g, hotel, 232, 234, fmt.Sprintf("dcr-1-%d", n))
	newTotal := quoteChange(g, id, 236, 239).num("new_total_santim")
	r := confirmChange(g, id, 236, 239, newTotal)
	first := pendingOf(r)["payment"].(map[string]any)["payment_url"].(string)
	completeAt(t, first, "failed")
	b := g.do("GET", "/bookings/"+id, nil)
	if b.str("check_in") != day(232) || pendingOf(b) == nil {
		t.Fatalf("a failed payment leaves the request waiting: %s", b.raw)
	}
	// While a payment page is still open, asking again returns the same one; after a failure it makes a new one.
	again := g.do("POST", "/bookings/"+id+"/change-dates/pay", map[string]string{"method": "chapa"})
	if again.status != 200 {
		t.Fatalf("pay again: %d %s", again.status, again.raw)
	}
	second := pendingOf(again)["payment"].(map[string]any)["payment_url"].(string)
	if second == first {
		t.Fatalf("after a failure there is a new payment page")
	}
	completeAt(t, second, "succeeded")
	if b := g.do("GET", "/bookings/"+id, nil); b.str("check_in") != day(236) || b.num("total_santim") != newTotal {
		t.Errorf("the second payment moves the booking: %s", b.raw)
	}
	if x := g.do("POST", "/bookings/"+id+"/change-dates/pay", map[string]string{"method": "chapa"}); x.status != 409 {
		t.Errorf("nothing is waiting any more: %d", x.status)
	}
}

func TestACheaperMoveRefundsTheDifferenceAndTheHotelBearsIt(t *testing.T) {
	n := time.Now().UnixNano()
	g, _ := supportGuest(t, "dcc")
	hotel := hotelFor(t)
	id, _, _, _ := confirmedBooking(t, g, hotel, 242, 245, fmt.Sprintf("dcc-1-%d", n)) // three nights
	oldTotal := g.do("GET", "/bookings/"+id, nil).num("total_santim")

	q := quoteChange(g, id, 246, 248) // two nights
	if q.status != 200 || q.str("kind") != "refund" || q.num("refund_now_santim") != oldTotal-q.num("new_total_santim") || q.num("pay_now_santim") != 0 {
		t.Fatalf("quote: %d %s", q.status, q.raw)
	}
	newTotal := q.num("new_total_santim")
	if r := changeDates(g, id, 246, 248); r.status != 409 || r.str("error", "code") != "PRICE_DIFFERS" {
		t.Errorf("the new price must be confirmed: %d %s", r.status, r.raw)
	}
	r := confirmChange(g, id, 246, 248, newTotal)
	if r.status != 200 || r.str("check_in") != day(246) || r.num("total_santim") != newTotal || r.num("partially_refunded_santim") != oldTotal-newTotal || pendingOf(r) != nil {
		t.Fatalf("move: %d %s", r.status, r.raw)
	}
	if _, bk := heldBooked(t, standard, day(242)); bk != 0 {
		t.Errorf("the old nights are free")
	}
	if _, bk := heldBooked(t, standard, day(246)); bk != 1 {
		t.Errorf("the new nights are taken")
	}
	var owed, paid int
	pool.QueryRow(context.Background(), `SELECT count(*) FILTER (WHERE status = 'owed'), count(*) FILTER (WHERE status = 'paid') FROM partial_refunds WHERE booking_id = $1`, id).Scan(&owed, &paid)
	if owed != 0 || paid != 1 {
		t.Errorf("the difference was sent back to the guest: owed %d paid %d", owed, paid)
	}
	// What the hotel keeps is what the new price is worth to it: the three accounts add up to the new price.
	if s := ledgerSum(t, id, "property_payable", "commission_revenue", "vat_on_commission_payable"); s != -int64(newTotal) {
		t.Errorf("the hotel's share, the commission and its VAT make up the new price: %d vs %v", s, -newTotal)
	}
	if c := changeDates(g, id, 250, 252); c.status != 409 || c.str("error", "code") != "CHANGE_LIMIT" {
		t.Errorf("only one move: %d", c.status)
	}
	// Cancelling now returns what is left, and everything nets to zero.
	c := g.do("POST", "/bookings/"+id+"/cancel", nil)
	if c.status != 200 || c.num("refund_santim") != newTotal {
		t.Fatalf("cancel: %d %s", c.status, c.raw)
	}
	for account, net := range ledgerNetFor(t, id) {
		if net != 0 {
			t.Errorf("everything nets to zero: %s = %d", account, net)
		}
	}
}

func TestMovingABookingPaidAtTheHotelJustReprices(t *testing.T) {
	n := time.Now().UnixNano()
	g, _ := supportGuest(t, "dch")
	hotel := hotelFor(t)
	bid := hold(g, quote(g, bole, boleSingle, 254, 256, 1, 1).str("id"), fmt.Sprintf("dch-1-%d", n)).str("id")
	if r := g.do("POST", "/bookings/"+bid+"/pay", map[string]string{"method": "pay_at_hotel"}); r.status != 200 {
		t.Fatalf("pay at hotel: %d %s", r.status, r.raw)
	}
	if r := hotel.do("POST", "/partner/bookings/"+bid+"/accept", nil); r.status != 200 {
		t.Fatalf("accept: %d %s", r.status, r.raw)
	}
	q := g.do("POST", "/bookings/"+bid+"/change-dates/quote", map[string]string{"check_in": day(258), "check_out": day(261)})
	if q.status != 200 || q.str("kind") != "reprice" || q.body["difference_at_hotel"] != true || q.num("pay_now_santim") != 0 || q.num("refund_now_santim") != 0 {
		t.Fatalf("quote: %d %s", q.status, q.raw)
	}
	r := g.do("POST", "/bookings/"+bid+"/change-dates", map[string]any{"check_in": day(258), "check_out": day(261), "confirm_total_santim": int64(q.num("new_total_santim"))})
	if r.status != 200 || r.str("check_in") != day(258) || r.num("total_santim") != q.num("new_total_santim") || pendingOf(r) != nil {
		t.Fatalf("move: %d %s", r.status, r.raw)
	}
	var payments int
	pool.QueryRow(context.Background(), `SELECT count(*) FROM payments WHERE booking_id = $1`, bid).Scan(&payments)
	if payments != 0 || len(ledgerNetFor(t, bid)) != 0 {
		t.Errorf("no money moved through BookDan")
	}
}

func TestOnlyOneChangeWaitsEvenWhenAskedAtTheSameTime(t *testing.T) {
	n := time.Now().UnixNano()
	g, _ := supportGuest(t, "dcz")
	hotel := hotelFor(t)
	id, _, _, _ := confirmedBooking(t, g, hotel, 302, 304, fmt.Sprintf("dcz-1-%d", n))
	newTotal := quoteChange(g, id, 306, 309).num("new_total_santim")
	var wg sync.WaitGroup
	var mu sync.Mutex
	ok := 0
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			status, _ := rawDo("guest", g.token, g.deviceID, "POST", "/bookings/"+id+"/change-dates", map[string]any{"check_in": day(306), "check_out": day(309), "confirm_total_santim": int64(newTotal)})
			if status == 200 {
				mu.Lock()
				ok++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if ok != 1 {
		t.Errorf("exactly one request is accepted: %d", ok)
	}
	var waiting, held int
	pool.QueryRow(context.Background(), `SELECT count(*) FROM date_change_requests WHERE booking_id = $1 AND status = 'pending'`, id).Scan(&waiting)
	pool.QueryRow(context.Background(), `SELECT held FROM inventory WHERE room_type_id = $1 AND night = $2::date`, standard, day(306)).Scan(&held)
	if waiting != 1 || held != 1 {
		t.Errorf("one waiting request holding one room: %d / %d", waiting, held)
	}
}

func TestChangeQuotesAreChecked(t *testing.T) {
	n := time.Now().UnixNano()
	g, _ := supportGuest(t, "dcq")
	other, _ := supportGuest(t, "dcq2")
	hotel := hotelFor(t)
	id, _, _, _ := confirmedBooking(t, g, hotel, 272, 274, fmt.Sprintf("dcq-1-%d", n))
	for name, r := range map[string]resp{
		"someone else's booking": other.do("POST", "/bookings/"+id+"/change-dates/quote", map[string]string{"check_in": day(276), "check_out": day(278)}),
		"the same dates":         quoteChange(g, id, 272, 274),
		"bad dates":              g.do("POST", "/bookings/"+id+"/change-dates/quote", map[string]string{"check_in": "x", "check_out": "y"}),
		"past":                   g.do("POST", "/bookings/"+id+"/change-dates/quote", map[string]string{"check_in": day(-3), "check_out": day(-1)}),
		"too soon":               quoteChange(g, id, 1, 3),
		"garbled id":             g.do("POST", "/bookings/not-a-uuid/change-dates/quote", map[string]string{"check_in": day(276), "check_out": day(278)}),
		"bad method":             g.do("POST", "/bookings/"+id+"/change-dates", map[string]any{"check_in": day(276), "check_out": day(279), "method": "cash"}),
	} {
		if r.status == 200 || r.status >= 500 {
			t.Errorf("%s: %d %s", name, r.status, r.raw)
		}
	}
	if r := quoteChange(g, id, 276, 278); r.status != 200 || r.str("kind") != "same_price" || r.num("difference_santim") != 0 {
		t.Errorf("same length, same price: %d %s", r.status, r.raw)
	}
}
