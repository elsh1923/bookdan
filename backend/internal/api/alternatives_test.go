package api

import (
	"testing"
	"time"
)

// base is the date of day 0 in these tests. The requested stay starts at day 2 (altShiftDays), as in a real search.
var testBase = time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)

// room builds one room type: free[i] rooms free on day i, all days with a rate of price, unless sellable[i] is false.
func room(capacity int, price int64, free []int, sellable ...bool) roomStock {
	days := make([]dayStock, len(free))
	for i, f := range free {
		ok := true
		if len(sellable) > i {
			ok = sellable[i]
		}
		days[i] = dayStock{free: f, price: price, cur: "ETB", ok: ok}
	}
	return roomStock{capacity: capacity, days: days}
}

func hotel(rooms ...roomStock) hotelStock { return hotelStock{base: testBase, rooms: rooms} }

func dates(ws []altWindow) []string {
	var out []string
	for _, w := range ws {
		out = append(out, w.CheckIn+"→"+w.CheckOut)
	}
	return out
}

func TestWholeStayAlternativesAreOnlyForTheNightsTheHotelCanSell(t *testing.T) {
	// Requested: 3 nights, days 2, 3 and 4. The room is free only on days 2 and 3.
	h := hotel(room(2, 1400, []int{0, 0, 1, 1, 0, 0, 0}))
	got := findWindows(h, 2, 3, 0, 2, 1)
	if len(got) != 1 || got[0].CheckIn != "2026-11-12" || got[0].CheckOut != "2026-11-14" || got[0].Nights != 2 || got[0].FromPrice != 1400 {
		t.Fatalf("a shorter stay the hotel can sell: %+v", got)
	}
}

func TestTheSameNumberOfNightsADayOrTwoAwayIsOfferedWhenNothingInsideWorks(t *testing.T) {
	// Requested: 2 nights, days 2 and 3, both full. Days 0, 1, 4, 5 and 6 are free.
	h := hotel(room(2, 900, []int{1, 1, 0, 0, 1, 1, 1}))
	got := findWindows(h, 2, 2, 0, 2, 1)
	want := []string{"2026-11-10→2026-11-12", "2026-11-14→2026-11-16"}
	if g := dates(got); len(g) != 2 || g[0] != want[0] || g[1] != want[1] {
		t.Fatalf("nearby dates: got %v want %v", g, want)
	}
}

func TestANightBeforeTodayIsNeverOffered(t *testing.T) {
	h := hotel(room(2, 900, []int{1, 1, 0, 0, 1, 1, 1}))
	got := findWindows(h, 2, 2, 1, 2, 1) // today is day 1: day 0 has passed
	if g := dates(got); len(g) != 1 || g[0] != "2026-11-14→2026-11-16" {
		t.Fatalf("nothing in the past: %v", g)
	}
	// Today is after every night we look at: nothing at all.
	if got := findWindows(h, 2, 2, 9, 2, 1); len(got) != 0 {
		t.Fatalf("every night is in the past: %+v", got)
	}
}

func TestARoomThePartyCannotFitIntoIsNotOffered(t *testing.T) {
	h := hotel(room(1, 700, []int{1, 1, 1, 1, 1, 1, 1})) // a single room, two guests
	if got := findWindows(h, 2, 2, 0, 2, 1); len(got) != 0 {
		t.Fatalf("a room for one cannot take two: %+v", got)
	}
}

func TestANightWithoutARateCannotBeSold(t *testing.T) {
	// Free every night, but the rate stops after day 3.
	h := hotel(room(2, 1000, []int{1, 1, 1, 1, 1, 1, 1}, true, true, true, true, false, false, false))
	got := findWindows(h, 2, 3, 0, 2, 1) // requested days 2, 3 and 4: day 4 has no rate
	for _, w := range got {
		if w.CheckOut > "2026-11-14" { // nights are days 0 to 3 at most (check-out on day 4 or earlier)
			t.Fatalf("a stay that reaches a night without a rate: %+v", w)
		}
	}
	if len(got) == 0 {
		t.Fatalf("the sellable nights should still be offered")
	}
}

func TestEnoughRoomsMustBeFreeForTheWholeParty(t *testing.T) {
	h := hotel(room(2, 1000, []int{1, 1, 1, 1, 1, 1, 1})) // one room free each night, the guest wants two rooms
	if got := findWindows(h, 2, 2, 0, 4, 2); len(got) != 0 {
		t.Fatalf("two rooms needed, one free: %+v", got)
	}
}

func TestTheCheapestRoomThatFitsGivesThePrice(t *testing.T) {
	h := hotel(
		room(2, 2000, []int{1, 1, 0, 1, 1, 1, 1}),
		room(4, 3000, []int{1, 1, 1, 1, 1, 1, 1}),
	)
	// Three guests: the room for two is too small, so the price is the family room's, even where the small room is free.
	got := findWindows(h, 2, 1, 0, 3, 1)
	if len(got) == 0 {
		t.Fatalf("expected suggestions")
	}
	for _, w := range got {
		if w.FromPrice != 3000 {
			t.Fatalf("only the room that fits sets the price: %+v", w)
		}
	}
}

func TestSuggestionsAreFewAndNeverInsideAnotherSuggestion(t *testing.T) {
	h := hotel(room(2, 1000, []int{1, 1, 1, 1, 1, 1, 1})) // free everywhere (not a real case: the hotel would be available)
	got := findWindows(h, 2, 3, 0, 2, 1)
	if len(got) > altPerHotel {
		t.Fatalf("at most %d suggestions: %d", altPerHotel, len(got))
	}
	for _, w := range got {
		if w.CheckIn == "2026-11-12" && w.CheckOut == "2026-11-15" {
			t.Fatalf("the requested stay itself is never a suggestion: %+v", w)
		}
	}
	for i, a := range got {
		for j, b := range got {
			if i != j && b.CheckIn <= a.CheckIn && a.CheckOut <= b.CheckOut {
				t.Fatalf("%+v is inside %+v", a, b)
			}
		}
	}
}

func TestAOneNightStayOnlyLooksAtNearbyNights(t *testing.T) {
	h := hotel(room(2, 1000, []int{1, 0, 0, 1, 0, 0, 0})) // requested day 2 is full; days 1 and 4 are not free, day 3 and 0 are
	got := findWindows(h, 2, 1, 0, 2, 1)
	if g := dates(got); len(g) != 2 || g[0] != "2026-11-13→2026-11-14" || g[1] != "2026-11-10→2026-11-11" {
		t.Fatalf("day 3 (one away) comes before day 0 (two away): %v", g)
	}
}

// Searches are limited to 365 days ahead, so a leap day cannot be reached by a real search any more. The night count itself is what matters:
// check-out is exclusive, a leap day counts as a night, and a year end is nothing special.
func TestNightCountsAcrossLeapDaysAndYearEnds(t *testing.T) {
	for _, c := range []struct {
		in, out string
		nights  int
	}{
		{"2028-02-27", "2028-03-02", 4}, // 27, 28, 29 February and 1 March
		{"2027-02-27", "2027-03-02", 3}, // no leap day
		{"2026-12-30", "2027-01-02", 3}, // across the new year
		{"2026-03-28", "2026-03-30", 2}, // no daylight-saving change applies: Addis Ababa is a fixed UTC+3
		{"2026-11-12", "2026-11-13", 1},
	} {
		if got := nightsBetween(c.in, c.out); got != c.nights {
			t.Errorf("%s to %s is %d nights, got %d", c.in, c.out, c.nights, got)
		}
	}
}
