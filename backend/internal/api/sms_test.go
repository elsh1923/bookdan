package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/bookdan/backend/internal/config"
)

func TestAfricasTalkingSendsTheCodeAndReadsItsAnswer(t *testing.T) {
	var form url.Values
	var key, ctype string
	status := 101
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		form, _ = url.ParseQuery(string(body))
		key, ctype = r.Header.Get("apiKey"), r.Header.Get("Content-Type")
		if key != "atsk_good" {
			w.WriteHeader(http.StatusUnauthorized)
			io.WriteString(w, `The supplied authentication is invalid`)
			return
		}
		w.WriteHeader(http.StatusCreated)
		io.WriteString(w, `{"SMSMessageData":{"Message":"Sent to 1/1","Recipients":[{"statusCode":`+strconv.Itoa(status)+`,"number":"+251911223344","status":"Success"}]}}`)
	}))
	defer srv.Close()

	at, err := newAfricasTalking("bookdan", "atsk_good", "BookDan", false)
	if err != nil {
		t.Fatal(err)
	}
	at.url = srv.URL
	ctx := context.Background()

	if err := at.Send(ctx, "sms", "+251911223344", "482913"); err != nil {
		t.Fatalf("send: %v", err)
	}
	if form.Get("to") != "+251911223344" || form.Get("username") != "bookdan" || form.Get("from") != "BookDan" || ctype != "application/x-www-form-urlencoded" {
		t.Errorf("the request: %v %s", form, ctype)
	}
	if m := form.Get("message"); !strings.Contains(m, "482913") || !strings.Contains(m, "5 minutes") || len(m) > 160 {
		t.Errorf("the message holds the code and the lifetime, in one SMS: %q", m)
	}

	// Queued and processed are also fine; anything else is a failure, and the error never contains the code.
	for _, ok := range []int{100, 102} {
		status = ok
		if err := at.Send(ctx, "sms", "+251911223344", "111222"); err != nil {
			t.Errorf("status %d is on its way: %v", ok, err)
		}
	}
	status = 403
	if err := at.Send(ctx, "sms", "+251911223344", "654321"); err == nil || strings.Contains(err.Error(), "654321") {
		t.Errorf("a refused number is an error without the code: %v", err)
	}
	bad, _ := newAfricasTalking("bookdan", "wrong-key", "", false)
	bad.url = srv.URL
	if err := bad.Send(ctx, "sms", "+251911223344", "654321"); err == nil || strings.Contains(err.Error(), "654321") {
		t.Errorf("a wrong key is an error without the code: %v", err)
	}
	if _, err := newAfricasTalking("", "k", "", false); err == nil {
		t.Errorf("credentials are required")
	}
	if sb, _ := newAfricasTalking("sandbox", "k", "", true); !strings.Contains(sb.url, "sandbox") {
		t.Errorf("the sandbox uses its own address: %s", sb.url)
	}
}

func TestTwilioSendsTheCodeAndReadsItsAnswer(t *testing.T) {
	var form url.Values
	var user, pass, path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		form, _ = url.ParseQuery(string(body))
		var ok bool
		user, pass, ok = r.BasicAuth()
		path = r.URL.Path
		if !ok || pass != "tok_good" {
			w.WriteHeader(http.StatusUnauthorized)
			io.WriteString(w, `{"code":20003,"message":"Authenticate","status":401}`)
			return
		}
		if form.Get("To") == "+251700000000" {
			w.WriteHeader(http.StatusBadRequest)
			io.WriteString(w, `{"code":21211,"message":"The 'To' number is not a valid phone number.","status":400}`)
			return
		}
		w.WriteHeader(http.StatusCreated)
		io.WriteString(w, `{"sid":"SM123","status":"queued"}`)
	}))
	defer srv.Close()

	tw, err := newTwilio("AC123", "tok_good", "+15005550006")
	if err != nil {
		t.Fatal(err)
	}
	tw.url = srv.URL + "/2010-04-01/Accounts/AC123/Messages.json"
	ctx := context.Background()

	if err := tw.Send(ctx, "sms", "+251911223344", "482913"); err != nil {
		t.Fatalf("send: %v", err)
	}
	if user != "AC123" || path != "/2010-04-01/Accounts/AC123/Messages.json" || form.Get("To") != "+251911223344" || form.Get("From") != "+15005550006" || !strings.Contains(form.Get("Body"), "482913") {
		t.Errorf("the request: user %s path %s form %v", user, path, form)
	}
	if err := tw.Send(ctx, "sms", "+251700000000", "999888"); err == nil || !strings.Contains(err.Error(), "21211") || strings.Contains(err.Error(), "999888") {
		t.Errorf("Twilio's reason is kept, the code is not: %v", err)
	}
	badTok, _ := newTwilio("AC123", "wrong", "+15005550006")
	badTok.url = tw.url
	if err := badTok.Send(ctx, "sms", "+251911223344", "999888"); err == nil || strings.Contains(err.Error(), "999888") {
		t.Errorf("wrong credentials: %v", err)
	}
	// A Messaging Service id is sent as such.
	ms, _ := newTwilio("AC123", "tok_good", "MG0123456789")
	ms.url = tw.url
	if err := ms.Send(ctx, "sms", "+251911223344", "111222"); err != nil || form.Get("MessagingServiceSid") != "MG0123456789" || form.Get("From") != "" {
		t.Errorf("messaging service: %v %v", err, form)
	}
	if _, err := newTwilio("AC1", "t", ""); err == nil {
		t.Errorf("a sender is required")
	}
}

func TestSMSSendersRefuseWhatIsNotAnEthiopianMobileNumber(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	defer srv.Close()
	at, _ := newAfricasTalking("bookdan", "k", "", false)
	at.url = srv.URL
	tw, _ := newTwilio("AC1", "t", "+15005550006")
	tw.url = srv.URL
	for _, s := range []OTPSender{at, tw} {
		for _, to := range []string{"+14155550100", "0911223344", "+251111223344", "+251911223344\r\n+14155550100", "", "a@b.com"} {
			if err := s.Send(context.Background(), "sms", to, "123456"); err == nil {
				t.Errorf("%T must refuse %q", s, to)
			}
		}
		if err := s.Send(context.Background(), "email", "a@b.com", "123456"); err == nil {
			t.Errorf("%T must not carry an email code", s)
		}
	}
	if called {
		t.Errorf("a refused number never reaches the provider")
	}
}

func TestTheProviderSettingChoosesTheRealSMSSender(t *testing.T) {
	base := config.Config{}
	p, err := ProvidersFor(base)
	if err != nil || p.SMSReal {
		t.Fatalf("log: %v %v", err, p.SMSReal)
	}
	at := base
	at.OTPProvider, at.ATUsername, at.ATAPIKey = "africastalking", "bookdan", "k"
	if p, err := ProvidersFor(at); err != nil || !p.SMSReal {
		t.Errorf("africastalking: %v %v", err, p.SMSReal)
	}
	tw := base
	tw.OTPProvider, tw.TwilioSID, tw.TwilioToken, tw.SMSFrom = "twilio", "AC1", "t", "+15005550006"
	if p, err := ProvidersFor(tw); err != nil || !p.SMSReal {
		t.Errorf("twilio: %v %v", err, p.SMSReal)
	}
	for _, c := range []config.Config{{OTPProvider: "africastalking"}, {OTPProvider: "twilio"}, {OTPProvider: "carrier-pigeon"}} {
		if _, err := ProvidersFor(c); err == nil {
			t.Errorf("%q without what it needs must not start", c.OTPProvider)
		}
	}
	off := base
	off.OTPProvider = "off"
	if p, err := ProvidersFor(off); err != nil || p.SMSReal || p.OTP.Send(context.Background(), "sms", "+251911223344", "1") == nil {
		t.Errorf("off: phone codes cannot be sent: %v", err)
	}
	// The email sender is kept when a phone provider is chosen too.
	both := tw
	both.EmailProvider, both.BrevoAPIKey, both.EmailFrom = "brevo", "k", "BookDan <a@b.com>"
	if p, err := ProvidersFor(both); err != nil || !p.SMSReal || !p.EmailReal {
		t.Errorf("both real: %v %v %v", err, p.SMSReal, p.EmailReal)
	}
}
