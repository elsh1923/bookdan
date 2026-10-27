package api_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func syncCheckins(c *client, items ...map[string]string) resp {
	list := make([]any, len(items))
	for i, it := range items {
		list[i] = it
	}
	return c.do("POST", "/partner/checkin/sync", map[string]any{"items": list})
}

func resultsOf(r resp) []string {
	var out []string
	for _, x := range r.body["results"].([]any) {
		out = append(out, x.(map[string]any)["result"].(string))
	}
	return out
}

func TestCheckInsMadeOfflineAreAppliedWhenTheHotelIsBackOnline(t *testing.T) {
	ctx := context.Background()
	n := time.Now().UnixNano()
	g, _ := supportGuest(t, "off")
	hotel := hotelFor(t)
	admin := adminClient(t)
	desk := receptionist(t, admin, "offdesk")

	// A stay that is under way: the guest's voucher was scanned an hour ago while the hotel had no internet.
	id, ref, token, _ := confirmedBooking(t, g, hotel, 0, 2, fmt.Sprintf("off-1-%d", n))
	at := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	r := syncCheckins(desk, map[string]string{"token": token, "at": at})
	if r.status != 200 || strings.Join(resultsOf(r), ",") != "checked_in" {
		t.Fatalf("sync: %d %s", r.status, r.raw)
	}
	if got := r.body["results"].([]any)[0].(map[string]any)["reference"]; got != ref {
		t.Errorf("the answer names the booking: %v vs %s", got, ref)
	}
	b := g.do("GET", "/bookings/"+id, nil)
	var checkedAt time.Time
	pool.QueryRow(ctx, `SELECT checked_in_at FROM bookings WHERE id = $1`, id).Scan(&checkedAt)
	if b.str("status") != "CHECKED_IN" || time.Since(checkedAt) < 50*time.Minute || time.Since(checkedAt) > 70*time.Minute {
		t.Errorf("the booking is checked in, at the time the hotel did it: %s at %v", b.str("status"), checkedAt)
	}
	var audits int
	pool.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE action = 'guest_checked_in_offline' AND details->>'booking_id' = $1`, id).Scan(&audits)
	if audits != 1 {
		t.Errorf("it is in the audit log: %d", audits)
	}
	// Sending the same thing again is harmless.
	if again := syncCheckins(desk, map[string]string{"token": token, "at": at}); strings.Join(resultsOf(again), ",") != "already" {
		t.Errorf("a repeat: %s", again.raw)
	}
	if online := hotel.do("POST", "/partner/bookings/"+id+"/check-in", nil); online.status != 200 {
		t.Errorf("checking in online afterwards is also harmless: %d", online.status)
	}
}

func TestOfflineCheckInsThatCannotBeAppliedAreReportedOneByOne(t *testing.T) {
	ctx := context.Background()
	n := time.Now().UnixNano()
	g, _ := supportGuest(t, "off2")
	other, _ := supportGuest(t, "off3")
	hotel := hotelFor(t)
	admin := adminClient(t)
	desk := receptionist(t, admin, "off2desk") // staff of Meskel only
	now := time.Now().UTC().Format(time.RFC3339)

	_, _, good, _ := confirmedBooking(t, g, hotel, 0, 2, fmt.Sprintf("off2-1-%d", n))
	cid, _, cancelled, _ := confirmedBooking(t, g, hotel, 40, 42, fmt.Sprintf("off2-2-%d", n))
	if c := g.do("POST", "/bookings/"+cid+"/cancel", nil); c.status != 200 {
		t.Fatalf("cancel: %d", c.status)
	}
	_, _, future, _ := confirmedBooking(t, g, hotel, 44, 46, fmt.Sprintf("off2-3-%d", n))
	eid, _, over, _ := confirmedBooking(t, g, hotel, 48, 50, fmt.Sprintf("off2-4-%d", n))
	pool.Exec(ctx, fmt.Sprintf(`UPDATE bookings SET check_in = %s - 5, check_out = %s - 3 WHERE id = $1`, addisToday, addisToday), eid)

	// A voucher of another hotel (Bole takes pay at hotel; its owner is the demo owner).
	bid := hold(other, quote(other, bole, boleSingle, 52, 53, 1, 1).str("id"), fmt.Sprintf("off2-5-%d", n)).str("id")
	other.do("POST", "/bookings/"+bid+"/pay", map[string]string{"method": "pay_at_hotel"})
	hotel.do("POST", "/partner/bookings/"+bid+"/accept", nil)
	foreign := other.do("GET", "/bookings/"+bid+"/voucher", nil).str("token")

	r := syncCheckins(desk,
		map[string]string{"token": good, "at": now},
		map[string]string{"token": cancelled, "at": now},
		map[string]string{"token": future, "at": now},
		map[string]string{"token": over, "at": now},
		map[string]string{"token": foreign, "at": now},
		map[string]string{"token": good[:len(good)-4] + "AAAA", "at": now},
		map[string]string{"token": "garbage", "at": now},
		map[string]string{"token": good, "at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339)},
		map[string]string{"token": good, "at": time.Now().Add(-40 * 24 * time.Hour).UTC().Format(time.RFC3339)},
		map[string]string{"token": good, "at": "yesterday"},
	)
	want := "checked_in,cancelled,too_early,ended,wrong_hotel,invalid,invalid,bad_time,bad_time,bad_time"
	if got := strings.Join(resultsOf(r), ","); r.status != 200 || got != want {
		t.Errorf("each item has its own answer:\n got  %s\n want %s", got, want)
	}
	// Nothing was changed for the ones that could not be applied.
	if s := g.do("GET", "/bookings/"+cid, nil).str("status"); s != "CANCELLED_REFUNDED" {
		t.Errorf("a cancelled booking stays cancelled: %s", s)
	}
	if s := g.do("GET", "/bookings/"+bid, nil).str("status"); s == "CHECKED_IN" {
		t.Errorf("another hotel's guest was not checked in")
	}

	for name, body := range map[string]any{"no items": map[string]any{"items": []any{}}, "no body": map[string]any{}} {
		if x := desk.do("POST", "/partner/checkin/sync", body); x.status != 400 {
			t.Errorf("%s: %d", name, x.status)
		}
	}
	many := make([]any, 101)
	for i := range many {
		many[i] = map[string]string{"token": good, "at": now}
	}
	if x := desk.do("POST", "/partner/checkin/sync", map[string]any{"items": many}); x.status != 400 {
		t.Errorf("101 items: %d", x.status)
	}
	if x := g.do("POST", "/partner/checkin/sync", map[string]any{"items": []any{map[string]string{"token": good, "at": now}}}); x.status != 401 {
		t.Errorf("a guest token does not work: %d", x.status)
	}
	if x := newClient(t, "partner").do("POST", "/partner/checkin/sync", map[string]any{"items": []any{}}); x.status != 401 {
		t.Errorf("signed out: %d", x.status)
	}
}
