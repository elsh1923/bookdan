package api_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func newPhone() string {
	return fmt.Sprintf("+2519%08d", time.Now().UnixNano()%100000000)
}

func userID(t *testing.T, col, value string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(context.Background(), `SELECT id::text FROM users WHERE `+col+` = $1`, value).Scan(&id); err != nil {
		t.Fatalf("no account with %s %s: %v", col, value, err)
	}
	return id
}

func link(c *client, typ, value, code string, merge bool) resp {
	if r := c.do("POST", "/me/identities/code", map[string]string{"type": typ, "value": value}); r.status != 200 {
		return r
	}
	return c.do("POST", "/me/identities/confirm", map[string]any{"type": typ, "value": value, "code": code, "merge": merge})
}

func TestAGuestAddsASecondSignInMethodAndEitherOpensTheSameAccount(t *testing.T) {
	email := uniqueEmail("link")
	g := newClient(t, "guest")
	g.signIn("email", email)
	me := g.do("GET", "/me/identities", nil)
	if me.status != 200 || me.str("email") != email || me.body["phone"] != nil {
		t.Fatalf("a new account has the one method it signed in with: %d %s", me.status, me.raw)
	}

	phone := newPhone()
	if r := g.do("POST", "/me/identities/confirm", map[string]any{"type": "phone", "value": phone, "code": "000000"}); r.status != 401 {
		t.Fatalf("no code was asked for: %d %s", r.status, r.raw)
	}
	if r := g.do("POST", "/me/identities/code", map[string]string{"type": "phone", "value": phone}); r.status != 200 {
		t.Fatalf("send the code: %d %s", r.status, r.raw)
	}
	if r := g.do("POST", "/me/identities/confirm", map[string]any{"type": "phone", "value": phone, "code": "654321"}); r.status != 401 {
		t.Fatalf("a wrong code: %d %s", r.status, r.raw)
	}
	r := g.do("POST", "/me/identities/confirm", map[string]any{"type": "phone", "value": phone, "code": "123456"})
	if r.status != 200 || r.str("phone") != phone || r.str("email") != email || r.body["merged"] != false {
		t.Fatalf("confirm: %d %s", r.status, r.raw)
	}
	if userID(t, "phone", phone) != userID(t, "email", email) {
		t.Fatal("the phone must belong to the same account as the email")
	}

	// Signing in with the new number opens the same account.
	again := newClient(t, "guest")
	again.signIn("phone", phone)
	if got := again.do("GET", "/me/identities", nil); got.str("email") != email || got.str("phone") != phone {
		t.Fatalf("the phone opens the same account: %s", got.raw)
	}

	// An account holds one email and one phone: adding another of the same kind is refused.
	if r := g.do("POST", "/me/identities/code", map[string]string{"type": "phone", "value": newPhone()}); r.status != 409 || r.str("error", "code") != "ALREADY_HAS_IDENTITY" {
		t.Fatalf("a second phone: %d %s", r.status, r.raw)
	}
	if r := g.do("POST", "/me/identities/code", map[string]string{"type": "email", "value": uniqueEmail("link2")}); r.status != 409 {
		t.Fatalf("a second email: %d %s", r.status, r.raw)
	}
	if r := g.do("POST", "/me/identities/code", map[string]string{"type": "phone", "value": "12345"}); r.status != 400 {
		t.Fatalf("not a phone number: %d %s", r.status, r.raw)
	}
	var audits int
	pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_log WHERE action = 'identity_linked' AND user_id = $1`, userID(t, "email", email)).Scan(&audits)
	if audits != 1 {
		t.Errorf("linking is in the audit log: %d", audits)
	}
}

func TestTwoAccountsOfOnePersonAreMergedWhenTheyAgree(t *testing.T) {
	ctx := context.Background()
	n := time.Now().UnixNano()
	phone := newPhone()

	// The phone account: a profile, a booking, a saved hotel.
	p := newClient(t, "guest")
	p.signIn("phone", phone)
	withProfile(p)
	pid := userID(t, "phone", phone)
	pb := hold(p, quote(p, meskel, standard, 358, 359, 1, 1).str("id"), fmt.Sprintf("merge-p-%d", n))
	payOnline(t, p, pb.str("id"), "succeeded")
	if r := p.do("PUT", "/me/saved/"+meskel, nil); r.status != 200 {
		t.Fatalf("save a hotel on the phone account: %d %s", r.status, r.raw)
	}

	// The email account: its own booking and saved hotel.
	email := uniqueEmail("mergee")
	e := newClient(t, "guest")
	e.signIn("email", email)
	withProfile(e)
	eid := userID(t, "email", email)
	eb := hold(e, quote(e, meskel, standard, 360, 361, 1, 1).str("id"), fmt.Sprintf("merge-e-%d", n))
	payOnline(t, e, eb.str("id"), "succeeded")
	if r := e.do("PUT", "/me/saved/"+bole, nil); r.status != 200 {
		t.Fatalf("save a hotel on the email account: %d %s", r.status, r.raw)
	}

	// Adding the phone number finds the other account. The code is checked but not spent, and nothing moves until they say yes.
	e.do("POST", "/me/identities/code", map[string]string{"type": "phone", "value": phone})
	ask := e.do("POST", "/me/identities/confirm", map[string]any{"type": "phone", "value": phone, "code": "123456"})
	if ask.status != 409 || ask.str("error", "code") != "IDENTITY_IN_USE" || !strings.Contains(ask.str("error", "message"), "1 booking") {
		t.Fatalf("the person is asked first: %d %s", ask.status, ask.raw)
	}
	var owner string
	pool.QueryRow(ctx, `SELECT user_id::text FROM bookings WHERE id = $1`, pb.str("id")).Scan(&owner)
	if owner != pid {
		t.Fatal("nothing moves before they agree")
	}

	// Yes: the same code finishes it.
	done := e.do("POST", "/me/identities/confirm", map[string]any{"type": "phone", "value": phone, "code": "123456", "merge": true})
	if done.status != 200 || done.body["merged"] != true || done.str("phone") != phone || done.str("email") != email {
		t.Fatalf("merge: %d %s", done.status, done.raw)
	}
	var mine int
	pool.QueryRow(ctx, `SELECT count(*) FROM bookings WHERE user_id = $1 AND id IN ($2, $3)`, eid, pb.str("id"), eb.str("id")).Scan(&mine)
	if mine != 2 {
		t.Errorf("both bookings are now on the kept account: %d", mine)
	}
	var saved int
	pool.QueryRow(ctx, `SELECT count(*) FROM saved_properties WHERE user_id = $1`, eid).Scan(&saved)
	if saved != 2 {
		t.Errorf("saved hotels are joined: %d", saved)
	}
	var status, mail string
	pool.QueryRow(ctx, `SELECT status, email FROM users WHERE id = $1`, pid).Scan(&status, &mail)
	if status != "suspended" || !strings.HasSuffix(mail, "@deleted.invalid") {
		t.Errorf("the other account is closed: %s %s", status, mail)
	}
	if userID(t, "phone", phone) != eid {
		t.Error("the phone now opens the kept account")
	}
	if r := p.do("GET", "/me/profile", nil); r.status != 401 {
		t.Errorf("the closed account's phone is signed out: %d", r.status)
	}
	var audits int
	pool.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE action = 'accounts_merged' AND user_id = $1`, eid).Scan(&audits)
	if audits != 1 {
		t.Errorf("a merge is in the audit log: %d", audits)
	}
	// Both stays are listed for the person now.
	if list := e.do("GET", "/bookings", nil); list.status != 200 || len(list.body["items"].([]any)) < 2 {
		t.Errorf("the merged account lists both bookings: %s", list.raw)
	}
}

func TestAccountsThatMustNotBeMergedAreRefused(t *testing.T) {
	ctx := context.Background()
	n := time.Now().UnixNano()

	// A hotel's own account (it signs in with an email, so a guest who tries to add that email is told no).
	g := newClient(t, "guest")
	g.signIn("phone", newPhone())
	if r := link(g, "email", "owner@bookdan.test", "123456", true); r.status != 403 || r.str("error", "code") != "IDENTITY_NOT_MERGEABLE" {
		t.Fatalf("hotel staff accounts are never merged: %d %s", r.status, r.raw)
	}

	// An account with a payment in progress waits.
	phone := newPhone()
	busy := newClient(t, "guest")
	busy.signIn("phone", phone)
	withProfile(busy)
	hold(busy, quote(busy, meskel, standard, 362, 363, 1, 1).str("id"), fmt.Sprintf("merge-busy-%d", n))
	e := newClient(t, "guest")
	e.signIn("email", uniqueEmail("busy"))
	if r := link(e, "phone", phone, "123456", true); r.status != 409 {
		t.Fatalf("a payment in progress blocks the merge: %d %s", r.status, r.raw)
	}

	// A suspended account is not merged either.
	phone2 := newPhone()
	s := newClient(t, "guest")
	s.signIn("phone", phone2)
	pool.Exec(ctx, `UPDATE users SET status = 'suspended' WHERE phone = $1`, phone2)
	if r := link(e, "phone", phone2, "123456", true); r.status != 403 {
		t.Fatalf("a suspended account: %d %s", r.status, r.raw)
	}
}

// Every column that points at a user is either moved by the merge or belongs to hotel staff and admins (which are never merged). A new table that points
// at users must be added to one of the two lists, or this test fails and says which.
func TestEveryColumnThatPointsAtAUserIsKnownToTheMerge(t *testing.T) {
	known := map[string]bool{}
	for _, c := range []string{
		// moved or merged for a guest
		"bookings.user_id", "quotes.user_id", "coupon_redemptions.user_id", "hotel_calls.user_id", "reviews.user_id", "support_tickets.user_id",
		"saved_properties.user_id", "guest_profiles.user_id", "guest_id_photos.user_id", "idempotency_keys.user_id", "devices.user_id", "refresh_tokens.user_id",
		// hotel staff and admins: a guest account has none of these, and such accounts are refused
		"admin_accounts.created_by", "admin_accounts.disabled_by", "admin_accounts.user_id", "admin_recovery_codes.user_id", "audit_log.user_id", "bookings.checked_in_by", "hotel_payouts.paid_by",
		"hotel_receipts.received_by", "property_photos.uploaded_by", "property_staff.user_id", "reviews.hidden_by", "reviews.hotel_reply_by", "room_blocks.created_by",
		"room_blocks.released_by", "users.suspended_by",
	} {
		known[c] = true
	}
	rows, err := pool.Query(context.Background(), `SELECT c.conrelid::regclass::text || '.' || a.attname FROM pg_constraint c
		JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = ANY (c.conkey) WHERE c.contype = 'f' AND c.confrelid = 'users'::regclass`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var col string
		rows.Scan(&col)
		if !known[col] {
			t.Errorf("%s points at a user: decide what merging two guest accounts does with it (see linkaccounts.go), then list it here", col)
		}
	}
}
