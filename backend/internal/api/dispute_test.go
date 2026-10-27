package api_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

// A stay that is under way: the guest checks in today, so a dispute can be opened.
func ongoingStay(t *testing.T, g *client, hotel *client, key string) string {
	t.Helper()
	id, _, _, _ := confirmedBooking(t, g, hotel, 0, 2, key)
	return id
}

// completedStay is a stay that ended `daysAgo` days ago (its dates moved back in the database, then the daily job completes it).
func completedStay(t *testing.T, g *client, hotel *client, in int, daysAgo int, key string) string {
	t.Helper()
	id, _, _, _ := confirmedBooking(t, g, hotel, in, in+2, key)
	pool.Exec(context.Background(), fmt.Sprintf(`UPDATE bookings SET check_in = %s - %d, check_out = %s - %d WHERE id = $1`, addisToday, daysAgo+2, addisToday, daysAgo), id)
	app.RunMaintenance(context.Background())
	return id
}

func dispute(g *client, id, reason, message string) resp {
	return g.do("POST", "/bookings/"+id+"/dispute", map[string]string{"reason": reason, "message": message})
}

func bookingStatus(g *client, id string) string {
	return g.do("GET", "/bookings/"+id, nil).str("status")
}

func disputeTicket(g *client, id string) string {
	return g.do("GET", "/bookings/"+id, nil).str("dispute_ticket_id")
}

func resolve(admin *client, ticket, outcome, note string) resp {
	return admin.do("POST", "/admin/support/tickets/"+ticket+"/resolve", map[string]string{"outcome": outcome, "note": note})
}

func TestADisputeFreezesTheBookingsMoneyAndIsOpenedOnlyWhenItMakesSense(t *testing.T) {
	n := time.Now().UnixNano()
	g, _ := supportGuest(t, "disp")
	other, _ := supportGuest(t, "dispo")
	hotel := hotelFor(t)
	admin := adminClient(t)

	// A stay to come is cancelled, not disputed.
	future, _, _, _ := confirmedBooking(t, g, hotel, 195, 197, fmt.Sprintf("disp-f-%d", n))
	if b := g.do("GET", "/bookings/"+future, nil); b.body["can_dispute"] != false {
		t.Errorf("a stay that has not started cannot be disputed: %s", b.raw)
	}
	if r := dispute(g, future, "other", "I want to complain before I even arrive"); r.status != 409 {
		t.Errorf("before the stay: %d %s", r.status, r.raw)
	}

	// A stay that ended is payable to the hotel; a dispute takes it out of what is payable.
	id := completedStay(t, g, hotel, 197, 3, fmt.Sprintf("disp-c-%d", n))
	if bookingStatus(g, id) != "COMPLETED" {
		t.Fatalf("setup: the stay should be completed: %s", bookingStatus(g, id))
	}
	ready0, _, _, _ := hotelMoney(t, admin, meskel)
	if b := g.do("GET", "/bookings/"+id, nil); b.body["can_dispute"] != true {
		t.Fatalf("a finished stay can be disputed: %s", b.raw)
	}

	for name, r := range map[string]resp{
		"unknown reason": dispute(g, id, "angry", "The room was not what the photos showed"),
		"short message":  dispute(g, id, "other", "bad"),
		"empty message":  dispute(g, id, "other", "   "),
		"not my booking": dispute(other, id, "other", "This is not my booking at all"),
	} {
		if r.status != 400 && r.status != 404 {
			t.Errorf("%s must be refused: %d", name, r.status)
		}
	}

	r := dispute(g, id, "not_as_described", "The room had no window and the photos showed a balcony.")
	if r.status != 200 || r.str("status") != "DISPUTED" {
		t.Fatalf("raise: %d %s", r.status, r.raw)
	}
	tid := r.str("dispute_ticket_id")
	if tid == "" || r.body["can_dispute"] != false {
		t.Fatalf("the booking points to its dispute and cannot be disputed twice: %s", r.raw)
	}
	if again := dispute(g, id, "other", "Trying to open a second dispute"); again.status != 409 || again.str("error", "code") != "DISPUTE_EXISTS" {
		t.Errorf("one dispute per booking: %d %s", again.status, again.raw)
	}

	ready1, _, _, _ := hotelMoney(t, admin, meskel)
	if ready1 >= ready0 {
		t.Errorf("a disputed booking's money is not payable: ready %d -> %d", ready0, ready1)
	}
	if c := g.do("POST", "/bookings/"+id+"/cancel", nil); c.status != 409 {
		t.Errorf("a disputed booking cannot be cancelled: %d", c.status)
	}
	if d := g.do("DELETE", "/me/account", map[string]string{"confirm": "DELETE"}); d.status != 409 {
		t.Errorf("an account with a dispute open cannot be deleted: %d", d.status)
	}

	// The conversation is a support ticket, and support sees the booking next to it.
	got := admin.do("GET", "/admin/support/tickets/"+tid, nil)
	if got.status != 200 || got.str("ticket", "kind") != "dispute" || got.str("ticket", "booking", "status") != "DISPUTED" || got.str("ticket", "booking", "disputed_from") != "COMPLETED" {
		t.Fatalf("the admin sees the dispute and its booking: %s", got.raw)
	}
	// Support cannot just close a dispute; it must decide.
	if c := admin.do("POST", "/admin/support/tickets/"+tid+"/close", nil); c.status != 409 {
		t.Errorf("closing a dispute without a decision: %d", c.status)
	}
	// Writing to the hotel owner's side: another guest cannot read it.
	if x := other.do("GET", "/support/tickets/"+tid, nil); x.status != 404 {
		t.Errorf("another guest's dispute: %d", x.status)
	}
}

func TestAGuestCanWithdrawADisputeAndTheBookingGoesBack(t *testing.T) {
	n := time.Now().UnixNano()
	g, _ := supportGuest(t, "dispw")
	hotel := hotelFor(t)
	id := completedStay(t, g, hotel, 199, 2, fmt.Sprintf("disp-w-%d", n))
	tid := dispute(g, id, "other", "I think I was overcharged for the stay").str("dispute_ticket_id")
	if bookingStatus(g, id) != "DISPUTED" {
		t.Fatalf("setup")
	}
	if c := g.do("POST", "/support/tickets/"+tid+"/close", nil); c.status != 200 {
		t.Fatalf("withdraw: %d %s", c.status, c.raw)
	}
	if s := bookingStatus(g, id); s != "COMPLETED" {
		t.Errorf("a withdrawn dispute puts the booking back: %s", s)
	}
	if tk := g.do("GET", "/support/tickets/"+tid, nil); tk.str("ticket", "status") != "closed" || tk.str("ticket", "outcome") != "withdrawn" {
		t.Errorf("the ticket is closed as withdrawn: %s", tk.raw)
	}
	if again := dispute(g, id, "other", "Opening it again after withdrawing"); again.status != 409 {
		t.Errorf("a withdrawn dispute is not reopened by a new one: %d", again.status)
	}
}

func TestRejectingADisputeRestoresTheBookingAndTellsTheGuest(t *testing.T) {
	n := time.Now().UnixNano()
	g, email := supportGuest(t, "dispr")
	hotel := hotelFor(t)
	admin := adminClient(t)
	id := ongoingStay(t, g, hotel, fmt.Sprintf("disp-r-%d", n))
	if b := g.do("GET", "/bookings/"+id, nil); b.body["can_dispute"] != true {
		t.Fatalf("a stay that started can be disputed: %s", b.raw)
	}
	tid := dispute(g, id, "refused_entry", "The desk would not give me my room tonight.").str("dispute_ticket_id")
	if bookingStatus(g, id) != "DISPUTED" {
		t.Fatalf("setup")
	}
	mailsBefore := len(mails.to(email))
	if c := hotel.do("POST", "/partner/bookings/"+id+"/check-in", nil); c.status != 409 || !strings.Contains(string(c.raw), "dispute") {
		t.Errorf("the hotel cannot check in a disputed booking, and is told why: %d %s", c.status, c.raw)
	}

	for name, r := range map[string]resp{
		"bad outcome": resolve(admin, tid, "split_it", "Some reason here"),
		"no note":     resolve(admin, tid, "rejected", " "),
		"guest":       resolve(g, tid, "rejected", "I am the guest"),
	} {
		if r.status == 200 {
			t.Errorf("%s must be refused", name)
		}
	}
	if bookingStatus(g, id) != "DISPUTED" {
		t.Fatalf("a refused decision changes nothing")
	}

	if r := resolve(admin, tid, "rejected", "The hotel shows you were given room 12 at 14:05."); r.status != 200 {
		t.Fatalf("reject: %d %s", r.status, r.raw)
	}
	if s := bookingStatus(g, id); s != "CONFIRMED" {
		t.Errorf("the booking goes back to what it was: %s", s)
	}
	tk := g.do("GET", "/support/tickets/"+tid, nil)
	if tk.str("ticket", "status") != "closed" || tk.str("ticket", "outcome") != "rejected" || !strings.Contains(string(tk.raw), "room 12 at 14:05") || !strings.Contains(string(tk.raw), "not upheld") {
		t.Errorf("the guest reads the decision and the reason: %s", tk.raw)
	}
	if len(mails.to(email)) != mailsBefore+1 {
		t.Errorf("the guest is emailed the decision")
	}
	if again := resolve(admin, tid, "rejected", "Deciding a second time"); again.status != 409 {
		t.Errorf("a resolved dispute cannot be resolved again: %d", again.status)
	}
	// The guest can still use the booking: the voucher works and the hotel can check them in.
	if v := g.do("GET", "/bookings/"+id+"/voucher", nil); v.status != 200 {
		t.Errorf("the voucher still works: %d", v.status)
	}
}

func TestUpholdingADisputeRefundsTheGuestInFullAndTheHotelPaysOrBookDanDoes(t *testing.T) {
	n := time.Now().UnixNano()
	admin := adminClient(t)
	hotel := hotelFor(t)

	// 1. The hotel pays: its share and BookDan's commission are reversed, exactly as for a cancellation.
	g1, _ := supportGuest(t, "dispu1")
	id1 := ongoingStay(t, g1, hotel, fmt.Sprintf("disp-u1-%d", n))
	taken0 := takenNights(t, id1)
	if taken0 == 0 {
		t.Fatalf("setup: the booking holds rooms")
	}
	tid1 := dispute(g1, id1, "not_as_described", "The room was not the one I booked and I left.").str("dispute_ticket_id")
	if r := resolve(admin, tid1, "refunded_hotel_pays", "Confirmed with the hotel: the room was not available."); r.status != 200 {
		t.Fatalf("resolve: %d %s", r.status, r.raw)
	}
	b1 := g1.do("GET", "/bookings/"+id1, nil)
	var paid1 int64
	pool.QueryRow(context.Background(), `SELECT amount_santim FROM payments WHERE booking_id = $1`, id1).Scan(&paid1)
	if b1.str("status") != "CANCELLED_REFUNDED" || int64(b1.num("refund_santim")) != paid1 || paid1 == 0 {
		t.Fatalf("the guest is refunded everything: %s (paid %d)", b1.raw, paid1)
	}
	if shown := admin.do("GET", "/admin/support/tickets/"+tid1, nil); paidShown(shown) != paid1 {
		t.Errorf("the admin still sees what was paid after the refund: %s", shown.raw)
	}
	net1 := ledgerNetFor(t, id1)
	if net1["property_payable"] != 0 || net1["commission_revenue"] != 0 || net1["vat_on_commission_payable"] != 0 {
		t.Errorf("the hotel's share and the commission are reversed: %v", net1)
	}
	if paymentStatus(t, id1) != "refunded" || refundsPaid(id1) != 1 {
		t.Errorf("the money went back through the gateway once: payment %s, refunds paid %d", paymentStatus(t, id1), refundsPaid(id1))
	}
	if takenNights(t, id1) >= taken0 {
		t.Errorf("nights still to come are back on sale: %d -> %d", taken0, takenNights(t, id1))
	}
	if tk := g1.do("GET", "/support/tickets/"+tid1, nil); tk.str("ticket", "outcome") != "refunded_hotel_pays" || !strings.Contains(string(tk.raw), "refunded in full") {
		t.Errorf("the guest is told: %s", tk.raw)
	}

	// 2. BookDan pays: the guest is refunded in full, the hotel keeps its share, BookDan books an expense.
	g2, _ := supportGuest(t, "dispu2")
	id2 := completedStay(t, g2, hotel, 201, 2, fmt.Sprintf("disp-u2-%d", n))
	tid2 := dispute(g2, id2, "charged_wrong", "I was charged for two nights but stayed one night.").str("dispute_ticket_id")
	before := ledgerNetFor(t, id2)
	if r := resolve(admin, tid2, "refunded_platform_pays", "Our mistake, refunded from BookDan."); r.status != 200 {
		t.Fatalf("resolve: %d %s", r.status, r.raw)
	}
	var paid2 int64
	pool.QueryRow(context.Background(), `SELECT amount_santim FROM payments WHERE booking_id = $1`, id2).Scan(&paid2)
	after := ledgerNetFor(t, id2)
	if bookingStatus(g2, id2) != "CANCELLED_REFUNDED" {
		t.Fatalf("status %s", bookingStatus(g2, id2))
	}
	if after["property_payable"] != before["property_payable"] || after["property_payable"] == 0 {
		t.Errorf("the hotel keeps its share: %v -> %v", before, after)
	}
	if after["dispute_expense"] != paid2 {
		t.Errorf("BookDan books the refund as its own expense: %v (paid %d)", after, paid2)
	}
	if paymentStatus(t, id2) != "refunded" || refundsPaid(id2) != 1 {
		t.Errorf("the guest got their money back once: %s, %d", paymentStatus(t, id2), refundsPaid(id2))
	}
}

func refundsPaid(id string) int {
	var n int
	pool.QueryRow(context.Background(), `SELECT count(*) FROM ledger_transactions WHERE booking_id = $1 AND kind = 'refund' AND memo LIKE 'refund paid%'`, id).Scan(&n)
	return n
}

func takenNights(t *testing.T, bookingID string) int {
	t.Helper()
	var room, in, out string
	pool.QueryRow(context.Background(), `SELECT room_type_id::text, check_in::text, check_out::text FROM bookings WHERE id = $1`, bookingID).Scan(&room, &in, &out)
	var n int
	pool.QueryRow(context.Background(), `SELECT coalesce(sum(booked + held), 0)::int FROM inventory WHERE room_type_id = $1 AND night >= $2::date AND night < $3::date`, room, in, out).Scan(&n)
	return n
}

func TestADisputeAboutABookingPaidAtTheHotelCannotBeRefundedOnline(t *testing.T) {
	n := time.Now().UnixNano()
	g, _ := supportGuest(t, "dispp")
	admin := adminClient(t)
	id := hold(g, quote(g, bole, boleSingle, 0, 1, 1, 1).str("id"), fmt.Sprintf("disp-p-%d", n)).str("id")
	if r := g.do("POST", "/bookings/"+id+"/pay", map[string]string{"method": "pay_at_hotel"}); r.status != 200 {
		t.Fatalf("setup pay: %d %s", r.status, r.raw)
	}
	if _, err := pool.Exec(context.Background(), `UPDATE bookings SET status = 'CONFIRMED' WHERE id = $1`, id); err != nil {
		t.Fatalf("setup: %v", err)
	}
	tid := dispute(g, id, "other", "The hotel asked for more money at the desk.").str("dispute_ticket_id")
	if tid == "" {
		t.Fatalf("a pay-at-hotel stay can still be disputed")
	}
	if r := resolve(admin, tid, "refunded_hotel_pays", "Refund please"); r.status != 400 || r.str("error", "code") != "NO_PAYMENT" {
		t.Errorf("nothing was paid online, so nothing can be refunded: %d %s", r.status, r.raw)
	}
	if bookingStatus(g, id) != "DISPUTED" {
		t.Errorf("a refused decision leaves the dispute open")
	}
	if r := resolve(admin, tid, "rejected", "The hotel's price was as agreed."); r.status != 200 {
		t.Errorf("it can still be rejected: %d %s", r.status, r.raw)
	}
}

func TestADisputeIsOpenedWithin14DaysOfCheckOutNotAfter(t *testing.T) {
	n := time.Now().UnixNano()
	g, _ := supportGuest(t, "dispt")
	hotel := hotelFor(t)
	old := completedStay(t, g, hotel, 203, 20, fmt.Sprintf("disp-old-%d", n))
	if b := g.do("GET", "/bookings/"+old, nil); b.body["can_dispute"] != false {
		t.Errorf("20 days after check-out is too late: %s", b.raw)
	}
	if r := dispute(g, old, "other", "I found a problem a long time afterwards"); r.status != 409 || r.str("error", "code") != "DISPUTE_WINDOW_CLOSED" {
		t.Errorf("window: %d %s", r.status, r.raw)
	}
	recent := completedStay(t, g, hotel, 205, 13, fmt.Sprintf("disp-rec-%d", n))
	if r := dispute(g, recent, "other", "A problem found a couple of weeks later"); r.status != 200 {
		t.Errorf("13 days after check-out is still fine: %d %s", r.status, r.raw)
	}
}

// paidShown reads ticket.booking.paid_santim from an admin ticket response.
func paidShown(r resp) int64 {
	tk, _ := r.body["ticket"].(map[string]any)
	bk, _ := tk["booking"].(map[string]any)
	n, _ := bk["paid_santim"].(float64)
	return int64(n)
}
