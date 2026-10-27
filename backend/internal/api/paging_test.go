package api_test

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func idsOf(r resp) []string {
	var out []string
	for _, x := range r.body["items"].([]any) {
		out = append(out, x.(map[string]any)["id"].(string))
	}
	return out
}

func TestAGuestsBookingsComeInPagesNewestFirstWithNothingSkippedOrRepeated(t *testing.T) {
	n := time.Now().UnixNano()
	g, _ := supportGuest(t, "pg")
	var made []string
	for i := 0; i < 5; i++ {
		made = append([]string{hold(g, quote(g, meskel, standard, 140+i, 141+i, 1, 1).str("id"), fmt.Sprintf("pg-%d-%d", i, n)).str("id")}, made...)
	}

	all := g.do("GET", "/bookings", nil)
	if all.status != 200 || strings.Join(idsOf(all), ",") != strings.Join(made, ",") || all.body["next_cursor"] != nil {
		t.Fatalf("without parameters the list is as before, newest first, and has no next page: %s", all.raw)
	}

	var got []string
	cursor, pages := "", 0
	for {
		path := "/bookings?limit=2"
		if cursor != "" {
			path += "&cursor=" + cursor
		}
		page := g.do("GET", path, nil)
		if page.status != 200 {
			t.Fatalf("page: %d %s", page.status, page.raw)
		}
		ids := idsOf(page)
		if len(ids) > 2 {
			t.Fatalf("a page is at most the limit: %d", len(ids))
		}
		got = append(got, ids...)
		pages++
		next, _ := page.body["next_cursor"].(string)
		if next == "" {
			break
		}
		cursor = next
		if pages > 10 {
			t.Fatalf("paging does not end")
		}
	}
	if pages != 3 || strings.Join(got, ",") != strings.Join(made, ",") {
		t.Errorf("five bookings in pages of two: %d pages, got %v want %v", pages, got, made)
	}
	// A new booking arriving between pages does not make an old one repeat or vanish (the cursor is a position, not an offset).
	first := g.do("GET", "/bookings?limit=2", nil)
	hold(g, quote(g, meskel, standard, 150, 151, 1, 1).str("id"), fmt.Sprintf("pg-new-%d", n))
	second := g.do("GET", "/bookings?limit=2&cursor="+first.body["next_cursor"].(string), nil)
	for _, id := range idsOf(second) {
		for _, seen := range idsOf(first) {
			if id == seen {
				t.Errorf("a booking repeated across pages: %s", id)
			}
		}
	}

	for name, path := range map[string]string{
		"zero limit":     "/bookings?limit=0",
		"huge limit":     "/bookings?limit=101",
		"text limit":     "/bookings?limit=many",
		"garbled cursor": "/bookings?cursor=not-a-cursor",
		"forged cursor":  "/bookings?cursor=" + "Zm9yZ2Vk",
	} {
		if r := g.do("GET", path, nil); r.status != 400 {
			t.Errorf("%s: %d", name, r.status)
		}
	}
	// A guest never sees another guest's bookings through a cursor.
	other, _ := supportGuest(t, "pg2")
	if r := other.do("GET", "/bookings?limit=2&cursor="+first.body["next_cursor"].(string), nil); r.status != 200 || len(idsOf(r)) != 0 {
		t.Errorf("another guest's list is theirs: %s", r.raw)
	}
}

func TestTheHotelsBookingsCanBePagedToo(t *testing.T) {
	n := time.Now().UnixNano()
	g, _ := supportGuest(t, "pgh")
	hotel := hotelFor(t)
	for i := 0; i < 3; i++ {
		id := hold(g, quote(g, meskel, standard, 155+i, 156+i, 1, 1).str("id"), fmt.Sprintf("pgh-%d-%d", i, n)).str("id")
		payOnline(t, g, id, "succeeded")
	}
	all := hotel.do("GET", "/partner/bookings", nil)
	if all.status != 200 {
		t.Fatalf("list: %d", all.status)
	}
	total := len(idsOf(all))
	if total < 3 {
		t.Fatalf("the hotel has bookings: %d", total)
	}
	var got []string
	cursor := ""
	for i := 0; i < 50; i++ {
		path := "/partner/bookings?limit=2"
		if cursor != "" {
			path += "&cursor=" + cursor
		}
		page := hotel.do("GET", path, nil)
		if page.status != 200 || len(idsOf(page)) > 2 {
			t.Fatalf("page: %d %s", page.status, page.raw)
		}
		got = append(got, idsOf(page)...)
		next, _ := page.body["next_cursor"].(string)
		if next == "" {
			break
		}
		cursor = next
	}
	seen := map[string]bool{}
	for _, id := range got {
		if seen[id] {
			t.Errorf("repeated: %s", id)
		}
		seen[id] = true
	}
	if len(got) < total-5 { // bookings arriving meanwhile can only add
		t.Errorf("paging reaches every booking: %d of %d", len(got), total)
	}
	if r := hotel.do("GET", "/partner/bookings?limit=201", nil); r.status != 400 {
		t.Errorf("limit above the cap: %d", r.status)
	}
	if r := hotel.do("GET", "/partner/bookings?cursor=bogus", nil); r.status != 400 {
		t.Errorf("bad cursor: %d", r.status)
	}
}
