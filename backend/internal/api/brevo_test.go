package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/joho/godotenv"

	"github.com/bookdan/backend/internal/config"
)

func TestBrevoSendsTheCodeThroughItsWebAPI(t *testing.T) {
	var got map[string]any
	var key string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		key = r.Header.Get("api-key")
		json.Unmarshal(body, &got)
		if key != "xkeysib-good" {
			w.WriteHeader(http.StatusUnauthorized)
			io.WriteString(w, `{"code":"unauthorized","message":"Key not found"}`)
			return
		}
		if strings.Contains(string(body), "unknown@") {
			w.WriteHeader(http.StatusBadRequest)
			io.WriteString(w, `{"code":"invalid_parameter","message":"sender not verified"}`)
			return
		}
		w.WriteHeader(http.StatusCreated)
		io.WriteString(w, `{"messageId":"<1@brevo>"}`)
	}))
	defer srv.Close()

	b, err := newBrevoSender("xkeysib-good", "BookDan <bookdan.noreply@gmail.com>")
	if err != nil {
		t.Fatal(err)
	}
	b.url = srv.URL
	if err := b.Send(context.Background(), "email", "guest@mailbox.test", "482915"); err != nil {
		t.Fatal(err)
	}
	sender := got["sender"].(map[string]any)
	to := got["to"].([]any)[0].(map[string]any)
	if key != "xkeysib-good" || sender["email"] != "bookdan.noreply@gmail.com" || sender["name"] != "BookDan" || to["email"] != "guest@mailbox.test" {
		t.Fatalf("wrong request: %v", got)
	}
	if !strings.Contains(got["subject"].(string), "482915") || !strings.Contains(got["htmlContent"].(string), "482915") || !strings.Contains(got["textContent"].(string), "482915") {
		t.Fatalf("the code must be in the subject, the text and the HTML: %v", got)
	}

	// Brevo's reason is passed on (it contains no secrets), a wrong key is clear, and bad recipients never leave this server.
	bad, _ := newBrevoSender("wrong-key", "BookDan <bookdan.noreply@gmail.com>")
	bad.url = srv.URL
	if err := bad.Send(context.Background(), "email", "guest@mailbox.test", "111111"); err == nil || !strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "wrong-key") {
		t.Fatalf("a refused key must be reported without echoing it: %v", err)
	}
	for _, addr := range []string{"a@b.test\r\nBcc: x@y.test", "Name <a@b.test>", "nope", ""} {
		if err := b.Send(context.Background(), "email", addr, "111111"); err == nil {
			t.Errorf("%q must be refused", addr)
		}
	}
	if err := b.Send(context.Background(), "sms", "+251911000111", "111111"); err == nil {
		t.Errorf("the email sender must not claim to send an SMS")
	}
	if _, err := newBrevoSender("", "BookDan <a@b.com>"); err == nil {
		t.Errorf("a missing API key must be refused at startup")
	}
	if _, err := newBrevoSender("k", "not an address <<"); err == nil {
		t.Errorf("a bad EMAIL_FROM must be refused at startup")
	}
}

// TestBrevoLive sends one real email through Brevo using BREVO_API_KEY and EMAIL_FROM from backend/.env. It does nothing unless you say where to send:
//
//	PowerShell:  $env:EMAIL_TEST_TO="you@gmail.com"; go test ./internal/api -run TestBrevoLive -v
func TestBrevoLive(t *testing.T) {
	to := os.Getenv("EMAIL_TEST_TO")
	if to == "" {
		t.Skip("set EMAIL_TEST_TO to the address that should receive the test email")
	}
	_ = godotenv.Load("../../.env")
	cfg := config.Load()
	b, err := newBrevoSender(cfg.BrevoAPIKey, cfg.EmailFrom)
	if err != nil {
		t.Fatalf("settings in .env: %v", err)
	}
	if err := b.Send(context.Background(), "email", to, "135790"); err != nil {
		t.Fatalf("sending failed: %v", err)
	}
	t.Logf("OK: Brevo accepted the email with the test code 135790 for %s. Check the inbox and the spam folder.", to)
}
