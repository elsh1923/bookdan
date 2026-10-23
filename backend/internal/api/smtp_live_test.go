package api

import (
	"context"
	"os"
	"testing"

	"github.com/joho/godotenv"

	"github.com/bookdan/backend/internal/config"
)

// TestSMTPLive sends one real sign-in email using the SMTP settings in backend/.env. It does nothing unless you say where to send:
//
//	PowerShell:  $env:EMAIL_TEST_TO="you@gmail.com"; go test ./internal/api -run TestSMTPLive -v
//
// A pass means the account, password and port work. Check your inbox (and the spam folder).
func TestSMTPLive(t *testing.T) {
	to := os.Getenv("EMAIL_TEST_TO")
	if to == "" {
		t.Skip("set EMAIL_TEST_TO to the address that should receive the test email")
	}
	_ = godotenv.Load("../../.env")
	cfg := config.Load()
	s, err := newSMTPSender(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUser, cfg.SMTPPassword, cfg.EmailFrom)
	if err != nil {
		t.Fatalf("settings in .env: %v", err)
	}
	if err := s.Send(context.Background(), "email", to, "246810"); err != nil {
		t.Fatalf("sending failed: %v", err)
	}
	t.Logf("OK: an email with the test code 246810 was handed to %s:%s for %s. Check the inbox and the spam folder.", cfg.SMTPHost, cfg.SMTPPort, to)
}
