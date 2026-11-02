package api_test

import (
	"fmt"
	"testing"
)

// ownerClient signs a hotel owner in to the admin page with the code emailed to a real (non-demo) address.
func ownerClient(t *testing.T, email string) *client {
	t.Helper()
	c := newClient(t, "admin")
	if r := c.do("POST", "/admin/otp/request", map[string]any{"type": "email", "value": email}); r.status != 200 {
		t.Fatalf("owner code request: %d %s", r.status, r.raw)
	}
	r := c.do("POST", "/admin/otp/verify", map[string]any{"type": "email", "value": email, "code": latestEmailedCode(t, email)})
	if r.status != 200 || r.str("role") != "owner" {
		t.Fatalf("owner sign-in: %d %s", r.status, r.raw)
	}
	c.token = r.str("token")
	return c
}

// An owner adds and removes receptionists and rooms of their own hotel, and nothing else: no owners, no other hotels.
func TestOwnersManageTheirOwnStaffAndRooms(t *testing.T) {
	admin := adminClient(t)
	n := fmt.Sprint(uniqueEmail("own"))

	// A new hotel with one owner (the demo owner account), and a second hotel that owner has nothing to do with.
	h := admin.do("POST", "/admin/hotels", newHotelBody("Owner Test Lodge "+n[len(n)-6:]))
	mine := h.str("id")
	other := admin.do("POST", "/admin/hotels", newHotelBody("Someone Else Lodge "+n[len(n)-6:]))
	theirs := other.str("id")
	if mine == "" || theirs == "" {
		t.Fatalf("create hotels: %s %s", h.raw, other.raw)
	}
	// A real-looking address (not a demo one): the admin page accepts it, and its sign-in code is emailed, not fixed.
	owner := "owner.lodge.staff@gmail.com"
	if r := admin.do("POST", "/admin/hotels/"+mine+"/staff", map[string]any{"contact": owner, "role": "owner"}); r.status != 200 {
		t.Fatalf("admin adds the owner: %d %s", r.status, r.raw)
	}
	defer admin.do("DELETE", "/admin/hotels/"+mine+"/staff/"+staffID(admin, mine, owner), nil)

	o := ownerClient(t, owner)

	// Receptionists: an owner may add and remove them on their own hotel.
	rec := uniqueEmail("rec")
	if r := o.do("POST", "/admin/hotels/"+mine+"/staff", map[string]any{"contact": rec, "role": "receptionist"}); r.status != 200 {
		t.Fatalf("owner adds a receptionist: %d %s", r.status, r.raw)
	}
	if r := o.do("DELETE", "/admin/hotels/"+mine+"/staff/"+staffID(admin, mine, rec), nil); r.status != 200 {
		t.Fatalf("owner removes a receptionist: %d %s", r.status, r.raw)
	}

	// Owners cannot add another owner.
	if r := o.do("POST", "/admin/hotels/"+mine+"/staff", map[string]any{"contact": uniqueEmail("boss"), "role": "owner"}); r.status != 403 {
		t.Fatalf("an owner must not add an owner: %d %s", r.status, r.raw)
	}

	// Rooms: an owner adds a room type and removes it again (it has no bookings).
	room := map[string]any{"name": "Owner Room", "capacity": 2, "total_rooms": 1, "amenities": []string{"wifi"}}
	created := o.do("POST", "/admin/hotels/"+mine+"/rooms", room)
	if created.status != 200 {
		t.Fatalf("owner adds a room: %d %s", created.status, created.raw)
	}
	if r := o.do("DELETE", "/admin/hotels/"+mine+"/rooms/"+created.str("id"), nil); r.status != 200 {
		t.Fatalf("owner removes an unbooked room: %d %s", r.status, r.raw)
	}

	// Other hotels are out of reach for the owner.
	// Another hotel looks like it does not exist to this owner (404), so the server does not reveal which hotels there are.
	if r := o.do("POST", "/admin/hotels/"+theirs+"/staff", map[string]any{"contact": uniqueEmail("x"), "role": "receptionist"}); r.status != 404 {
		t.Fatalf("an owner must not add staff to another hotel: %d %s", r.status, r.raw)
	}
	if r := o.do("POST", "/admin/hotels/"+theirs+"/rooms", room); r.status != 404 {
		t.Fatalf("an owner must not add rooms to another hotel: %d %s", r.status, r.raw)
	}
}

// A room that has been booked cannot be removed, so the money records behind it stay intact.
func TestARoomWithBookingsCannotBeRemoved(t *testing.T) {
	admin := adminClient(t)
	g := newClient(t, "guest")
	g.signIn("email", uniqueEmail("keep"))
	withProfile(g)
	q := quote(g, meskel, family, 270, 271, 2, 1)
	if q.status != 200 {
		t.Skipf("quote: %d %s", q.status, q.raw)
	}
	hold(g, q.str("id"), "key-keep-room-1")
	r := admin.do("DELETE", "/admin/hotels/"+meskel+"/rooms/"+suiteID(admin), nil)
	if r.status != 409 {
		t.Fatalf("a room with a booking must not be removable: %d %s", r.status, r.raw)
	}
}

// staffID finds the staff row for a contact on a hotel, through the admin hotel page.
func staffID(admin *client, hotelID, contact string) string {
	d := admin.do("GET", "/admin/hotels/"+hotelID, nil)
	for _, s := range d.body["staff"].([]any) {
		m := s.(map[string]any)
		if m["contact"] == contact {
			return m["user_id"].(string)
		}
	}
	return "00000000-0000-0000-0000-000000000000"
}

// latestEmailedCode returns the newest sign-in code the test mailer sent to an address.
func latestEmailedCode(t *testing.T, email string) string {
	t.Helper()
	mailer.mu.Lock()
	defer mailer.mu.Unlock()
	for i := len(mailer.sent) - 1; i >= 0; i-- {
		if mailer.sent[i].to == email {
			return mailer.sent[i].code
		}
	}
	t.Fatalf("no code was emailed to %s", email)
	return ""
}
