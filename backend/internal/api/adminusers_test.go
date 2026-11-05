package api_test

import (
	"context"
	"net/url"
	"testing"
)

func TestSupportCanSuspendReinstateAndSignOutAPerson(t *testing.T) {
	boot := adminClient(t)
	support, _, supportID := newAdmin(t, boot, "support")
	finance, _, _ := newAdmin(t, boot, "finance")

	g := newClient(t, "guest")
	email := uniqueEmail("susp")
	g.signIn("email", email)
	if r := g.do("GET", "/me/profile", nil); r.status != 200 {
		t.Fatalf("a new guest uses the app: %d %s", r.status, r.raw)
	}

	// Only support and super admins may look people up.
	if r := finance.do("GET", "/admin/users?q="+url.QueryEscape(email), nil); r.status != 403 {
		t.Fatalf("finance must not open the users page: %d %s", r.status, r.raw)
	}
	list := support.do("GET", "/admin/users?q="+url.QueryEscape(email), nil)
	items, _ := list.body["items"].([]any)
	if list.status != 200 || len(items) != 1 {
		t.Fatalf("support finds the guest by email: %d %s", list.status, list.raw)
	}
	id := items[0].(map[string]any)["id"].(string)

	// A reason is required, and it is kept.
	if r := support.do("POST", "/admin/users/"+id+"/suspend", map[string]string{"reason": " "}); r.status != 400 {
		t.Fatalf("suspending needs a reason: %d %s", r.status, r.raw)
	}
	if r := support.do("POST", "/admin/users/"+id+"/suspend", map[string]string{"reason": "Fake reviews on three hotels"}); r.status != 200 {
		t.Fatalf("suspend: %d %s", r.status, r.raw)
	}
	if r := g.do("GET", "/me/profile", nil); r.status != 401 {
		t.Fatalf("a suspended guest's token must stop working at once: %d %s", r.status, r.raw)
	}
	if r := g.do("POST", "/auth/refresh", map[string]string{"refresh_token": g.refresh}); r.status == 200 {
		t.Fatalf("a suspended guest must not refresh a session: %s", r.raw)
	}
	again := newClient(t, "guest")
	again.do("POST", "/auth/otp/request", map[string]string{"type": "email", "value": email})
	if r := again.do("POST", "/auth/otp/verify", map[string]string{"type": "email", "value": email, "code": "123456"}); r.status != 403 {
		t.Fatalf("a suspended guest must not sign in: %d %s", r.status, r.raw)
	}
	shown := support.do("GET", "/admin/users?status=suspended&q="+url.QueryEscape(email), nil)
	row := shown.body["items"].([]any)[0].(map[string]any)
	if row["status"] != "suspended" || row["suspended_reason"] != "Fake reviews on three hotels" {
		t.Fatalf("the list shows the suspension and why: %s", shown.raw)
	}
	var audits int
	pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_log WHERE action = 'user_suspended' AND details->>'user_id' = $1`, id).Scan(&audits)
	if audits != 1 {
		t.Fatalf("the suspension must be in the audit log, found %d", audits)
	}

	// Suspending twice changes nothing; reinstating lets them back in.
	if r := support.do("POST", "/admin/users/"+id+"/suspend", map[string]string{"reason": "Again"}); r.status != 200 {
		t.Fatalf("suspending again is harmless: %d %s", r.status, r.raw)
	}
	if r := support.do("POST", "/admin/users/"+id+"/reinstate", nil); r.status != 200 {
		t.Fatalf("reinstate: %d %s", r.status, r.raw)
	}
	back := newClient(t, "guest")
	back.signIn("email", email)
	if r := back.do("GET", "/me/profile", nil); r.status != 200 {
		t.Fatalf("a reinstated guest signs in again: %d %s", r.status, r.raw)
	}

	// Signing out everywhere ends the sessions without suspending the person.
	if r := support.do("POST", "/admin/users/"+id+"/sign-out", nil); r.status != 200 {
		t.Fatalf("sign out everywhere: %d %s", r.status, r.raw)
	}
	if r := back.do("GET", "/me/profile", nil); r.status != 401 {
		t.Fatalf("a signed-out phone must stop working: %d %s", r.status, r.raw)
	}
	fresh := newClient(t, "guest")
	fresh.signIn("email", email)
	if r := fresh.do("GET", "/me/profile", nil); r.status != 200 {
		t.Fatalf("the person can sign in again afterwards: %d %s", r.status, r.raw)
	}

	// Admins and oneself are not touched here.
	if r := support.do("POST", "/admin/users/"+supportID+"/suspend", map[string]string{"reason": "test"}); r.status != 403 {
		t.Fatalf("an admin account is managed on the Admins page: %d %s", r.status, r.raw)
	}
	var financeID string
	pool.QueryRow(context.Background(), `SELECT user_id FROM admin_accounts a JOIN users u ON u.id = a.user_id WHERE a.role = 'finance' ORDER BY a.created_at DESC LIMIT 1`).Scan(&financeID)
	if r := support.do("POST", "/admin/users/"+financeID+"/suspend", map[string]string{"reason": "test"}); r.status != 403 {
		t.Fatalf("another admin cannot be suspended here: %d %s", r.status, r.raw)
	}
	if r := support.do("POST", "/admin/users/not-an-id/suspend", map[string]string{"reason": "test"}); r.status != 404 {
		t.Fatalf("an unknown id: %d %s", r.status, r.raw)
	}
}

func TestADeletedAccountCannotBeReinstated(t *testing.T) {
	boot := adminClient(t)
	g := newClient(t, "guest")
	email := uniqueEmail("gone")
	g.signIn("email", email)
	var id string
	if err := pool.QueryRow(context.Background(), `SELECT id FROM users WHERE email = $1`, email).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if r := g.do("DELETE", "/me/account", map[string]string{"confirm": "DELETE"}); r.status != 200 {
		t.Fatalf("delete account: %d %s", r.status, r.raw)
	}
	if r := boot.do("POST", "/admin/users/"+id+"/reinstate", nil); r.status != 409 {
		t.Fatalf("a deleted account stays closed: %d %s", r.status, r.raw)
	}
	list := boot.do("GET", "/admin/users?status=suspended&q=deleted-"+id[:8], nil)
	items, _ := list.body["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["deleted"] != true {
		t.Fatalf("the list marks it as deleted: %s", list.raw)
	}
}

func TestTheUsersListIsPaged(t *testing.T) {
	boot := adminClient(t)
	for i := 0; i < 3; i++ {
		c := newClient(t, "guest")
		c.signIn("email", uniqueEmail("page"))
	}
	first := boot.do("GET", "/admin/users?role=guest&limit=2", nil)
	items, _ := first.body["items"].([]any)
	cursor := first.str("next_cursor")
	if first.status != 200 || len(items) != 2 || cursor == "" {
		t.Fatalf("first page: %d %s", first.status, first.raw)
	}
	second := boot.do("GET", "/admin/users?role=guest&limit=2&cursor="+url.QueryEscape(cursor), nil)
	more, _ := second.body["items"].([]any)
	if second.status != 200 || len(more) == 0 {
		t.Fatalf("second page: %d %s", second.status, second.raw)
	}
	if more[0].(map[string]any)["id"] == items[0].(map[string]any)["id"] || more[0].(map[string]any)["id"] == items[1].(map[string]any)["id"] {
		t.Fatalf("a page must not repeat the one before")
	}
	if r := boot.do("GET", "/admin/users?limit=0", nil); r.status != 400 {
		t.Fatalf("a bad limit: %d", r.status)
	}
}
