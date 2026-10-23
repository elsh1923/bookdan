package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"strings"
	"time"
)

// brevoSender sends the sign-in code through Brevo's web API (HTTPS). Unlike SMTP it works on hosts that block the mail ports,
// such as Render's free plan. The sender address must be verified in Brevo (Senders & IP, Add a sender).
type brevoSender struct {
	apiKey string
	from   mail.Address
	url    string
	client *http.Client
}

func newBrevoSender(apiKey, from string) (*brevoSender, error) {
	if apiKey == "" {
		return nil, errors.New("EMAIL_PROVIDER=brevo needs BREVO_API_KEY")
	}
	addr, err := mail.ParseAddress(from)
	if err != nil {
		return nil, fmt.Errorf("EMAIL_FROM is not a valid address (use: BookDan <you@example.com>): %w", err)
	}
	return &brevoSender{apiKey: apiKey, from: *addr, url: "https://api.brevo.com/v3/smtp/email", client: &http.Client{Timeout: 10 * time.Second}}, nil
}

func (b *brevoSender) Send(ctx context.Context, channel, to, code string) error {
	if channel != "email" {
		return fmt.Errorf("the email sender cannot send a %s message", channel)
	}
	rcpt, err := mail.ParseAddress(to)
	if err != nil || rcpt.Address != to || strings.ContainsAny(to, "\r\n") {
		return errors.New("refusing to send to an invalid address")
	}
	subject, text, html := otpEmailParts(code)
	body, _ := json.Marshal(map[string]any{
		"sender":      map[string]string{"name": b.from.Name, "email": b.from.Address},
		"to":          []map[string]string{{"email": rcpt.Address}},
		"subject":     subject,
		"textContent": text,
		"htmlContent": html,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("api-key", b.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	res, err := b.client.Do(req)
	if err != nil {
		return fmt.Errorf("connect to Brevo: %w", err)
	}
	defer res.Body.Close()
	resp, _ := io.ReadAll(io.LimitReader(res.Body, 1<<14))
	if res.StatusCode/100 != 2 {
		// Brevo's answer says why (unknown sender, wrong key); it contains no secrets.
		return fmt.Errorf("Brevo refused the email (%d): %s", res.StatusCode, strings.TrimSpace(string(resp)))
	}
	return nil
}
