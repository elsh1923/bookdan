package api

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"
)

// smtpSender sends the sign-in code by email through any SMTP service: Gmail (with an app password), Brevo, Mailgun, Amazon SES,
// Resend's SMTP, a company mail server. It speaks plain SMTP over TLS: port 465 is encrypted from the start, any other port
// must offer STARTTLS. Passwords are never sent over an unencrypted link, except to this computer (for tests).
type smtpSender struct {
	host, port, user, password string
	from                       mail.Address
	timeout                    time.Duration
}

// newSMTPSender checks the settings once at startup, so a typo fails the deploy instead of the first sign-in.
func newSMTPSender(host, port, user, password, from string) (*smtpSender, error) {
	if host == "" || user == "" || password == "" {
		return nil, errors.New("EMAIL_PROVIDER=smtp needs SMTP_HOST, SMTP_USER and SMTP_PASSWORD")
	}
	if port == "" {
		port = "587"
	}
	if from == "" {
		from = user
	}
	addr, err := mail.ParseAddress(from)
	if err != nil {
		return nil, fmt.Errorf("EMAIL_FROM is not a valid address (use: BookDan <you@example.com>): %w", err)
	}
	return &smtpSender{host: host, port: port, user: user, password: password, from: *addr, timeout: 10 * time.Second}, nil
}

func (s *smtpSender) Send(ctx context.Context, channel, to, code string) error {
	if channel != "email" {
		return fmt.Errorf("the email sender cannot send a %s message", channel)
	}
	rcpt, err := mail.ParseAddress(to)
	if err != nil || rcpt.Address != to || strings.ContainsAny(to, "\r\n") {
		return errors.New("refusing to send to an invalid address")
	}
	subject, text, html := otpEmailParts(code)
	return s.deliver(ctx, rcpt.Address, s.message(rcpt.Address, subject, text, html))
}

// SendMail sends any email (invitations, application decisions...), plain text plus HTML.
func (s *smtpSender) SendMail(ctx context.Context, to, subject, text, html string) error {
	rcpt, err := mail.ParseAddress(to)
	if err != nil || rcpt.Address != to || strings.ContainsAny(to, "\r\n") {
		return errors.New("refusing to send to an invalid address")
	}
	return s.deliver(ctx, rcpt.Address, s.message(rcpt.Address, subject, text, html))
}

// message builds the email: plain text and an HTML version. Subject and body are encoded so any language (Amharic hotel names) arrives intact.
func (s *smtpSender) message(to, subject, text, html string) []byte {
	id := make([]byte, 12)
	_, _ = rand.Read(id)
	boundary := "bd-" + hex.EncodeToString(id)

	var b strings.Builder
	h := func(k, v string) { b.WriteString(k + ": " + v + "\r\n") }
	h("From", s.from.String())
	h("To", to)
	h("Subject", mime.QEncoding.Encode("utf-8", strings.NewReplacer("\r", " ", "\n", " ").Replace(subject)))
	h("Date", time.Now().UTC().Format(time.RFC1123Z))
	h("Message-ID", "<"+hex.EncodeToString(id)+"@bookdan>")
	h("MIME-Version", "1.0")
	h("Auto-Submitted", "auto-generated")
	h("Content-Type", `multipart/alternative; boundary="`+boundary+`"`)
	b.WriteString("\r\n")
	part := func(contentType, body string) {
		b.WriteString("--" + boundary + "\r\nContent-Type: " + contentType + "; charset=utf-8\r\nContent-Transfer-Encoding: base64\r\n\r\n" + wrap76(base64.StdEncoding.EncodeToString([]byte(body))) + "\r\n")
	}
	part("text/plain", text)
	part("text/html", html)
	b.WriteString("--" + boundary + "--\r\n")
	return []byte(b.String())
}

func (s *smtpSender) deliver(ctx context.Context, to string, msg []byte) error {
	addr := net.JoinHostPort(s.host, s.port)
	dialer := &net.Dialer{Timeout: s.timeout}
	tlsCfg := &tls.Config{ServerName: s.host, MinVersion: tls.VersionTLS12}

	var conn net.Conn
	var err error
	if s.port == "465" {
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, tlsCfg) // encrypted from the first byte
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("connect to the mail server: %w", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(s.timeout))

	c, err := smtp.NewClient(conn, s.host)
	if err != nil {
		return fmt.Errorf("mail server greeting: %w", err)
	}
	defer c.Close()
	encrypted := s.port == "465"
	if !encrypted {
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err := c.StartTLS(tlsCfg); err != nil {
				return fmt.Errorf("start TLS: %w", err)
			}
			encrypted = true
		}
	}
	if !encrypted && !isLoopback(s.host) {
		return errors.New("the mail server does not offer encryption: refusing to send the password")
	}
	if err := c.Auth(smtp.PlainAuth("", s.user, s.password, s.host)); err != nil {
		return fmt.Errorf("mail server login failed (check SMTP_USER and SMTP_PASSWORD): %w", err)
	}
	if err := c.Mail(s.from.Address); err != nil {
		return fmt.Errorf("sender refused: %w", err)
	}
	if err := c.Rcpt(to); err != nil {
		return fmt.Errorf("recipient refused: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// otpEmailParts is the wording of the sign-in email: subject (with the code, so it shows in a notification), plain text and HTML.
func otpEmailParts(code string) (subject, text, html string) {
	subject = fmt.Sprintf("%s is your BookDan sign-in code", code)
	text = fmt.Sprintf("Your BookDan sign-in code is %s\r\n\r\nIt works for 5 minutes. If you did not ask for it, you can ignore this email: nobody can sign in without the code.\r\n", code)
	html = fmt.Sprintf(`<div style="font-family:Arial,sans-serif;max-width:420px;margin:0 auto;padding:24px;color:#0f1b33">
<h2 style="margin:0 0 8px">BookDan</h2><p>Your sign-in code is</p>
<p style="font-size:34px;font-weight:700;letter-spacing:6px;background:#eaf1ff;border-radius:12px;padding:14px;text-align:center;margin:12px 0">%s</p>
<p style="color:#4b5b76;font-size:14px">It works for 5 minutes. If you did not ask for it, you can ignore this email: nobody can sign in without the code.</p></div>`, code)
	return subject, text, html
}

// wrap76 breaks a base64 string into lines of at most 76 characters, as email requires.
func wrap76(s string) string {
	var b strings.Builder
	for len(s) > 76 {
		b.WriteString(s[:76] + "\r\n")
		s = s[76:]
	}
	b.WriteString(s)
	return b.String()
}
