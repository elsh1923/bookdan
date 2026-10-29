package api_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func reviewsOnPage(g *client) []map[string]any {
	prop := g.do("GET", "/properties/"+meskel+"?check_in="+day(200)+"&check_out="+day(201), nil)
	var out []map[string]any
	for _, x := range prop.body["reviews"].([]any) {
		out = append(out, x.(map[string]any))
	}
	return out
}

func TestAnAdminCanHideAnAbusiveReviewAndBringItBack(t *testing.T) {
	n := time.Now().UnixNano()
	g, _ := supportGuest(t, "rmg")
	hotel := hotelFor(t)
	admin := adminClient(t)
	id := completedStay(t, g, hotel, 96, 2, fmt.Sprintf("rm-%d", n))
	text := fmt.Sprintf("Terrible people, call 0911 000 %d and shout at them.", n%10000)
	if r := g.do("POST", "/bookings/"+id+"/review", map[string]any{"rating": 1, "text": text}); r.status != 200 {
		t.Fatalf("review: %d %s", r.status, r.raw)
	}
	shown := func() bool {
		for _, v := range reviewsOnPage(g) {
			if strings.Contains(v["text"].(string), "Terrible people") && strings.Contains(v["text"].(string), fmt.Sprint(n%10000)) {
				return true
			}
		}
		return false
	}
	ownerSees := func() (rid string) {
		for _, x := range hotel.do("GET", "/partner/reviews", nil).body["reviews"].([]any) {
			if m := x.(map[string]any); strings.Contains(m["text"].(string), fmt.Sprint(n%10000)) {
				rid = m["id"].(string)
			}
		}
		return rid
	}
	count := func() float64 {
		prop := g.do("GET", "/properties/"+meskel+"?check_in="+day(200)+"&check_out="+day(201), nil)
		rs, _ := prop.body["rating_summary"].(map[string]any)
		c, _ := rs["count"].(float64)
		return c
	}
	if !shown() || ownerSees() == "" {
		t.Fatalf("the review is public at first")
	}
	before := count()

	// The admin finds it by its words and hides it.
	list := admin.do("GET", "/admin/reviews?q="+fmt.Sprint(n%10000), nil)
	var rid string
	for _, x := range list.body["reviews"].([]any) {
		m := x.(map[string]any)
		rid = m["id"].(string)
		if m["hidden"] != false || m["hotel"] == "" || m["booking_code"] == "" {
			t.Errorf("a visible review: %v", m)
		}
	}
	if rid == "" {
		t.Fatalf("the admin finds the review: %s", list.raw)
	}
	for name, r := range map[string]resp{
		"no reason":    admin.do("POST", "/admin/reviews/"+rid+"/hide", map[string]string{}),
		"short reason": admin.do("POST", "/admin/reviews/"+rid+"/hide", map[string]string{"reason": "no"}),
		"unknown":      admin.do("POST", "/admin/reviews/00000000-0000-0000-0000-000000000000/hide", map[string]string{"reason": "Shows a phone number"}),
		"garbled":      admin.do("POST", "/admin/reviews/not-a-uuid/hide", map[string]string{"reason": "Shows a phone number"}),
		"not hidden":   admin.do("POST", "/admin/reviews/"+rid+"/restore", nil),
	} {
		if r.status == 200 || r.status >= 500 {
			t.Errorf("%s: %d %s", name, r.status, r.raw)
		}
	}
	if r := g.do("POST", "/admin/reviews/"+rid+"/hide", map[string]string{"reason": "Not an admin"}); r.status != 401 && r.status != 403 {
		t.Errorf("a guest cannot moderate: %d", r.status)
	}
	if r := hotel.do("POST", "/admin/reviews/"+rid+"/hide", map[string]string{"reason": "Not an admin"}); r.status != 401 && r.status != 403 {
		t.Errorf("a hotel cannot moderate: %d", r.status)
	}
	if r := admin.do("POST", "/admin/reviews/"+rid+"/hide", map[string]string{"reason": "It shows a private phone number"}); r.status != 200 {
		t.Fatalf("hide: %d %s", r.status, r.raw)
	}
	if r := admin.do("POST", "/admin/reviews/"+rid+"/hide", map[string]string{"reason": "Twice"}); r.status != 409 {
		t.Errorf("hiding twice: %d", r.status)
	}

	// Gone from the hotel page, the rating and the owner's list; the owner cannot answer it.
	if shown() {
		t.Errorf("a hidden review is not on the hotel page")
	}
	if got := count(); got != before-1 {
		t.Errorf("a hidden review is not counted: %v -> %v", before, got)
	}
	if ownerSees() != "" {
		t.Errorf("the owner's list leaves it out")
	}
	if r := hotel.do("PUT", "/partner/reviews/"+rid+"/reply", map[string]string{"text": "We are sorry to hear this"}); r.status != 404 {
		t.Errorf("a hidden review cannot be answered: %d", r.status)
	}
	// The guest still sees their own review, and cannot write a second one.
	if b := g.do("GET", "/bookings/"+id, nil); b.str("review", "text") == "" {
		t.Errorf("the author still sees their review: %s", b.raw)
	}
	if r := g.do("POST", "/bookings/"+id+"/review", map[string]any{"rating": 5, "text": "Second try"}); r.status != 409 {
		t.Errorf("one review per stay, hidden or not: %d", r.status)
	}
	hid := admin.do("GET", "/admin/reviews?status=hidden&q="+fmt.Sprint(n%10000), nil)
	hl := hid.body["reviews"].([]any)
	if len(hl) != 1 || hl[0].(map[string]any)["hidden_reason"] != "It shows a private phone number" || hl[0].(map[string]any)["hidden_by"] == nil {
		t.Errorf("the hidden list keeps the reason and who did it: %s", hid.raw)
	}
	if vis := admin.do("GET", "/admin/reviews?q="+fmt.Sprint(n%10000), nil); len(vis.body["reviews"].([]any)) != 0 {
		t.Errorf("a hidden review is not in the default list")
	}
	var audits int
	pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_log WHERE action = 'review_hidden' AND details->>'review_id' = $1`, rid).Scan(&audits)
	if audits != 1 {
		t.Errorf("hiding is in the audit log: %d", audits)
	}

	// Restoring brings everything back.
	if r := admin.do("POST", "/admin/reviews/"+rid+"/restore", nil); r.status != 200 {
		t.Fatalf("restore: %d %s", r.status, r.raw)
	}
	if !shown() || count() != before || ownerSees() == "" {
		t.Errorf("a restored review is public again")
	}
}
