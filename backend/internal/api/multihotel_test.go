package api_test

import (
	"context"
	"strings"
	"testing"
)

func TestAnOwnerOfSeveralHotelsAnswersForAllAndASingleHotelOnlyForItself(t *testing.T) {
	guest := newClient(t, "guest")
	guest.signIn("email", uniqueEmail("mh"))
	withProfile(guest)

	// A guest books Bole Garden (not the hotel the demo owner used to be limited to).
	q := quote(guest, bole, boleSingle, 120, 121, 1, 1)
	b := hold(guest, q.str("id"), "key-multi-1")
	payOnline(t, guest, b.str("id"), "succeeded")

	// The demo owner works for every demo hotel: the request reaches them, labelled with the hotel.
	owner := hotelFor(t)
	list := owner.do("GET", "/partner/bookings", nil)
	var mine map[string]any
	for _, it := range list.body["items"].([]any) {
		if it.(map[string]any)["id"] == b.str("id") {
			mine = it.(map[string]any)
		}
	}
	if mine == nil || mine["property_name"] != "Bole Garden Guesthouse" {
		t.Fatalf("the owner should see the Bole request, labelled with its hotel: %s", list.raw)
	}
	if prop, _ := list.body["property"].(map[string]any); !strings.HasSuffix(prop["name"].(string), "hotels") {
		t.Fatalf("an owner of several hotels is shown as such: %v", prop)
	}

	// A person who works only for Meskel Square cannot see, reveal or answer Bole's booking.
	email := uniqueEmail("staff")
	if _, err := pool.Exec(context.Background(), `WITH u AS (INSERT INTO users (email, role) VALUES ($1, 'receptionist') RETURNING id)
		INSERT INTO property_staff (property_id, user_id, role) SELECT $2::uuid, id, 'receptionist' FROM u`, email, meskel); err != nil {
		t.Fatal(err)
	}
	other := newClient(t, "partner")
	other.signIn("email", email)
	if l := other.do("GET", "/partner/bookings", nil); strings.Contains(string(l.raw), b.str("id")) || strings.Contains(string(l.raw), `"property_name"`) {
		t.Fatalf("a single-hotel account must see only its own hotel, without labels: %s", l.raw)
	}
	for _, path := range []string{"/accept", "/guest/reveal"} {
		if r := other.do("POST", "/partner/bookings/"+b.str("id")+path, nil); r.status != 404 {
			t.Fatalf("another hotel's staff must not reach %s, got %d", path, r.status)
		}
	}
	if r := other.do("POST", "/partner/bookings/"+b.str("id")+"/decline", map[string]string{"reason": "other"}); r.status != 404 {
		t.Fatalf("another hotel's staff must not decline: %d", r.status)
	}

	// The owner can answer it.
	if r := owner.do("POST", "/partner/bookings/"+b.str("id")+"/accept", nil); r.status != 200 || r.str("status") != "CONFIRMED" {
		t.Fatalf("the owner accepts: %d %s", r.status, r.raw)
	}
}
