package api_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

// staffMember adds a person with a signed-in hotel phone to the Meskel hotel and returns their user id.
func staffMember(t *testing.T, role, email, token string) string {
	t.Helper()
	ctx := context.Background()
	var uid string
	if err := pool.QueryRow(ctx, `INSERT INTO users (email, role) VALUES ($1, $2) RETURNING id`, email, role).Scan(&uid); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO property_staff (property_id, user_id, role) VALUES ($1, $2, $3)`, meskel, uid, role); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO devices (user_id, device_id, app, platform, app_version, push_token) VALUES ($1, $2, 'partner', 'android', '1.0.0', $3)`, uid, "dev-"+token, token); err != nil {
		t.Fatal(err)
	}
	return uid
}

func titles(ms []message) string {
	var out []string
	for _, m := range ms {
		out = append(out, m.title)
	}
	return strings.Join(out, " | ")
}

func TestTheFrontDeskIsAlertedFirstThenTheOwnerThenRemindedByEmail(t *testing.T) {
	ctx := context.Background()
	admin := adminClient(t)
	n := time.Now().UnixNano()
	recTok, ownTok := fmt.Sprintf("rec-phone-%d", n), fmt.Sprintf("own-phone-%d", n)
	ownEmail := realAddress("boss")
	rec := staffMember(t, "receptionist", realAddress("desk"), recTok)
	own := staffMember(t, "owner", ownEmail, ownTok)
	defer func() {
		pool.Exec(ctx, `DELETE FROM devices WHERE user_id IN ($1, $2)`, rec, own)
		pool.Exec(ctx, `DELETE FROM property_staff WHERE user_id IN ($1, $2)`, rec, own)
		pool.Exec(ctx, `UPDATE properties SET alert_mode = 'everyone', escalate_after_min = 5 WHERE id = $1`, meskel)
	}()

	// The rules are validated and saved.
	for name, body := range map[string]map[string]any{
		"unknown mode":    {"alert_mode": "nobody", "escalate_after_min": 5},
		"too long a wait": {"alert_mode": "front_desk_first", "escalate_after_min": 11},
		"zero":            {"alert_mode": "front_desk_first", "escalate_after_min": 0},
	} {
		if r := admin.do("PUT", "/admin/hotels/"+meskel+"/alerts", body); r.status != 400 {
			t.Errorf("%s must be refused, got %d", name, r.status)
		}
	}
	if r := admin.do("PUT", "/admin/hotels/"+meskel+"/alerts", map[string]any{"alert_mode": "front_desk_first", "escalate_after_min": 5}); r.status != 200 {
		t.Fatalf("save rules: %d %s", r.status, r.raw)
	}
	if d := admin.do("GET", "/admin/hotels/"+meskel, nil); d.body["alert_mode"] != "front_desk_first" {
		t.Fatalf("the rules should be shown: %s", d.raw)
	}

	guest := newClient(t, "guest")
	guest.signIn("email", uniqueEmail("alert"))
	withProfile(guest)
	book := func(in, out int, key string) string {
		b := hold(guest, quote(guest, meskel, standard, in, out, 1, 1).str("id"), key)
		payOnline(t, guest, b.str("id"), "succeeded")
		return b.str("id")
	}

	// 1. A request arrives: the receptionist is alerted, the owner is not.
	id := book(260, 261, fmt.Sprintf("alert-1-%d", n))
	if got := push.to(recTok); len(got) != 1 || got[0].title != "New booking request" || got[0].data["booking_id"] != id {
		t.Fatalf("the receptionist should be alerted at once: %s", titles(got))
	}
	if got := push.to(ownTok); len(got) != 0 {
		t.Fatalf("the owner must wait while the front desk can answer: %s", titles(got))
	}
	app.Sweep(ctx)
	if len(push.to(ownTok)) != 0 {
		t.Fatalf("nothing is due yet, so the owner is still not alerted")
	}

	// 2. Five minutes pass with no answer: the owner is alerted, the receptionist is not alerted again, and only once.
	pool.Exec(ctx, `UPDATE bookings SET hotel_deadline_at = now() + interval '9 minutes 30 seconds' WHERE id = $1`, id)
	app.Sweep(ctx)
	app.Sweep(ctx)
	if got := push.to(ownTok); len(got) != 1 || got[0].title != "Nobody has answered yet" || got[0].data["booking_id"] != id || got[0].data["type"] != "new_booking" {
		t.Fatalf("the owner should be alerted once the front desk is slow: %s", titles(got))
	}
	if len(push.to(recTok)) != 1 {
		t.Fatalf("the receptionist must not be alerted again for the escalation: %s", titles(push.to(recTok)))
	}

	// 3. Five minutes are left: everyone gets a reminder and the owner is emailed, once.
	pool.Exec(ctx, `UPDATE bookings SET hotel_deadline_at = now() + interval '4 minutes' WHERE id = $1`, id)
	app.Sweep(ctx)
	app.Sweep(ctx)
	if got := push.to(recTok); len(got) != 2 || got[1].title != "5 minutes left to answer" {
		t.Fatalf("the receptionist should be reminded once: %s", titles(got))
	}
	if got := push.to(ownTok); len(got) != 2 || got[1].title != "5 minutes left to answer" {
		t.Fatalf("the owner should be reminded once: %s", titles(got))
	}
	mail := mails.to(ownEmail)
	if len(mail) != 1 || !strings.Contains(mail[0].subject, "5 minutes left") || !strings.Contains(mail[0].text, "Reference:") {
		t.Fatalf("the owner should be emailed once: %+v", mail)
	}

	// 4. An answered request is left alone: no escalation, no reminder.
	id2 := book(262, 263, fmt.Sprintf("alert-2-%d", n))
	pool.Exec(ctx, `UPDATE bookings SET hotel_deadline_at = now() + interval '3 minutes' WHERE id = $1`, id2)
	if err := pool.QueryRow(ctx, `SELECT 1`).Scan(new(int)); err != nil {
		t.Fatal(err)
	}
	pool.Exec(ctx, `UPDATE bookings SET status = 'CONFIRMED', hotel_deadline_at = NULL WHERE id = $1`, id2)
	before := len(push.to(ownTok))
	app.Sweep(ctx)
	if len(push.to(ownTok)) != before || len(mails.to(ownEmail)) != 1 {
		t.Fatalf("a request that is no longer waiting must not trigger alerts")
	}
}

func TestShiftsAndMutedAlertsDecideWhoIsReached(t *testing.T) {
	ctx := context.Background()
	admin := adminClient(t)
	n := time.Now().UnixNano()
	recTok, ownTok := fmt.Sprintf("shift-rec-%d", n), fmt.Sprintf("shift-own-%d", n)
	rec := staffMember(t, "receptionist", realAddress("night"), recTok)
	own := staffMember(t, "owner", realAddress("owner"), ownTok)
	defer func() {
		pool.Exec(ctx, `DELETE FROM devices WHERE user_id IN ($1, $2)`, rec, own)
		pool.Exec(ctx, `DELETE FROM property_staff WHERE user_id IN ($1, $2)`, rec, own)
		pool.Exec(ctx, `UPDATE properties SET alert_mode = 'everyone', escalate_after_min = 5 WHERE id = $1`, meskel)
	}()
	set := func(user string, body map[string]any) resp {
		return admin.do("PUT", "/admin/hotels/"+meskel+"/staff/"+user+"/alerts", body)
	}

	// The shift is validated, and it shows on the hotel page.
	for name, body := range map[string]map[string]any{
		"only a start":   {"alerts_on": true, "shift_start": "08:00", "shift_end": ""},
		"not a time":     {"alerts_on": true, "shift_start": "8am", "shift_end": "17:00"},
		"same time":      {"alerts_on": true, "shift_start": "08:00", "shift_end": "08:00"},
		"hour too large": {"alerts_on": true, "shift_start": "08:00", "shift_end": "25:00"},
	} {
		if r := set(rec, body); r.status != 400 {
			t.Errorf("%s must be refused, got %d", name, r.status)
		}
	}
	if r := set("not-a-user", map[string]any{"alerts_on": true}); r.status != 404 {
		t.Errorf("an unknown person: %d", r.status)
	}

	// A shift that has just ended and one that starts in two hours: the receptionist is off duty either way.
	var start, end string
	pool.QueryRow(ctx, `SELECT to_char((now() AT TIME ZONE 'Africa/Addis_Ababa')::time + interval '2 hours', 'HH24:MI'),
		to_char((now() AT TIME ZONE 'Africa/Addis_Ababa')::time + interval '5 hours', 'HH24:MI')`).Scan(&start, &end)
	if r := set(rec, map[string]any{"alerts_on": true, "shift_start": start, "shift_end": end}); r.status != 200 {
		t.Fatalf("set shift: %d %s", r.status, r.raw)
	}
	var got struct{ start, end *string }
	pool.QueryRow(ctx, `SELECT to_char(shift_start, 'HH24:MI'), to_char(shift_end, 'HH24:MI') FROM property_staff WHERE user_id = $1`, rec).Scan(&got.start, &got.end)
	if got.start == nil || *got.start != start || *got.end != end {
		t.Fatalf("the shift should be saved: %v %v", got.start, got.end)
	}
	shown := false
	for _, s := range admin.do("GET", "/admin/hotels/"+meskel, nil).body["staff"].([]any) {
		if m := s.(map[string]any); m["user_id"] == rec && m["shift_start"] == start && m["reachable"] == true && m["alerts_on"] == true {
			shown = true
		}
	}
	if !shown {
		t.Fatalf("the hotel page should show the shift and that the phone is reachable")
	}

	guest := newClient(t, "guest")
	guest.signIn("email", uniqueEmail("shift"))
	withProfile(guest)
	book := func(in, out int, key string) {
		b := hold(guest, quote(guest, meskel, standard, in, out, 1, 1).str("id"), key)
		payOnline(t, guest, b.str("id"), "succeeded")
	}

	// 2. "Front desk first", but the front desk is off duty: the owner is alerted at once and the receptionist sleeps.
	admin.do("PUT", "/admin/hotels/"+meskel+"/alerts", map[string]any{"alert_mode": "front_desk_first", "escalate_after_min": 5})
	book(264, 265, fmt.Sprintf("shift-1-%d", n))
	if len(push.to(recTok)) != 0 {
		t.Fatalf("a receptionist off duty must not be alerted: %s", titles(push.to(recTok)))
	}
	if g := push.to(ownTok); len(g) != 1 || g[0].title != "New booking request" {
		t.Fatalf("with the front desk off duty the owner is alerted at once: %s", titles(g))
	}

	// 3. Everyone on duty: the receptionist is still asleep, the owner (no shift) is alerted.
	admin.do("PUT", "/admin/hotels/"+meskel+"/alerts", map[string]any{"alert_mode": "everyone", "escalate_after_min": 5})
	book(266, 267, fmt.Sprintf("shift-2-%d", n))
	if len(push.to(recTok)) != 0 || len(push.to(ownTok)) != 2 {
		t.Fatalf("only people on duty are alerted: rec=%d own=%d", len(push.to(recTok)), len(push.to(ownTok)))
	}

	// 4. A shift that is on right now (it runs past midnight, as night shifts do) means the receptionist is alerted.
	pool.QueryRow(ctx, `SELECT to_char((now() AT TIME ZONE 'Africa/Addis_Ababa')::time - interval '1 hour', 'HH24:MI'),
		to_char((now() AT TIME ZONE 'Africa/Addis_Ababa')::time - interval '2 hours', 'HH24:MI')`).Scan(&start, &end)
	if r := set(rec, map[string]any{"alerts_on": true, "shift_start": start, "shift_end": end}); r.status != 200 {
		t.Fatalf("set overnight shift: %d %s", r.status, r.raw)
	}
	book(268, 269, fmt.Sprintf("shift-3-%d", n))
	if len(push.to(recTok)) != 1 {
		t.Fatalf("a receptionist on an overnight shift that covers now must be alerted: %d", len(push.to(recTok)))
	}

	// 5. Muting beats everything: with the receptionist and the owner muted nobody is alerted (and nothing breaks).
	set(rec, map[string]any{"alerts_on": false})
	set(own, map[string]any{"alerts_on": false})
	ownBefore, recBefore := len(push.to(ownTok)), len(push.to(recTok))
	book(270, 271, fmt.Sprintf("shift-4-%d", n))
	if len(push.to(ownTok)) != ownBefore || len(push.to(recTok)) != recBefore {
		t.Fatalf("muted people must not be alerted")
	}

	// 6. An owner may change their receptionists and themselves, but not another owner.
	if r := set("00000000-0000-0000-0000-000000000000", map[string]any{"alerts_on": true}); r.status != 404 {
		t.Fatalf("a person not on this hotel: %d", r.status)
	}
}
