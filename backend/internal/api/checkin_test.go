package api_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

const addisToday = `(now() AT TIME ZONE 'Africa/Addis_Ababa')::date`

// confirmedBooking books, pays and has the hotel accept; it returns the booking id, its reference, and the voucher (token, PIN).
func confirmedBooking(t *testing.T, guest, hotel *client, in, out int, key string) (id, ref, token, pin string) {
	t.Helper()
	b := hold(guest, quote(guest, meskel, standard, in, out, 1, 1).str("id"), key)
	id = b.str("id")
	payOnline(t, guest, id, "succeeded")
	if r := hotel.do("POST", "/partner/bookings/"+id+"/accept", nil); r.status != 200 {
		t.Fatalf("accept: %d %s", r.status, r.raw)
	}
	pool.QueryRow(context.Background(), `SELECT code FROM bookings WHERE id = $1`, id).Scan(&ref)
	v := guest.do("GET", "/bookings/"+id+"/voucher", nil)
	if v.status != 200 {
		t.Fatalf("voucher: %d %s", v.status, v.raw)
	}
	return id, ref, v.str("token"), v.str("pin")
}

func TestAGuestIsCheckedInWithTheQRCodeOrThePIN(t *testing.T) {
	ctx := context.Background()
	n := time.Now().UnixNano()
	guest := newClient(t, "guest")
	guest.signIn("email", uniqueEmail("checkin"))
	withProfile(guest)
	hotel := hotelFor(t)

	id, ref, token, pin := confirmedBooking(t, guest, hotel, 280, 282, fmt.Sprintf("ci-1-%d", n))
	lookup := func(c *client, body map[string]string) resp { return c.do("POST", "/partner/checkin/lookup", body) }

	// 1. Before the stay: the QR is understood, but it is too early, and checking in is refused.
	r := lookup(hotel, map[string]string{"token": token})
	if r.status != 200 || r.str("state") != "too_early" || r.str("booking", "reference") != ref {
		t.Fatalf("a booking for later should say so: %d %s", r.status, r.raw)
	}
	if c := hotel.do("POST", "/partner/bookings/"+id+"/check-in", nil); c.status != 409 {
		t.Fatalf("too early to check in: %d %s", c.status, c.raw)
	}

	// 2. Bad vouchers: a changed one and a stranger's.
	parts := strings.Split(token, ".")
	if r := lookup(hotel, map[string]string{"token": parts[0] + "." + strings.Repeat("A", len(parts[1]))}); r.status != 400 || r.str("error", "code") != "INVALID_VOUCHER" {
		t.Fatalf("a tampered voucher: %d %s", r.status, r.raw)
	}
	if r := lookup(hotel, map[string]string{"token": "hello"}); r.status != 400 {
		t.Fatalf("not a voucher: %d", r.status)
	}
	if r := lookup(hotel, map[string]string{}); r.status != 400 {
		t.Fatalf("an empty lookup: %d", r.status)
	}

	// 3. A hotel other than the one on the voucher cannot see or check in the booking.
	admin := adminClient(t)
	other := admin.do("POST", "/admin/hotels", newHotelBody(fmt.Sprintf("Other Hotel %d", n%100000)))
	deskMail := realAddress("otherdesk")
	if r := admin.do("POST", "/admin/hotels/"+other.str("id")+"/staff", map[string]string{"contact": deskMail, "role": "receptionist"}); r.status != 200 {
		t.Fatalf("add staff: %d %s", r.status, r.raw)
	}
	desk := newClient(t, "partner")
	dr := signInWithEmailedCode(desk, "/auth/otp/request", "/auth/otp/verify", deskMail)
	if dr.status != 200 {
		t.Fatalf("desk sign-in: %d %s", dr.status, dr.raw)
	}
	desk.token = dr.str("token")
	if r := lookup(desk, map[string]string{"token": token}); r.status != 409 || r.str("error", "code") != "WRONG_HOTEL" {
		t.Fatalf("a voucher for another hotel: %d %s", r.status, r.raw)
	}
	if r := lookup(desk, map[string]string{"reference": ref, "pin": pin}); r.status != 400 {
		t.Fatalf("another hotel must not find the booking by reference: %d %s", r.status, r.raw)
	}
	if r := desk.do("POST", "/partner/bookings/"+id+"/check-in", nil); r.status != 404 {
		t.Fatalf("another hotel cannot check in this booking: %d", r.status)
	}

	// 4. The reference and PIN work; wrong PINs are counted and lock the booking after five. The QR still works.
	if r := lookup(hotel, map[string]string{"reference": strings.ToLower(ref), "pin": pin}); r.status != 200 || r.str("booking", "id") != id {
		t.Fatalf("reference and PIN: %d %s", r.status, r.raw)
	}
	wrong := "0000"
	if wrong == pin {
		wrong = "1111"
	}
	for i := 1; i <= 4; i++ {
		r := lookup(hotel, map[string]string{"reference": ref, "pin": wrong})
		if r.status != 400 || r.str("error", "code") != "INVALID_PIN" || !strings.Contains(r.str("error", "message"), fmt.Sprintf("%d tries left", 5-i)) {
			t.Fatalf("wrong PIN %d: %d %s", i, r.status, r.raw)
		}
	}
	if r := lookup(hotel, map[string]string{"reference": ref, "pin": wrong}); r.status != 429 || r.str("error", "code") != "PIN_LOCKED" {
		t.Fatalf("the fifth wrong PIN locks: %d %s", r.status, r.raw)
	}
	if r := lookup(hotel, map[string]string{"reference": ref, "pin": pin}); r.status != 429 {
		t.Fatalf("even the right PIN is refused while locked: %d", r.status)
	}
	if r := lookup(hotel, map[string]string{"token": token}); r.status != 200 {
		t.Fatalf("the QR code still works while the PIN is locked: %d", r.status)
	}
	if r := lookup(hotel, map[string]string{"reference": ref, "pin": "12"}); r.status != 400 {
		t.Fatalf("a short PIN: %d", r.status)
	}
	pool.Exec(ctx, `UPDATE vouchers SET locked_until = NULL, pin_attempts = 0 WHERE booking_id = $1`, id)

	// 5. The guest arrives (the stay begins today): ready, then checked in, and again is harmless.
	pool.Exec(ctx, `UPDATE bookings SET check_in = `+addisToday+`, check_out = `+addisToday+` + 2 WHERE id = $1`, id)
	if r := lookup(hotel, map[string]string{"reference": ref, "pin": pin}); r.str("state") != "ready" {
		t.Fatalf("arriving today is ready: %s", r.raw)
	}
	c := hotel.do("POST", "/partner/bookings/"+id+"/check-in", nil)
	if c.status != 200 || c.str("status") != "CHECKED_IN" || c.str("checked_in_at") == "" {
		t.Fatalf("check in: %d %s", c.status, c.raw)
	}
	first := c.str("checked_in_at")
	if again := hotel.do("POST", "/partner/bookings/"+id+"/check-in", nil); again.status != 200 || again.str("checked_in_at") != first {
		t.Fatalf("checking in twice must be harmless and keep the first time: %d %s", again.status, again.raw)
	}
	if r := lookup(hotel, map[string]string{"token": token}); r.str("state") != "checked_in" {
		t.Fatalf("a second scan says it already happened: %s", r.raw)
	}
	var by *string
	var logged int
	pool.QueryRow(ctx, `SELECT checked_in_by::text FROM bookings WHERE id = $1`, id).Scan(&by)
	pool.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE action = 'guest_checked_in' AND details->>'booking_id' = $1`, id).Scan(&logged)
	if by == nil || logged != 1 {
		t.Fatalf("who checked the guest in must be recorded (by=%v, audit rows=%d)", by, logged)
	}
	found := false
	for _, it := range hotel.do("GET", "/partner/bookings", nil).body["items"].([]any) {
		if m := it.(map[string]any); m["id"] == id && m["status"] == "CHECKED_IN" && m["checked_in_at"] != nil {
			found = true
		}
	}
	if !found {
		t.Fatalf("the hotel's list should show the guest as checked in")
	}
	// A guest who is already in the hotel can no longer cancel.
	if r := guest.do("POST", "/bookings/"+id+"/cancel", nil); r.status == 200 {
		t.Fatalf("a checked-in guest must not be able to cancel")
	}

	// 6. Other states: waiting for the hotel's answer, cancelled, and a stay that is over.
	b2 := hold(guest, quote(guest, meskel, standard, 284, 285, 1, 1).str("id"), fmt.Sprintf("ci-2-%d", n))
	payOnline(t, guest, b2.str("id"), "succeeded")
	if c := hotel.do("POST", "/partner/bookings/"+b2.str("id")+"/check-in", nil); c.status != 409 {
		t.Fatalf("an unanswered request cannot be checked in: %d", c.status)
	}
	hotel.do("POST", "/partner/bookings/"+b2.str("id")+"/accept", nil)
	tok2 := guest.do("GET", "/bookings/"+b2.str("id")+"/voucher", nil).str("token")
	pool.Exec(ctx, `UPDATE bookings SET check_in = `+addisToday+` - 3, check_out = `+addisToday+` - 1 WHERE id = $1`, b2.str("id"))
	if r := lookup(hotel, map[string]string{"token": tok2}); r.str("state") != "ended" {
		t.Fatalf("a finished stay: %s", r.raw)
	}
	pool.Exec(ctx, `UPDATE bookings SET status = 'CANCELLED_REFUNDED' WHERE id = $1`, b2.str("id"))
	if r := lookup(hotel, map[string]string{"token": tok2}); r.str("state") != "cancelled" {
		t.Fatalf("a cancelled booking: %s", r.raw)
	}
	if c := hotel.do("POST", "/partner/bookings/"+b2.str("id")+"/check-in", nil); c.status != 409 {
		t.Fatalf("a cancelled booking cannot be checked in: %d", c.status)
	}
}
