package api_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func paidOf(id string) int64 {
	var n int64
	pool.QueryRow(context.Background(), `SELECT amount_santim FROM payments WHERE booking_id = $1 AND status IN ('succeeded', 'refunded')`, id).Scan(&n)
	return n
}

func goodwill(admin *client, ticket, payer string, amount int64, note string) resp {
	return admin.do("POST", "/admin/support/tickets/"+ticket+"/refund", map[string]any{"payer": payer, "amount_santim": amount, "note": note})
}

func partialRows(id string) (owed, paid int) {
	pool.QueryRow(context.Background(), `SELECT count(*) FILTER (WHERE status = 'owed'), count(*) FILTER (WHERE status = 'paid') FROM partial_refunds WHERE booking_id = $1`, id).Scan(&owed, &paid)
	return
}

func abs64(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}

// A confirmed booking in the future, with a support request about it.
func bookingWithTicket(t *testing.T, g *client, hotel *client, in int, key string) (id, ticket string) {
	t.Helper()
	id, _, _, _ = confirmedBooking(t, g, hotel, in, in+2, key)
	r := openTicket(g, "problem", "A problem with my room", "The room had no hot water.", id)
	if r.status != 200 {
		t.Fatalf("ticket: %d %s", r.status, r.raw)
	}
	return id, r.str("id")
}

func TestAPartialRefundShrinksTheHotelsShareInProportionAndPaysTheGuest(t *testing.T) {
	n := time.Now().UnixNano()
	g, _ := supportGuest(t, "prh")
	hotel := hotelFor(t)
	admin := adminClient(t)
	id, tid := bookingWithTicket(t, g, hotel, 100, fmt.Sprintf("pr-h-%d", n))
	paid := paidOf(id)
	before := ledgerNetFor(t, id)
	x := paid / 4

	if r := goodwill(admin, tid, "hotel", x, "No hot water for the first night."); r.status != 200 {
		t.Fatalf("refund: %d %s", r.status, r.raw)
	}
	after := ledgerNetFor(t, id)
	dN, dC, dV := before["property_payable"]-after["property_payable"], before["commission_revenue"]-after["commission_revenue"], before["vat_on_commission_payable"]-after["vat_on_commission_payable"]
	// Property payable and the commission are credits (negative): they shrink, so before - after is negative. The three reversals add up to the refund.
	if abs64(dN)+abs64(dC)+abs64(dV) != x {
		t.Errorf("the hotel's share, the commission and its VAT shrink by exactly what is refunded: %d %d %d vs %d", dN, dC, dV, x)
	}
	if want := before["commission_revenue"] * x / paid; abs64(abs64(dC)-abs64(want)) > 1 {
		t.Errorf("the commission shrinks in proportion: %d vs about %d", dC, want)
	}
	if after["refunds_payable"] != 0 || after["gateway_clearing"]-before["gateway_clearing"] != -x {
		t.Errorf("the gateway paid it and the ledger shows it: refunds_payable %d, clearing moved %d", after["refunds_payable"], after["gateway_clearing"]-before["gateway_clearing"])
	}
	if owed, done := partialRows(id); owed != 0 || done != 1 {
		t.Errorf("one partial refund, paid: owed %d paid %d", owed, done)
	}
	b := g.do("GET", "/bookings/"+id, nil)
	if b.str("status") != "CONFIRMED" || int64(b.num("partially_refunded_santim")) != x || int64(b.num("refund_if_cancelled_santim")) != paid-x {
		t.Errorf("the booking stands; the guest sees what came back and what a cancellation would now return: %s", b.raw)
	}
	if tk := g.do("GET", "/support/tickets/"+tid, nil); !strings.Contains(string(tk.raw), "No hot water for the first night.") || !strings.Contains(string(tk.raw), "refunded") {
		t.Errorf("the guest is told in the conversation: %s", tk.raw)
	}

	// Cancelling afterwards returns only what is left, and the hotel's share goes to zero (it had already borne its part).
	if c := g.do("POST", "/bookings/"+id+"/cancel", nil); c.status != 200 || int64(c.num("refund_santim")) != paid-x {
		t.Fatalf("cancel: %d %s", c.status, c.raw)
	}
	end := ledgerNetFor(t, id)
	if end["property_payable"] != 0 || end["commission_revenue"] != 0 || end["vat_on_commission_payable"] != 0 {
		t.Errorf("after the cancellation nothing is owed to the hotel and BookDan keeps no commission: %v", end)
	}
	if end["refunds_payable"] != 0 || paymentStatus(t, id) != "refunded" {
		t.Errorf("everything went back: refunds_payable %d, payment %s", end["refunds_payable"], paymentStatus(t, id))
	}
	var sum int64
	pool.QueryRow(context.Background(), `SELECT coalesce(sum(e.amount_santim), 0) FROM ledger_entries e JOIN ledger_transactions t ON t.id = e.transaction_id
		WHERE t.booking_id = $1 AND e.account_code = 'gateway_clearing'`, id).Scan(&sum)
	if sum != 0 {
		t.Errorf("the guest got back exactly what they paid, in two gateway refunds: clearing net %d", sum)
	}
}

func TestAPartialRefundPaidByBookDanLeavesTheHotelItsShare(t *testing.T) {
	n := time.Now().UnixNano()
	g, _ := supportGuest(t, "prp")
	hotel := hotelFor(t)
	admin := adminClient(t)
	id, tid := bookingWithTicket(t, g, hotel, 104, fmt.Sprintf("pr-p-%d", n))
	paid := paidOf(id)
	before := ledgerNetFor(t, id)
	x := paid / 5
	if r := goodwill(admin, tid, "platform", x, "A goodwill credit for the trouble."); r.status != 200 {
		t.Fatalf("refund: %d %s", r.status, r.raw)
	}
	after := ledgerNetFor(t, id)
	if after["property_payable"] != before["property_payable"] || after["commission_revenue"] != before["commission_revenue"] {
		t.Errorf("the hotel keeps its share and BookDan its commission: %v -> %v", before, after)
	}
	if after["dispute_expense"] != x {
		t.Errorf("BookDan books the credit as its own expense: %d vs %d", after["dispute_expense"], x)
	}
	// Cancelling now reverses the hotel's share only for what is left: the part BookDan already paid stays with the hotel.
	if c := g.do("POST", "/bookings/"+id+"/cancel", nil); c.status != 200 {
		t.Fatalf("cancel: %d %s", c.status, c.raw)
	}
	end := ledgerNetFor(t, id)
	keeps := -before["property_payable"] * x / paid // what BookDan's own money bought for the hotel
	if abs64(-end["property_payable"]-keeps) > 1 {
		t.Errorf("the hotel keeps about its proportion of the part BookDan paid: %d vs %d", -end["property_payable"], keeps)
	}
	if end["refunds_payable"] != 0 || paymentStatus(t, id) != "refunded" {
		t.Errorf("all of it is paid back: %v %s", end, paymentStatus(t, id))
	}
}

func TestAPartialRefundIsCheckedAndNeverExceedsThePayment(t *testing.T) {
	n := time.Now().UnixNano()
	g, _ := supportGuest(t, "prv")
	other, _ := supportGuest(t, "prv2")
	hotel := hotelFor(t)
	admin := adminClient(t)
	id, tid := bookingWithTicket(t, g, hotel, 108, fmt.Sprintf("pr-v-%d", n))
	paid := paidOf(id)

	for name, r := range map[string]resp{
		"zero":            goodwill(admin, tid, "hotel", 0, "Nothing at all"),
		"negative":        goodwill(admin, tid, "hotel", -5, "Minus five"),
		"the whole thing": goodwill(admin, tid, "hotel", paid, "That is a cancellation"),
		"more":            goodwill(admin, tid, "hotel", paid+1, "More than paid"),
		"no payer":        goodwill(admin, tid, "", 100, "Who pays"),
		"unknown payer":   goodwill(admin, tid, "the moon", 100, "Who pays"),
		"no reason":       goodwill(admin, tid, "hotel", 100, " "),
		"guest":           goodwill(g, tid, "hotel", 100, "I will refund myself"),
	} {
		if r.status == 200 {
			t.Errorf("%s must be refused", name)
		}
		if r.status >= 500 {
			t.Errorf("%s must not be a server error: %d %s", name, r.status, r.raw)
		}
	}
	if owed, done := partialRows(id); owed+done != 0 {
		t.Fatalf("refused refunds leave nothing behind")
	}

	// Several partial refunds are fine until the payment is used up.
	big := paid * 6 / 10
	if r := goodwill(admin, tid, "hotel", big, "Sixty percent"); r.status != 200 {
		t.Fatalf("first: %d %s", r.status, r.raw)
	}
	if r := goodwill(admin, tid, "platform", big, "Another sixty percent"); r.status != 409 || r.str("error", "code") != "REFUND_TOO_LARGE" {
		t.Errorf("together they would be more than was paid: %d %s", r.status, r.raw)
	}
	if r := goodwill(admin, tid, "platform", paid-big-1, "Almost everything else"); r.status != 200 {
		t.Errorf("up to what is left: %d %s", r.status, r.raw)
	}
	if owed, done := partialRows(id); owed != 0 || done != 2 {
		t.Errorf("two partial refunds: %d %d", owed, done)
	}

	// Not about a booking, a booking that is not confirmed, and someone else's ticket.
	plain := openTicket(g, "question", "A question", "How does it work?", "")
	if r := goodwill(admin, plain.str("id"), "hotel", 100, "No booking here"); r.status != 409 {
		t.Errorf("a request that is not about a booking: %d", r.status)
	}
	waiting := hold(other, quote(other, meskel, standard, 112, 114, 1, 1).str("id"), fmt.Sprintf("pr-w-%d", n)).str("id")
	payOnline(t, other, waiting, "succeeded") // paid, waiting for the hotel: nothing is booked yet
	wt := openTicket(other, "problem", "Waiting for the hotel", "Still waiting", waiting).str("id")
	if r := goodwill(admin, wt, "hotel", 100, "Too early"); r.status != 409 {
		t.Errorf("a booking the hotel has not confirmed: %d %s", r.status, r.raw)
	}
	// A closed request takes no refund.
	admin.do("POST", "/admin/support/tickets/"+tid+"/close", nil)
	if r := goodwill(admin, tid, "hotel", 1, "Too late"); r.status != 409 {
		t.Errorf("a closed request: %d", r.status)
	}
}

func TestTwoPartialRefundsAtOnceCannotSpendTheSameMoneyTwice(t *testing.T) {
	n := time.Now().UnixNano()
	g, _ := supportGuest(t, "prc")
	hotel := hotelFor(t)
	admin := adminClient(t)
	id, tid := bookingWithTicket(t, g, hotel, 116, fmt.Sprintf("pr-c-%d", n))
	paid := paidOf(id)
	each := paid * 6 / 10
	var wg sync.WaitGroup
	start := make(chan struct{})
	var mu sync.Mutex
	ok := 0
	for _, payer := range []string{"hotel", "platform", "hotel", "platform"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if status, _ := rawDo("admin", admin.token, admin.deviceID, "POST", "/admin/support/tickets/"+tid+"/refund", map[string]any{"payer": payer, "amount_santim": each, "note": "A race between two admins"}); status == 200 {
				mu.Lock()
				ok++
				mu.Unlock()
			}
		}()
	}
	close(start)
	wg.Wait()
	if ok != 1 {
		t.Errorf("only one of the 60%% refunds fits: %d succeeded", ok)
	}
	var total int64
	pool.QueryRow(context.Background(), `SELECT coalesce(sum(amount_santim), 0) FROM partial_refunds WHERE booking_id = $1`, id).Scan(&total)
	if total > paid {
		t.Errorf("never more refunded than paid: %d of %d", total, paid)
	}
}

func TestADisputeCanBeSettledForPartOfThePrice(t *testing.T) {
	n := time.Now().UnixNano()
	g, _ := supportGuest(t, "prd")
	hotel := hotelFor(t)
	admin := adminClient(t)
	id := ongoingStay(t, g, hotel, fmt.Sprintf("pr-d-%d", n))
	paid := paidOf(id)
	tid := dispute(g, id, "not_as_described", "The room was smaller than the photos showed and had no window.").str("dispute_ticket_id")
	before := ledgerNetFor(t, id)

	for name, r := range map[string]resp{
		"no amount": resolve(admin, tid, "refunded_partial_hotel_pays", "Part of it"),
		"too much":  admin.do("POST", "/admin/support/tickets/"+tid+"/resolve", map[string]any{"outcome": "refunded_partial_hotel_pays", "amount_santim": paid, "note": "All of it is a full refund"}),
	} {
		if r.status == 200 {
			t.Errorf("%s must be refused", name)
		}
	}
	if bookingStatus(g, id) != "DISPUTED" {
		t.Fatalf("a refused decision changes nothing")
	}
	x := paid * 3 / 10
	r := admin.do("POST", "/admin/support/tickets/"+tid+"/resolve", map[string]any{"outcome": "refunded_partial_hotel_pays", "amount_santim": x, "note": "The room was not as shown; we refund 30 percent."})
	if r.status != 200 {
		t.Fatalf("resolve: %d %s", r.status, r.raw)
	}
	if s := bookingStatus(g, id); s != "CONFIRMED" {
		t.Errorf("the booking stands and goes back to what it was: %s", s)
	}
	tk := g.do("GET", "/support/tickets/"+tid, nil)
	if tk.str("ticket", "status") != "closed" || tk.str("ticket", "outcome") != "refunded_partial_hotel_pays" || !strings.Contains(string(tk.raw), "upheld in part") {
		t.Errorf("the guest reads the decision: %s", tk.raw)
	}
	after := ledgerNetFor(t, id)
	if before["property_payable"] == after["property_payable"] {
		t.Errorf("the hotel pays its part: %v -> %v", before, after)
	}
	b := g.do("GET", "/bookings/"+id, nil)
	if int64(b.num("partially_refunded_santim")) != x {
		t.Errorf("the guest sees the part refunded: %s", b.raw)
	}

	// Platform-paid part on a second dispute.
	g2, _ := supportGuest(t, "prd2")
	id2 := ongoingStay(t, g2, hotel, fmt.Sprintf("pr-d2-%d", n))
	tid2 := dispute(g2, id2, "charged_wrong", "I was charged for breakfast I did not have.").str("dispute_ticket_id")
	b2 := ledgerNetFor(t, id2)
	y := paidOf(id2) / 10
	if r := admin.do("POST", "/admin/support/tickets/"+tid2+"/resolve", map[string]any{"outcome": "refunded_partial_platform_pays", "amount_santim": y, "note": "Breakfast refunded by BookDan."}); r.status != 200 {
		t.Fatalf("resolve 2: %d %s", r.status, r.raw)
	}
	a2 := ledgerNetFor(t, id2)
	if a2["property_payable"] != b2["property_payable"] || a2["dispute_expense"] != y {
		t.Errorf("BookDan pays: %v -> %v", b2, a2)
	}
}

func TestAnAdminSeesWhatIsLeftToRefundNextToABooking(t *testing.T) {
	n := time.Now().UnixNano()
	g, _ := supportGuest(t, "prs")
	hotel := hotelFor(t)
	admin := adminClient(t)
	id, tid := bookingWithTicket(t, g, hotel, 120, fmt.Sprintf("pr-s-%d", n))
	paid := paidOf(id)
	goodwill(admin, tid, "platform", paid/10, "A small credit for the inconvenience")
	got := admin.do("GET", "/admin/support/tickets/"+tid, nil)
	bk, _ := got.body["ticket"].(map[string]any)["booking"].(map[string]any)
	if int64(bk["paid_santim"].(float64)) != paid || int64(bk["refunded_santim"].(float64)) != paid/10 || int64(bk["left_santim"].(float64)) != paid-paid/10 {
		t.Errorf("paid, refunded and left: %v (paid %d)", bk, paid)
	}
}
