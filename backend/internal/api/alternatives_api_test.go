package api_test

import (
	"fmt"
	"testing"
	"time"
)

// otherDatesFor returns the windows the search offers for one hotel in its other_dates list (nil when the hotel is not listed).
func otherDatesFor(t *testing.T, r resp, hotelID string) []map[string]any {
	t.Helper()
	list, _ := r.body["other_dates"].([]any)
	for _, it := range list {
		m, _ := it.(map[string]any)
		if m["id"] != hotelID {
			continue
		}
		var out []map[string]any
		for _, w := range m["windows"].([]any) {
			out = append(out, w.(map[string]any))
		}
		return out
	}
	return nil
}

func TestSearchOffersOtherDatesForAHotelThatIsFullOnSomeNights(t *testing.T) {
	n := time.Now().UnixNano()
	guest := newClient(t, "guest")
	guest.signIn("email", uniqueEmail("alt"))
	withProfile(guest)

	// Take every room the Meskel hotel has on one night: 5 Standard Doubles, 2 Deluxe Kings and the Family Suite.
	night := 356 // a date no other test uses; the seed rates cover 365 days
	if b := hold(guest, quote(guest, meskel, standard, night, night+1, 5, 5).str("id"), fmt.Sprintf("alt-std-%d", n)); b.status != 200 {
		t.Fatalf("hold the standard rooms: %d %s", b.status, b.raw)
	}
	if b := hold(guest, quote(guest, meskel, family, night, night+1, 1, 1).str("id"), fmt.Sprintf("alt-fam-%d", n)); b.status != 200 {
		t.Fatalf("hold the family suite: %d %s", b.status, b.raw)
	}

	if b := hold(guest, quote(guest, meskel, "b0000000-0000-0000-0000-000000000002", night, night+1, 2, 2).str("id"), fmt.Sprintf("alt-dlx-%d", n)); b.status != 200 {
		t.Fatalf("hold the deluxe king rooms: %d %s", b.status, b.raw)
	}

	// The guest asks for two nights starting on the full night.
	r := guest.do("GET", fmt.Sprintf("/search?q=meskel&check_in=%s&check_out=%s&guests=2&rooms=1", day(night), day(night+2)), nil)
	if r.status != 200 {
		t.Fatalf("search: %d %s", r.status, r.raw)
	}

	// The main list still has the hotel, marked as not available (as before).
	items, _ := r.body["items"].([]any)
	found := false
	for _, it := range items {
		m := it.(map[string]any)
		if m["id"] == meskel {
			found = true
			if m["available"] != false {
				t.Fatalf("the hotel is full on the first night, so it is not available for the whole stay: %v", m["available"])
			}
		}
	}
	if !found {
		t.Fatalf("the hotel stays in the main list: %s", r.raw)
	}

	// The other-dates list offers stays the hotel can sell, and none of them uses the full night.
	windows := otherDatesFor(t, r, meskel)
	if len(windows) == 0 {
		t.Fatalf("expected other dates for the hotel: %s", r.raw)
	}
	offeredAfter := false
	for _, w := range windows {
		in, out := w["check_in"].(string), w["check_out"].(string)
		if in <= day(night) && day(night) < out {
			t.Fatalf("a suggestion includes the full night: %v", w)
		}
		if in == day(night+1) {
			offeredAfter = true
		}
		if w["from_price_santim"].(float64) <= 0 || w["nights"].(float64) < 1 || len(windows) > 3 {
			t.Fatalf("a suggestion needs a price, at least one night, and at most 3 per hotel: %v", w)
		}
	}
	if !offeredAfter {
		t.Fatalf("the night after the full one is free, so a stay from there is offered: %v", windows)
	}

	// A suggestion can really be booked: the quote for it is accepted.
	var pick map[string]any
	for _, w := range windows {
		if w["check_in"] == day(night+1) {
			pick = w
		}
	}
	nights := int(pick["nights"].(float64))
	if q := quote(guest, meskel, standard, night+1, night+1+nights, 2, 1); q.status != 200 {
		t.Fatalf("the quote for a suggested stay must work: %d %s", q.status, q.raw)
	}

	// A date with rooms free for the whole stay has no other-dates entry.
	free := guest.do("GET", fmt.Sprintf("/search?q=meskel&check_in=%s&check_out=%s&guests=2&rooms=1", day(361), day(363)), nil)
	if w := otherDatesFor(t, free, meskel); w != nil {
		t.Fatalf("a hotel that is free needs no other dates: %v", w)
	}

	// Only the first page carries the list, so a later page does not repeat it.
	if r2 := guest.do("GET", fmt.Sprintf("/search?q=meskel&check_in=%s&check_out=%s&guests=2&rooms=1&cursor=12", day(night), day(night+2)), nil); len(r2.body["other_dates"].([]any)) != 0 {
		t.Fatalf("later pages do not repeat the other dates: %s", r2.raw)
	}
}
