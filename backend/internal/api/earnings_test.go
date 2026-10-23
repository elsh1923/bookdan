package api_test

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// hotelMoney reads one hotel's position from the admin payouts list: ready, upcoming, paid, to_pay (all santim).
func hotelMoney(t *testing.T, admin *client, id string) (ready, upcoming, paid, toPay int64) {
	t.Helper()
	r := admin.do("GET", "/admin/payouts", nil)
	if r.status != 200 {
		t.Fatalf("payouts: %d %s", r.status, r.raw)
	}
	for _, h := range r.body["hotels"].([]any) {
		if m := h.(map[string]any); m["id"] == id {
			return int64(m["ready_santim"].(float64)), int64(m["upcoming_santim"].(float64)), int64(m["paid_santim"].(float64)), int64(m["to_pay_santim"].(float64))
		}
	}
	return 0, 0, 0, 0
}

func TestEarningsFollowTheStayAndPayoutsSpendOnlyWhatIsReady(t *testing.T) {
	ctx := context.Background()
	n := time.Now().UnixNano()
	admin := adminClient(t)
	guest := newClient(t, "guest")
	guest.signIn("email", uniqueEmail("earn"))
	withProfile(guest)
	hotel := hotelFor(t)
	// The hotel must have its business record before it can be paid.
	if r := admin.do("PUT", "/admin/hotels/"+meskel+"/business", businessBody("")); r.status != 200 {
		t.Fatalf("business record: %d %s", r.status, r.raw)
	}

	// The commission is 15% (migration 00015); set it explicitly so the split below does not depend on the seed.
	pool.Exec(ctx, `UPDATE pricing_config SET commission_bps = 1500`)
	defer pool.Exec(ctx, `UPDATE pricing_config SET commission_bps = 1500`)

	r0, u0, _, _ := hotelMoney(t, admin, meskel)

	// 1. A confirmed booking is "upcoming": the hotel's share, our commission and VAT add up to what the guest paid.
	id, ref, _, _ := confirmedBooking(t, guest, hotel, 290, 291, fmt.Sprintf("earn-1-%d", n))
	r1, u1, _, to1 := hotelMoney(t, admin, meskel)
	net := u1 - u0
	if net <= 0 || r1 != r0 {
		t.Fatalf("a future stay is upcoming, not ready: upcoming +%d, ready %d->%d", net, r0, r1)
	}
	e := hotel.do("GET", "/partner/earnings", nil)
	if e.status != 200 {
		t.Fatalf("earnings: %d %s", e.status, e.raw)
	}
	var line map[string]any
	for _, h := range e.body["hotels"].([]any) {
		hm := h.(map[string]any)
		if hm["id"] != meskel {
			continue
		}
		for _, it := range hm["items"].([]any) {
			if m := it.(map[string]any); m["booking_id"] == id {
				line = m
			}
		}
	}
	if line == nil || line["reference"] != ref || line["ready"] != false {
		t.Fatalf("the booking should be listed as not ready: %v", line)
	}
	gross, comm, vat, lnet := line["gross_santim"].(float64), line["commission_santim"].(float64), line["vat_santim"].(float64), line["net_santim"].(float64)
	if int64(lnet) != net || gross != lnet+comm+vat || comm <= 0 || vat <= 0 {
		t.Fatalf("gross must equal share + commission + VAT: %v", line)
	}

	// 2. A receptionist sees no money; a platform admin is not an owner either.
	desk := newClient(t, "partner")
	deskMail := realAddress("deskearn")
	if r := admin.do("POST", "/admin/hotels/"+meskel+"/staff", map[string]string{"contact": deskMail, "role": "receptionist"}); r.status != 200 {
		t.Fatalf("add receptionist: %d %s", r.status, r.raw)
	}
	defer pool.Exec(ctx, `DELETE FROM property_staff WHERE user_id IN (SELECT id FROM users WHERE email = $1)`, deskMail)
	dr := signInWithEmailedCode(desk, "/auth/otp/request", "/auth/otp/verify", deskMail)
	desk.token = dr.str("token")
	if r := desk.do("GET", "/partner/earnings", nil); r.status != 403 {
		t.Fatalf("a receptionist must not see earnings: %d", r.status)
	}

	// 3. Nothing is payable yet: paying out is refused, and only platform admins can pay.
	if r := admin.do("POST", "/admin/hotels/"+meskel+"/payouts", map[string]any{"amount_santim": to1 + 1, "reference": "TRX-TOO-EARLY"}); r.status != 409 {
		t.Fatalf("upcoming money must not be payable: %d", r.status)
	}
	if r := hotel.do("POST", "/admin/hotels/"+meskel+"/payouts", map[string]any{"amount_santim": 100, "reference": "TRX-1"}); r.status != 401 && r.status != 403 {
		t.Fatalf("a hotel must not record payouts: %d", r.status)
	}

	// 4. The guest arrives and is checked in: the money becomes ready.
	pool.Exec(ctx, `UPDATE bookings SET check_in = `+addisToday+`, check_out = `+addisToday+` + 2 WHERE id = $1`, id)
	if c := hotel.do("POST", "/partner/bookings/"+id+"/check-in", nil); c.status != 200 {
		t.Fatalf("check in: %d %s", c.status, c.raw)
	}
	r2, u2, p2, to2 := hotelMoney(t, admin, meskel)
	if r2-r0 != net || u2 != u0 || to2 < net {
		t.Fatalf("after check-in the share is ready: ready %d->%d, upcoming %d->%d", r0, r2, u0, u2)
	}

	// 5. Recording a payout: validated, bounded by what is ready, logged, emailed, and shown to the hotel.
	pay := func(body map[string]any) resp { return admin.do("POST", "/admin/hotels/"+meskel+"/payouts", body) }
	for name, body := range map[string]map[string]any{
		"zero":         {"amount_santim": 0, "reference": "TRX-ZERO"},
		"no reference": {"amount_santim": 100, "reference": ""},
		"too much":     {"amount_santim": to2 + 1, "reference": "TRX-TOO-MUCH"},
	} {
		if r := pay(body); r.status != 400 && r.status != 409 {
			t.Errorf("%s must be refused, got %d", name, r.status)
		}
	}
	ref1 := fmt.Sprintf("TRX-%d", n)
	if r := pay(map[string]any{"amount_santim": net, "reference": ref1, "note": "March payout"}); r.status != 200 {
		t.Fatalf("record payout: %d %s", r.status, r.raw)
	}
	_, uB, p3, to3 := hotelMoney(t, admin, meskel)
	if p3-p2 != net || to2-to3 != net {
		t.Fatalf("paying %d must move it from to-pay to paid: paid %d->%d, to-pay %d->%d", net, p2, p3, to2, to3)
	}
	// Money cannot be spent twice: what remains is the most that can be paid.
	if r := pay(map[string]any{"amount_santim": to3 + 1, "reference": "TRX-AGAIN"}); r.status != 409 {
		t.Fatalf("paying more than remains: %d", r.status)
	}
	var entries int
	pool.QueryRow(ctx, `SELECT count(*) FROM ledger_transactions t JOIN ledger_entries e ON e.transaction_id = t.id WHERE t.kind = 'payout' AND t.property_id = $1 AND t.memo LIKE '%' || $2`, meskel, ref1).Scan(&entries)
	var audited int
	pool.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE action = 'admin_payout_recorded' AND details->>'reference' = $1`, ref1).Scan(&audited)
	if entries != 2 || audited != 1 {
		t.Fatalf("a payout is a balanced two-entry ledger transaction and is audited (entries=%d, audit=%d)", entries, audited)
	}
	if got := mails.to("almaz@kazanchis-hotel.test"); len(got) == 0 {
		t.Fatalf("the business contact should be told about the payout")
	}
	hist := admin.do("GET", "/admin/hotels/"+meskel+"/payouts", nil)
	if list, _ := hist.body["payouts"].([]any); len(list) == 0 || list[0].(map[string]any)["reference"] != ref1 {
		t.Fatalf("the payout history should list it first: %s", hist.raw)
	}
	e2 := hotel.do("GET", "/partner/earnings", nil)
	seen := false
	for _, h := range e2.body["hotels"].([]any) {
		hm := h.(map[string]any)
		if hm["id"] != meskel {
			continue
		}
		for _, p := range hm["payouts"].([]any) {
			seen = seen || p.(map[string]any)["reference"] == ref1
		}
	}
	if !seen {
		t.Fatalf("the hotel should see the payout in its earnings")
	}

	// 6. A cancelled booking owes the hotel nothing.
	id2, _, _, _ := confirmedBooking(t, guest, hotel, 292, 293, fmt.Sprintf("earn-2-%d", n))
	_, u4, _, _ := hotelMoney(t, admin, meskel)
	if u4 <= uB {
		t.Fatalf("a new confirmed booking adds to upcoming")
	}
	if c := guest.do("POST", "/bookings/"+id2+"/cancel", nil); c.status != 200 {
		t.Fatalf("cancel: %d %s", c.status, c.raw)
	}
	_, u5, _, _ := hotelMoney(t, admin, meskel)
	if u5 != uB {
		t.Fatalf("a full-refund cancellation must remove the hotel's share again: upcoming %d -> %d (before the booking: %d)", u4, u5, uB)
	}
}
