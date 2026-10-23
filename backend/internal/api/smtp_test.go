package api

import (
	"bufio"
	"context"
	"encoding/base64"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/mail"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeSMTP is a tiny mail server on this computer: it checks the login, collects what is sent and can refuse a recipient.
type fakeSMTP struct {
	ln       net.Listener
	mu       sync.Mutex
	mails    []string // full text of each message received
	froms    []string
	rcpts    []string
	user     string
	password string
}

func startFakeSMTP(t *testing.T, user, password string) *fakeSMTP {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeSMTP{ln: ln, user: user, password: password}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go f.serve(c)
		}
	}()
	t.Cleanup(func() { ln.Close() })
	return f
}

func (f *fakeSMTP) port() string { _, p, _ := net.SplitHostPort(f.ln.Addr().String()); return p }

func (f *fakeSMTP) serve(c net.Conn) {
	defer c.Close()
	rd := bufio.NewReader(c)
	say := func(s string) { io.WriteString(c, s+"\r\n") }
	say("220 fake.smtp ready")
	authed := false
	for {
		line, err := rd.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		up := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(up, "EHLO"), strings.HasPrefix(up, "HELO"):
			say("250-fake.smtp")
			say("250 AUTH PLAIN")
		case strings.HasPrefix(up, "AUTH PLAIN"):
			parts := strings.Fields(line)
			raw, _ := base64.StdEncoding.DecodeString(parts[len(parts)-1])
			fields := strings.Split(string(raw), "\x00") // identity, user, password
			if len(fields) == 3 && fields[1] == f.user && fields[2] == f.password {
				authed = true
				say("235 2.7.0 accepted")
			} else {
				say("535 5.7.8 bad credentials")
			}
		case strings.HasPrefix(up, "MAIL FROM:"):
			if !authed {
				say("530 5.7.0 authentication required")
				continue
			}
			f.mu.Lock()
			f.froms = append(f.froms, line)
			f.mu.Unlock()
			say("250 ok")
		case strings.HasPrefix(up, "RCPT TO:"):
			f.mu.Lock()
			f.rcpts = append(f.rcpts, line)
			f.mu.Unlock()
			if strings.Contains(line, "nobody@") {
				say("550 5.1.1 no such mailbox")
			} else {
				say("250 ok")
			}
		case up == "DATA":
			say("354 go ahead")
			var b strings.Builder
			for {
				l, err := rd.ReadString('\n')
				if err != nil || l == ".\r\n" {
					break
				}
				b.WriteString(l)
			}
			f.mu.Lock()
			f.mails = append(f.mails, b.String())
			f.mu.Unlock()
			say("250 queued")
		case up == "QUIT":
			say("221 bye")
			return
		default:
			say("250 ok")
		}
	}
}

func newTestSender(t *testing.T, f *fakeSMTP, password string) *smtpSender {
	t.Helper()
	s, err := newSMTPSender("127.0.0.1", f.port(), "bookdan@gmail.test", password, "BookDan <bookdan@gmail.test>")
	if err != nil {
		t.Fatal(err)
	}
	s.timeout = 5 * time.Second
	return s
}

func TestEmailCodeIsDeliveredWithTheCodeInSubjectAndBody(t *testing.T) {
	f := startFakeSMTP(t, "bookdan@gmail.test", "app-password")
	s := newTestSender(t, f, "app-password")
	if err := s.Send(context.Background(), "email", "guest@mailbox.test", "482915"); err != nil {
		t.Fatal(err)
	}
	if len(f.mails) != 1 {
		t.Fatalf("expected one email, got %d", len(f.mails))
	}
	m := decodeMail(t, f.mails[0])
	for _, want := range []string{
		"Subject: 482915 is your BookDan sign-in code", "To: guest@mailbox.test", "From: \"BookDan\" <bookdan@gmail.test>",
		"Your BookDan sign-in code is 482915", "<p style=",
	} {
		if !strings.Contains(m, want) {
			t.Errorf("the email is missing %q:\n%s", want, m)
		}
	}
	if !strings.Contains(f.froms[0], "<bookdan@gmail.test>") || !strings.Contains(f.rcpts[0], "<guest@mailbox.test>") {
		t.Errorf("envelope wrong: %v %v", f.froms, f.rcpts)
	}
}

func TestEmailFailuresAreClearAndSafe(t *testing.T) {
	f := startFakeSMTP(t, "bookdan@gmail.test", "app-password")

	if err := newTestSender(t, f, "wrong").Send(context.Background(), "email", "guest@mailbox.test", "111111"); err == nil || !strings.Contains(err.Error(), "login failed") {
		t.Fatalf("a wrong password must say the login failed, got %v", err)
	}
	good := newTestSender(t, f, "app-password")
	if err := good.Send(context.Background(), "email", "nobody@mailbox.test", "111111"); err == nil || !strings.Contains(err.Error(), "recipient refused") {
		t.Fatalf("a refused recipient must be reported, got %v", err)
	}
	// Anything that could smuggle extra headers or recipients is refused before a connection is even made.
	for _, bad := range []string{"a@b.test\r\nBcc: x@y.test", "a@b.test\nSubject: hi", "Name <a@b.test>", "not-an-address", ""} {
		if err := good.Send(context.Background(), "email", bad, "111111"); err == nil {
			t.Errorf("%q must be refused", bad)
		}
	}
	if err := good.Send(context.Background(), "sms", "+251911000111", "111111"); err == nil {
		t.Errorf("the email sender must not claim to send an SMS")
	}
	// A server that is not there fails quickly instead of hanging the request.
	dead := *good
	dead.port = "1"
	dead.timeout = time.Second
	start := time.Now()
	if err := dead.Send(context.Background(), "email", "guest@mailbox.test", "111111"); err == nil || time.Since(start) > 4*time.Second {
		t.Fatalf("a dead mail server must fail fast: %v after %v", err, time.Since(start))
	}
	if len(f.mails) != 0 {
		t.Fatalf("no email should have been delivered in these failures, got %d", len(f.mails))
	}
}

func TestEmailSettingsAreCheckedAtStartup(t *testing.T) {
	for name, args := range map[string][5]string{
		"no host":     {"", "587", "u", "p", "u@x.com"},
		"no user":     {"smtp.x.com", "587", "", "p", "u@x.com"},
		"no password": {"smtp.x.com", "587", "u", "", "u@x.com"},
		"bad from":    {"smtp.x.com", "587", "u", "p", "not an address <<"},
	} {
		if _, err := newSMTPSender(args[0], args[1], args[2], args[3], args[4]); err == nil {
			t.Errorf("%s must be refused at startup", name)
		}
	}
	s, err := newSMTPSender("smtp.gmail.com", "", "me@gmail.com", "pw", "")
	if err != nil || s.port != "587" || s.from.Address != "me@gmail.com" {
		t.Fatalf("defaults: port 587 and From = the login. got %+v err %v", s, err)
	}
	if !isLoopback("localhost") || !isLoopback("127.0.0.1") || isLoopback("smtp.gmail.com") || isLoopback("10.0.0.5") {
		t.Fatalf("only this computer may be reached without encryption")
	}
}

// decodeMail turns the raw message the fake server received into readable text: decoded subject and the decoded text and HTML parts.
func decodeMail(t *testing.T, raw string) string {
	t.Helper()
	msg, err := mail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	dec := new(mime.WordDecoder)
	subject, _ := dec.DecodeHeader(msg.Header.Get("Subject"))
	out := "Subject: " + subject + "\nTo: " + msg.Header.Get("To") + "\nFrom: " + msg.Header.Get("From") + "\n"
	_, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil {
		t.Fatal(err)
	}
	mr := multipart.NewReader(msg.Body, params["boundary"])
	for {
		p, err := mr.NextPart()
		if err != nil {
			break
		}
		b, _ := io.ReadAll(base64.NewDecoder(base64.StdEncoding, p))
		out += string(b) + "\n"
	}
	return out
}
