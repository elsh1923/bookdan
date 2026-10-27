package api_test

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestAHotelOwnerAnswersAReviewAndTheGuestAndTheHotelPageShowIt(t *testing.T) {
	n := time.Now().UnixNano()
	g, _ := supportGuest(t, "rrg")
	hotel := hotelFor(t)
	admin := adminClient(t)
	desk := receptionist(t, admin, "rrdesk")
	id := completedStay(t, g, hotel, 92, 2, fmt.Sprintf("rr-%d", n))
	if r := g.do("POST", "/bookings/"+id+"/review", map[string]any{"rating": 2, "text": "The room was noisy and the Wi-Fi did not work."}); r.status != 200 {
		t.Fatalf("review: %d %s", r.status, r.raw)
	}

	list := hotel.do("GET", "/partner/reviews", nil)
	var rid string
	for _, x := range list.body["reviews"].([]any) {
		if m := x.(map[string]any); m["booking_code"] != nil && strings.Contains(m["text"].(string), "noisy") {
			rid = m["id"].(string)
			if m["reply"] != nil {
				t.Errorf("no reply yet: %v", m)
			}
		}
	}
	if rid == "" {
		t.Fatalf("the owner sees the new review: %s", list.raw)
	}
	if r := desk.do("GET", "/partner/reviews", nil); r.status != 403 {
		t.Errorf("a receptionist does not speak for the hotel: %d", r.status)
	}
	if r := desk.do("PUT", "/partner/reviews/"+rid+"/reply", map[string]string{"text": "We are sorry about this."}); r.status != 403 {
		t.Errorf("a receptionist cannot answer: %d", r.status)
	}
	for name, r := range map[string]resp{
		"too short": hotel.do("PUT", "/partner/reviews/"+rid+"/reply", map[string]string{"text": "Hi"}),
		"too long":  hotel.do("PUT", "/partner/reviews/"+rid+"/reply", map[string]string{"text": strings.Repeat("a", 1001)}),
		"unknown":   hotel.do("PUT", "/partner/reviews/00000000-0000-0000-0000-000000000000/reply", map[string]string{"text": "Thank you for staying"}),
		"garbled":   hotel.do("PUT", "/partner/reviews/not-a-uuid/reply", map[string]string{"text": "Thank you for staying"}),
	} {
		if r.status == 200 || r.status >= 500 {
			t.Errorf("%s: %d %s", name, r.status, r.raw)
		}
	}
	if r := hotel.do("PUT", "/partner/reviews/"+rid+"/reply", map[string]string{"text": "We are sorry. The Wi-Fi has been repaired and the room is now on a quieter floor."}); r.status != 200 {
		t.Fatalf("reply: %d %s", r.status, r.raw)
	}
	// Changing the reply replaces it.
	if r := hotel.do("PUT", "/partner/reviews/"+rid+"/reply", map[string]string{"text": "We are sorry. The Wi-Fi was repaired on Tuesday."}); r.status != 200 {
		t.Fatalf("change: %d", r.status)
	}

	prop := g.do("GET", "/properties/"+meskel+"?check_in="+day(200)+"&check_out="+day(201), nil)
	var shown bool
	for _, x := range prop.body["reviews"].([]any) {
		if m := x.(map[string]any); m["id"] == rid {
			rep, _ := m["reply"].(map[string]any)
			shown = rep != nil && strings.Contains(rep["text"].(string), "repaired on Tuesday")
		}
	}
	if !shown {
		t.Errorf("the hotel page shows the reply under the review: %s", prop.raw)
	}
	b := g.do("GET", "/bookings/"+id, nil)
	if rv, _ := b.body["review"].(map[string]any); rv == nil || rv["hotel_reply"] == nil {
		t.Errorf("the guest sees the reply on their own booking: %s", b.raw)
	}
	if mine := hotel.do("GET", "/partner/reviews", nil); !strings.Contains(string(mine.raw), "repaired on Tuesday") {
		t.Errorf("the owner sees their reply")
	}

	if r := hotel.do("DELETE", "/partner/reviews/"+rid+"/reply", nil); r.status != 200 {
		t.Fatalf("remove: %d", r.status)
	}
	if after := g.do("GET", "/properties/"+meskel+"?check_in="+day(200)+"&check_out="+day(201), nil); strings.Contains(string(after.raw), "repaired on Tuesday") {
		t.Errorf("a removed reply is gone")
	}
	if r := newClient(t, "partner").do("GET", "/partner/reviews", nil); r.status != 401 {
		t.Errorf("signed out: %d", r.status)
	}
}
