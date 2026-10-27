package api_test

import (
	"fmt"
	"testing"
	"time"
)

func analyticsOf(t *testing.T, admin *client, days int) map[string]any {
	t.Helper()
	r := admin.do("GET", fmt.Sprintf("/admin/analytics?days=%d", days), nil)
	if r.status != 200 {
		t.Fatalf("analytics: %d %s", r.status, r.raw)
	}
	return r.body
}

func num(m map[string]any, k string) int64 { return int64(m[k].(float64)) }

func hotelRow(a map[string]any, id string) map[string]any {
	for _, h := range a["hotels"].([]any) {
		if m := h.(map[string]any); m["id"] == id {
			return m
		}
	}
	return nil
}

func TestTheAdminDashboardCountsTheFunnelTheMoneyAndHowHotelsAnswer(t *testing.T) {
	n := time.Now().UnixNano()
	admin := adminClient(t)
	hotel := hotelFor(t)
	g, _ := supportGuest(t, "ana")
	before := analyticsOf(t, admin, 1)
	hb := hotelRow(before, meskel)

	// Three paid requests: one accepted and stays, one accepted then cancelled by the hotel, one declined.
	a := hold(g, quote(g, meskel, standard, 118, 120, 1, 1).str("id"), fmt.Sprintf("ana-1-%d", n)).str("id")
	b := hold(g, quote(g, meskel, standard, 121, 123, 1, 1).str("id"), fmt.Sprintf("ana-2-%d", n)).str("id")
	c := hold(g, quote(g, meskel, standard, 124, 126, 1, 1).str("id"), fmt.Sprintf("ana-3-%d", n)).str("id")
	for _, id := range []string{a, b, c} {
		payOnline(t, g, id, "succeeded")
	}
	hotel.do("POST", "/partner/bookings/"+a+"/accept", nil)
	hotel.do("POST", "/partner/bookings/"+b+"/accept", nil)
	hotel.do("POST", "/partner/bookings/"+c+"/decline", map[string]string{"reason": "fully_booked"})
	hotel.do("POST", "/partner/bookings/"+b+"/cancel", map[string]string{"reason": "maintenance"})
	totalA, totalB := paidOf(a), paidOf(b)

	after := analyticsOf(t, admin, 1)
	if d := num(after, "holds") - num(before, "holds"); d != 3 {
		t.Errorf("three bookings were started: %d", d)
	}
	if d := num(after, "paid") - num(before, "paid"); d != 3 {
		t.Errorf("three were paid: %d", d)
	}
	if d := num(after, "confirmed") - num(before, "confirmed"); d != 2 {
		t.Errorf("two were confirmed: %d", d)
	}
	if d := num(after, "declined") - num(before, "declined"); d != 1 {
		t.Errorf("one was declined: %d", d)
	}
	if d := num(after, "cancelled_by_hotel") - num(before, "cancelled_by_hotel"); d != 1 {
		t.Errorf("one was cancelled by the hotel: %d", d)
	}
	if d := num(after, "quotes") - num(before, "quotes"); d < 3 {
		t.Errorf("three prices were asked for: %d", d)
	}
	if d := num(after, "gmv_santim") - num(before, "gmv_santim"); d != totalA+totalB {
		t.Errorf("GMV is the price of what the hotels confirmed: %d vs %d", d, totalA+totalB)
	}
	if d := num(after, "confirmed_paid_online") - num(before, "confirmed_paid_online"); d != 2 {
		t.Errorf("both were paid online: %d", d)
	}
	if num(after, "commission_santim") <= num(before, "commission_santim") {
		t.Errorf("BookDan earned commission: %d -> %d", num(before, "commission_santim"), num(after, "commission_santim"))
	}
	if num(after, "refunded_santim") <= num(before, "refunded_santim") {
		t.Errorf("money went back to guests: %d -> %d", num(before, "refunded_santim"), num(after, "refunded_santim"))
	}

	ha := hotelRow(after, meskel)
	if ha == nil {
		t.Fatalf("the hotel is listed: %v", after["hotels"])
	}
	req0, acc0, dec0, canc0 := int64(0), int64(0), int64(0), int64(0)
	if hb != nil {
		req0, acc0, dec0, canc0 = num(hb, "requests"), num(hb, "accepted"), num(hb, "declined"), num(hb, "hotel_cancellations")
	}
	if num(ha, "requests")-req0 != 3 || num(ha, "accepted")-acc0 != 2 || num(ha, "declined")-dec0 != 1 || num(ha, "hotel_cancellations")-canc0 != 1 {
		t.Errorf("how the hotel answered: requests %d accepted %d declined %d cancelled %d (before %d %d %d %d)", num(ha, "requests"), num(ha, "accepted"), num(ha, "declined"), num(ha, "hotel_cancellations"), req0, acc0, dec0, canc0)
	}
	if ha["avg_reply_minutes"] == nil || ha["avg_reply_minutes"].(float64) < 0 {
		t.Errorf("the average reply time is known: %v", ha["avg_reply_minutes"])
	}

	daily := after["daily"].([]any)
	if len(daily) != 1 || daily[0].(map[string]any)["confirmed"].(float64) < 2 {
		t.Errorf("a point for the day: %v", daily)
	}
	if got := analyticsOf(t, admin, 30)["daily"].([]any); len(got) != 30 {
		t.Errorf("a point for each of the 30 days: %d", len(got))
	}
	if got := analyticsOf(t, admin, 9999)["days"].(float64); got != 30 {
		t.Errorf("an absurd period falls back to 30 days: %v", got)
	}
	for _, k := range []string{"open_support_requests", "open_disputes", "open_relocations", "open_payment_issues", "awaiting_hotel_now"} {
		if _, ok := after[k]; !ok {
			t.Errorf("%s is reported", k)
		}
	}
	if r := g.do("GET", "/admin/analytics", nil); r.status != 401 && r.status != 403 {
		t.Errorf("a guest cannot read it: %d", r.status)
	}
	if r := hotel.do("GET", "/admin/analytics", nil); r.status != 401 && r.status != 403 {
		t.Errorf("a hotel cannot read it: %d", r.status)
	}
}
