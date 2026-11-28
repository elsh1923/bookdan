package api_test

import (
	"context"
	"slices"
	"testing"
)

// The "Pay at hotel" label guests see follows the hotel's switch, whatever was typed into its features.
func TestPayAtHotelLabelFollowsTheSwitch(t *testing.T) {
	ctx := context.Background()
	hotel := meskel
	var amenities []string
	var allowed bool
	if err := pool.QueryRow(ctx, `SELECT amenities, pay_at_hotel FROM properties WHERE id = $1`, hotel).Scan(&amenities, &allowed); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE properties SET amenities = $2, pay_at_hotel = $3 WHERE id = $1`, hotel, amenities, allowed)
	})
	guest := newClient(t, "guest")
	check := func(tag, on bool) {
		t.Helper()
		stored := []string{"wifi"}
		if tag {
			stored = append(stored, "pay_at_hotel")
		}
		if _, err := pool.Exec(ctx, `UPDATE properties SET amenities = $2, pay_at_hotel = $3 WHERE id = $1`, hotel, stored, on); err != nil {
			t.Fatal(err)
		}
		page := guest.do("GET", "/properties/"+hotel+"?check_in="+day(3)+"&check_out="+day(4), nil)
		if page.status != 200 {
			t.Fatalf("hotel page: %d %s", page.status, page.raw)
		}
		var inList, found bool
		for _, it := range itemsOf(guest.do("GET", searchPath("&amenities=pay_at_hotel"), nil)) {
			inList = inList || it["id"] == hotel
		}
		prop, _ := page.body["property"].(map[string]any)
		list, _ := prop["amenities"].([]any)
		found = slices.Contains(list, any("pay_at_hotel"))
		if found != on || inList != on {
			t.Fatalf("tag typed=%v switch=%v: label on hotel page=%v, in filtered search=%v: %s", tag, on, found, inList, page.raw)
		}
	}
	check(false, true)  // switch on, nothing typed: the label appears
	check(true, false)  // label typed but switch off: it does not appear
	check(true, true)   // both: once
	check(false, false) // neither
}
