package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// rawDo makes one request with plain net/http, so it can run in a goroutine.
func rawDo(app, token, deviceID, method, path string, body any) (int, map[string]any) {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, srv.URL+"/v1"+path, rd)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-App", app)
	req.Header.Set("X-Device-Id", deviceID)
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, nil
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return res.StatusCode, out
}

// realGuest signs in with a real (not demo) address, so the emails sent to the guest are recorded.
func realGuest(t *testing.T, prefix string) (*client, string) {
	t.Helper()
	email := realAddress(prefix)
	g := newClient(t, "guest")
	signed := signInWithEmailedCode(g, "/auth/otp/request", "/auth/otp/verify", email)
	if signed.status != 200 {
		t.Fatalf("sign in: %d %s", signed.status, signed.raw)
	}
	g.token = signed.str("token")
	withProfile(g)
	return g, email
}

func countSubject(email, contains string) int {
	n := 0
	for _, m := range mails.to(email) {
		if strings.Contains(m.subject, contains) {
			n++
		}
	}
	return n
}

func bookedNight(t *testing.T, room string, night int) int {
	t.Helper()
	var n int
	pool.QueryRow(context.Background(), `SELECT coalesce(sum(booked + held), 0)::int FROM inventory WHERE room_type_id = $1 AND night = $2::date`, room, day(night)).Scan(&n)
	return n
}

func confirmedRoom(t *testing.T, g, hotel *client, room string, in, out int, key string) string {
	t.Helper()
	id := hold(g, quote(g, meskel, room, in, out, 1, 1).str("id"), key).str("id")
	payOnline(t, g, id, "succeeded")
	if r := hotel.do("POST", "/partner/bookings/"+id+"/accept", nil); r.status != 200 {
		t.Fatalf("accept: %d %s", r.status, r.raw)
	}
	return id
}

func changeDates(g *client, id string, in, out int) resp {
	return g.do("POST", "/bookings/"+id+"/change-dates", map[string]string{"check_in": day(in), "check_out": day(out)})
}

func TestAGuestMovesAConfirmedBookingToOtherDatesAtTheSamePrice(t *testing.T) {
	n := time.Now().UnixNano()
	g, email := realGuest(t, "chg")
	hotel := hotelFor(t)
	id, _, _, _ := confirmedBooking(t, g, hotel, 150, 152, fmt.Sprintf("chg-1-%d", n))
	before := ledgerNetFor(t, id)
	room0, room1 := bookedNight(t, standard, 150), bookedNight(t, standard, 153)

	if b := g.do("GET", "/bookings/"+id, nil); b.body["can_change_dates"] != true {
		t.Fatalf("a refundable confirmed booking far from check-in can be moved: %s", b.raw)
	}
	mailsBefore := len(mails.to(email))
	r := changeDates(g, id, 153, 155)
	if r.status != 200 || r.str("check_in") != day(153) || r.str("check_out") != day(155) {
		t.Fatalf("move: %d %s", r.status, r.raw)
	}
	if r.str("original_check_in") != day(150) || r.str("original_check_out") != day(152) || r.body["can_change_dates"] != false {
		t.Errorf("the first dates are kept and it cannot be moved again: %s", r.raw)
	}
	if bookedNight(t, standard, 150) != room0-1 || bookedNight(t, standard, 153) != room1+1 {
		t.Errorf("the old nights are free and the new nights taken: old %d->%d new %d->%d", room0, bookedNight(t, standard, 150), room1, bookedNight(t, standard, 153))
	}
	after := ledgerNetFor(t, id)
	for k, v := range before {
		if after[k] != v {
			t.Errorf("the ledger is untouched: %s %d -> %d", k, v, after[k])
		}
	}
	if len(after) != len(before) {
		t.Errorf("no new ledger entries: %v -> %v", before, after)
	}
	if v := g.do("GET", "/bookings/"+id+"/voucher", nil); v.str("check_in") != day(153) {
		t.Errorf("the voucher shows the new dates: %s", v.raw)
	}
	if len(mails.to(email)) != mailsBefore+1 {
		t.Errorf("the guest is emailed once")
	}
	if again := changeDates(g, id, 156, 158); again.status != 409 || again.str("error", "code") != "CHANGE_LIMIT" {
		t.Errorf("only one move: %d %s", again.status, again.raw)
	}
	// The hotel sees the new dates.
	if list := hotel.do("GET", "/partner/bookings", nil); !strings.Contains(string(list.raw), day(153)) {
		t.Errorf("the hotel's list shows the new dates")
	}
}

func TestAMoveThatOverlapsTheOldNightsDoesNotCountTheGuestsOwnRoomTwice(t *testing.T) {
	n := time.Now().UnixNano()
	g, _ := supportGuest(t, "chgo")
	hotel := hotelFor(t)
	// The Family Suite is a single room: moving one night later overlaps the old stay.
	id := confirmedRoom(t, g, hotel, family, 160, 162, fmt.Sprintf("chg-o-%d", n))
	if r := changeDates(g, id, 161, 163); r.status != 200 {
		t.Fatalf("overlapping move: %d %s", r.status, r.raw)
	}
	if bookedNight(t, family, 160) != 0 || bookedNight(t, family, 161) != 1 || bookedNight(t, family, 162) != 1 {
		t.Errorf("nights 160/161/162 hold 0/1/1 rooms: %d/%d/%d", bookedNight(t, family, 160), bookedNight(t, family, 161), bookedNight(t, family, 162))
	}
}

func TestAMoveIntoNightsThatAreFullChangesNothing(t *testing.T) {
	n := time.Now().UnixNano()
	g, _ := supportGuest(t, "chgf")
	other, _ := supportGuest(t, "chgf2")
	hotel := hotelFor(t)
	id := confirmedRoom(t, g, hotel, family, 166, 168, fmt.Sprintf("chg-f-%d", n))
	confirmedRoom(t, other, hotel, family, 169, 171, fmt.Sprintf("chg-f2-%d", n))
	r := changeDates(g, id, 169, 171)
	if r.status != 409 || r.str("error", "code") != "SOLD_OUT" {
		t.Fatalf("those nights are taken: %d %s", r.status, r.raw)
	}
	b := g.do("GET", "/bookings/"+id, nil)
	if b.str("check_in") != day(166) || b.body["can_change_dates"] != true {
		t.Errorf("a refused move leaves the booking as it was: %s", b.raw)
	}
	if bookedNight(t, family, 166) != 1 || bookedNight(t, family, 167) != 1 {
		t.Errorf("the old nights are still held")
	}
}

func TestADifferentPriceIsRefusedWithTheNewPrice(t *testing.T) {
	n := time.Now().UnixNano()
	g, _ := supportGuest(t, "chgp")
	hotel := hotelFor(t)
	id, _, _, _ := confirmedBooking(t, g, hotel, 172, 174, fmt.Sprintf("chg-p-%d", n))
	r := changeDates(g, id, 175, 178) // three nights cost more than two
	if r.status != 409 || r.str("error", "code") != "PRICE_DIFFERS" || !strings.Contains(string(r.raw), "instead of") {
		t.Fatalf("a different price: %d %s", r.status, r.raw)
	}
	if b := g.do("GET", "/bookings/"+id, nil); b.str("check_in") != day(172) {
		t.Errorf("nothing changed: %s", b.raw)
	}
}

func TestOnlyABookingThatCanStillBeCancelledForFreeCanBeMoved(t *testing.T) {
	n := time.Now().UnixNano()
	g, _ := supportGuest(t, "chgr")
	other, _ := supportGuest(t, "chgr2")
	hotel := hotelFor(t)
	ctx := context.Background()

	// Not confirmed yet: waiting for the hotel.
	waiting := hold(g, quote(g, meskel, standard, 180, 182, 1, 1).str("id"), fmt.Sprintf("chg-w-%d", n)).str("id")
	payOnline(t, g, waiting, "succeeded")
	if r := changeDates(g, waiting, 183, 185); r.status != 409 {
		t.Errorf("a booking the hotel has not confirmed: %d", r.status)
	}

	id, _, _, _ := confirmedBooking(t, g, hotel, 186, 188, fmt.Sprintf("chg-r-%d", n))
	for name, tc := range map[string]struct {
		in, out int
		code    string
		status  int
	}{
		"same dates":      {186, 188, "", 400},
		"backwards":       {190, 189, "", 400},
		"in the past":     {-3, -1, "", 400},
		"too long":        {190, 225, "", 400},
		"too far ahead":   {500, 502, "", 400},
		"starts tomorrow": {1, 3, "NEW_DATES_TOO_SOON", 409},
	} {
		r := changeDates(g, id, tc.in, tc.out)
		if r.status != tc.status || (tc.code != "" && r.str("error", "code") != tc.code) {
			t.Errorf("%s: %d %s", name, r.status, r.raw)
		}
	}
	if r := changeDates(other, id, 190, 192); r.status != 404 {
		t.Errorf("another guest's booking: %d", r.status)
	}
	if r := g.do("POST", "/bookings/"+id+"/change-dates", map[string]string{"check_in": "tomorrow"}); r.status != 400 {
		t.Errorf("garbled dates: %d", r.status)
	}

	// Non-refundable: no.
	pool.Exec(ctx, `UPDATE bookings SET refundable = false WHERE id = $1`, id)
	if r := changeDates(g, id, 190, 192); r.status != 409 || r.str("error", "code") != "NON_REFUNDABLE" {
		t.Errorf("non-refundable: %d %s", r.status, r.raw)
	}
	pool.Exec(ctx, `UPDATE bookings SET refundable = true WHERE id = $1`, id)

	// Too close to check-in: less than 48 hours away.
	pool.Exec(ctx, `UPDATE bookings SET check_in = `+addisToday+` + 1, check_out = `+addisToday+` + 3 WHERE id = $1`, id)
	if r := changeDates(g, id, 190, 192); r.status != 409 || r.str("error", "code") != "CHANGE_TOO_LATE" {
		t.Errorf("inside 48 hours: %d %s", r.status, r.raw)
	}
	if b := g.do("GET", "/bookings/"+id, nil); b.body["can_change_dates"] != false {
		t.Errorf("the app is told it cannot be moved: %s", b.raw)
	}
}

func TestABookingWithACouponCannotBeMoved(t *testing.T) {
	n := time.Now().UnixNano()
	g, _ := supportGuest(t, "chgc")
	hotel := hotelFor(t)
	admin := adminClient(t)
	code := fmt.Sprintf("MOVE%d", n%1000000)
	if r := admin.do("POST", "/admin/coupons", map[string]any{"code": code, "kind": "percent", "value": 1000, "per_user_limit": 5, "active": true}); r.status != 200 {
		t.Fatalf("setup coupon: %d %s", r.status, r.raw)
	}
	q := g.do("POST", "/quotes", map[string]any{"property_id": meskel, "room_type_id": standard, "check_in": day(192), "check_out": day(194), "guests": 1, "rooms": 1, "coupon_code": code})
	if q.status != 200 {
		t.Fatalf("setup quote: %d %s", q.status, q.raw)
	}
	id := hold(g, q.str("id"), fmt.Sprintf("chg-c-%d", n)).str("id")
	payOnline(t, g, id, "succeeded")
	if r := hotel.do("POST", "/partner/bookings/"+id+"/accept", nil); r.status != 200 {
		t.Fatalf("accept: %d %s", r.status, r.raw)
	}
	if r := changeDates(g, id, 195, 197); r.status != 409 || r.str("error", "code") != "COUPON_NO_CHANGE" {
		t.Errorf("a coupon booking: %d %s", r.status, r.raw)
	}
}

func TestTwoGuestsMovingIntoTheLastRoomAtOnceOnlyOneGetsIt(t *testing.T) {
	n := time.Now().UnixNano()
	hotel := hotelFor(t)
	a, _ := supportGuest(t, "chgra")
	b, _ := supportGuest(t, "chgrb")
	ida := confirmedRoom(t, a, hotel, family, 130, 132, fmt.Sprintf("chg-ra-%d", n))
	idb := confirmedRoom(t, b, hotel, family, 133, 135, fmt.Sprintf("chg-rb-%d", n))
	statuses := make(chan int, 2)
	start := make(chan struct{})
	for _, c := range []struct {
		g  *client
		id string
	}{{a, ida}, {b, idb}} {
		go func() {
			<-start
			s, _ := rawDo("guest", c.g.token, c.g.deviceID, "POST", "/bookings/"+c.id+"/change-dates", map[string]string{"check_in": day(137), "check_out": day(139)})
			statuses <- s
		}()
	}
	close(start)
	ok, full := 0, 0
	for i := 0; i < 2; i++ {
		switch <-statuses {
		case 200:
			ok++
		case 409:
			full++
		}
	}
	if ok != 1 || full != 1 {
		t.Fatalf("exactly one move succeeds and the other finds the room taken: ok %d, refused %d", ok, full)
	}
	for _, night := range []int{137, 138} {
		if got := bookedNight(t, family, night); got != 1 {
			t.Errorf("night %d holds exactly the one room: %d", night, got)
		}
	}
}
