package api_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

// receptionist signs in a receptionist of Meskel; the account is removed when the test ends.
func receptionist(t *testing.T, admin *client, prefix string) *client {
	t.Helper()
	mail := realAddress(prefix)
	if r := admin.do("POST", "/admin/hotels/"+meskel+"/staff", map[string]string{"contact": mail, "role": "receptionist"}); r.status != 200 {
		t.Fatalf("add receptionist: %d %s", r.status, r.raw)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), `DELETE FROM property_staff WHERE user_id IN (SELECT id FROM users WHERE email = $1)`, mail)
	})
	c := newClient(t, "partner")
	c.token = signInWithEmailedCode(c, "/auth/otp/request", "/auth/otp/verify", mail).str("token")
	return c
}

func TestAHotelOwnerCanCancelAConfirmedBookingAndTheGuestIsRefundedInFull(t *testing.T) {
	n := time.Now().UnixNano()
	g, email := realGuest(t, "hcan")
	hotel := hotelFor(t)
	admin := adminClient(t)
	id, _, _, _ := confirmedBooking(t, g, hotel, 60, 62, fmt.Sprintf("hc-1-%d", n))
	paid := paidOf(id)
	room0 := bookedNight(t, standard, 60)
	mails0 := len(mails.to(email))

	desk := receptionist(t, admin, "hcdesk")
	if r := desk.do("POST", "/partner/bookings/"+id+"/cancel", map[string]string{"reason": "overbooked"}); r.status != 403 {
		t.Errorf("a receptionist cannot cancel a guest's booking: %d", r.status)
	}
	if r := hotel.do("POST", "/partner/bookings/"+id+"/cancel", map[string]string{"reason": "tired"}); r.status != 400 {
		t.Errorf("an unknown reason: %d", r.status)
	}
	r := hotel.do("POST", "/partner/bookings/"+id+"/cancel", map[string]string{"reason": "overbooked"})
	if r.status != 200 || r.str("status") != "CANCELLED_REFUNDED" {
		t.Fatalf("cancel: %d %s", r.status, r.raw)
	}
	b := g.do("GET", "/bookings/"+id, nil)
	if b.str("status") != "CANCELLED_REFUNDED" || int64(b.num("refund_santim")) != paid || paid == 0 {
		t.Errorf("the guest is refunded in full: %s", b.raw)
	}
	net := ledgerNetFor(t, id)
	if net["property_payable"] != 0 || net["commission_revenue"] != 0 || net["vat_on_commission_payable"] != 0 || net["refunds_payable"] != 0 {
		t.Errorf("the hotel bears the refund: its share and the commission are reversed: %v", net)
	}
	if paymentStatus(t, id) != "refunded" || refundsPaid(id) != 1 {
		t.Errorf("the money went back once: %s %d", paymentStatus(t, id), refundsPaid(id))
	}
	if bookedNight(t, standard, 60) != room0-1 {
		t.Errorf("the nights are back on sale: %d -> %d", room0, bookedNight(t, standard, 60))
	}
	if got := countSubject(email, "cancelled by the hotel"); got != 1 || len(mails.to(email)) != mails0+1 {
		t.Errorf("the guest is emailed once: %d", got)
	}
	if again := hotel.do("POST", "/partner/bookings/"+id+"/cancel", map[string]string{"reason": "overbooked"}); again.status != 409 {
		t.Errorf("cancelling twice: %d", again.status)
	}
	var by, reason string
	pool.QueryRow(context.Background(), `SELECT cancelled_by, cancel_reason FROM bookings WHERE id = $1`, id).Scan(&by, &reason)
	if by != "hotel" || reason != "overbooked" {
		t.Errorf("it is recorded who cancelled and why: %q %q", by, reason)
	}
}

func TestAHotelCannotCancelABookingThatIsOverOrNotConfirmed(t *testing.T) {
	n := time.Now().UnixNano()
	g, _ := supportGuest(t, "hcno")
	hotel := hotelFor(t)
	waiting := hold(g, quote(g, meskel, standard, 64, 66, 1, 1).str("id"), fmt.Sprintf("hc-w-%d", n)).str("id")
	payOnline(t, g, waiting, "succeeded")
	if r := hotel.do("POST", "/partner/bookings/"+waiting+"/cancel", map[string]string{"reason": "other"}); r.status != 409 {
		t.Errorf("a booking waiting for an answer is declined, not cancelled: %d", r.status)
	}
	done := completedStay(t, g, hotel, 68, 3, fmt.Sprintf("hc-d-%d", n))
	if r := hotel.do("POST", "/partner/bookings/"+done+"/cancel", map[string]string{"reason": "other"}); r.status != 409 {
		t.Errorf("a stay that is over: %d", r.status)
	}
	if r := hotel.do("POST", "/partner/bookings/not-a-uuid/cancel", map[string]string{"reason": "other"}); r.status != 404 {
		t.Errorf("garbled id: %d", r.status)
	}
}

func TestAHotelThatCannotHostAGuestStartsARelocationTheGuestCanEndWithAFullRefund(t *testing.T) {
	n := time.Now().UnixNano()
	g, email := realGuest(t, "hrel")
	hotel := hotelFor(t)
	admin := adminClient(t)
	desk := receptionist(t, admin, "hreldesk")
	id, _, _, _ := confirmedBooking(t, g, hotel, 72, 74, fmt.Sprintf("hr-1-%d", n))
	paid := paidOf(id)

	if r := desk.do("POST", "/partner/bookings/"+id+"/relocate", map[string]string{"reason": "nonsense"}); r.status != 400 {
		t.Errorf("a bad reason: %d", r.status)
	}
	r := desk.do("POST", "/partner/bookings/"+id+"/relocate", map[string]string{"reason": "maintenance"})
	if r.status != 200 || r.str("status") != "NEEDS_RELOCATION" {
		t.Fatalf("a receptionist can report that the hotel cannot host: %d %s", r.status, r.raw)
	}
	if again := hotel.do("POST", "/partner/bookings/"+id+"/relocate", map[string]string{"reason": "maintenance"}); again.status != 409 {
		t.Errorf("a second report: %d", again.status)
	}
	if countSubject(email, "cannot host you") != 1 {
		t.Errorf("the guest is emailed")
	}
	b := g.do("GET", "/bookings/"+id, nil)
	tid := ""
	list := g.do("GET", "/support/tickets", nil)
	for _, x := range list.body["tickets"].([]any) {
		if m := x.(map[string]any); m["booking_id"] == id && m["kind"] == "relocation" {
			tid = m["id"].(string)
		}
	}
	if b.str("status") != "NEEDS_RELOCATION" || b.body["can_cancel"] != true || int64(b.num("refund_if_cancelled_santim")) != paid || tid == "" {
		t.Fatalf("the guest sees the booking under relocation, can cancel for everything, and has a support request: %s / %s", b.raw, list.raw)
	}
	if tk := g.do("GET", "/support/tickets/"+tid, nil); !strings.Contains(string(tk.raw), "cannot host you") || tk.str("ticket", "status") != "answered" {
		t.Errorf("the request starts with what happened: %s", tk.raw)
	}

	// Even a non-refundable booking is refunded in full when the hotel is the one that cannot host (an ordinary guest would get nothing back).
	pool.Exec(context.Background(), `UPDATE bookings SET refundable = false WHERE id = $1`, id)
	c := g.do("POST", "/bookings/"+id+"/cancel", nil)
	if c.status != 200 || int64(c.num("refund_santim")) != paid {
		t.Fatalf("cancel: %d %s", c.status, c.raw)
	}
	if tk := g.do("GET", "/support/tickets/"+tid, nil); tk.str("ticket", "status") != "closed" || tk.str("ticket", "outcome") != "relocation_cancelled" {
		t.Errorf("the request is closed by the cancellation: %s", tk.raw)
	}
	net := ledgerNetFor(t, id)
	if net["property_payable"] != 0 || net["refunds_payable"] != 0 || paymentStatus(t, id) != "refunded" {
		t.Errorf("the hotel bears it: %v %s", net, paymentStatus(t, id))
	}
}

func TestSupportSettlesARelocationByRestoringTheBookingOrRefunding(t *testing.T) {
	n := time.Now().UnixNano()
	hotel := hotelFor(t)
	admin := adminClient(t)

	g1, _ := supportGuest(t, "hrs1")
	id1, _, _, _ := confirmedBooking(t, g1, hotel, 76, 78, fmt.Sprintf("hr-s1-%d", n))
	hotel.do("POST", "/partner/bookings/"+id1+"/relocate", map[string]string{"reason": "overbooked"})
	tid1 := ticketOfKind(t, g1, id1, "relocation")
	for name, r := range map[string]resp{
		"bad outcome": admin.do("POST", "/admin/support/tickets/"+tid1+"/relocation", map[string]string{"outcome": "moved", "note": "We moved them"}),
		"no note":     admin.do("POST", "/admin/support/tickets/"+tid1+"/relocation", map[string]string{"outcome": "restored", "note": " "}),
		"guest":       g1.do("POST", "/admin/support/tickets/"+tid1+"/relocation", map[string]string{"outcome": "restored", "note": "I decide"}),
	} {
		if r.status == 200 {
			t.Errorf("%s must be refused", name)
		}
	}
	if r := admin.do("POST", "/admin/support/tickets/"+tid1+"/relocation", map[string]string{"outcome": "restored", "note": "The hotel found a room after all."}); r.status != 200 {
		t.Fatalf("restore: %d %s", r.status, r.raw)
	}
	if s := bookingStatus(g1, id1); s != "CONFIRMED" {
		t.Errorf("the booking is confirmed again: %s", s)
	}
	if tk := g1.do("GET", "/support/tickets/"+tid1, nil); tk.str("ticket", "outcome") != "relocation_restored" || !strings.Contains(string(tk.raw), "found a room") {
		t.Errorf("the guest reads it: %s", tk.raw)
	}
	if again := admin.do("POST", "/admin/support/tickets/"+tid1+"/relocation", map[string]string{"outcome": "restored", "note": "Again"}); again.status != 409 {
		t.Errorf("settled twice: %d", again.status)
	}

	g2, _ := supportGuest(t, "hrs2")
	id2, _, _, _ := confirmedBooking(t, g2, hotel, 80, 82, fmt.Sprintf("hr-s2-%d", n))
	paid2 := paidOf(id2)
	hotel.do("POST", "/partner/bookings/"+id2+"/relocate", map[string]string{"reason": "emergency"})
	tid2 := ticketOfKind(t, g2, id2, "relocation")
	if r := admin.do("POST", "/admin/support/tickets/"+tid2+"/relocation", map[string]string{"outcome": "refunded", "note": "We found no other hotel for those dates."}); r.status != 200 {
		t.Fatalf("refund: %d %s", r.status, r.raw)
	}
	b := g2.do("GET", "/bookings/"+id2, nil)
	if b.str("status") != "CANCELLED_REFUNDED" || int64(b.num("refund_santim")) != paid2 {
		t.Errorf("the guest is refunded in full: %s", b.raw)
	}
	if net := ledgerNetFor(t, id2); net["property_payable"] != 0 || paymentStatus(t, id2) != "refunded" {
		t.Errorf("the hotel bears it: %v", net)
	}
}

func ticketOfKind(t *testing.T, g *client, bookingID, kind string) string {
	t.Helper()
	list := g.do("GET", "/support/tickets", nil)
	for _, x := range list.body["tickets"].([]any) {
		if m := x.(map[string]any); m["booking_id"] == bookingID && m["kind"] == kind {
			return m["id"].(string)
		}
	}
	t.Fatalf("no %s request for the booking: %s", kind, list.raw)
	return ""
}

func TestTheHotelSeesAGuestsDisputeAndAnswersItButNeverSeesSupportsWords(t *testing.T) {
	n := time.Now().UnixNano()
	g, _ := supportGuest(t, "hdis")
	hotel := hotelFor(t)
	admin := adminClient(t)
	id := ongoingStay(t, g, hotel, fmt.Sprintf("hd-%d", n))

	if r := hotel.do("GET", "/partner/bookings/"+id+"/dispute", nil); r.status != 404 {
		t.Errorf("no dispute yet: %d", r.status)
	}
	if r := hotel.do("POST", "/partner/bookings/"+id+"/dispute/statement", map[string]string{"message": "There is nothing to answer here."}); r.status != 409 {
		t.Errorf("no dispute to answer: %d", r.status)
	}
	tid := dispute(g, id, "not_as_described", "The room had no window and the photos showed a balcony.").str("dispute_ticket_id")
	admin.do("POST", "/admin/support/tickets/"+tid+"/reply", map[string]string{"message": "Internal: please call the manager before deciding."})

	d := hotel.do("GET", "/partner/bookings/"+id+"/dispute", nil)
	if d.status != 200 || d.str("dispute", "reason") != "The stay was not as described" || !strings.Contains(string(d.raw), "no window") {
		t.Fatalf("the hotel reads the reason and the guest's account: %d %s", d.status, d.raw)
	}
	if strings.Contains(string(d.raw), "call the manager") || strings.Contains(string(d.raw), "admin@") {
		t.Errorf("the hotel never sees what support wrote: %s", d.raw)
	}
	if r := hotel.do("POST", "/partner/bookings/"+id+"/dispute/statement", map[string]string{"message": "short"}); r.status != 400 {
		t.Errorf("a statement needs substance: %d", r.status)
	}
	if r := hotel.do("POST", "/partner/bookings/"+id+"/dispute/statement", map[string]string{"message": "Room 12 has a window; the guest was shown the photo of the deluxe room by mistake."}); r.status != 200 {
		t.Fatalf("statement: %d %s", r.status, r.raw)
	}

	// The guest sees the hotel's words as the hotel's; support sees who wrote them.
	tk := g.do("GET", "/support/tickets/"+tid, nil)
	var seen bool
	for _, m := range tk.body["ticket"].(map[string]any)["messages"].([]any) {
		if mm := m.(map[string]any); mm["role"] == "hotel" && mm["author"] == "Hotel" && strings.Contains(mm["body"].(string), "Room 12") {
			seen = true
		}
	}
	if !seen {
		t.Errorf("the guest sees the hotel's statement as the hotel's: %s", tk.raw)
	}
	if a := admin.do("GET", "/admin/support/tickets/"+tid, nil); !strings.Contains(string(a.raw), "hotel:") || a.str("ticket", "status") != "open" {
		t.Errorf("support sees it and the dispute is waiting for support again: %s", a.raw)
	}
	// Another hotel's staff cannot read or answer it (an unrelated owner has no booking here).
	if r := hotel.do("GET", "/partner/bookings/00000000-0000-0000-0000-000000000000/dispute", nil); r.status != 404 {
		t.Errorf("unknown booking: %d", r.status)
	}
}
