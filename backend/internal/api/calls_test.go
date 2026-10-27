package api_test

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestAHotelsNumberIsShownToAGuestOnlyOnceTheBookingIsConfirmed(t *testing.T) {
	n := time.Now().UnixNano()
	g, _ := supportGuest(t, "call")
	other, _ := supportGuest(t, "callo")
	hotel := hotelFor(t)
	admin := adminClient(t)

	// Not on the hotel's page.
	prop := g.do("GET", "/properties/"+meskel+"?check_in="+day(110)+"&check_out="+day(111), nil)
	if prop.status != 200 || prop.str("property", "phone") != "" {
		t.Errorf("the hotel page carries no phone number: %d %q", prop.status, prop.str("property", "phone"))
	}
	if strings.Contains(string(prop.raw), "+251 11") {
		t.Errorf("no phone number anywhere in the page: %s", prop.raw)
	}

	// Not while the booking is held, or waiting for the hotel.
	id := hold(g, quote(g, meskel, standard, 112, 114, 1, 1).str("id"), fmt.Sprintf("call-%d", n)).str("id")
	if b := g.do("GET", "/bookings/"+id, nil); b.str("property_phone") != "" {
		t.Errorf("a held booking shows no number: %s", b.raw)
	}
	payOnline(t, g, id, "succeeded")
	if b := g.do("GET", "/bookings/"+id, nil); b.str("property_phone") != "" {
		t.Errorf("a booking waiting for the hotel shows no number: %s", b.raw)
	}
	if r := g.do("POST", "/bookings/"+id+"/call", nil); r.status != 409 {
		t.Errorf("no call can be logged before the number is shown: %d", r.status)
	}

	// Once the hotel has confirmed it, the number is there.
	if r := hotel.do("POST", "/partner/bookings/"+id+"/accept", nil); r.status != 200 {
		t.Fatalf("accept: %d", r.status)
	}
	b := g.do("GET", "/bookings/"+id, nil)
	if b.str("property_phone") == "" {
		t.Fatalf("a confirmed booking shows the hotel's number: %s", b.raw)
	}
	if list := g.do("GET", "/bookings", nil); !strings.Contains(string(list.raw), b.str("property_phone")) {
		t.Errorf("the list agrees")
	}

	// A tap on "Call the hotel" is logged; the admin sees it, the guest cannot, and nobody else's booking can be logged.
	for i := 0; i < 2; i++ {
		if r := g.do("POST", "/bookings/"+id+"/call", nil); r.status != 200 {
			t.Fatalf("log: %d %s", r.status, r.raw)
		}
	}
	if r := other.do("POST", "/bookings/"+id+"/call", nil); r.status != 404 {
		t.Errorf("another guest's booking: %d", r.status)
	}
	if r := g.do("POST", "/bookings/not-a-uuid/call", nil); r.status != 404 {
		t.Errorf("garbled id: %d", r.status)
	}
	calls := admin.do("GET", "/admin/calls?days=7", nil)
	if calls.status != 200 {
		t.Fatalf("calls: %d %s", calls.status, calls.raw)
	}
	var row map[string]any
	for _, h := range calls.body["hotels"].([]any) {
		if m := h.(map[string]any); m["id"] == meskel {
			row = m
		}
	}
	if row == nil || row["calls"].(float64) < 2 || row["bookings_with_calls"].(float64) < 1 {
		t.Errorf("the admin sees the calls per hotel: %s", calls.raw)
	}
	var recent bool
	for _, c := range calls.body["recent"].([]any) {
		if m := c.(map[string]any); m["status"] == "CONFIRMED" && strings.HasPrefix(m["booking_code"].(string), "BD-") {
			recent = true
		}
	}
	if !recent {
		t.Errorf("the latest calls are listed with their booking: %s", calls.raw)
	}
	if r := g.do("GET", "/admin/calls", nil); r.status != 401 && r.status != 403 {
		t.Errorf("a guest cannot read the call log: %d", r.status)
	}
	if r := hotel.do("GET", "/admin/calls", nil); r.status != 401 && r.status != 403 {
		t.Errorf("a hotel cannot read the call log: %d", r.status)
	}
	// A cancelled booking no longer shows the number.
	if c := g.do("POST", "/bookings/"+id+"/cancel", nil); c.status != 200 || c.str("property_phone") != "" {
		t.Errorf("a cancelled booking shows no number: %d %s", c.status, c.raw)
	}
}
