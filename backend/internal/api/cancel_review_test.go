package api_test

import (
	"context"
	"strings"
	"testing"
)

// ledgerNetFor returns, per account, what a booking's money entries add up to.
func ledgerNetFor(t *testing.T, bookingID string) map[string]int64 {
	t.Helper()
	rows, err := pool.Query(context.Background(), `SELECT e.account_code, sum(e.amount_santim)::bigint FROM ledger_entries e
		JOIN ledger_transactions x ON x.id = e.transaction_id WHERE x.booking_id = $1 GROUP BY 1`, bookingID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var a string
		var n int64
		rows.Scan(&a, &n)
		out[a] = n
	}
	return out
}

func paymentStatus(t *testing.T, bookingID string) string {
	var s string
	pool.QueryRow(context.Background(), `SELECT status FROM payments WHERE booking_id = $1 ORDER BY created_at DESC LIMIT 1`, bookingID).Scan(&s)
	return s
}

// paid books and pays for a stay and returns the booking id.
func paid(t *testing.T, c *client, property, room string, inDays, outDays int, key string) string {
	t.Helper()
	q := quote(c, property, room, inDays, outDays, 1, 1)
	b := hold(c, q.str("id"), key)
	if b.status != 200 {
		t.Fatalf("hold: %s", b.raw)
	}
	payOnline(t, c, b.str("id"), "succeeded")
	return b.str("id")
}

func TestCancellingBeforeTheHotelAnswersRefundsEverything(t *testing.T) {
	guest := newClient(t, "guest")
	guest.signIn("email", uniqueEmail("c1"))
	withProfile(guest)
	hotel := hotelFor(t)
	htoken := "cancel-hotel-phone-1"
	registerToken(hotel, "/partner/devices/push-token", htoken)

	id := paid(t, guest, meskel, standard, 300, 302, "key-cancel-1")
	b := guest.do("GET", "/bookings/"+id, nil)
	if b.body["can_cancel"] != true || b.num("refund_if_cancelled_santim") != 736000 {
		t.Fatalf("the app must be told what a cancellation returns: %s", b.raw)
	}
	held, _ := heldBooked(t, standard, day(300))

	r := guest.do("POST", "/bookings/"+id+"/cancel", nil)
	if r.status != 200 || r.str("status") != "CANCELLED_REFUNDED" || r.num("refund_santim") != 736000 || r.body["can_cancel"] != false {
		t.Fatalf("cancel: %d %s", r.status, r.raw)
	}
	if h, _ := heldBooked(t, standard, day(300)); h != held-1 {
		t.Fatalf("the held room must be released: held %d -> %d", held, h)
	}
	if paymentStatus(t, id) != "refunded" {
		t.Fatalf("the payment must be refunded through the gateway")
	}
	for account, net := range ledgerNetFor(t, id) {
		if net != 0 {
			t.Errorf("after a full refund every account must net to zero, %s = %d", account, net)
		}
	}
	// The hotel is told, and cannot answer a cancelled booking.
	alerts := push.to(htoken)
	if len(alerts) == 0 || alerts[len(alerts)-1].data["type"] != "booking_cancelled" || alerts[len(alerts)-1].data["booking_id"] != id {
		t.Fatalf("the hotel should be told about the cancellation: %+v", alerts)
	}
	if a := hotel.do("POST", "/partner/bookings/"+id+"/accept", nil); a.status != 409 {
		t.Fatalf("a cancelled booking cannot be accepted: %d", a.status)
	}
	// Twice is refused, someone else cannot cancel it, and nothing is refunded twice.
	if again := guest.do("POST", "/bookings/"+id+"/cancel", nil); again.status != 409 {
		t.Fatalf("cancelling twice: %d", again.status)
	}
	other := newClient(t, "guest")
	other.signIn("email", uniqueEmail("c1o"))
	if o := other.do("POST", "/bookings/"+id+"/cancel", nil); o.status != 404 {
		t.Fatalf("someone else's booking: %d", o.status)
	}
	if bal := ledgerBalance(t, "refunds_payable"); bal != 0 {
		t.Fatalf("no refund should be left owing, balance %d", bal)
	}
}

func TestCancellingAConfirmedBookingEarlyRefundsAndReversesTheHotelsShare(t *testing.T) {
	guest := newClient(t, "guest")
	guest.signIn("email", uniqueEmail("c2"))
	withProfile(guest)
	hotel := hotelFor(t)
	id := paid(t, guest, meskel, standard, 310, 311, "key-cancel-2") // far more than 48 hours away
	hotel.do("POST", "/partner/bookings/"+id+"/accept", nil)
	_, booked0 := heldBooked(t, standard, day(310))
	if booked0 < 1 {
		t.Fatalf("the room should be booked")
	}
	b := guest.do("GET", "/bookings/"+id, nil)
	if b.num("refund_if_cancelled_santim") != 368000 || b.str("full_refund_until") == "" {
		t.Fatalf("the guest must see the refund and the deadline: %s", b.raw)
	}

	r := guest.do("POST", "/bookings/"+id+"/cancel", nil)
	if r.status != 200 || r.num("refund_santim") != 368000 || r.str("status") != "CANCELLED_REFUNDED" {
		t.Fatalf("cancel: %d %s", r.status, r.raw)
	}
	if _, n := heldBooked(t, standard, day(310)); n != booked0-1 {
		t.Fatalf("the booked room must be given back: %d -> %d", booked0, n)
	}
	for account, net := range ledgerNetFor(t, id) {
		if net != 0 {
			t.Errorf("the hotel's share and the refund must net to zero, %s = %d", account, net)
		}
	}
	if paymentStatus(t, id) != "refunded" {
		t.Fatalf("the guest must be paid back")
	}
	if v := guest.do("GET", "/bookings/"+id+"/voucher", nil); v.status != 409 {
		t.Fatalf("a cancelled booking has no voucher: %d", v.status)
	}
	// The room can be sold again.
	if q := quote(guest, meskel, standard, 310, 311, 1, 1); q.status != 200 {
		t.Fatalf("the room should be free again: %s", q.raw)
	}
}

func TestCancellingConfirmedBookingTooLateKeepsTheMoneyWithTheHotel(t *testing.T) {
	guest := newClient(t, "guest")
	guest.signIn("email", uniqueEmail("c3"))
	withProfile(guest)
	hotel := hotelFor(t)
	id := paid(t, guest, meskel, standard, 1, 2, "key-cancel-3") // check-in tomorrow: inside the 48 hours
	hotel.do("POST", "/partner/bookings/"+id+"/accept", nil)

	b := guest.do("GET", "/bookings/"+id, nil)
	if b.body["can_cancel"] != true || b.num("refund_if_cancelled_santim") != 0 {
		t.Fatalf("inside 48 hours the app must say there is no refund: %s", b.raw)
	}
	owed := ledgerBalance(t, "property_payable")
	r := guest.do("POST", "/bookings/"+id+"/cancel", nil)
	if r.status != 200 || r.str("status") != "CANCELLED_REFUNDED" || r.num("refund_santim") != 0 {
		t.Fatalf("a late cancellation still cancels, with 0 refunded: %d %s", r.status, r.raw)
	}
	if paymentStatus(t, id) != "succeeded" {
		t.Fatalf("a late cancellation must not refund the payment, status %s", paymentStatus(t, id))
	}
	if ledgerBalance(t, "property_payable") != owed {
		t.Fatalf("the hotel still keeps what it was owed")
	}
}

func TestAStayThatAlreadyStartedCannotBeCancelled(t *testing.T) {
	guest := newClient(t, "guest")
	guest.signIn("email", uniqueEmail("c4"))
	withProfile(guest)
	hotel := hotelFor(t)
	id := paid(t, guest, meskel, standard, 320, 321, "key-cancel-4")
	hotel.do("POST", "/partner/bookings/"+id+"/accept", nil)
	// Move the stay into the past (as if time had passed).
	pool.Exec(context.Background(), `UPDATE bookings SET check_in = (now() AT TIME ZONE 'Africa/Addis_Ababa')::date - 3, check_out = (now() AT TIME ZONE 'Africa/Addis_Ababa')::date - 2 WHERE id = $1`, id)
	if b := guest.do("GET", "/bookings/"+id, nil); b.body["can_cancel"] != false {
		t.Fatalf("a stay that has started cannot be cancelled: %s", b.raw)
	}
	if r := guest.do("POST", "/bookings/"+id+"/cancel", nil); r.status != 409 || r.str("error", "code") != "CANCEL_TOO_LATE" {
		t.Fatalf("cancelling after check-in: %d %s", r.status, r.raw)
	}
}

func TestPayAtHotelAndOtherStatesCancelCorrectly(t *testing.T) {
	guest := newClient(t, "guest")
	guest.signIn("email", uniqueEmail("c5"))
	withProfile(guest)
	// Pay at hotel: nothing was paid online, so nothing is refunded, and the room is released.
	q := quote(guest, bole, boleSingle, 330, 331, 1, 1)
	b := hold(guest, q.str("id"), "key-cancel-5")
	guest.do("POST", "/bookings/"+b.str("id")+"/pay", map[string]string{"method": "pay_at_hotel"})
	r := guest.do("POST", "/bookings/"+b.str("id")+"/cancel", nil)
	if r.status != 200 || r.num("refund_santim") != 0 || r.str("status") != "CANCELLED_REFUNDED" {
		t.Fatalf("pay-at-hotel cancel: %d %s", r.status, r.raw)
	}
	var n int
	pool.QueryRow(context.Background(), `SELECT count(*) FROM payments WHERE booking_id = $1`, b.str("id")).Scan(&n)
	if n != 0 {
		t.Fatalf("there is no payment to refund")
	}
	// A hold that was never paid is not a cancellation: it simply runs out.
	q2 := quote(guest, meskel, standard, 331, 332, 1, 1)
	h := hold(guest, q2.str("id"), "key-cancel-6")
	if r := guest.do("POST", "/bookings/"+h.str("id")+"/cancel", nil); r.status != 409 {
		t.Fatalf("a hold is not cancelled this way: %d", r.status)
	}
}

func TestReviewsNeedACompletedStayAndComeOncePerBooking(t *testing.T) {
	guest := newClient(t, "guest")
	guest.signIn("email", uniqueEmail("rv"))
	withProfile(guest) // the saved name is "Hanna Tesfaye": the review shows "Hanna" only
	hotel := hotelFor(t)
	id := paid(t, guest, meskel, standard, 340, 341, "key-review-1")
	hotel.do("POST", "/partner/bookings/"+id+"/accept", nil)

	if b := guest.do("GET", "/bookings/"+id, nil); b.body["can_review"] != false {
		t.Fatalf("no review before the stay: %s", b.raw)
	}
	if r := guest.do("POST", "/bookings/"+id+"/review", map[string]any{"rating": 5, "text": "Lovely"}); r.status != 409 || r.str("error", "code") != "REVIEW_NOT_READY" {
		t.Fatalf("a review before check-out: %d %s", r.status, r.raw)
	}
	pool.Exec(context.Background(), `UPDATE bookings SET check_in = (now() AT TIME ZONE 'Africa/Addis_Ababa')::date - 3, check_out = (now() AT TIME ZONE 'Africa/Addis_Ababa')::date - 2 WHERE id = $1`, id)
	if b := guest.do("GET", "/bookings/"+id, nil); b.body["can_review"] != true {
		t.Fatalf("after check-out a review is possible: %s", b.raw)
	}
	for _, bad := range []map[string]any{{"rating": 0, "text": "x"}, {"rating": 6, "text": "x"}, {"rating": 4, "text": strings.Repeat("a", 1001)}} {
		if r := guest.do("POST", "/bookings/"+id+"/review", bad); r.status != 400 {
			t.Fatalf("bad review %v: %d", bad["rating"], r.status)
		}
	}
	other := newClient(t, "guest")
	other.signIn("email", uniqueEmail("rvo"))
	if r := other.do("POST", "/bookings/"+id+"/review", map[string]any{"rating": 1, "text": "I was never there"}); r.status != 404 {
		t.Fatalf("someone else reviewing: %d", r.status)
	}

	text := "Clean rooms and kind staff."
	r := guest.do("POST", "/bookings/"+id+"/review", map[string]any{"rating": 4, "text": "  " + text + "  "})
	if r.status != 200 || r.body["can_review"] != false || r.str("review", "text") != text {
		t.Fatalf("review: %d %s", r.status, r.raw)
	}
	if again := guest.do("POST", "/bookings/"+id+"/review", map[string]any{"rating": 5, "text": "again"}); again.status != 409 || again.str("error", "code") != "ALREADY_REVIEWED" {
		t.Fatalf("a second review: %d %s", again.status, again.raw)
	}

	// It shows on the hotel page with the first name only, and counts in the rating.
	d := guest.do("GET", "/properties/"+meskel+"?check_in="+day(400)+"&check_out="+day(401), nil)
	found := false
	for _, rv := range d.body["reviews"].([]any) {
		m := rv.(map[string]any)
		if m["text"] == text {
			found = true
			if m["author"] != "Hanna" || m["rating"].(float64) != 4 {
				t.Fatalf("the review should show the first name and the stars: %v", m)
			}
		}
	}
	if !found {
		t.Fatalf("the review must appear on the hotel page: %s", d.raw)
	}
	if prop := d.body["property"].(map[string]any); prop["rating"] == nil || prop["review_count"].(float64) < 1 {
		t.Fatalf("the hotel's rating must include it: %v", prop["rating"])
	}
}
