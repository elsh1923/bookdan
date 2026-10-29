package api_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha1" //nolint:gosec // RFC 6238
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"strings"
	"testing"
	"time"
)

const strongPassword = "a-long-and-private-passphrase-1"

func adminSignIn(t *testing.T, email, password, code string) resp {
	t.Helper()
	c := newClient(t, "admin")
	body := map[string]string{"email": email, "password": password}
	if code != "" {
		body["code"] = code
	}
	return c.do("POST", "/admin/login", body)
}

func adminAs(t *testing.T, token string) *client {
	c := newClient(t, "admin")
	c.token = token
	return c
}

// newAdmin creates an admin account (with a password the super admin chooses), signs in, replaces the password as required, and returns a client for it.
func newAdmin(t *testing.T, boot *client, role string) (*client, string, string) {
	t.Helper()
	email := fmt.Sprintf("%s-%d@bookdan.test", role, time.Now().UnixNano())
	r := boot.do("POST", "/admin/admins", map[string]string{"email": email, "role": role, "password": "first-password-chosen-by-super"})
	if r.status != 200 {
		t.Fatalf("create %s: %d %s", role, r.status, r.raw)
	}
	l := adminSignIn(t, email, "first-password-chosen-by-super", "")
	if l.status != 200 || l.body["must_change_password"] != true || l.str("admin_role") != role {
		t.Fatalf("first sign-in: %d %s", l.status, l.raw)
	}
	c := adminAs(t, l.str("token"))
	ch := c.do("POST", "/admin/me/password", map[string]string{"current_password": "first-password-chosen-by-super", "new_password": strongPassword})
	if ch.status != 200 || ch.body["must_change_password"] != false {
		t.Fatalf("change password: %d %s", ch.status, ch.raw)
	}
	c.token = ch.str("token")
	return c, email, r.str("id")
}

func totpCode(secretB32 string, step int64) string {
	secret, _ := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secretB32)
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(step))
	m := hmac.New(sha1.New, secret)
	m.Write(msg[:])
	sum := m.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	code := (uint32(sum[off]&0x7f) << 24) | uint32(sum[off+1])<<16 | uint32(sum[off+2])<<8 | uint32(sum[off+3])
	return fmt.Sprintf("%06d", code%1_000_000)
}

func TestEachAdminHasTheirOwnAccountThatMustChangeTheGivenPassword(t *testing.T) {
	boot := adminClient(t)
	// The first-time shared password is a super admin without an account of their own.
	if me := boot.do("GET", "/admin/me", nil); me.str("admin_role") != "super" || me.body["has_account"] != false {
		t.Fatalf("the shared password acts as a super admin: %s", me.raw)
	}
	email := fmt.Sprintf("newcomer-%d@bookdan.test", time.Now().UnixNano())
	r := boot.do("POST", "/admin/admins", map[string]string{"email": email, "role": "support"})
	if r.status != 200 || len(r.str("temporary_password")) != 16 {
		t.Fatalf("create with a generated password: %d %s", r.status, r.raw)
	}
	temp := r.str("temporary_password")
	l := adminSignIn(t, email, temp, "")
	if l.status != 200 || l.body["must_change_password"] != true {
		t.Fatalf("sign in: %d %s", l.status, l.raw)
	}
	c := adminAs(t, l.str("token"))
	if me := c.do("GET", "/admin/me", nil); me.status != 200 || me.str("admin_role") != "support" || me.body["must_change_password"] != true {
		t.Errorf("me before changing: %d %s", me.status, me.raw)
	}
	if x := c.do("GET", "/admin/hotels", nil); x.status != 403 || x.str("error", "code") != "PASSWORD_CHANGE_REQUIRED" {
		t.Errorf("nothing else is allowed until the password is replaced: %d %s", x.status, x.raw)
	}
	for name, body := range map[string]map[string]string{
		"wrong current": {"current_password": "not-the-password", "new_password": strongPassword},
		"too short":     {"current_password": temp, "new_password": "short"},
		"has the name":  {"current_password": temp, "new_password": "x-" + strings.Split(email, "@")[0] + "-yy"},
		"same":          {"current_password": temp, "new_password": temp},
	} {
		if x := c.do("POST", "/admin/me/password", body); x.status == 200 || x.status >= 500 {
			t.Errorf("%s: %d %s", name, x.status, x.raw)
		}
	}
	time.Sleep(1100 * time.Millisecond) // a token is told apart from a later password change by the second it was issued
	ch := c.do("POST", "/admin/me/password", map[string]string{"current_password": temp, "new_password": strongPassword})
	if ch.status != 200 {
		t.Fatalf("change: %d %s", ch.status, ch.raw)
	}
	if x := c.do("GET", "/admin/hotels", nil); x.status != 401 {
		t.Errorf("the old session ends when the password changes: %d", x.status)
	}
	fresh := adminAs(t, ch.str("token"))
	if x := fresh.do("GET", "/admin/hotels", nil); x.status != 200 {
		t.Errorf("the new session works: %d %s", x.status, x.raw)
	}
	if adminSignIn(t, email, temp, "").status != 401 || adminSignIn(t, email, strongPassword, "").status != 200 {
		t.Errorf("only the new password works")
	}
}

func TestAdminRolesLimitWhatEachPersonCanDo(t *testing.T) {
	boot := adminClient(t)
	support, _, _ := newAdmin(t, boot, "support")
	finance, _, _ := newAdmin(t, boot, "finance")
	super, _, _ := newAdmin(t, boot, "super")
	hotel := hotelFor(t)
	meskelPath := "/admin/hotels/" + meskel

	type probe struct {
		name, method, path string
		body               any
		allowed            map[string]bool // which roles may pass the role check (a 200, 400, 404 or 409 from the handler, not a 403)
	}
	for _, p := range []probe{
		{"hotel list", "GET", "/admin/hotels", nil, map[string]bool{"support": true, "finance": true, "super": true}},
		{"analytics", "GET", "/admin/analytics", nil, map[string]bool{"support": true, "finance": true, "super": true}},
		{"calls", "GET", "/admin/calls", nil, map[string]bool{"support": true, "finance": true, "super": true}},
		{"rates (read)", "GET", "/admin/rates", nil, map[string]bool{"support": true, "finance": true, "super": true}},
		{"support queue", "GET", "/admin/support/tickets", nil, map[string]bool{"support": true, "super": true}},
		{"reviews", "GET", "/admin/reviews", nil, map[string]bool{"support": true, "super": true}},
		{"payouts", "GET", "/admin/payouts", nil, map[string]bool{"finance": true, "super": true}},
		{"payment issues", "GET", "/admin/payment-issues", nil, map[string]bool{"finance": true, "super": true}},
		{"statements", "GET", meskelPath + "/statements", nil, map[string]bool{"finance": true, "super": true}},
		{"record payout", "POST", meskelPath + "/payouts", map[string]any{"amount_santim": 0, "reference": "x"}, map[string]bool{"finance": true, "super": true}},
		{"set a rate", "PUT", "/admin/rates/USD", map[string]any{"birr_per_unit": 1}, map[string]bool{"finance": true, "super": true}},
		{"payout reveal", "POST", meskelPath + "/business/payout/reveal", nil, map[string]bool{"finance": true, "super": true}},
		{"create a hotel", "POST", "/admin/hotels", map[string]any{}, map[string]bool{"super": true}},
		{"edit a hotel", "PUT", meskelPath, map[string]any{}, map[string]bool{"super": true}},
		{"coupons (read)", "GET", "/admin/coupons", nil, map[string]bool{"support": true, "finance": true, "super": true}},
		{"create a coupon", "POST", "/admin/coupons", map[string]any{}, map[string]bool{"super": true}},
		{"applications", "GET", "/admin/applications", nil, map[string]bool{"super": true}},
		{"list admins", "GET", "/admin/admins", nil, map[string]bool{"super": true}},
		{"add an admin", "POST", "/admin/admins", map[string]any{}, map[string]bool{"super": true}},
		{"staff", "POST", meskelPath + "/staff", map[string]any{}, map[string]bool{"super": true}},
	} {
		for role, c := range map[string]*client{"support": support, "finance": finance, "super": super} {
			r := c.do(p.method, p.path, p.body)
			if p.allowed[role] && (r.status == 403 || r.status == 401 || r.status >= 500) {
				t.Errorf("%s may %s: %d %s", role, p.name, r.status, r.raw)
			}
			if !p.allowed[role] && r.status != 403 {
				t.Errorf("%s may not %s: %d %s", role, p.name, r.status, r.raw)
			}
		}
	}
	// A hotel's own staff are not admins at all.
	if r := hotel.do("GET", "/admin/admins", nil); r.status != 401 && r.status != 403 {
		t.Errorf("a hotel is not an admin: %d", r.status)
	}
	// The page is told what each role can do.
	if me := support.do("GET", "/admin/me", nil); me.body["can"].(map[string]any)["support"] != true || me.body["can"].(map[string]any)["finance"] != false {
		t.Errorf("support's capabilities: %s", me.raw)
	}
}

func TestAnAdminAccountIsLockedAfterTooManyWrongPasswords(t *testing.T) {
	boot := adminClient(t)
	_, email, id := newAdmin(t, boot, "support")

	// A wrong password and an unknown address look the same.
	a, b := adminSignIn(t, email, "wrong-password-12345", ""), adminSignIn(t, "nobody-here@bookdan.test", "wrong-password-12345", "")
	if a.status != 401 || b.status != 401 || a.str("error", "message") != b.str("error", "message") {
		t.Errorf("wrong password and unknown address answer alike: %d %q / %d %q", a.status, a.str("error", "message"), b.status, b.str("error", "message"))
	}
	// Five wrong passwords lock the account, even for the right password, until a super admin unlocks it.
	for i := 0; i < 4; i++ {
		adminSignIn(t, email, fmt.Sprintf("guess-number-%d-wrong", i), "")
	}
	if r := adminSignIn(t, email, strongPassword, ""); r.status != 429 {
		t.Fatalf("locked after too many wrong passwords: %d %s", r.status, r.raw)
	}
	var locked bool
	pool.QueryRow(context.Background(), `SELECT locked_until > now() FROM admin_accounts WHERE user_id = $1`, id).Scan(&locked)
	if !locked {
		t.Errorf("the lock is kept in the database")
	}
	list := boot.do("GET", "/admin/admins", nil)
	if !strings.Contains(string(list.raw), "\"locked\":true") {
		t.Errorf("the list shows who is locked: %s", list.raw)
	}
	if r := boot.do("PUT", "/admin/admins/"+id, map[string]any{"unlock": true}); r.status != 200 {
		t.Fatalf("unlock: %d %s", r.status, r.raw)
	}
	if l := adminSignIn(t, email, strongPassword, ""); l.status != 200 {
		t.Fatalf("after unlocking: %d %s", l.status, l.raw)
	}
	var failed int
	pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_log WHERE action = 'admin_login_failed' AND details->>'email' = $1`, email).Scan(&failed)
	if failed < 5 {
		t.Errorf("failed sign-ins are recorded: %d", failed)
	}
}

func TestChangingARoleOrDisablingAnAdminTakesEffectAtOnce(t *testing.T) {
	boot := adminClient(t)
	live, email, id := newAdmin(t, boot, "support")
	if x := live.do("GET", "/admin/support/tickets", nil); x.status != 200 {
		t.Fatalf("working session: %d", x.status)
	}
	// Changing the role ends the session (the new role is signed in again); disabling stops everything at once.
	if r := boot.do("PUT", "/admin/admins/"+id, map[string]any{"role": "finance"}); r.status != 200 {
		t.Fatalf("role: %d %s", r.status, r.raw)
	}
	if x := live.do("GET", "/admin/support/tickets", nil); x.status != 401 {
		t.Errorf("a session of the old role ends: %d", x.status)
	}
	l := adminSignIn(t, email, strongPassword, "")
	if l.str("admin_role") != "finance" {
		t.Fatalf("signs in with the new role: %s", l.raw)
	}
	live = adminAs(t, l.str("token"))
	if x := live.do("GET", "/admin/payouts", nil); x.status != 200 {
		t.Fatalf("finance session: %d", x.status)
	}
	if r := boot.do("PUT", "/admin/admins/"+id, map[string]any{"disabled": true}); r.status != 200 {
		t.Fatalf("disable: %d %s", r.status, r.raw)
	}
	if x := live.do("GET", "/admin/payouts", nil); x.status != 401 {
		t.Errorf("a disabled admin's session stops at once: %d", x.status)
	}
	if r := adminSignIn(t, email, strongPassword, ""); r.status != 401 {
		t.Errorf("a disabled admin cannot sign in: %d", r.status)
	}
	if r := boot.do("PUT", "/admin/admins/"+id, map[string]any{"disabled": false}); r.status != 200 {
		t.Fatalf("enable: %d %s", r.status, r.raw)
	}
	if r := adminSignIn(t, email, strongPassword, ""); r.status != 200 {
		t.Errorf("an enabled admin signs in again: %d", r.status)
	}
	var audits int
	pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_log WHERE details->>'admin_id' = $1 AND action IN ('admin_created', 'admin_role_changed', 'admin_disabled', 'admin_enabled')`, id).Scan(&audits)
	if audits != 4 {
		t.Errorf("every change is in the audit log: %d", audits)
	}
}

func TestResettingAnAdminPasswordGivesATemporaryOneAndEndsTheirSessions(t *testing.T) {
	boot := adminClient(t)
	_, email, id := newAdmin(t, boot, "finance")
	time.Sleep(1100 * time.Millisecond)
	l := adminSignIn(t, email, strongPassword, "")
	old := adminAs(t, l.str("token"))
	if x := old.do("GET", "/admin/payouts", nil); x.status != 200 {
		t.Fatalf("working session: %d", x.status)
	}
	time.Sleep(1100 * time.Millisecond)
	rs := boot.do("POST", "/admin/admins/"+id+"/reset-password", nil)
	if rs.status != 200 || len(rs.str("temporary_password")) != 16 {
		t.Fatalf("reset: %d %s", rs.status, rs.raw)
	}
	if x := old.do("GET", "/admin/payouts", nil); x.status != 401 {
		t.Errorf("a reset ends the old sessions: %d", x.status)
	}
	if r := adminSignIn(t, email, strongPassword, ""); r.status != 401 {
		t.Errorf("the old password no longer works: %d", r.status)
	}
	if r := adminSignIn(t, email, rs.str("temporary_password"), ""); r.status != 200 || r.body["must_change_password"] != true {
		t.Errorf("the temporary password works, once: %d %s", r.status, r.raw)
	}
}

func TestTheLastSuperAdminAndOneselfAreProtected(t *testing.T) {
	boot := adminClient(t)
	s1, _, id1 := newAdmin(t, boot, "super")
	// Make this super admin the only active one.
	pool.Exec(context.Background(), `UPDATE admin_accounts SET disabled_at = now(), disabled_by = user_id WHERE role = 'super' AND user_id <> $1 AND disabled_at IS NULL`, id1)
	if r := boot.do("PUT", "/admin/admins/"+id1, map[string]any{"disabled": true}); r.status != 409 {
		t.Errorf("the last active super admin cannot be disabled: %d %s", r.status, r.raw)
	}
	if r := boot.do("PUT", "/admin/admins/"+id1, map[string]any{"role": "support"}); r.status != 409 {
		t.Errorf("or demoted: %d %s", r.status, r.raw)
	}
	if r := s1.do("PUT", "/admin/admins/"+id1, map[string]any{"disabled": true}); r.status != 409 {
		t.Errorf("nobody disables themselves: %d %s", r.status, r.raw)
	}
	// With a second one the first may be demoted.
	_, _, id2 := newAdmin(t, boot, "super")
	if r := boot.do("PUT", "/admin/admins/"+id1, map[string]any{"role": "support"}); r.status != 200 {
		t.Errorf("a super admin can be demoted while another is active: %d %s", r.status, r.raw)
	}
	_ = id2
}

func TestCreatingAdminsIsChecked(t *testing.T) {
	boot := adminClient(t)
	support, _, _ := newAdmin(t, boot, "support")
	guest, guestEmail := supportGuest(t, "adm")
	_ = guest
	ok := fmt.Sprintf("valid-%d@bookdan.test", time.Now().UnixNano())
	for name, body := range map[string]map[string]string{
		"no email":      {"role": "support"},
		"bad email":     {"email": "not-an-email", "role": "support"},
		"no role":       {"email": ok},
		"unknown role":  {"email": ok, "role": "owner"},
		"weak password": {"email": ok, "role": "support", "password": "short"},
		"a guest":       {"email": guestEmail, "role": "support"},
	} {
		if r := boot.do("POST", "/admin/admins", body); r.status == 200 || r.status >= 500 {
			t.Errorf("%s: %d %s", name, r.status, r.raw)
		}
	}
	if r := boot.do("POST", "/admin/admins", map[string]string{"email": ok, "role": "support"}); r.status != 200 {
		t.Fatalf("create: %d %s", r.status, r.raw)
	}
	if r := boot.do("POST", "/admin/admins", map[string]string{"email": ok, "role": "finance"}); r.status != 409 {
		t.Errorf("the same person twice: %d", r.status)
	}
	if r := support.do("POST", "/admin/admins", map[string]string{"email": "x" + ok, "role": "support"}); r.status != 403 {
		t.Errorf("only a super admin adds admins: %d", r.status)
	}
	if r := boot.do("PUT", "/admin/admins/00000000-0000-0000-0000-000000000000", map[string]any{"disabled": true}); r.status != 404 {
		t.Errorf("unknown admin: %d", r.status)
	}
	if r := boot.do("POST", "/admin/admins/not-a-uuid/reset-password", nil); r.status != 404 {
		t.Errorf("garbled id: %d", r.status)
	}
	// The page lists them without any secret.
	list := boot.do("GET", "/admin/admins", nil)
	if list.status != 200 || strings.Contains(string(list.raw), "password") && !strings.Contains(string(list.raw), "must_change_password") {
		t.Errorf("list: %d", list.status)
	}
	if strings.Contains(string(list.raw), "pbkdf2") || strings.Contains(string(list.raw), "totp_secret") {
		t.Errorf("no secrets in the list")
	}
}

func TestAnAuthenticatorAppAddsASecondStep(t *testing.T) {
	boot := adminClient(t)
	c, email, id := newAdmin(t, boot, "support")
	// The shared first-time password has no account to protect.
	if r := boot.do("POST", "/admin/me/mfa/setup", nil); r.status != 409 {
		t.Errorf("the shared password has no account: %d", r.status)
	}
	setup := c.do("POST", "/admin/me/mfa/setup", nil)
	secret := setup.str("secret")
	if setup.status != 200 || secret == "" || !strings.HasPrefix(setup.str("uri"), "otpauth://totp/") || !strings.Contains(setup.str("uri"), secret) {
		t.Fatalf("setup: %d %s", setup.status, setup.raw)
	}
	step := time.Now().Unix() / 30
	if r := c.do("POST", "/admin/me/mfa/enable", map[string]string{"code": "000000"}); r.status != 400 {
		t.Errorf("a wrong code does not turn it on: %d", r.status)
	}
	if r := c.do("POST", "/admin/me/mfa/enable", map[string]string{"code": totpCode(secret, step)}); r.status != 200 {
		t.Fatalf("enable: %d %s", r.status, r.raw)
	}
	if me := c.do("GET", "/admin/me", nil); me.body["mfa"] != true {
		t.Errorf("me shows it is on: %s", me.raw)
	}
	if r := adminSignIn(t, email, strongPassword, ""); r.status != 401 || r.str("error", "code") != "MFA_REQUIRED" {
		t.Errorf("the password alone is no longer enough: %d %s", r.status, r.raw)
	}
	if r := adminSignIn(t, email, strongPassword, "123456"); r.status != 401 || r.str("error", "code") != "MFA_INVALID" {
		t.Errorf("a wrong code: %d %s", r.status, r.raw)
	}
	// The code of the step used to turn it on cannot be used again; the next step's code works once.
	if r := adminSignIn(t, email, strongPassword, totpCode(secret, step)); r.status != 401 {
		t.Errorf("a code cannot be used twice: %d", r.status)
	}
	next := totpCode(secret, step+1)
	if r := adminSignIn(t, email, strongPassword, next); r.status != 200 {
		t.Fatalf("a fresh code signs in: %d %s", r.status, r.raw)
	}
	if r := adminSignIn(t, email, strongPassword, next); r.status != 401 {
		t.Errorf("a used code does not work again: %d", r.status)
	}
	// A super admin removes it for someone who lost their phone.
	if r := boot.do("POST", "/admin/admins/"+id+"/reset-mfa", nil); r.status != 200 {
		t.Fatalf("reset: %d %s", r.status, r.raw)
	}
	if r := adminSignIn(t, email, strongPassword, ""); r.status != 200 {
		t.Errorf("after the reset the password is enough: %d", r.status)
	}

	// Turning it off needs the password and a current code.
	setup = c.do("POST", "/admin/me/mfa/setup", nil)
	secret = setup.str("secret")
	step = time.Now().Unix() / 30
	c.do("POST", "/admin/me/mfa/enable", map[string]string{"code": totpCode(secret, step)})
	if r := c.do("POST", "/admin/me/mfa/disable", map[string]string{"password": "wrong-password-12345", "code": totpCode(secret, step+1)}); r.status != 401 {
		t.Errorf("disable needs the password: %d", r.status)
	}
	if r := c.do("POST", "/admin/me/mfa/disable", map[string]string{"password": strongPassword, "code": totpCode(secret, step+1)}); r.status != 200 {
		t.Errorf("disable: %d %s", r.status, r.raw)
	}
	if me := c.do("GET", "/admin/me", nil); me.body["mfa"] != false {
		t.Errorf("off again: %s", me.raw)
	}
}

func TestOnceAnEmailHasItsOwnAccountTheSharedPasswordNoLongerWorksForIt(t *testing.T) {
	boot := adminClient(t)
	email := "admin@bookdan.test"
	r := boot.do("POST", "/admin/admins", map[string]string{"email": email, "role": "super", "password": strongPassword})
	if r.status != 200 {
		t.Fatalf("create the first admin's own account: %d %s", r.status, r.raw)
	}
	defer pool.Exec(context.Background(), `DELETE FROM admin_accounts WHERE user_id = $1`, r.str("id"))
	if x := adminSignIn(t, email, adminPassword, ""); x.status != 401 {
		t.Errorf("the shared password is refused for an email with its own account: %d %s", x.status, x.raw)
	}
	if x := adminSignIn(t, email, strongPassword, ""); x.status != 200 || x.str("admin_role") != "super" {
		t.Errorf("the account's own password works: %d %s", x.status, x.raw)
	}
}
