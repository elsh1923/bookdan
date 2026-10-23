package api_test

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"sync"
	"testing"
)

// otpRecorder stands in for the SMS and email providers: it remembers every code that was "sent".
type otpRecorder struct {
	mu   sync.Mutex
	sent []sentCode
}

type sentCode struct{ channel, to, code string }

func (o *otpRecorder) Send(_ context.Context, channel, to, code string) error {
	if strings.HasPrefix(to, "fail@") {
		return errors.New("mail server down")
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.sent = append(o.sent, sentCode{channel, to, code})
	return nil
}

func (o *otpRecorder) to(addr string) []sentCode {
	o.mu.Lock()
	defer o.mu.Unlock()
	var out []sentCode
	for _, s := range o.sent {
		if s.to == addr {
			out = append(out, s)
		}
	}
	return out
}

func TestRealAddressesGetARealEmailedCodeAndDemoAddressesKeepTheDemoCode(t *testing.T) {
	c := newClient(t, "guest")

	// A real address: a random six-digit code is emailed, and the public demo code does NOT work for it.
	real := "someone." + strings.ReplaceAll(uniqueEmail("r"), "@example.com", "") + "@realmail.dev"
	if r := c.do("POST", "/auth/otp/request", map[string]string{"type": "email", "value": real}); r.status != 200 {
		t.Fatalf("request: %d %s", r.status, r.raw)
	}
	got := mailer.to(real)
	if len(got) != 1 || got[0].channel != "email" || !regexp.MustCompile(`^[0-9]{6}$`).MatchString(got[0].code) {
		t.Fatalf("one six-digit code should have been emailed: %+v", got)
	}
	if got[0].code == "123456" {
		t.Fatalf("a real address must never get the public demo code")
	}
	if r := c.do("POST", "/auth/otp/verify", map[string]string{"type": "email", "value": real, "code": "123456"}); r.status != 401 {
		t.Fatalf("the demo code must not sign in a real address: %d", r.status)
	}
	if r := c.do("POST", "/auth/otp/verify", map[string]string{"type": "email", "value": real, "code": got[0].code}); r.status != 200 {
		t.Fatalf("the emailed code must sign in: %d %s", r.status, r.raw)
	}

	// A demo address (no inbox): the fixed code works and nothing is emailed.
	demo := uniqueEmail("demo")
	c.do("POST", "/auth/otp/request", map[string]string{"type": "email", "value": demo})
	if len(mailer.to(demo)) != 0 {
		t.Fatalf("nothing may be emailed to a demo address")
	}
	if r := c.do("POST", "/auth/otp/verify", map[string]string{"type": "email", "value": demo, "code": "123456"}); r.status != 200 {
		t.Fatalf("the demo code signs in a demo address: %d", r.status)
	}
	// The demo hotel owner is a demo address too: the hotel app keeps working with no inbox behind it.
	h := newClient(t, "partner")
	h.signIn("email", "owner@bookdan.test")

	// A phone number: no SMS provider yet, so the demo code applies and nothing is sent.
	phone := "+251911555" + strings.Repeat("1", 3)
	c.do("POST", "/auth/otp/request", map[string]string{"type": "phone", "value": phone})
	if len(mailer.to(phone)) != 0 {
		t.Fatalf("no SMS is sent without an SMS provider")
	}
	if r := c.do("POST", "/auth/otp/verify", map[string]string{"type": "phone", "value": phone, "code": "123456"}); r.status != 200 {
		t.Fatalf("the demo code signs in a phone number for now: %d", r.status)
	}
}

func TestWhenTheEmailCannotBeSentTheAppIsTold(t *testing.T) {
	c := newClient(t, "guest")
	r := c.do("POST", "/auth/otp/request", map[string]string{"type": "email", "value": "fail@realmail.dev"})
	if r.status != 502 || r.str("error", "code") != "SEND_FAILED" {
		t.Fatalf("a failed send must be reported, not silently accepted: %d %s", r.status, r.raw)
	}
}
