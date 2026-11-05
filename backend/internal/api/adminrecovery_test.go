package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/bookdan/backend/internal/api"
	"github.com/bookdan/backend/internal/config"
	"github.com/bookdan/backend/internal/server"
)

var recoveryShape = regexp.MustCompile(`^[A-HJ-NP-Z2-9]{5}-[A-HJ-NP-Z2-9]{5}$`)

// enableMFA turns the authenticator app on for an admin client and returns its secret, the step used, and the recovery codes shown.
func enableMFA(t *testing.T, c *client) (secret string, step int64, codes []string) {
	t.Helper()
	setup := c.do("POST", "/admin/me/mfa/setup", nil)
	secret = setup.str("secret")
	step = time.Now().Unix() / 30
	r := c.do("POST", "/admin/me/mfa/enable", map[string]string{"code": totpCode(secret, step)})
	if r.status != 200 {
		t.Fatalf("enable: %d %s", r.status, r.raw)
	}
	for _, x := range r.body["recovery_codes"].([]any) {
		codes = append(codes, x.(string))
	}
	return secret, step, codes
}

func TestARecoveryCodeSignsInOnceWhenThePhoneIsLost(t *testing.T) {
	boot := adminClient(t)
	c, email, _ := newAdmin(t, boot, "finance")
	secret, step, codes := enableMFA(t, c)

	if len(codes) != 8 {
		t.Fatalf("eight recovery codes are shown: %d", len(codes))
	}
	seen := map[string]bool{}
	for _, code := range codes {
		if !recoveryShape.MatchString(code) || seen[code] {
			t.Fatalf("each code looks like XXXXX-XXXXX and is different: %q", code)
		}
		seen[code] = true
	}
	if me := c.do("GET", "/admin/me", nil); int(me.num("recovery_codes_left")) != 8 {
		t.Errorf("me shows how many are left: %s", me.raw)
	}

	// A code typed by hand (small letters, no dash) signs in, once, and the page is told how many are left.
	typed := strings.ToLower(strings.ReplaceAll(codes[0], "-", ""))
	r := adminSignIn(t, email, strongPassword, typed)
	if r.status != 200 || int(r.num("recovery_codes_left")) != 7 {
		t.Fatalf("a recovery code signs in: %d %s", r.status, r.raw)
	}
	if r := adminSignIn(t, email, strongPassword, codes[0]); r.status != 401 {
		t.Errorf("a recovery code works once: %d", r.status)
	}
	if r := adminSignIn(t, email, strongPassword, "AAAAA-AAAAA"); r.status != 401 {
		t.Errorf("a made-up code: %d", r.status)
	}
	var audits int
	pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_log WHERE action = 'admin_recovery_code_used' AND details->>'email' = $1`, email).Scan(&audits)
	if audits != 1 {
		t.Errorf("using a recovery code is in the audit log: %d", audits)
	}

	// Another admin's code is no good for this account.
	other, otherEmail, _ := newAdmin(t, boot, "support")
	_, _, otherCodes := enableMFA(t, other)
	if r := adminSignIn(t, email, strongPassword, otherCodes[0]); r.status != 401 {
		t.Errorf("a code belongs to one account: %d", r.status)
	}
	if r := adminSignIn(t, otherEmail, strongPassword, otherCodes[0]); r.status != 200 {
		t.Errorf("but works for its own: %d", r.status)
	}

	// New codes need the password and a current authenticator code, and replace the old set.
	if r := c.do("POST", "/admin/me/mfa/recovery-codes", map[string]string{"password": "wrong-password-12345", "code": totpCode(secret, step+1)}); r.status != 401 {
		t.Errorf("renewing needs the password: %d", r.status)
	}
	if r := c.do("POST", "/admin/me/mfa/recovery-codes", map[string]string{"password": strongPassword, "code": "000000"}); r.status != 400 {
		t.Errorf("renewing needs a current code: %d", r.status)
	}
	renew := c.do("POST", "/admin/me/mfa/recovery-codes", map[string]string{"password": strongPassword, "code": totpCode(secret, step+1)})
	if renew.status != 200 || len(renew.body["recovery_codes"].([]any)) != 8 {
		t.Fatalf("renew: %d %s", renew.status, renew.raw)
	}
	if r := adminSignIn(t, email, strongPassword, codes[1]); r.status != 401 {
		t.Errorf("the old set stops working: %d", r.status)
	}
	fresh := renew.body["recovery_codes"].([]any)[0].(string)
	if r := adminSignIn(t, email, strongPassword, fresh); r.status != 200 {
		t.Errorf("the new set works: %d %s", r.status, r.raw)
	}

	// A super admin removing the app (for someone who lost the phone and the codes) removes the codes too.
	var uid string
	pool.QueryRow(context.Background(), `SELECT id::text FROM users WHERE email = $1`, email).Scan(&uid)
	if r := boot.do("POST", "/admin/admins/"+uid+"/reset-mfa", nil); r.status != 200 {
		t.Fatalf("reset: %d %s", r.status, r.raw)
	}
	var left int
	pool.QueryRow(context.Background(), `SELECT count(*) FROM admin_recovery_codes WHERE user_id = $1`, uid).Scan(&left)
	if left != 0 {
		t.Errorf("no codes remain after a reset: %d", left)
	}

	// Turning the app off yourself does the same.
	d, _, _ := newAdmin(t, boot, "finance")
	dSecret, dStep, _ := enableMFA(t, d)
	if r := d.do("POST", "/admin/me/mfa/disable", map[string]string{"password": strongPassword, "code": totpCode(dSecret, dStep+1)}); r.status != 200 {
		t.Fatalf("disable: %d %s", r.status, r.raw)
	}
	if me := d.do("GET", "/admin/me", nil); int(me.num("recovery_codes_left")) != 0 || me.body["mfa"] != false {
		t.Errorf("off, and no codes: %s", me.raw)
	}
}

// adminServerRequiringMFA is the API with ADMIN_REQUIRE_MFA on, on the same database.
func adminServerRequiringMFA(t *testing.T) string {
	t.Helper()
	cfg := config.Config{Env: "test", DemoMode: true, PublicURL: "http://example.test", PaymentProvider: "dev", PaymentWebhookSecret: "whsec", AdminRequireMFA: true,
		AdminPassword: "correct-horse-battery-staple", AdminEmails: []string{"admin@bookdan.test"}}
	a2, err := api.New(context.Background(), cfg, pool, keys, api.DevProviders(cfg))
	if err != nil {
		t.Fatal(err)
	}
	s2 := httptest.NewServer(server.New(cfg, pool, func(r chi.Router) { a2.Routes(r) }, a2.AdminPage))
	t.Cleanup(s2.Close)
	return s2.URL
}

func rawCall(t *testing.T, base, method, path, token string, body any) (int, map[string]any) {
	t.Helper()
	var rd *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	req, _ := http.NewRequest(method, base+"/v1"+path, rd)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, out
}

func TestWhenTheAppIsRequiredAnAccountWithoutItCanOnlySetItUp(t *testing.T) {
	boot := adminClient(t)
	c, email, _ := newAdmin(t, boot, "support")
	base := adminServerRequiringMFA(t)

	status, login := rawCall(t, base, "POST", "/admin/login", "", map[string]string{"email": email, "password": strongPassword})
	if status != 200 || login["must_setup_mfa"] != true {
		t.Fatalf("sign-in says the app must be set up: %d %v", status, login)
	}
	token, _ := login["token"].(string)

	if status, me := rawCall(t, base, "GET", "/admin/me", token, nil); status != 200 || me["must_setup_mfa"] != true {
		t.Fatalf("it can still read who it is: %d %v", status, me)
	}
	for _, path := range []string{"/admin/hotels", "/admin/analytics", "/admin/support/tickets"} {
		if status, body := rawCall(t, base, "GET", path, token, nil); status != 403 {
			t.Errorf("%s must wait for the app: %d %v", path, status, body)
		} else if e, _ := body["error"].(map[string]any); e["code"] != "MFA_SETUP_REQUIRED" {
			t.Errorf("%s: the refusal says why: %v", path, body)
		}
	}
	// Setting it up is allowed, and afterwards everything opens.
	status, setup := rawCall(t, base, "POST", "/admin/me/mfa/setup", token, nil)
	secret, _ := setup["secret"].(string)
	if status != 200 || secret == "" {
		t.Fatalf("setup is allowed: %d %v", status, setup)
	}
	if status, body := rawCall(t, base, "POST", "/admin/me/mfa/enable", token, map[string]string{"code": totpCode(secret, time.Now().Unix()/30)}); status != 200 {
		t.Fatalf("enable: %d %v", status, body)
	}
	if status, body := rawCall(t, base, "GET", "/admin/hotels", token, nil); status != 200 {
		t.Fatalf("with the app on, the page opens: %d %v", status, body)
	}
	_ = c
}

func TestWithoutTheRequirementAccountsWithoutTheAppAreNotBlocked(t *testing.T) {
	boot := adminClient(t)
	c, _, _ := newAdmin(t, boot, "support")
	if r := c.do("GET", "/admin/hotels", nil); r.status != 200 {
		t.Fatalf("the default is that the app is optional: %d %s", r.status, r.raw)
	}
	if me := c.do("GET", "/admin/me", nil); me.body["must_setup_mfa"] != false {
		t.Fatalf("%s", me.raw)
	}
}
