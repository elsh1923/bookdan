package api_test

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestPlaceSuggestionsComeFromLandmarksAreasAndHotels(t *testing.T) {
	c := newClient(t, "guest")
	texts := func(r resp) map[string]string {
		out := map[string]string{}
		items, ok := r.body["items"].([]any)
		if !ok {
			t.Fatalf("items must be a list, never null: %s", r.raw)
		}
		for _, it := range items {
			m := it.(map[string]any)
			out[m["text"].(string)] = m["kind"].(string)
		}
		return out
	}
	// Nothing typed: the popular places (landmarks).
	if got := texts(c.do("GET", "/suggest", nil)); got["Meskel Square"] != "landmark" || len(got) < 3 {
		t.Fatalf("with nothing typed the landmarks are offered: %v", got)
	}
	// "bol": the landmark, the area, and the hotel by name (with its id).
	r := c.do("GET", "/suggest?q=BOL", nil)
	got := texts(r)
	if got["Bole Airport"] != "landmark" || got["Bole"] != "area" || got["Bole Garden Guesthouse"] != "hotel" {
		t.Fatalf("typing bol should suggest the landmark, the area and the hotel: %v", got)
	}
	for _, it := range r.body["items"].([]any) {
		if m := it.(map[string]any); m["kind"] == "hotel" && m["property_id"] == nil {
			t.Fatalf("a hotel suggestion carries its id: %v", m)
		}
	}
	// A city, in Amharic for a landmark, and nothing for nonsense.
	if got := texts(c.do("GET", "/suggest?q=bahir", nil)); got["Bahir Dar"] != "city" {
		t.Fatalf("a city with a hotel: %v", got)
	}
	if got := texts(c.do("GET", "/suggest?q="+url.QueryEscape("መስቀል"), nil)); got["Meskel Square"] != "landmark" {
		t.Fatalf("an Amharic landmark name finds the landmark: %v", got)
	}
	if got := texts(c.do("GET", "/suggest?q=zzzzqq", nil)); len(got) != 0 {
		t.Fatalf("nonsense finds nothing: %v", got)
	}
	// A percent sign is just a character, not "match everything".
	if got := texts(c.do("GET", "/suggest?q="+url.QueryEscape("%"), nil)); len(got) != 0 {
		t.Fatalf("%% must not match everything: %v", got)
	}
}

func TestChildrenRequestsAndBookingForSomeoneElseReachTheHotel(t *testing.T) {
	n := time.Now().UnixNano()
	guest := newClient(t, "guest")
	guest.signIn("email", uniqueEmail("kids"))
	withProfile(guest)
	hotel := hotelFor(t)

	// Every room needs an adult: with 2 rooms and 3 guests, at most 1 child.
	if r := quoteWith(guest, meskel, standard, 322, 323, 3, 2, map[string]any{"children": 2}); r.status != 400 {
		t.Fatalf("2 rooms with only one adult: %d", r.status)
	}
	if r := quoteWith(guest, meskel, standard, 322, 323, 2, 1, map[string]any{"children": -1}); r.status != 400 {
		t.Fatalf("a negative number of children: %d", r.status)
	}
	q := quoteWith(guest, meskel, standard, 322, 323, 2, 1, map[string]any{"children": 1})
	if q.status != 200 || q.num("children") != 1 {
		t.Fatalf("a quote for 1 adult and 1 child: %d %s", q.status, q.raw)
	}

	hold := func(key string, extra map[string]any) resp {
		body := map[string]any{"quote_id": q.str("id"), "guest_name": "Selam Bekele", "idempotency_key": key}
		for k, v := range extra {
			body[k] = v
		}
		return guest.do("POST", "/bookings", body)
	}
	if r := hold(fmt.Sprintf("kid-long-%d", n), map[string]any{"special_requests": strings.Repeat("a", 401)}); r.status != 400 {
		t.Fatalf("requests over 400 characters: %d", r.status)
	}
	if r := hold(fmt.Sprintf("kid-phone-%d", n), map[string]any{"guest_phone": "call me"}); r.status != 400 {
		t.Fatalf("a bad phone number: %d", r.status)
	}
	key := fmt.Sprintf("kid-ok-%d", n)
	b := hold(key, map[string]any{"special_requests": "  Late arrival around midnight, high floor please  ", "booked_for_other": true, "guest_phone": "+251 911 223 344"})
	if b.status != 200 || b.str("special_requests") != "Late arrival around midnight, high floor please" || b.body["booked_for_other"] != true ||
		b.str("guest_phone") != "+251 911 223 344" || b.num("children") != 1 {
		t.Fatalf("the booking carries who and what: %d %s", b.status, b.raw)
	}
	// The same key with different requests is a different request.
	if r := hold(key, map[string]any{"special_requests": "something else", "booked_for_other": true, "guest_phone": "+251 911 223 344"}); r.status != 409 {
		t.Fatalf("the same key for a changed request: %d", r.status)
	}
	// Retrying the same request returns the same booking.
	if r := hold(key, map[string]any{"special_requests": "  Late arrival around midnight, high floor please  ", "booked_for_other": true, "guest_phone": "+251 911 223 344"}); r.status != 200 || r.str("id") != b.str("id") {
		t.Fatalf("a retry returns the same booking: %d %s", r.status, r.raw)
	}

	payOnline(t, guest, b.str("id"), "succeeded")
	seen := false
	for _, it := range hotel.do("GET", "/partner/bookings", nil).body["items"].([]any) {
		if m := it.(map[string]any); m["id"] == b.str("id") {
			seen = m["children"] == float64(1) && m["special_requests"] == "Late arrival around midnight, high floor please" &&
				m["booked_for_other"] == true && m["guest_phone"] == "+251 911 223 344" && m["guest_name"] == "Selam Bekele"
		}
	}
	if !seen {
		t.Fatalf("the hotel should see the children, the requests, and who is staying")
	}
}

func TestSearchCanAskForBreakfastAndCheaperRatesAndShowsHowFastHotelsReply(t *testing.T) {
	ctx := context.Background()
	n := time.Now().UnixNano()
	admin := adminClient(t)
	bf, nr := int64(15000), 1000
	setRoomOptions(t, admin, meskel, standard, &bf, &nr)
	defer setRoomOptions(t, admin, meskel, standard, nil, nil)
	guest := newClient(t, "guest")
	guest.signIn("email", uniqueEmail("flt"))
	withProfile(guest)
	hotel := hotelFor(t)

	// Only hotels that offer breakfast (or a cheaper non-refundable rate) are kept, and each card says what it offers.
	for _, filter := range []string{"breakfast=1", "deals=1"} {
		r := guest.do("GET", searchPath("&"+filter), nil)
		items := itemsOf(r)
		if len(items) != 1 || items[0]["name"] != "Meskel Square Palace Hotel" {
			t.Fatalf("%s should keep only the hotel that offers it: %s", filter, r.raw)
		}
		if items[0]["breakfast"] != true || items[0]["nonrefundable"] != true {
			t.Fatalf("the card says what is offered: %v", items[0])
		}
	}
	all := itemsOf(guest.do("GET", searchPath(""), nil))
	flagged := 0
	for _, it := range all {
		if it["breakfast"] == true {
			flagged++
		}
	}
	if flagged != 1 || len(all) < 5 {
		t.Fatalf("only Meskel offers breakfast (%d flagged of %d)", flagged, len(all))
	}

	// How fast a hotel answers: from three answers on, in minutes.
	for i := 0; i < 3; i++ {
		id, _, _, _ := confirmedBooking(t, guest, hotel, 324+2*i, 325+2*i, fmt.Sprintf("reply-%d-%d", i, n))
		pool.Exec(ctx, `UPDATE bookings SET paid_at = now() - interval '9 minutes' WHERE id = $1`, id)
	}
	d := guest.do("GET", fmt.Sprintf("/properties/%s?check_in=%s&check_out=%s", meskel, day(3), day(4)), nil)
	reply, _ := d.body["property"].(map[string]any)["reply_minutes"].(float64)
	if reply < 1 {
		t.Fatalf("a hotel that answers should show how fast: %v", d.body["property"])
	}
	for _, it := range itemsOf(guest.do("GET", searchPath("&q=meskel"), nil)) {
		if it["name"] == "Meskel Square Palace Hotel" && it["reply_minutes"] == nil {
			t.Fatalf("the result card shows the reply time too")
		}
	}
}

func TestHotelsCanWriteAmharicTextAndDirections(t *testing.T) {
	admin := adminClient(t)
	body := newHotelBody(fmt.Sprintf("Directions House %d", time.Now().UnixNano()%100000))
	body["amenities"] = []string{"wifi", "water_tank", "pool", "ac"} // 24-hour water is a feature a hotel can list
	body["description_am"] = "ንጹህ እና ሰላማዊ ማረፊያ"
	body["directions"] = "Behind the blue church, next to the pharmacy"
	body["directions_am"] = "ከሰማያዊው ቤተ ክርስቲያን ጀርባ፣ ከፋርማሲው አጠገብ"
	h := admin.do("POST", "/admin/hotels", body)
	if h.status != 200 {
		t.Fatalf("create: %d %s", h.status, h.raw)
	}
	d := admin.do("GET", "/admin/hotels/"+h.str("id"), nil)
	if d.str("description_am") != body["description_am"] || d.str("directions") != body["directions"] || d.str("directions_am") != body["directions_am"] {
		t.Fatalf("the texts should be saved: %s", d.raw)
	}
	body["directions"] = strings.Repeat("x", 601)
	if r := admin.do("PUT", "/admin/hotels/"+h.str("id"), body); r.status != 400 {
		t.Fatalf("directions over 600 characters: %d", r.status)
	}
	body["directions"] = "Second turn after the bridge"
	body["status"] = "draft"
	if r := admin.do("PUT", "/admin/hotels/"+h.str("id"), body); r.status != 200 {
		t.Fatalf("update: %d %s", r.status, r.raw)
	}
	if d := admin.do("GET", "/admin/hotels/"+h.str("id"), nil); d.str("directions") != "Second turn after the bridge" {
		t.Fatalf("the update is saved: %s", d.raw)
	}
	bad := newHotelBody("Bad Amenity")
	bad["amenities"] = []string{"helipad"}
	if r := admin.do("POST", "/admin/hotels", bad); r.status != 400 {
		t.Fatalf("an unknown feature: %d", r.status)
	}
}

func TestTheGuestIsEmailedARequestAConfirmationAndARefund(t *testing.T) {
	n := time.Now().UnixNano()
	email := realAddress("traveller")
	guest := newClient(t, "guest")
	signed := signInWithEmailedCode(guest, "/auth/otp/request", "/auth/otp/verify", email)
	if signed.status != 200 {
		t.Fatalf("sign in: %d %s", signed.status, signed.raw)
	}
	guest.token = signed.str("token")
	withProfile(guest)
	hotel := hotelFor(t)
	last := func(subject string) sentMail {
		t.Helper()
		for i := len(mails.to(email)) - 1; i >= 0; i-- {
			if m := mails.to(email)[i]; strings.Contains(m.subject, subject) {
				return m
			}
		}
		t.Fatalf("no email with %q; got %+v", subject, mails.to(email))
		return sentMail{}
	}

	// Paying sends "we sent your request"; the hotel accepting sends the confirmation, which is the receipt.
	q := quoteWith(guest, meskel, standard, 330, 332, 1, 1, nil)
	b := hold(guest, q.str("id"), fmt.Sprintf("mail-1-%d", n))
	payOnline(t, guest, b.str("id"), "succeeded")
	req := last("Your booking request")
	if !strings.Contains(req.text, "within 15 minutes") || !strings.Contains(req.text, "Reference: BD-") {
		t.Fatalf("the request email: %s", req.text)
	}
	hotel.do("POST", "/partner/bookings/"+b.str("id")+"/accept", nil)
	conf := last("Booking confirmed")
	for _, want := range []string{"Meskel Square Palace Hotel", "Total: ETB", "VAT", "Check-in:", "Check-out:", "Free cancellation until 48 hours", "Standard Double", "QR code"} {
		if !strings.Contains(conf.text, want) {
			t.Fatalf("the confirmation/receipt should say %q:\n%s", want, conf.text)
		}
	}

	// Cancelling sends the refund.
	if c := guest.do("POST", "/bookings/"+b.str("id")+"/cancel", nil); c.status != 200 {
		t.Fatalf("cancel: %d %s", c.status, c.raw)
	}
	if m := last("Booking cancelled"); !strings.Contains(m.text, "is on its way back to you") {
		t.Fatalf("the cancellation email: %s", m.text)
	}

	// The hotel declining sends the refund too.
	b2 := hold(guest, quoteWith(guest, meskel, standard, 334, 335, 1, 1, nil).str("id"), fmt.Sprintf("mail-2-%d", n))
	payOnline(t, guest, b2.str("id"), "succeeded")
	hotel.do("POST", "/partner/bookings/"+b2.str("id")+"/decline", map[string]string{"reason": "fully_booked"})
	if m := last("Booking not available"); !strings.Contains(m.text, "full refund") {
		t.Fatalf("the declined email: %s", m.text)
	}

	// A guest who signed in with a phone number has no address: nothing is sent, and nothing breaks.
	phone := newClient(t, "guest")
	phone.signIn("phone", "+251911000"+fmt.Sprintf("%03d", n%1000))
	withProfile(phone)
	before := len(mails.sent)
	pb := hold(phone, quoteWith(phone, meskel, standard, 336, 337, 1, 1, nil).str("id"), fmt.Sprintf("mail-3-%d", n))
	payOnline(t, phone, pb.str("id"), "succeeded")
	hotel.do("POST", "/partner/bookings/"+pb.str("id")+"/accept", nil)
	if len(mails.sent) != before {
		t.Fatalf("a phone-only guest gets no emails")
	}
}
