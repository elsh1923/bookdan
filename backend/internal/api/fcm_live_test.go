package api

import (
	"context"
	"encoding/base64"
	"os"
	"testing"
)

// TestFCMLiveKey checks a real Firebase service-account key against Google. It does nothing unless you point it at the key file:
//
//	set FCM_LIVE_SERVICE_ACCOUNT_FILE=C:\path\to\key.json   (PowerShell: $env:FCM_LIVE_SERVICE_ACCOUNT_FILE="...")
//	go test ./internal/api -run TestFCMLiveKey -v
//
// It sends one message to a made-up phone token. A working setup answers "that phone is not registered", which proves the key
// signs in and the Cloud Messaging API is enabled. Access-denied errors mean the key or the project settings are wrong.
func TestFCMLiveKey(t *testing.T) {
	path := os.Getenv("FCM_LIVE_SERVICE_ACCOUNT_FILE")
	if path == "" {
		t.Skip("set FCM_LIVE_SERVICE_ACCOUNT_FILE to run this against Firebase")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	p, err := newFCMPusher(base64.StdEncoding.EncodeToString(raw))
	if err != nil {
		t.Fatalf("the key file is not usable: %v", err)
	}
	invalid, err := p.Notify(context.Background(), []string{"not-a-real-phone-token"}, "BookDan check", "This is a test of the push setup.", map[string]string{"type": "check"})
	if err != nil {
		t.Fatalf("Firebase refused the message (key, permissions or the Cloud Messaging API): %v", err)
	}
	if len(invalid) != 1 {
		t.Fatalf("expected Firebase to say the fake phone is unknown, got %v", invalid)
	}
	t.Logf("OK: signed in to Google as project %q and Firebase answered about the fake phone, so real messages can be sent", p.projectID)
}
