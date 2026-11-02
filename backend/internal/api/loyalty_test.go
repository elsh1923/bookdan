package api_test

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// finishStay moves a booking's dates into the past and runs the daily job, which marks the stay COMPLETED.
func finishStay(t *testing.T, id string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), fmt.Sprintf(`UPDATE bookings SET check_in = %s - 5, check_out = %s - 3 WHERE id = $1`, addisToday, addisToday), id); err != nil {
		t.Fatal(err)
	}
	app.RunMaintenance(context.Background())
}

func loyalty(c *client) resp { return c.do("GET", "/me/loyalty", nil) }

// TestMemberTiersCountFinishedStaysAtDifferentHotels: a guest moves up by the number of different hotels they have finished a stay at.
func TestMemberTiersCountFinishedStaysAtDifferentHotels(t *testing.T) {
	n := time.Now().UnixNano()
	admin := adminClient(t)
	g := newClient(t, "guest")
	g.signIn("email", uniqueEmail("loyal"))
	withProfile(g)
	hotel := hotelFor(t)

	// A new guest has no tier, and the first one (bronze) is one finished stay away.
	r := loyalty(g)
	if r.status != 200 || r.body["tier"] != nil || r.num("hotels") != 0 || r.str("next_tier") != "bronze" || r.num("hotels_to_next") != 1 {
		t.Fatalf("a new guest: %d %s", r.status, r.raw)
	}

	// One finished stay makes them bronze, with the bronze discount.
	id, _, _, _ := confirmedBooking(t, g, hotel, 300, 301, fmt.Sprintf("loy-1-%d", n))
	finishStay(t, id)
	r = loyalty(g)
	if r.str("tier") != "bronze" || r.num("hotels") != 1 || r.num("discount_bps") != 300 || r.str("next_tier") != "silver" || r.num("hotels_to_next") != 2 {
		t.Fatalf("after one finished stay: %d %s", r.status, r.raw)
	}

	// Another stay at the SAME hotel does not count twice.
	id2, _, _, _ := confirmedBooking(t, g, hotel, 302, 303, fmt.Sprintf("loy-2-%d", n))
	finishStay(t, id2)
	if r = loyalty(g); r.num("hotels") != 1 {
		t.Fatalf("the same hotel counts once: %d %s", r.status, r.raw)
	}

	// A stay that was cancelled, declined or never happened does not count: a held booking is not a finished stay.
	hold(g, quote(g, meskel, standard, 304, 305, 1, 1).str("id"), fmt.Sprintf("loy-held-%d", n))
	if r = loyalty(g); r.num("hotels") != 1 {
		t.Fatalf("a held booking is not a finished stay: %s", r.raw)
	}

	// The thresholds come from the admin page. Silver at 2 hotels: a second hotel takes the guest up.
	defaults := []map[string]any{
		{"name": "bronze", "min_hotels": 1, "discount_bps": 300},
		{"name": "silver", "min_hotels": 3, "discount_bps": 500},
		{"name": "gold", "min_hotels": 6, "discount_bps": 800},
	}
	defer admin.do("PUT", "/admin/loyalty", map[string]any{"tiers": defaults})
	if r := admin.do("PUT", "/admin/loyalty", map[string]any{"tiers": []map[string]any{
		{"name": "bronze", "min_hotels": 1, "discount_bps": 300},
		{"name": "silver", "min_hotels": 2, "discount_bps": 500},
	}}); r.status != 200 {
		t.Fatalf("admin changes the tiers: %d %s", r.status, r.raw)
	}
	bid := hold(g, quote(g, bole, boleSingle, 306, 307, 1, 1).str("id"), fmt.Sprintf("loy-bole-%d", n))
	payOnline(t, g, bid.str("id"), "succeeded")
	if acc := hotelFor(t).do("POST", "/partner/bookings/"+bid.str("id")+"/accept", nil); acc.status != 200 {
		t.Fatalf("accept at a second hotel: %d %s", acc.status, acc.raw)
	}
	finishStay(t, bid.str("id"))
	if r = loyalty(g); r.str("tier") != "silver" || r.num("hotels") != 2 || r.num("discount_bps") != 500 || r.body["next_tier"] != nil {
		t.Fatalf("a second hotel moves the guest to silver under the new threshold: %d %s", r.status, r.raw)
	}
}

// TestMemberDiscountAppliesAtCheckoutAndNotWithACoupon: the discount comes off the quote; a coupon replaces it.
func TestMemberDiscountAppliesAtCheckoutAndNotWithACoupon(t *testing.T) {
	n := time.Now().UnixNano()
	admin := adminClient(t)
	g := newClient(t, "guest")
	g.signIn("email", uniqueEmail("memberq"))
	withProfile(g)
	hotel := hotelFor(t)

	// Without a finished stay: no discount.
	plain := quote(g, meskel, standard, 310, 311, 2, 1)
	if plain.status != 200 || plain.body["member_tier"] != nil || plain.num("discount_santim") != 0 {
		t.Fatalf("no tier, no discount: %d %s", plain.status, plain.raw)
	}

	id, _, _, _ := confirmedBooking(t, g, hotel, 312, 313, fmt.Sprintf("memberq-1-%d", n))
	finishStay(t, id)

	// Bronze now: the quote carries the tier and a discount, and the total goes down by it.
	member := quote(g, meskel, standard, 314, 315, 2, 1)
	if member.status != 200 || member.str("member_tier") != "bronze" || member.num("discount_santim") <= 0 {
		t.Fatalf("bronze gets a discount at checkout: %d %s", member.status, member.raw)
	}
	if member.num("total_santim") >= member.num("list_total_santim") {
		t.Fatalf("the total must be below the list price: %s", member.raw)
	}

	// A coupon takes the place of the member discount, so the two never stack.
	code := fmt.Sprintf("LOYAL%d", n%100000)
	if r := admin.do("POST", "/admin/coupons", map[string]any{"code": code, "kind": "fixed", "value": 5, "active": true}); r.status != 200 {
		t.Fatalf("make a coupon: %d %s", r.status, r.raw)
	}
	withCoupon := g.do("POST", "/quotes", map[string]any{"property_id": meskel, "room_type_id": standard, "check_in": day(316), "check_out": day(317), "guests": 2, "rooms": 1, "coupon_code": code})
	if withCoupon.status != 200 || withCoupon.body["member_tier"] != nil || withCoupon.str("coupon_code") != code {
		t.Fatalf("with a coupon, the coupon is used and no member discount is added: %d %s", withCoupon.status, withCoupon.raw)
	}
}

// TestOnlyAdminsChangeTheTiers: the tiers are checked and only platform admins can change them.
func TestOnlyAdminsChangeTheTiers(t *testing.T) {
	admin := adminClient(t)
	defaults := []map[string]any{
		{"name": "bronze", "min_hotels": 1, "discount_bps": 300},
		{"name": "silver", "min_hotels": 3, "discount_bps": 500},
		{"name": "gold", "min_hotels": 6, "discount_bps": 800},
	}
	if r := admin.do("GET", "/admin/loyalty", nil); r.status != 200 || len(r.body["tiers"].([]any)) != 3 {
		t.Fatalf("the admin reads the tiers: %d %s", r.status, r.raw)
	}
	bad := map[string][]map[string]any{
		"two tiers with the same name": {{"name": "gold", "min_hotels": 1, "discount_bps": 100}, {"name": "gold", "min_hotels": 2, "discount_bps": 100}},
		"two tiers at the same count":  {{"name": "bronze", "min_hotels": 2, "discount_bps": 100}, {"name": "silver", "min_hotels": 2, "discount_bps": 100}},
		"discount over 30%":            {{"name": "gold", "min_hotels": 1, "discount_bps": 3100}},
		"a name with digits":           {{"name": "gold2", "min_hotels": 1, "discount_bps": 100}},
	}
	for name, tiers := range bad {
		if r := admin.do("PUT", "/admin/loyalty", map[string]any{"tiers": tiers}); r.status != 400 {
			t.Errorf("%s must be refused, got %d %s", name, r.status, r.raw)
		}
	}
	defer admin.do("PUT", "/admin/loyalty", map[string]any{"tiers": defaults})
	if r := admin.do("PUT", "/admin/loyalty", map[string]any{"tiers": defaults}); r.status != 200 {
		t.Fatalf("the defaults are accepted: %d %s", r.status, r.raw)
	}
	if r := newClient(t, "admin").do("PUT", "/admin/loyalty", map[string]any{"tiers": defaults}); r.status != 401 {
		t.Fatalf("no sign-in, no change: %d", r.status)
	}
	g := newClient(t, "guest")
	g.signIn("email", uniqueEmail("loyalguest"))
	if r := g.do("GET", "/me/loyalty", nil); r.status != 200 {
		t.Fatalf("a guest reads their own tier: %d", r.status)
	}
}
