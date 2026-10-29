package api

import (
	"strings"
	"testing"
	"time"
)

func TestPasswordsAreHashedAndCheckedInConstantForm(t *testing.T) {
	h, err := hashPassword("a long private passphrase")
	if err != nil || !strings.HasPrefix(h, "pbkdf2-sha256$600000$") {
		t.Fatalf("hash: %v %q", err, h)
	}
	if strings.Contains(h, "private") {
		t.Fatalf("the hash must not contain the password")
	}
	if !checkPassword("a long private passphrase", h) || checkPassword("a long private passphrasE", h) || checkPassword("", h) {
		t.Errorf("only the right password matches")
	}
	h2, _ := hashPassword("a long private passphrase")
	if h == h2 {
		t.Errorf("each hash has its own salt")
	}
	for _, bad := range []string{"", "plain", "pbkdf2-sha256$600000$x$y", "pbkdf2-sha256$5$c2FsdA$a2V5", "pbkdf2-sha256$99999999$c2FsdA$a2V5", "other$600000$c2FsdA$a2V5"} {
		if checkPassword("a long private passphrase", bad) {
			t.Errorf("a damaged hash never matches: %q", bad)
		}
	}
}

func TestPasswordRules(t *testing.T) {
	for pw, ok := range map[string]bool{
		"short":                        false,
		"elevenchars":                  false,
		"twelve chars":                 true,
		"aaaaaaaaaaaaaaaa":             false,
		"mypassword-for-adaeze-admin":  false, // contains the email name
		"correct horse battery staple": true,
	} {
		if got := passwordProblem(pw, "adaeze@bookdan.test") == ""; got != ok {
			t.Errorf("%q accepted=%v want %v", pw, got, ok)
		}
	}
	if passwordProblem(strings.Repeat("ab", 70), "x@y.test") == "" {
		t.Errorf("a very long password is refused")
	}
	if a, b := tempPassword(), tempPassword(); len(a) != 16 || a == b || strings.ContainsAny(a, "0O1lI") {
		t.Errorf("temporary passwords are random, 16 characters and have no look-alike letters: %q %q", a, b)
	}
}

// RFC 6238 appendix B: the shared secret "12345678901234567890", HMAC-SHA1; at time 59 the code is 94287082 (8 digits), so 287082 in 6.
func TestAuthenticatorCodesMatchTheStandard(t *testing.T) {
	secret := []byte("12345678901234567890")
	for ts, want := range map[int64]string{59: "287082", 1111111109: "081804", 1111111111: "050471", 1234567890: "005924", 2000000000: "279037"} {
		if got := totpAt(secret, ts/30); got != want {
			t.Errorf("t=%d: %s want %s", ts, got, want)
		}
	}
	now := time.Unix(1234567890, 0)
	step := int64(1234567890 / 30)
	code := totpAt(secret, step)
	if s, ok := totpAccept(secret, code, now, 0); !ok || s != step {
		t.Errorf("the current code is accepted: %d %v", s, ok)
	}
	if _, ok := totpAccept(secret, code, now, step); ok {
		t.Errorf("a step already used is refused")
	}
	if _, ok := totpAccept(secret, totpAt(secret, step-1), now, 0); !ok {
		t.Errorf("one step of clock drift is allowed")
	}
	if _, ok := totpAccept(secret, totpAt(secret, step-3), now, 0); ok {
		t.Errorf("an old code is refused")
	}
	for _, bad := range []string{"", "12345", "1234567", "abcdef"} {
		if _, ok := totpAccept(secret, bad, now, 0); ok {
			t.Errorf("%q is not a code", bad)
		}
	}
}
