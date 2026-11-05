package api_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func partnerSignedIn(t *testing.T, email string) *client {
	t.Helper()
	c := newClient(t, "partner")
	c.token = signInWithEmailedCode(c, "/auth/otp/request", "/auth/otp/verify", email).str("token")
	return c
}

func prefFor(r resp, property string) map[string]any {
	hotels, _ := r.body["hotels"].([]any)
	for _, h := range hotels {
		if m := h.(map[string]any); m["property_id"] == property {
			return m
		}
	}
	return nil
}

func hasTitle(ms []message, title string) bool {
	for _, m := range ms {
		if m.title == title {
			return true
		}
	}
	return false
}

func TestAPersonChoosesWhichOptionalAlertsTheyGet(t *testing.T) {
	ctx := context.Background()
	n := time.Now().UnixNano()
	recTok, ownTok := fmt.Sprintf("pref-rec-%d", n), fmt.Sprintf("pref-own-%d", n)
	recMail, ownMail := realAddress("prefdesk"), realAddress("prefboss")
	rec := staffMember(t, "receptionist", recMail, recTok)
	own := staffMember(t, "owner", ownMail, ownTok)
	defer func() {
		pool.Exec(ctx, `DELETE FROM devices WHERE user_id IN ($1, $2)`, rec, own)
		pool.Exec(ctx, `DELETE FROM property_staff WHERE user_id IN ($1, $2)`, rec, own)
	}()
	recC, ownC := partnerSignedIn(t, recMail), partnerSignedIn(t, ownMail)

	// Each person sees their own choices: a receptionist is offered fewer kinds than an owner, and nothing is muted at first.
	recPref := prefFor(recC.do("GET", "/partner/alert-preferences", nil), meskel)
	ownPref := prefFor(ownC.do("GET", "/partner/alert-preferences", nil), meskel)
	if recPref == nil || ownPref == nil {
		t.Fatal("both people work for the hotel and must see it in their choices")
	}
	if got := fmt.Sprint(recPref["available_kinds"]); got != "[changes disputes]" {
		t.Errorf("a receptionist can choose changes and disputes: %s", got)
	}
	if got := fmt.Sprint(ownPref["available_kinds"]); got != "[changes disputes reviews]" {
		t.Errorf("an owner can also choose reviews: %s", got)
	}
	if got := fmt.Sprint(ownPref["muted_kinds"]); got != "[]" {
		t.Errorf("nothing is muted at first: %s", got)
	}

	// Bad input and other people's hotels are refused.
	if r := ownC.do("PUT", "/partner/alert-preferences/"+meskel, map[string]any{"muted_kinds": []string{"new_booking"}}); r.status != 400 {
		t.Errorf("new booking requests cannot be muted: %d %s", r.status, r.raw)
	}
	if r := ownC.do("PUT", "/partner/alert-preferences/"+bole, map[string]any{"muted_kinds": []string{"changes"}}); r.status != 404 {
		t.Errorf("a hotel they do not work for: %d %s", r.status, r.raw)
	}
	if r := ownC.do("PUT", "/partner/alert-preferences/not-an-id", map[string]any{"muted_kinds": []string{"changes"}}); r.status != 404 {
		t.Errorf("a garbled id: %d %s", r.status, r.raw)
	}

	// The owner stops getting cancellations and reviews (a repeated kind is kept once); the receptionist keeps everything.
	set := ownC.do("PUT", "/partner/alert-preferences/"+meskel, map[string]any{"muted_kinds": []string{"reviews", "changes", "reviews"}})
	if set.status != 200 || fmt.Sprint(set.body["muted_kinds"]) != "[changes reviews]" {
		t.Fatalf("save the choices: %d %s", set.status, set.raw)
	}
	if got := fmt.Sprint(prefFor(ownC.do("GET", "/partner/alert-preferences", nil), meskel)["muted_kinds"]); got != "[changes reviews]" {
		t.Errorf("the choices are kept: %s", got)
	}

	g, _ := supportGuest(t, "prefg")
	b := hold(g, quote(g, meskel, standard, 355, 356, 1, 1).str("id"), fmt.Sprintf("pref-1-%d", n))
	id := b.str("id")
	payOnline(t, g, id, "succeeded")
	// A new request is not optional: both are alerted whatever they chose.
	if !hasTitle(push.to(recTok), "New booking request") || !hasTitle(push.to(ownTok), "New booking request") {
		t.Fatalf("a request must reach everyone on duty: desk %q, owner %q", titles(push.to(recTok)), titles(push.to(ownTok)))
	}
	if r := g.do("POST", "/bookings/"+id+"/cancel", nil); r.status != 200 {
		t.Fatalf("cancel: %d %s", r.status, r.raw)
	}
	if !hasTitle(push.to(recTok), "Booking cancelled") {
		t.Errorf("the receptionist did not mute cancellations and must hear about it: %q", titles(push.to(recTok)))
	}
	if hasTitle(push.to(ownTok), "Booking cancelled") {
		t.Errorf("the owner muted changes and must not be alerted: %q", titles(push.to(ownTok)))
	}

	// A review reaches owners only, and not the one who muted reviews.
	hotel := hotelFor(t)
	g2, _ := supportGuest(t, "prefg2")
	stay := completedStay(t, g2, hotel, 97, 2, fmt.Sprintf("pref-2-%d", n))
	if r := g2.do("POST", "/bookings/"+stay+"/review", map[string]any{"rating": 4, "text": "A pleasant stay."}); r.status != 200 {
		t.Fatalf("review: %d %s", r.status, r.raw)
	}
	for _, m := range push.to(recTok) {
		if strings.Contains(m.title, "reviewed") {
			t.Errorf("a receptionist is not told about reviews: %q", titles(push.to(recTok)))
		}
	}
	if len(push.to(ownTok)) > 0 && hasReviewAlert(push.to(ownTok)) {
		t.Errorf("the owner muted reviews: %q", titles(push.to(ownTok)))
	}

	// Un-mute reviews: the next review is told to them.
	if r := ownC.do("PUT", "/partner/alert-preferences/"+meskel, map[string]any{"muted_kinds": []string{"changes"}}); r.status != 200 {
		t.Fatalf("un-mute: %d %s", r.status, r.raw)
	}
	g3, _ := supportGuest(t, "prefg3")
	stay3 := completedStay(t, g3, hotel, 99, 2, fmt.Sprintf("pref-3-%d", n))
	if r := g3.do("POST", "/bookings/"+stay3+"/review", map[string]any{"rating": 5, "text": "Wonderful."}); r.status != 200 {
		t.Fatalf("review 2: %d %s", r.status, r.raw)
	}
	if !hasReviewAlert(push.to(ownTok)) {
		t.Errorf("an owner who wants review alerts gets them: %q", titles(push.to(ownTok)))
	}
	for _, m := range push.to(ownTok) {
		if m.data["type"] == "new_review" && m.data["booking_id"] != stay3 {
			t.Errorf("only the second review was alerted: %v", m.data)
		}
	}
}

func hasReviewAlert(ms []message) bool {
	for _, m := range ms {
		if m.data["type"] == "new_review" {
			return true
		}
	}
	return false
}

func TestMutedAlertsAreShownOnTheHotelsTeamPage(t *testing.T) {
	ctx := context.Background()
	n := time.Now().UnixNano()
	mail := realAddress("prefshow")
	uid := staffMember(t, "receptionist", mail, fmt.Sprintf("pref-show-%d", n))
	defer func() {
		pool.Exec(ctx, `DELETE FROM devices WHERE user_id = $1`, uid)
		pool.Exec(ctx, `DELETE FROM property_staff WHERE user_id = $1`, uid)
	}()
	c := partnerSignedIn(t, mail)
	if r := c.do("PUT", "/partner/alert-preferences/"+meskel, map[string]any{"muted_kinds": []string{"disputes"}}); r.status != 200 {
		t.Fatalf("save: %d %s", r.status, r.raw)
	}
	d := hotelFor(t).do("GET", "/partner/hotels/"+meskel, nil)
	for _, s := range d.body["staff"].([]any) {
		if m := s.(map[string]any); m["user_id"] == uid {
			if fmt.Sprint(m["muted_kinds"]) != "[disputes]" {
				t.Fatalf("the owner sees what the person muted: %v", m)
			}
			return
		}
	}
	t.Fatal("the new person is on the team page")
}
