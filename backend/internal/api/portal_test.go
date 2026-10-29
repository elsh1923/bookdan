package api_test

import (
	"fmt"
	"testing"
	"time"
)

// The hotel portal: an owner runs their own hotel through the partner sign-in, and BookDan keeps what only BookDan decides.
func TestAnOwnerRunsTheirOwnHotelButCannotPublishOrVerifyIt(t *testing.T) {
	owner := hotelFor(t)
	admin := adminClient(t)

	list := owner.do("GET", "/partner/hotels", nil)
	if list.status != 200 {
		t.Fatalf("the owner lists their hotels: %d %s", list.status, list.raw)
	}
	found := false
	for _, x := range list.body["items"].([]any) {
		if x.(map[string]any)["id"] == meskel {
			found = true
		}
	}
	if !found {
		t.Fatalf("the owner must see their hotel: %s", list.raw)
	}

	before := owner.do("GET", "/partner/hotels/"+meskel, nil)
	if before.status != 200 {
		t.Fatalf("the owner reads their hotel: %d %s", before.status, before.raw)
	}
	if _, has := before.body["business"]; has && before.body["business"] != nil {
		t.Fatalf("an owner must not be given the business and payout details: %s", before.raw)
	}

	// Editing the hotel works, but asking for another status or a verified badge changes neither.
	status, verified := before.str("status"), before.body["verified"]
	body := map[string]any{
		"name": before.str("name"), "type": before.str("type"), "city": before.str("city"), "area": before.str("area"), "address": before.str("address"), "phone": before.str("phone"),
		"description": "Portal edit " + fmt.Sprint(time.Now().UnixNano()), "lat": before.body["lat"], "lng": before.body["lng"], "amenities": before.body["amenities"], "house_rules": before.body["house_rules"],
		"pay_at_hotel": before.body["pay_at_hotel"], "check_in_time": before.str("check_in_time"), "check_out_time": before.str("check_out_time"),
		"status": "hidden", "verified": verified != true,
	}
	if status == "hidden" {
		body["status"] = "active"
	}
	if r := owner.do("PUT", "/partner/hotels/"+meskel, body); r.status != 200 {
		t.Fatalf("an owner edits their hotel: %d %s", r.status, r.raw)
	}
	after := owner.do("GET", "/partner/hotels/"+meskel, nil)
	if after.str("description") != body["description"] {
		t.Fatalf("the edit must be saved: %s", after.raw)
	}
	if after.str("status") != status || after.body["verified"] != verified {
		t.Fatalf("an owner must not change status or verification: status %s->%s verified %v->%v", status, after.str("status"), verified, after.body["verified"])
	}

	// Rooms and prices go live at once.
	var room map[string]any
	for _, r := range after.body["rooms"].([]any) {
		if r.(map[string]any)["name"] == "Family Suite" {
			room = r.(map[string]any)
		}
	}
	if room == nil {
		t.Fatalf("demo hotel should have a Family Suite: %s", after.raw)
	}
	oldPrice := int64(room["price_santim"].(float64))
	newPrice := oldPrice + 5000
	defer owner.do("PUT", "/partner/hotels/"+meskel+"/rooms/"+room["id"].(string)+"/price", map[string]any{"price_santim": oldPrice})
	if r := owner.do("PUT", "/partner/hotels/"+meskel+"/rooms/"+room["id"].(string)+"/price", map[string]any{"price_santim": newPrice}); r.status != 200 {
		t.Fatalf("an owner sets a price: %d %s", r.status, r.raw)
	}
	g := newClient(t, "guest")
	g.signIn("email", uniqueEmail("portal"))
	withProfile(g)
	q := quote(g, meskel, family, 330, 331, 2, 1)
	if q.status != 200 || int64(q.num("total_santim")) < newPrice {
		t.Fatalf("a guest must now be quoted the new price (%d): %d %s", newPrice, q.status, q.raw)
	}

	// Business details, payout accounts and coupons stay with BookDan.
	if r := owner.do("GET", "/partner/hotels/"+meskel+"/business", nil); r.status == 200 {
		t.Fatalf("an owner must not reach the business details: %s", r.raw)
	}
	if r := owner.do("PUT", "/partner/hotels/"+meskel+"/business", map[string]any{"legal_name": "x"}); r.status == 200 {
		t.Fatalf("an owner must not edit the business details: %s", r.raw)
	}
	if r := owner.do("POST", "/admin/hotels/"+meskel+"/business/payout/reveal", nil); r.status == 200 {
		t.Fatalf("an owner must not reveal a payout account: %s", r.raw)
	}

	// A second hotel is not theirs.
	other := admin.do("POST", "/admin/hotels", newHotelBody(fmt.Sprintf("Portal Other %d", time.Now().UnixNano()%100000)))
	if other.status != 200 {
		t.Fatalf("create other hotel: %d %s", other.status, other.raw)
	}
	if r := owner.do("GET", "/partner/hotels/"+other.str("id"), nil); r.status == 200 {
		t.Fatalf("an owner must not read another hotel: %s", r.raw)
	}
	if r := owner.do("PUT", "/partner/hotels/"+other.str("id"), body); r.status == 200 {
		t.Fatalf("an owner must not edit another hotel: %s", r.raw)
	}
	for _, x := range owner.do("GET", "/partner/hotels", nil).body["items"].([]any) {
		if x.(map[string]any)["id"] == other.str("id") {
			t.Fatalf("another hotel must not be listed for the owner")
		}
	}

	// BookDan sees what the owner changed, and the log says the hotel did it.
	log := admin.do("GET", "/admin/activity?by=hotel&property_id="+meskel, nil)
	if log.status != 200 {
		t.Fatalf("activity: %d %s", log.status, log.raw)
	}
	var sawPrice, sawEdit bool
	for _, x := range log.body["items"].([]any) {
		a := x.(map[string]any)
		if a["by_hotel"] != true {
			t.Fatalf("by=hotel must list only the hotels' own changes: %v", a)
		}
		switch a["action"] {
		case "admin_price_set":
			sawPrice = true
		case "admin_hotel_updated":
			sawEdit = true
		}
	}
	if !sawPrice || !sawEdit {
		t.Fatalf("the price change and the edit must be in the activity log: %s", log.raw)
	}
	if r := owner.do("GET", "/admin/activity", nil); r.status == 200 {
		t.Fatalf("an owner must not read the activity log")
	}
}

func TestAReceptionistHasNoHotelManagement(t *testing.T) {
	admin := adminClient(t)
	rec := receptionist(t, admin, "portalrec")
	list := rec.do("GET", "/partner/hotels", nil)
	if list.status != 200 || len(list.body["items"].([]any)) != 0 {
		t.Fatalf("a receptionist owns no hotel: %d %s", list.status, list.raw)
	}
	if r := rec.do("GET", "/partner/hotels/"+meskel, nil); r.status == 200 {
		t.Fatalf("a receptionist must not open hotel management: %s", r.raw)
	}
	if r := rec.do("PUT", "/partner/hotels/"+meskel+"/rooms/00000000-0000-0000-0000-000000000000/price", map[string]any{"price_santim": 100000}); r.status == 200 {
		t.Fatalf("a receptionist must not set prices")
	}
	if r := rec.do("POST", "/partner/hotels/"+meskel+"/staff", map[string]string{"contact": realAddress("x"), "role": "receptionist"}); r.status == 200 {
		t.Fatalf("a receptionist must not add people")
	}
}

func TestAnOwnerCanAddReceptionistsButNotOtherOwners(t *testing.T) {
	owner := hotelFor(t)
	mail := realAddress("portalstaff")
	t.Cleanup(func() {
		pool.Exec(t.Context(), `DELETE FROM property_staff WHERE user_id IN (SELECT id FROM users WHERE email = $1)`, mail)
	})
	if r := owner.do("POST", "/partner/hotels/"+meskel+"/staff", map[string]string{"contact": mail, "role": "receptionist"}); r.status != 200 {
		t.Fatalf("an owner adds a receptionist: %d %s", r.status, r.raw)
	}
	if r := owner.do("POST", "/partner/hotels/"+meskel+"/staff", map[string]string{"contact": realAddress("portalowner"), "role": "owner"}); r.status == 200 {
		t.Fatalf("an owner must not add another owner: %s", r.raw)
	}
}
