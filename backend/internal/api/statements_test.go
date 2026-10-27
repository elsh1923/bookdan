package api_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

// payAtHotelStay books a pay-at-hotel stay at Bole, has the hotel accept it, and moves its dates into the past so the daily job completes it.
func payAtHotelStay(t *testing.T, g, hotel *client, in int, daysAgo int, key string) string {
	t.Helper()
	id := hold(g, quote(g, bole, boleSingle, in, in+1, 1, 1).str("id"), key).str("id")
	if r := g.do("POST", "/bookings/"+id+"/pay", map[string]string{"method": "pay_at_hotel"}); r.status != 200 {
		t.Fatalf("pay at hotel: %d %s", r.status, r.raw)
	}
	if r := hotel.do("POST", "/partner/bookings/"+id+"/accept", nil); r.status != 200 {
		t.Fatalf("accept: %d %s", r.status, r.raw)
	}
	pool.Exec(context.Background(), fmt.Sprintf(`UPDATE bookings SET check_in = %s - %d, check_out = %s - %d WHERE id = $1`, addisToday, daysAgo+1, addisToday, daysAgo), id)
	app.RunMaintenance(context.Background())
	return id
}

func owesOf(t *testing.T, admin *client, id string) (owes, toPay int64) {
	t.Helper()
	for _, h := range admin.do("GET", "/admin/payouts", nil).body["hotels"].([]any) {
		if m := h.(map[string]any); m["id"] == id {
			return int64(m["owes_santim"].(float64)), int64(m["to_pay_santim"].(float64))
		}
	}
	return 0, 0
}

func TestACompletedPayAtHotelStayBooksBookDansCommissionAgainstTheHotel(t *testing.T) {
	ctx := context.Background()
	n := time.Now().UnixNano()
	g, _ := supportGuest(t, "pah")
	hotel := hotelFor(t)
	admin := adminClient(t)
	pool.Exec(ctx, `UPDATE pricing_config SET commission_bps = 1500`)
	owes0, toPay0 := owesOf(t, admin, bole)

	id := payAtHotelStay(t, g, hotel, 96, 3, fmt.Sprintf("pah-%d", n))
	var total, commBP, vatBP int64
	pool.QueryRow(ctx, `SELECT total_santim FROM bookings WHERE id = $1`, id).Scan(&total)
	pool.QueryRow(ctx, `SELECT commission_bps, commission_vat_bps FROM pricing_config WHERE id = 1`).Scan(&commBP, &vatBP)
	commission := (total*commBP + 5000) / 10000
	vat := (commission*vatBP + 5000) / 10000

	net := ledgerNetFor(t, id)
	if net["commission_revenue"] != -commission || net["vat_on_commission_payable"] != -vat || net["property_payable"] != commission+vat {
		t.Errorf("BookDan books its commission and the VAT on it, and it reduces what it owes the hotel: %v (commission %d vat %d)", net, commission, vat)
	}
	if net["guest_deposits_held"] != 0 || net["gateway_clearing"] != 0 {
		t.Errorf("no guest money passed through BookDan: %v", net)
	}
	// Only once: running the daily job again books nothing more.
	app.RunMaintenance(ctx)
	if again := ledgerNetFor(t, id); again["property_payable"] != net["property_payable"] {
		t.Errorf("the commission is booked once: %v", again)
	}
	var marked bool
	pool.QueryRow(ctx, `SELECT hotel_commission_booked_at IS NOT NULL FROM bookings WHERE id = $1`, id).Scan(&marked)
	if !marked {
		t.Errorf("the booking is marked")
	}
	owes1, toPay1 := owesOf(t, admin, bole)
	if toPay1 > toPay0 || (owes1 == owes0 && toPay1 == toPay0) {
		t.Errorf("the hotel's position moved against it: owes %d -> %d, to pay %d -> %d", owes0, owes1, toPay0, toPay1)
	}
}

func TestAHotelThatOwesBookDanRecordsWhatItPaidBack(t *testing.T) {
	ctx := context.Background()
	n := time.Now().UnixNano()
	g, _ := supportGuest(t, "pahr")
	hotel := hotelFor(t)
	admin := adminClient(t)
	pool.Exec(ctx, `UPDATE pricing_config SET commission_bps = 1500`)
	payAtHotelStay(t, g, hotel, 98, 3, fmt.Sprintf("pahr-%d", n))
	owes, _ := owesOf(t, admin, bole)
	if owes <= 0 {
		t.Skip("setup: the hotel owes nothing (its online earnings cover the commission): the receipt rules need a debt")
	}
	if r := admin.do("POST", "/admin/hotels/"+bole+"/receipts", map[string]any{"amount_santim": owes + 1, "reference": "TRX-TOO-MUCH"}); r.status != 409 {
		t.Errorf("more than the hotel owes: %d", r.status)
	}
	for name, body := range map[string]map[string]any{"zero": {"amount_santim": 0, "reference": "TRX-1"}, "no reference": {"amount_santim": 100, "reference": " "}} {
		if r := admin.do("POST", "/admin/hotels/"+bole+"/receipts", body); r.status != 400 {
			t.Errorf("%s: %d", name, r.status)
		}
	}
	if r := hotel.do("POST", "/admin/hotels/"+bole+"/receipts", map[string]any{"amount_santim": 100, "reference": "TRX-HOTEL"}); r.status != 401 && r.status != 403 {
		t.Errorf("a hotel cannot record receipts: %d", r.status)
	}
	part := owes / 2
	if part == 0 {
		part = owes
	}
	if r := admin.do("POST", "/admin/hotels/"+bole+"/receipts", map[string]any{"amount_santim": part, "reference": fmt.Sprintf("TRX-%d", n)}); r.status != 200 {
		t.Fatalf("receipt: %d %s", r.status, r.raw)
	}
	if after, _ := owesOf(t, admin, bole); after != owes-part {
		t.Errorf("what the hotel owes goes down by what it paid: %d -> %d (paid %d)", owes, after, part)
	}
}

func TestAMonthlyStatementReconcilesAndHandsOverASafeCSV(t *testing.T) {
	ctx := context.Background()
	n := time.Now().UnixNano()
	g, _ := supportGuest(t, "stm")
	hotel := hotelFor(t)
	admin := adminClient(t)
	desk := receptionist(t, admin, "stmdesk")
	pool.Exec(ctx, `UPDATE pricing_config SET commission_bps = 1500`)

	online := completedStay(t, g, hotel, 102, 1, fmt.Sprintf("stm-1-%d", n))
	pool.Exec(ctx, `UPDATE bookings SET guest_name = '=HYPERLINK("http://evil.example","click")' WHERE id = $1`, online)
	var month string
	pool.QueryRow(ctx, `SELECT to_char(check_out, 'YYYY-MM') FROM bookings WHERE id = $1`, online).Scan(&month)

	r := hotel.do("GET", "/partner/statements?month="+month+"&property_id="+meskel, nil)
	if r.status != 200 {
		t.Fatalf("statement: %d %s", r.status, r.raw)
	}
	st := r.body["statements"].([]any)[0].(map[string]any)
	var found bool
	var movement int64
	for _, l := range st["lines"].([]any) {
		m := l.(map[string]any)
		movement += int64(m["net_santim"].(float64))
		if m["method"] == "online" && strings.Contains(m["guest_name"].(string), "HYPERLINK") {
			found = true
		}
	}
	if !found {
		t.Errorf("the online stay is on the statement of the month it ended: %s", r.raw)
	}
	opening, closing := int64(st["opening_santim"].(float64)), int64(st["closing_santim"].(float64))
	paid, recv := int64(st["paid_santim"].(float64)), int64(st["received_santim"].(float64))
	if closing != opening+movement-paid+recv {
		t.Errorf("the statement reconciles: opening %d + stays %d - paid %d + received %d != closing %d", opening, movement, paid, recv, closing)
	}
	if got := int64(st["online_commission_santim"].(float64)) + int64(st["online_hotel_share_santim"].(float64)) + int64(st["online_vat_santim"].(float64)); got != int64(st["online_gross_santim"].(float64)) {
		t.Errorf("online price = hotel share + commission + VAT: %v", st)
	}

	// The CSV: right type, a header, and a guest name that would run as a formula in a spreadsheet is defused.
	req := hotel.do("GET", "/partner/statements?month="+month+"&property_id="+meskel+"&format=csv", nil)
	csv := string(req.raw)
	if req.status != 200 || !strings.Contains(csv, "BookDan statement") || !strings.Contains(csv, "Closing balance") {
		t.Fatalf("csv: %d %s", req.status, csv)
	}
	if strings.Contains(csv, ",=HYPERLINK") || strings.Contains(csv, "\"=HYPERLINK") {
		t.Errorf("a cell that starts like a formula is defused: %s", csv)
	}

	// Who may read it, and what is refused.
	if x := desk.do("GET", "/partner/statements?month="+month, nil); x.status != 403 {
		t.Errorf("a receptionist does not see money: %d", x.status)
	}
	for name, path := range map[string]string{"bad month": "?month=2026-13", "garbled": "?month=lastmonth", "future": "?month=2099-01", "unknown hotel": "?month=" + month + "&property_id=00000000-0000-0000-0000-000000000000"} {
		if x := hotel.do("GET", "/partner/statements"+path, nil); x.status != 400 && x.status != 404 {
			t.Errorf("%s: %d", name, x.status)
		}
	}
	if a := admin.do("GET", "/admin/hotels/"+meskel+"/statements?month="+month, nil); a.status != 200 || int64(a.body["statements"].([]any)[0].(map[string]any)["closing_santim"].(float64)) != closing {
		t.Errorf("the admin reads the same statement: %d", a.status)
	}
	if x := newClient(t, "partner").do("GET", "/partner/statements", nil); x.status != 401 {
		t.Errorf("signed out: %d", x.status)
	}
	last := time.Now().UTC().Add(3*time.Hour).AddDate(0, -1, 0).Format("2006-01")
	if d := hotel.do("GET", "/partner/statements?property_id="+meskel, nil); d.status != 200 || d.body["statements"].([]any)[0].(map[string]any)["month"] != last {
		t.Errorf("last month by default: %s", d.raw)
	}
}

func TestLastMonthsStatementIsEmailedToTheOwnersOnce(t *testing.T) {
	ctx := context.Background()
	n := time.Now().UnixNano()
	g, _ := supportGuest(t, "stme")
	hotel := hotelFor(t)
	admin := adminClient(t)
	if r := admin.do("PUT", "/admin/hotels/"+meskel+"/business", businessBody("")); r.status != 200 {
		t.Fatalf("business record: %d %s", r.status, r.raw)
	}
	const contact = "almaz@kazanchis-hotel.test"
	// A stay that ended last month.
	id, _, _, _ := confirmedBooking(t, g, hotel, 106, 108, fmt.Sprintf("stme-%d", n))
	pool.Exec(ctx, `UPDATE bookings SET check_in = date_trunc('month', `+addisToday+`)::date - 6, check_out = date_trunc('month', `+addisToday+`)::date - 4 WHERE id = $1`, id)
	app.RunMaintenance(ctx)
	pool.Exec(ctx, `DELETE FROM statement_emails WHERE property_id = $1`, meskel)

	before := len(mails.to(contact))
	app.RunMaintenance(ctx)
	month := time.Now().UTC().Add(3*time.Hour).AddDate(0, -1, 0).Format("2006-01")
	got := mails.to(contact)
	if len(got) != before+1 || !strings.Contains(got[len(got)-1].subject, month) || !strings.Contains(got[len(got)-1].subject, "Meskel") {
		t.Fatalf("the statement is emailed to the hotel's business contact: %d -> %d", before, len(got))
	}
	app.RunMaintenance(ctx)
	if len(mails.to(contact)) != before+1 {
		t.Errorf("never twice for the same month")
	}
	var recorded int
	pool.QueryRow(ctx, `SELECT count(*) FROM statement_emails WHERE property_id = $1 AND month = $2`, meskel, month).Scan(&recorded)
	if recorded != 1 {
		t.Errorf("it is recorded: %d", recorded)
	}
}
