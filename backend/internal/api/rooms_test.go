package api_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func chartRoom(t *testing.T, c *client, from string, days int, roomID string) []map[string]any {
	t.Helper()
	r := c.do("GET", fmt.Sprintf("/partner/rooms?property_id=%s&from=%s&days=%d", meskel, from, days), nil)
	if r.status != 200 {
		t.Fatalf("chart: %d %s", r.status, r.raw)
	}
	for _, room := range r.body["rooms"].([]any) {
		if m := room.(map[string]any); m["id"] == roomID {
			var out []map[string]any
			for _, n := range m["nights"].([]any) {
				out = append(out, n.(map[string]any))
			}
			return out
		}
	}
	t.Fatalf("room %s is not on the chart", roomID)
	return nil
}

func TestWalkInsAndBlocksTakeRoomsOutOfSaleAndComeBack(t *testing.T) {
	hotel := hotelFor(t)
	guest := newClient(t, "guest")
	guest.signIn("email", uniqueEmail("chart"))
	withProfile(guest)
	from := day(220)

	// The chart: one row per room type, one cell per night, from the same inventory the app sells from.
	nights := chartRoom(t, hotel, from, 5, family)
	if len(nights) != 5 || nights[0]["date"] != from || nights[0]["capacity"].(float64) != 1 || nights[0]["free"].(float64) != 1 {
		t.Fatalf("an untouched Family Suite has its one room free every night: %v", nights)
	}

	// Blocking the only Family Suite for two nights: the chart shows it, and guests can no longer book it.
	block := func(body map[string]any) resp {
		body["property_id"] = meskel
		return hotel.do("POST", "/partner/rooms/blocks", body)
	}
	b := block(map[string]any{"kind": "block", "room_type_id": family, "check_in": day(220), "check_out": day(222), "rooms": 1, "note": "Repairs"})
	if b.status != 200 {
		t.Fatalf("block: %d %s", b.status, b.raw)
	}
	nights = chartRoom(t, hotel, from, 5, family)
	if nights[0]["blocked"].(float64) != 1 || nights[0]["free"].(float64) != 0 || nights[1]["free"].(float64) != 0 || nights[2]["free"].(float64) != 1 {
		t.Fatalf("two nights blocked, then free again: %v", nights)
	}
	if r := quoteWith(guest, meskel, family, 220, 221, 1, 1, nil); r.status != 409 || r.str("error", "code") != "SOLD_OUT" {
		t.Fatalf("a guest cannot book a blocked room: %d %s", r.status, r.raw)
	}
	// Taking rooms that are not free fails as a whole: nothing is taken on the nights that were free.
	overlap := block(map[string]any{"kind": "walk_in", "guest_name": "Abel Tadesse", "room_type_id": family, "check_in": day(221), "check_out": day(223), "rooms": 1})
	if overlap.status != 409 || overlap.str("error", "code") != "SOLD_OUT" {
		t.Fatalf("an overlapping walk-in: %d %s", overlap.status, overlap.raw)
	}
	if n := chartRoom(t, hotel, from, 5, family); n[3]["blocked"].(float64) != 0 || n[3]["free"].(float64) != 1 {
		t.Fatalf("a refused request takes nothing: %v", n[3])
	}

	// A walk-in guest in two Standard Doubles for a night.
	std0 := chartRoom(t, hotel, from, 2, standard)[0]
	w := block(map[string]any{"kind": "walk_in", "guest_name": "Abel Tadesse", "guest_phone": "+251 911 000 222", "room_type_id": standard, "check_in": day(220), "check_out": day(221), "rooms": 2})
	if w.status != 200 {
		t.Fatalf("walk-in: %d %s", w.status, w.raw)
	}
	after := chartRoom(t, hotel, from, 2, standard)[0]
	if after["blocked"].(float64) != std0["blocked"].(float64)+2 || after["free"].(float64) != std0["free"].(float64)-2 {
		t.Fatalf("two rooms taken by the walk-in: before %v, after %v", std0, after)
	}
	if after["booked"] != std0["booked"] {
		t.Fatalf("the walk-in is not counted as a guest booking")
	}
	// More rooms than there are free.
	if r := block(map[string]any{"kind": "block", "room_type_id": standard, "check_in": day(220), "check_out": day(221), "rooms": 50}); r.status != 409 {
		t.Fatalf("too many rooms: %d", r.status)
	}

	// Checks on what is asked.
	for name, body := range map[string]map[string]any{
		"a walk-in needs a name":  {"kind": "walk_in", "room_type_id": standard, "check_in": day(220), "check_out": day(221), "rooms": 1},
		"a night in the past":     {"kind": "block", "room_type_id": standard, "check_in": day(-2), "check_out": day(-1), "rooms": 1},
		"no rooms":                {"kind": "block", "room_type_id": standard, "check_in": day(220), "check_out": day(221), "rooms": 0},
		"an unknown kind":         {"kind": "party", "room_type_id": standard, "check_in": day(220), "check_out": day(221), "rooms": 1},
		"last night before first": {"kind": "block", "room_type_id": standard, "check_in": day(221), "check_out": day(220), "rooms": 1},
		"a bad phone":             {"kind": "walk_in", "guest_name": "Abel Tadesse", "guest_phone": "call me", "room_type_id": standard, "check_in": day(220), "check_out": day(221), "rooms": 1},
	} {
		if r := block(body); r.status != 400 {
			t.Errorf("%s must be refused, got %d", name, r.status)
		}
	}
	if r := block(map[string]any{"kind": "block", "room_type_id": boleSingle, "check_in": day(220), "check_out": day(221), "rooms": 1}); r.status != 404 {
		t.Fatalf("a room of another hotel: %d", r.status)
	}
	if r := guest.do("GET", "/partner/rooms", nil); r.status != 401 {
		t.Fatalf("a guest cannot see the rooms chart: %d", r.status)
	}

	// The list of what is on.
	list := hotel.do("GET", "/partner/rooms/blocks?property_id="+meskel, nil)
	var blockID, walkID string
	for _, it := range list.body["blocks"].([]any) {
		m := it.(map[string]any)
		switch {
		case m["note"] == "Repairs":
			blockID = m["id"].(string)
		case m["guest_name"] == "Abel Tadesse" && m["room_type_id"] == standard:
			walkID = m["id"].(string)
			if m["guest_phone"] != "+251 911 000 222" || m["kind"] != "walk_in" {
				t.Fatalf("the walk-in keeps the phone number: %v", m)
			}
		}
	}
	if blockID == "" || walkID == "" {
		t.Fatalf("both should be listed: %s", list.raw)
	}

	// Giving the rooms back: the chart and the guest app see them free again; doing it twice is harmless.
	for _, id := range []string{blockID, blockID} {
		if r := hotel.do("DELETE", "/partner/rooms/blocks/"+id, nil); r.status != 200 {
			t.Fatalf("release: %d %s", r.status, r.raw)
		}
	}
	if n := chartRoom(t, hotel, from, 5, family); n[0]["free"].(float64) != 1 || n[0]["blocked"].(float64) != 0 {
		t.Fatalf("the Family Suite is free again: %v", n[0])
	}
	if r := quoteWith(guest, meskel, family, 220, 221, 1, 1, nil); r.status != 200 {
		t.Fatalf("a guest can book the released room: %d %s", r.status, r.raw)
	}
	hotel.do("DELETE", "/partner/rooms/blocks/"+walkID, nil)
	if n := chartRoom(t, hotel, from, 2, standard)[0]; n["blocked"] != std0["blocked"] {
		t.Fatalf("the walk-in's rooms are back: %v", n)
	}
	for _, it := range hotel.do("GET", "/partner/rooms/blocks?property_id="+meskel, nil).body["blocks"].([]any) {
		if m := it.(map[string]any); m["id"] == blockID || m["id"] == walkID {
			t.Fatalf("a released block is not listed any more")
		}
	}
	if r := hotel.do("DELETE", "/partner/rooms/blocks/00000000-0000-0000-0000-000000000000", nil); r.status != 404 {
		t.Fatalf("an unknown block: %d", r.status)
	}
	if r := hotel.do("GET", "/partner/rooms?days=40", nil); r.status != 400 {
		t.Fatalf("a chart longer than 31 days: %d", r.status)
	}
	if r := hotel.do("GET", "/partner/rooms?property_id=a0000000-0000-0000-0000-0000000000ff", nil); r.status != 404 {
		t.Fatalf("a hotel the person does not work for: %d", r.status)
	}
}

func TestAGuestWhoDidNotArriveIsMarkedANoShowAndTheHotelKeepsItsShare(t *testing.T) {
	ctx := context.Background()
	n := time.Now().UnixNano()
	admin := adminClient(t)
	guest := newClient(t, "guest")
	guest.signIn("email", uniqueEmail("noshow"))
	withProfile(guest)
	hotel := hotelFor(t)

	id, _, _, _ := confirmedBooking(t, guest, hotel, 230, 232, fmt.Sprintf("noshow-%d", n))
	// Not before the check-in day.
	if r := hotel.do("POST", "/partner/bookings/"+id+"/no-show", nil); r.status != 409 {
		t.Fatalf("a guest whose day has not come: %d %s", r.status, r.raw)
	}
	// The stay starts today. Two Standard Doubles' worth of nights are booked, as the real booking would have.
	pool.Exec(ctx, `UPDATE bookings SET check_in = `+addisToday+`, check_out = `+addisToday+` + 2 WHERE id = $1`, id)
	pool.Exec(ctx, `INSERT INTO inventory (room_type_id, night, capacity, booked) SELECT $1, d::date, 5, 1 FROM generate_series(`+addisToday+`, `+addisToday+` + 1, '1 day') d
		ON CONFLICT (room_type_id, night) DO UPDATE SET booked = inventory.booked + 1`, standard)
	var before, tomorrow int
	pool.QueryRow(ctx, `SELECT booked FROM inventory WHERE room_type_id = $1 AND night = `+addisToday+` + 1`, standard).Scan(&before)
	ready0, _, _, _ := hotelMoney(t, admin, meskel)

	r := hotel.do("POST", "/partner/bookings/"+id+"/no-show", nil)
	if r.status != 200 || r.str("status") != "NO_SHOW" {
		t.Fatalf("no-show: %d %s", r.status, r.raw)
	}
	pool.QueryRow(ctx, `SELECT booked FROM inventory WHERE room_type_id = $1 AND night = `+addisToday+` + 1`, standard).Scan(&tomorrow)
	if tomorrow != before-1 {
		t.Fatalf("the nights still to come go back on sale: booked %d -> %d", before, tomorrow)
	}
	// The hotel keeps its share and can be paid for it now.
	if ready1, _, _, _ := hotelMoney(t, admin, meskel); ready1 <= ready0 {
		t.Fatalf("a no-show is payable to the hotel: ready %d -> %d", ready0, ready1)
	}
	// The same answer again; the guest sees the status, cannot cancel or review.
	if again := hotel.do("POST", "/partner/bookings/"+id+"/no-show", nil); again.status != 200 {
		t.Fatalf("a repeat is harmless: %d", again.status)
	}
	if b := guest.do("GET", "/bookings/"+id, nil); b.str("status") != "NO_SHOW" || b.body["can_cancel"] != false || b.body["can_review"] != false {
		t.Fatalf("what the guest sees: %s", b.raw)
	}
	// Only a confirmed booking can be marked.
	pending := hold(guest, quoteWith(guest, meskel, standard, 234, 235, 1, 1, nil).str("id"), fmt.Sprintf("noshow-p-%d", n))
	payOnline(t, guest, pending.str("id"), "succeeded")
	if r := hotel.do("POST", "/partner/bookings/"+pending.str("id")+"/no-show", nil); r.status != 409 {
		t.Fatalf("a request the hotel has not answered: %d", r.status)
	}
	if r := hotel.do("POST", "/partner/bookings/00000000-0000-0000-0000-000000000000/no-show", nil); r.status != 404 {
		t.Fatalf("an unknown booking: %d", r.status)
	}
}

func TestFinishedStaysAreCompletedAndOldIDNumbersAreErased(t *testing.T) {
	ctx := context.Background()
	n := time.Now().UnixNano()
	guest := newClient(t, "guest")
	guest.signIn("email", uniqueEmail("daily"))
	withProfile(guest)
	hotel := hotelFor(t)
	setDays := func(id string, in, out int) {
		pool.Exec(ctx, fmt.Sprintf(`UPDATE bookings SET check_in = %s + %d, check_out = %s + %d WHERE id = $1`, addisToday, in, addisToday, out), id)
	}
	statusOf := func(id string) string {
		var s string
		pool.QueryRow(ctx, `SELECT status FROM bookings WHERE id = $1`, id).Scan(&s)
		return s
	}

	// Three stays: one the guest was checked in for, one the hotel never marked, one still to come.
	checked, _, _, _ := confirmedBooking(t, guest, hotel, 240, 242, fmt.Sprintf("daily-1-%d", n))
	setDays(checked, -5, -3)
	pool.Exec(ctx, `UPDATE bookings SET status = 'CHECKED_IN' WHERE id = $1`, checked)
	unmarked, _, _, _ := confirmedBooking(t, guest, hotel, 244, 246, fmt.Sprintf("daily-2-%d", n))
	setDays(unmarked, -4, -3)
	future, _, _, _ := confirmedBooking(t, guest, hotel, 248, 250, fmt.Sprintf("daily-3-%d", n))
	// A stay that ended 45 days ago (past the 30-day retention of the test server).
	old, _, _, _ := confirmedBooking(t, guest, hotel, 252, 254, fmt.Sprintf("daily-4-%d", n))
	setDays(old, -47, -45)

	app.RunMaintenance(ctx)
	if statusOf(checked) != "COMPLETED" || statusOf(unmarked) != "COMPLETED" || statusOf(old) != "COMPLETED" {
		t.Fatalf("ended stays are completed: %s %s %s", statusOf(checked), statusOf(unmarked), statusOf(old))
	}
	if statusOf(future) != "CONFIRMED" {
		t.Fatalf("a stay still to come is left alone: %s", statusOf(future))
	}
	var actorName string
	pool.QueryRow(ctx, `SELECT actor FROM booking_events WHERE booking_id = $1 ORDER BY id DESC LIMIT 1`, checked).Scan(&actorName)
	if actorName != "system" {
		t.Fatalf("the history says who did it: %q", actorName)
	}
	if b := guest.do("GET", "/bookings/"+checked, nil); b.str("status") != "COMPLETED" || b.body["can_review"] != true {
		t.Fatalf("a completed stay can be reviewed: %s", b.raw)
	}
	app.RunMaintenance(ctx) // twice changes nothing
	if statusOf(future) != "CONFIRMED" {
		t.Fatalf("a second run is harmless")
	}

	// ID numbers: erased for the old stay, kept for the recent one; the hotel is told when it asks.
	if r := hotel.do("POST", "/partner/bookings/"+old+"/guest/reveal", nil); r.status != 410 || r.str("error", "code") != "GONE" {
		t.Fatalf("an erased ID number is gone: %d %s", r.status, r.raw)
	}
	if r := hotel.do("POST", "/partner/bookings/"+checked+"/guest/reveal", nil); r.status != 200 {
		t.Fatalf("a recent stay keeps its ID number for now: %d %s", r.status, r.raw)
	}
	var last4 string
	var purged *time.Time
	pool.QueryRow(ctx, `SELECT id_last4, purged_at FROM booking_guests WHERE booking_id = $1`, old).Scan(&last4, &purged)
	if last4 == "" || purged == nil {
		t.Fatalf("only the last characters remain: %q %v", last4, purged)
	}
	var logged int
	pool.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE action = 'id_numbers_erased'`).Scan(&logged)
	if logged < 1 {
		t.Fatalf("an erasure is written to the audit log")
	}

	// A request that never became a stay (the hotel declined) loses its ID number after a week, whatever the dates.
	declined := hold(guest, quoteWith(guest, meskel, standard, 256, 257, 1, 1, nil).str("id"), fmt.Sprintf("daily-5-%d", n))
	payOnline(t, guest, declined.str("id"), "succeeded")
	hotel.do("POST", "/partner/bookings/"+declined.str("id")+"/decline", map[string]string{"reason": "fully_booked"})
	app.RunMaintenance(ctx)
	var kept *time.Time
	pool.QueryRow(ctx, `SELECT purged_at FROM booking_guests WHERE booking_id = $1`, declined.str("id")).Scan(&kept)
	if kept != nil {
		t.Fatalf("a fresh declined request keeps its copy for now")
	}
	if _, err := pool.Exec(ctx, `UPDATE bookings SET created_at = now() - interval '10 days' WHERE id = $1`, declined.str("id")); err != nil {
		t.Skipf("cannot move the clock back in this database: %v", err)
	}
	app.RunMaintenance(ctx)
	pool.QueryRow(ctx, `SELECT purged_at FROM booking_guests WHERE booking_id = $1`, declined.str("id")).Scan(&kept)
	if kept == nil {
		t.Fatalf("a declined request older than a week loses its ID number")
	}
	_ = strings.TrimSpace
}
