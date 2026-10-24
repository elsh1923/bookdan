package api_test

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"
)

func searchPath(extra string) string {
	return fmt.Sprintf("/search?check_in=%s&check_out=%s&guests=1%s", day(3), day(4), extra)
}

func itemsOf(r resp) []map[string]any {
	var out []map[string]any
	for _, it := range r.body["items"].([]any) {
		out = append(out, it.(map[string]any))
	}
	return out
}

func TestSearchSortsFiltersAndCounts(t *testing.T) {
	c := newClient(t, "guest")
	all := c.do("GET", searchPath(""), nil)
	if all.status != 200 {
		t.Fatalf("search: %d %s", all.status, all.raw)
	}
	total := int(all.num("total"))
	if total != len(itemsOf(all)) && total < 8 {
		t.Fatalf("total should count every match: %d vs %d items", total, len(itemsOf(all)))
	}

	// Cheapest first, dearest first: prices are in order among the hotels that have a room.
	for sortName, asc := range map[string]bool{"price_asc": true, "price_desc": false} {
		r := c.do("GET", searchPath("&sort="+sortName), nil)
		var prices []int
		for _, it := range itemsOf(r) {
			if it["available"] == true {
				prices = append(prices, int(it["from_price_santim"].(float64)))
			}
		}
		if len(prices) < 3 || sort.SliceIsSorted(prices, func(i, j int) bool {
			if asc {
				return prices[i] < prices[j]
			}
			return prices[i] > prices[j]
		}) == false {
			t.Fatalf("%s: prices are not in order: %v", sortName, prices)
		}
	}
	// Whatever the sort, a hotel with no room for the party comes last.
	r := c.do("GET", fmt.Sprintf("/search?check_in=%s&check_out=%s&guests=3&rooms=1&sort=price_asc", day(3), day(4)), nil)
	seenUnavailable := false
	for _, it := range itemsOf(r) {
		if it["available"] != true {
			seenUnavailable = true
		} else if seenUnavailable {
			t.Fatalf("an available hotel is listed after an unavailable one")
		}
	}

	// A price filter keeps only hotels inside the range; price bounds and facets describe the whole result.
	mid := int64(200000)
	r = c.do("GET", searchPath(fmt.Sprintf("&max_price=%d", mid)), nil)
	if len(itemsOf(r)) == 0 {
		t.Fatalf("expected some hotels under the limit")
	}
	for _, it := range itemsOf(r) {
		if int64(it["from_price_santim"].(float64)) > mid {
			t.Fatalf("max_price let %v through", it["name"])
		}
	}
	if int(r.num("total")) >= total {
		t.Fatalf("filtering by price should reduce the total (%v vs %d)", r.num("total"), total)
	}
	if r.num("price_min_santim") <= 0 || r.num("price_max_santim") < r.num("price_min_santim") {
		t.Fatalf("price bounds: %v..%v", r.num("price_min_santim"), r.num("price_max_santim"))
	}
	facets := r.body["facets"].(map[string]any)
	if facets["types"].(map[string]any)["hotel"] == nil || facets["amenities"].(map[string]any)["wifi"] == nil {
		t.Fatalf("facets should count types and amenities: %v", facets)
	}
	if r := c.do("GET", searchPath(fmt.Sprintf("&min_price=%d", mid)), nil); len(itemsOf(r)) == 0 {
		t.Fatalf("min_price should keep the dearer hotels")
	} else {
		for _, it := range itemsOf(r) {
			if int64(it["from_price_santim"].(float64)) < mid {
				t.Fatalf("min_price let %v through", it["name"])
			}
		}
	}

	// Rating filter: only hotels rated at least that. Nobody has 5 stars here, so nothing passes a very high bar.
	if r := c.do("GET", searchPath("&min_rating=4.9"), nil); len(itemsOf(r)) != 0 || r.num("total") != 0 {
		t.Fatalf("no hotel has a 4.9 rating: %s", r.raw)
	}

	// Paging walks through the sorted list without repeating or losing hotels.
	seen := map[string]bool{}
	cursor := ""
	for page := 0; page < 20; page++ {
		path := searchPath("&sort=price_asc")
		if cursor != "" {
			path += "&cursor=" + cursor
		}
		pr := c.do("GET", path, nil)
		for _, it := range itemsOf(pr) {
			if seen[it["id"].(string)] {
				t.Fatalf("paging repeated %v", it["name"])
			}
			seen[it["id"].(string)] = true
		}
		next, _ := pr.body["next_cursor"].(string)
		if next == "" {
			break
		}
		cursor = next
	}
	if len(seen) != total {
		t.Fatalf("paging saw %d hotels, expected %d", len(seen), total)
	}

	// Distance: sorted nearest first from the point given, and every card says how far it is.
	r = c.do("GET", searchPath("&sort=distance&lat=9.0108&lng=38.7613"), nil)
	prev := -1
	for _, it := range itemsOf(r) {
		if it["available"] != true {
			break
		}
		d := int(it["distance_m"].(float64))
		if d < prev {
			t.Fatalf("distance sort is out of order: %d after %d", d, prev)
		}
		prev = d
	}
	if first := itemsOf(r)[0]; first["name"] != "Meskel Square Palace Hotel" {
		t.Fatalf("the hotel on Meskel Square should be nearest to Meskel Square: %v", first["name"])
	}
	// Each card names the closest landmark.
	for _, it := range itemsOf(c.do("GET", searchPath("&q=meskel"), nil)) {
		lm, _ := it["landmark"].(map[string]any)
		if it["name"] == "Meskel Square Palace Hotel" && (lm == nil || lm["name"] != "Meskel Square") {
			t.Fatalf("the card should say Meskel Square is the closest landmark: %v", it["landmark"])
		}
	}
	for _, bad := range []string{"&sort=cheapest", "&sort=distance", "&min_price=abc", "&min_rating=9", "&lat=95&lng=10"} {
		if r := c.do("GET", searchPath(bad), nil); r.status != 400 {
			t.Errorf("%s should be refused, got %d", bad, r.status)
		}
	}
}

func TestRoomsLeftShowsOnTheCardWhenTheyRunLow(t *testing.T) {
	c := newClient(t, "guest")
	// The Family Suite at Meskel has a single room; the cheapest room that fits is the Standard Double (5 rooms).
	r := c.do("GET", searchPath("&q=meskel"), nil)
	for _, it := range itemsOf(r) {
		if it["name"] == "Meskel Square Palace Hotel" {
			if left := it["rooms_left"].(float64); left < 1 || left > 5 {
				t.Fatalf("rooms_left should describe the cheapest room that fits: %v", left)
			}
			return
		}
	}
	t.Fatalf("Meskel Square Palace Hotel not found")
}

func TestHotelPageShowsRatingSummaryNearbyPlacesAndTimes(t *testing.T) {
	ctx := context.Background()
	n := time.Now().UnixNano()
	guest := newClient(t, "guest")
	guest.signIn("email", uniqueEmail("sum"))
	withProfile(guest)
	hotel := hotelFor(t)
	id := paid(t, guest, meskel, standard, 350, 351, fmt.Sprintf("sum-%d", n))
	hotel.do("POST", "/partner/bookings/"+id+"/accept", nil)
	pool.Exec(ctx, `UPDATE bookings SET check_in = `+addisToday+` - 3, check_out = `+addisToday+` - 2 WHERE id = $1`, id)

	for _, bad := range []map[string]any{{"rating": 4, "cleanliness": 6}, {"rating": 4, "staff": -1}} {
		if r := guest.do("POST", "/bookings/"+id+"/review", bad); r.status != 400 {
			t.Fatalf("a sub-rating out of range must be refused: %v -> %d", bad, r.status)
		}
	}
	if r := guest.do("POST", "/bookings/"+id+"/review", map[string]any{"rating": 5, "text": "Great", "cleanliness": 5, "location": 4, "staff": 5}); r.status != 200 {
		t.Fatalf("review with sub-ratings: %d %s", r.status, r.raw)
	}

	d := guest.do("GET", fmt.Sprintf("/properties/%s?check_in=%s&check_out=%s", meskel, day(3), day(4)), nil)
	sum := d.body["rating_summary"].(map[string]any)
	dist := sum["distribution"].([]any)
	if len(dist) != 5 || sum["count"].(float64) < 1 || dist[4].(float64) < 1 || sum["average"] == nil {
		t.Fatalf("the summary should count the review under five stars: %v", sum)
	}
	cats := sum["categories"].(map[string]any)
	if cats["cleanliness"] == nil || cats["location"] == nil || cats["staff"] == nil || cats["value"] != nil {
		t.Fatalf("sub-ratings are averaged where given and empty where nobody gave one: %v", cats)
	}
	if first := d.body["reviews"].([]any)[0].(map[string]any); first["verified"] != true {
		t.Fatalf("reviews come from finished stays and are marked verified: %v", first)
	}

	near := d.body["nearby"].([]any)
	if len(near) == 0 || near[0].(map[string]any)["name"] != "Meskel Square" {
		t.Fatalf("Meskel Square should be the nearest place to the hotel: %v", near)
	}
	prop := d.body["property"].(map[string]any)
	if !strings.Contains(prop["check_in_time"].(string), ":") || !strings.Contains(prop["check_out_time"].(string), ":") {
		t.Fatalf("the hotel page should carry check-in and check-out times: %v", prop)
	}

	// The booking carries the times and its history.
	b := guest.do("GET", "/bookings/"+id, nil)
	tl, _ := b.body["timeline"].([]any)
	var steps []string
	for _, e := range tl {
		steps = append(steps, e.(map[string]any)["status"].(string))
	}
	if len(steps) < 3 || steps[0] != "HELD" || steps[len(steps)-1] != "CONFIRMED" || b.str("check_in_time") == "" {
		t.Fatalf("the booking timeline should list the statuses it went through: %v / %s", steps, b.raw)
	}
}

func TestAdminSetsCheckInTimes(t *testing.T) {
	admin := adminClient(t)
	body := newHotelBody(fmt.Sprintf("Times Hotel %d", time.Now().UnixNano()%100000))
	body["check_in_time"], body["check_out_time"] = "13:30", "11:00"
	h := admin.do("POST", "/admin/hotels", body)
	if h.status != 200 {
		t.Fatalf("create: %d %s", h.status, h.raw)
	}
	d := admin.do("GET", "/admin/hotels/"+h.str("id"), nil)
	if d.str("check_in_time") != "13:30" || d.str("check_out_time") != "11:00" {
		t.Fatalf("times should be saved: %s", d.raw)
	}
	// Leaving them empty keeps them; a bad value is refused.
	body["check_in_time"], body["check_out_time"] = "", ""
	if r := admin.do("PUT", "/admin/hotels/"+h.str("id"), body); r.status != 200 {
		t.Fatalf("update: %d %s", r.status, r.raw)
	}
	if d := admin.do("GET", "/admin/hotels/"+h.str("id"), nil); d.str("check_in_time") != "13:30" {
		t.Fatalf("empty keeps the current times: %s", d.raw)
	}
	body["check_in_time"] = "25:00"
	if r := admin.do("PUT", "/admin/hotels/"+h.str("id"), body); r.status != 400 {
		t.Fatalf("a bad time: %d", r.status)
	}
	// A new hotel without times gets the usual ones.
	plain := admin.do("POST", "/admin/hotels", newHotelBody(fmt.Sprintf("Plain Hotel %d", time.Now().UnixNano()%100000)))
	if d := admin.do("GET", "/admin/hotels/"+plain.str("id"), nil); d.str("check_in_time") != "14:00" || d.str("check_out_time") != "12:00" {
		t.Fatalf("defaults: %s", d.raw)
	}
}
