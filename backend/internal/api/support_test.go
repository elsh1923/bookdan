package api_test

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func supportGuest(t *testing.T, prefix string) (*client, string) {
	t.Helper()
	email := uniqueEmail(prefix)
	g := newClient(t, "guest")
	g.signIn("email", email)
	withProfile(g)
	return g, email
}

func openTicket(g *client, kind, subject, message string, booking string) resp {
	body := map[string]any{"kind": kind, "subject": subject, "message": message}
	if booking != "" {
		body["booking_id"] = booking
	}
	return g.do("POST", "/support/tickets", body)
}

func TestASupportConversationFromQuestionToClosed(t *testing.T) {
	g, email := supportGuest(t, "sup")
	admin := adminClient(t)
	before := len(mails.to("admin@bookdan.test"))

	r := openTicket(g, "question", "Can I get an early check-in?", "I arrive at 7am. Is that possible?", "")
	if r.status != 200 || r.str("id") == "" {
		t.Fatalf("open: %d %s", r.status, r.raw)
	}
	id := r.str("id")
	if len(mails.to("admin@bookdan.test")) != before+1 {
		t.Errorf("support is emailed about a new request")
	}

	list := g.do("GET", "/support/tickets", nil)
	if list.status != 200 || !strings.Contains(string(list.raw), id) || !strings.Contains(string(list.raw), `"status":"open"`) {
		t.Fatalf("the guest sees the request, open: %s", list.raw)
	}

	// Support sees it, answers, and the guest is emailed.
	queue := admin.do("GET", "/admin/support/tickets", nil)
	if queue.status != 200 || !strings.Contains(string(queue.raw), id) || !strings.Contains(string(queue.raw), email) {
		t.Fatalf("the admin queue lists it with the guest: %d %s", queue.status, queue.raw)
	}
	if a := admin.do("POST", "/admin/support/tickets/"+id+"/reply", map[string]string{"message": "Yes, we asked the hotel; it is fine after 6am."}); a.status != 200 {
		t.Fatalf("answer: %d %s", a.status, a.raw)
	}
	if len(mails.to(email)) == 0 {
		t.Errorf("the guest is emailed the answer")
	}
	got := g.do("GET", "/support/tickets/"+id, nil)
	if got.status != 200 || !strings.Contains(string(got.raw), `"status":"answered"`) || !strings.Contains(string(got.raw), "fine after 6am") {
		t.Fatalf("the guest sees the answer: %s", got.raw)
	}
	if strings.Contains(string(got.raw), "admin@bookdan.test") {
		t.Errorf("the staff member's address is never shown to a guest: %s", got.raw)
	}

	// The guest replies: it goes back to the queue.
	if r := g.do("POST", "/support/tickets/"+id+"/messages", map[string]string{"message": "Thank you!"}); r.status != 200 {
		t.Fatalf("reply: %d %s", r.status, r.raw)
	}
	if got := g.do("GET", "/support/tickets/"+id, nil); !strings.Contains(string(got.raw), `"status":"open"`) {
		t.Errorf("a guest reply reopens the request: %s", got.raw)
	}

	// Support closes it; nobody can write on a closed request; closing again is harmless.
	if c := admin.do("POST", "/admin/support/tickets/"+id+"/close", nil); c.status != 200 {
		t.Fatalf("close: %d %s", c.status, c.raw)
	}
	if r := g.do("POST", "/support/tickets/"+id+"/messages", map[string]string{"message": "one more thing"}); r.status != 409 {
		t.Errorf("a closed request takes no more messages: %d", r.status)
	}
	if r := admin.do("POST", "/admin/support/tickets/"+id+"/reply", map[string]string{"message": "late"}); r.status != 409 {
		t.Errorf("support cannot answer a closed request either: %d", r.status)
	}
	if c := g.do("POST", "/support/tickets/"+id+"/close", nil); c.status != 200 {
		t.Errorf("closing twice is harmless: %d", c.status)
	}
}

func TestSupportRequestsAreValidatedAndPrivate(t *testing.T) {
	g, _ := supportGuest(t, "supv")
	other, _ := supportGuest(t, "supo")

	for name, c := range map[string]resp{
		"unknown kind":    openTicket(g, "complaint", "Something wrong", "text", ""),
		"dispute kind":    openTicket(g, "dispute", "Charged twice", "text", ""),
		"short subject":   openTicket(g, "question", "Hi", "text", ""),
		"empty message":   openTicket(g, "question", "A real subject", "   ", ""),
		"long message":    openTicket(g, "question", "A real subject", strings.Repeat("a", 2001), ""),
		"long subject":    openTicket(g, "question", strings.Repeat("s", 121), "text", ""),
		"booking not set": openTicket(g, "problem", "About a booking", "text", "00000000-0000-0000-0000-000000000000"),
		"booking garbled": openTicket(g, "problem", "About a booking", "text", "not-a-uuid"),
	} {
		wantNot := 200
		if c.status == wantNot {
			t.Errorf("%s must be refused, got %d", name, c.status)
		}
		if c.status >= 500 {
			t.Errorf("%s must not be a server error: %d", name, c.status)
		}
	}

	// A booking that belongs to someone else cannot be attached.
	hotel := hotelFor(t)
	id, _, _, _ := confirmedBooking(t, other, hotel, 236, 238, fmt.Sprintf("sup-%d", time.Now().UnixNano()))
	if r := openTicket(g, "problem", "About a booking", "text", id); r.status != 404 {
		t.Errorf("another guest's booking: %d", r.status)
	}
	// Their own can.
	own := openTicket(other, "problem", "My room is not ready", "I am at the desk.", id)
	if own.status != 200 {
		t.Fatalf("own booking: %d %s", own.status, own.raw)
	}
	if got := other.do("GET", "/support/tickets/"+own.str("id"), nil); !strings.Contains(string(got.raw), `"booking_code":"BD-`) {
		t.Errorf("the request shows its booking code: %s", got.raw)
	}

	// Privacy: another guest sees nothing; a guest cannot use the admin side; signed-out callers are refused.
	tid := own.str("id")
	if r := g.do("GET", "/support/tickets/"+tid, nil); r.status != 404 {
		t.Errorf("another guest's request: %d", r.status)
	}
	if r := g.do("POST", "/support/tickets/"+tid+"/messages", map[string]string{"message": "hello"}); r.status != 404 {
		t.Errorf("writing on another guest's request: %d", r.status)
	}
	if r := g.do("POST", "/support/tickets/"+tid+"/close", nil); r.status != 404 {
		t.Errorf("closing another guest's request: %d", r.status)
	}
	if list := g.do("GET", "/support/tickets", nil); strings.Contains(string(list.raw), tid) {
		t.Errorf("a guest's list holds only their own requests")
	}
	if r := g.do("GET", "/admin/support/tickets", nil); r.status != 401 && r.status != 403 {
		t.Errorf("a guest cannot open the admin queue: %d", r.status)
	}
	if r := newClient(t, "guest").do("GET", "/support/tickets", nil); r.status != 401 {
		t.Errorf("signed out: %d", r.status)
	}
}

func TestAGuestCannotHaveMoreThanFiveRequestsWaiting(t *testing.T) {
	g, _ := supportGuest(t, "supl")
	var ids []string
	for i := 0; i < 5; i++ {
		r := openTicket(g, "question", fmt.Sprintf("Question number %d", i), "text", "")
		if r.status != 200 {
			t.Fatalf("request %d: %d %s", i, r.status, r.raw)
		}
		ids = append(ids, r.str("id"))
	}
	if r := openTicket(g, "question", "One too many", "text", ""); r.status != 409 || r.str("error", "code") != "TOO_MANY_TICKETS" {
		t.Fatalf("the sixth is refused: %d %s", r.status, r.raw)
	}
	g.do("POST", "/support/tickets/"+ids[0]+"/close", nil)
	if r := openTicket(g, "question", "After closing one", "text", ""); r.status != 200 {
		t.Errorf("closing one makes room: %d %s", r.status, r.raw)
	}
}

func TestDeletingAnAccountErasesWhatTheGuestWroteToSupport(t *testing.T) {
	g, _ := supportGuest(t, "supd")
	admin := adminClient(t)
	id := openTicket(g, "problem", "My private problem", "My phone is 0911223344, please call.", "").str("id")
	admin.do("POST", "/admin/support/tickets/"+id+"/reply", map[string]string{"message": "We will look into it."})
	if r := g.do("DELETE", "/me/account", map[string]string{"confirm": "DELETE"}); r.status != 200 {
		t.Fatalf("delete: %d %s", r.status, r.raw)
	}
	got := admin.do("GET", "/admin/support/tickets/"+id, nil)
	if got.status != 200 {
		t.Fatalf("the admin still has the record: %d", got.status)
	}
	if strings.Contains(string(got.raw), "0911223344") || strings.Contains(string(got.raw), "My private problem") {
		t.Errorf("the guest's words are erased: %s", got.raw)
	}
	if !strings.Contains(string(got.raw), `"status":"closed"`) || !strings.Contains(string(got.raw), "We will look into it.") {
		t.Errorf("the request is closed and support's own words remain: %s", got.raw)
	}
}
