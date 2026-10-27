package api_test

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func hotelTicket(c *client, kind, subject, message, booking string) resp {
	body := map[string]any{"kind": kind, "subject": subject, "message": message}
	if booking != "" {
		body["booking_id"] = booking
	}
	return c.do("POST", "/partner/support/tickets", body)
}

func TestHotelStaffCanWriteToSupportAndSupportAnswers(t *testing.T) {
	n := time.Now().UnixNano()
	g, _ := supportGuest(t, "hsg")
	hotel := hotelFor(t)
	admin := adminClient(t)
	desk := receptionist(t, admin, "hsdesk")
	id, _, _, _ := confirmedBooking(t, g, hotel, 84, 86, fmt.Sprintf("hs-%d", n))

	r := hotelTicket(hotel, "problem", "A guest says the QR code does not scan", "The guest of this booking cannot be checked in with the QR code.", id)
	if r.status != 200 {
		t.Fatalf("open: %d %s", r.status, r.raw)
	}
	tid := r.str("id")
	general := hotelTicket(hotel, "question", "How do payouts work?", "When is our money paid out?", "")
	if general.status != 200 {
		t.Fatalf("general: %d %s", general.status, general.raw)
	}

	// The admin sees which hotel asks, and the booking next to it.
	got := admin.do("GET", "/admin/support/tickets/"+tid, nil)
	if got.str("ticket", "hotel") != "Meskel Square Palace Hotel" || got.str("ticket", "booking", "status") != "CONFIRMED" {
		t.Errorf("the admin sees the hotel and the booking: %s", got.raw)
	}
	if q := admin.do("GET", "/admin/support/tickets", nil); !strings.Contains(string(q.raw), "Meskel Square Palace Hotel") {
		t.Errorf("the queue names the hotel: %s", q.raw)
	}
	if a := admin.do("POST", "/admin/support/tickets/"+tid+"/reply", map[string]string{"message": "Ask for the 4-digit PIN instead, it works the same."}); a.status != 200 {
		t.Fatalf("answer: %d %s", a.status, a.raw)
	}
	tk := hotel.do("GET", "/partner/support/tickets/"+tid, nil)
	if tk.str("ticket", "status") != "answered" || !strings.Contains(string(tk.raw), "4-digit PIN") || strings.Contains(string(tk.raw), "admin@") {
		t.Errorf("the hotel reads the answer from Support: %s", tk.raw)
	}
	if r := hotel.do("POST", "/partner/support/tickets/"+tid+"/messages", map[string]string{"message": "Thank you, that worked."}); r.status != 200 {
		t.Errorf("the hotel replies: %d", r.status)
	}
	if list := hotel.do("GET", "/partner/support/tickets", nil); !strings.Contains(string(list.raw), tid) || !strings.Contains(string(list.raw), general.str("id")) {
		t.Errorf("the hotel's list: %s", list.raw)
	}

	// Privacy: a colleague, a guest and a signed-out caller cannot read it; the guest app's endpoints are not the hotel's.
	if x := desk.do("GET", "/partner/support/tickets/"+tid, nil); x.status != 404 {
		t.Errorf("a colleague's request is theirs: %d", x.status)
	}
	if x := g.do("GET", "/support/tickets/"+tid, nil); x.status != 404 {
		t.Errorf("a guest cannot read a hotel's request: %d", x.status)
	}
	if x := hotel.do("GET", "/support/tickets", nil); x.status != 401 {
		t.Errorf("the hotel's token does not work on the guest endpoints: %d", x.status)
	}
	if x := newClient(t, "partner").do("GET", "/partner/support/tickets", nil); x.status != 401 {
		t.Errorf("signed out: %d", x.status)
	}
	if x := g.do("GET", "/partner/support/tickets", nil); x.status != 401 {
		t.Errorf("a guest token does not work on the hotel endpoints: %d", x.status)
	}
}

func TestAHotelRequestMustBeAboutItsOwnBookings(t *testing.T) {
	n := time.Now().UnixNano()
	g, _ := supportGuest(t, "hso")
	hotel := hotelFor(t)
	desk := receptionist(t, adminClient(t), "hsodesk") // staff of Meskel only (the demo owner is staff of several hotels)
	// A booking at another hotel (Bole takes pay at hotel).
	other := hold(g, quote(g, bole, boleSingle, 88, 89, 1, 1).str("id"), fmt.Sprintf("hs-o-%d", n)).str("id")
	g.do("POST", "/bookings/"+other+"/pay", map[string]string{"method": "pay_at_hotel"})
	if r := hotelTicket(desk, "problem", "Not our booking", "A booking of another hotel.", other); r.status != 404 {
		t.Errorf("another hotel's booking: %d %s", r.status, r.raw)
	}
	held := hold(g, quote(g, meskel, standard, 90, 91, 1, 1).str("id"), fmt.Sprintf("hs-h-%d", n)).str("id")
	if r := hotelTicket(hotel, "problem", "A hold", "Not even booked yet.", held); r.status != 404 {
		t.Errorf("a hold is not a booking yet: %d", r.status)
	}
	for name, r := range map[string]resp{
		"bad kind":      hotelTicket(hotel, "dispute", "Not allowed", "Disputes are the guests'.", ""),
		"short subject": hotelTicket(hotel, "question", "Hi", "Text", ""),
		"empty message": hotelTicket(hotel, "question", "A real subject", "  ", ""),
		"bad booking":   hotelTicket(hotel, "question", "A real subject", "Text", "not-a-uuid"),
	} {
		if r.status == 200 || r.status >= 500 {
			t.Errorf("%s: %d", name, r.status)
		}
	}
	// The limit of five waiting requests applies to a hotel too.
	var ok int
	for i := 0; i < 7; i++ {
		if hotelTicket(hotel, "question", fmt.Sprintf("Question %d about something", i), "Text", "").status == 200 {
			ok++
		}
	}
	if ok > 5 {
		t.Errorf("at most five waiting requests: %d", ok)
	}
}
