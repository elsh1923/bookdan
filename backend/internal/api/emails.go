package api

import (
	"context"
	"fmt"
	"html"
	"log/slog"
	"net/mail"
	"strings"
)

// emailContent is one message: subject, plain text and HTML.
type emailContent struct{ subject, text, html string }

// frame wraps the HTML body of an email in the BookDan look.
func frame(title, body string) string {
	return `<div style="font-family:Arial,sans-serif;max-width:520px;margin:0 auto;padding:24px;color:#0f1b33;line-height:1.5">` +
		`<h2 style="margin:0 0 4px;color:#1d4ed8">BookDan</h2><h3 style="margin:0 0 16px">` + html.EscapeString(title) + `</h3>` + body +
		`<p style="color:#7b8aa3;font-size:12px;margin-top:28px">You received this email because of an action on BookDan. If it was not you, you can ignore it.</p></div>`
}

func esc(s string) string { return html.EscapeString(s) }

func para(s string) string { return `<p>` + esc(s) + `</p>` }

func button(url, label string) string {
	return `<p><a href="` + esc(url) + `" style="display:inline-block;background:#2563eb;color:#fff;text-decoration:none;font-weight:bold;padding:12px 20px;border-radius:10px">` + esc(label) + `</a></p>`
}

func (s *Server) staffInvitedEmail(role, hotel, email string) emailContent {
	what := "answer booking requests for"
	if role == "owner" {
		what = "manage and answer booking requests for"
	}
	install := "Install the BookDan Partner app (we will send you the download link)."
	if s.cfg.PartnerAppURL != "" {
		install = "Install the BookDan Partner app: " + s.cfg.PartnerAppURL
	}
	text := fmt.Sprintf("You were added to %q on BookDan and can now %s it.\n\n1. %s\n2. Open it and sign in with this email address (%s). We send you a code each time.\n\nBookDan\n", hotel, what, install, email)
	body := para("You were added to \""+hotel+"\" on BookDan and can now "+what+" it.") + `<ol><li>` + esc(install) + `</li><li>` + esc("Open it and sign in with this email address ("+email+"). We send you a code each time.") + `</li></ol>`
	return emailContent{"You were added to " + hotel + " on BookDan", text, frame("You were added to a hotel", body)}
}

// isDemoAddress reports whether an address is one of the demo domains that have no inbox (only while demo mode is on).
func (s *Server) isDemoAddress(addr string) bool {
	if !s.cfg.DemoMode {
		return false
	}
	at := strings.LastIndex(addr, "@")
	for _, d := range s.cfg.DemoAccountDomains {
		if addr[at+1:] == d {
			return true
		}
	}
	return false
}

// sendMail sends an email. It never fails the action that caused it (a booking approval must not be undone because the mail server
// had a bad minute): problems are logged. Demo addresses and a missing email provider only log, so nothing is sent to nobody.
func (s *Server) sendMail(ctx context.Context, to string, c emailContent) {
	addr, err := mail.ParseAddress(to)
	if err != nil || addr.Address != to {
		slog.Warn("email not sent: invalid address")
		return
	}
	if !s.p.EmailReal || s.isDemoAddress(to) || s.p.Mail == nil {
		slog.Info("email (not sent: demo address or no email provider)", "to", to, "subject", c.subject)
		return
	}
	if err := s.p.Mail.SendMail(ctx, to, c.subject, c.text, c.html); err != nil {
		slog.Error("sending an email failed", "subject", c.subject, "err", err)
	}
}

// ownerAdminEnabled reports whether hotel owners may use the admin page: it is switched on, and sign-in codes are real
// (a fixed demo code would let anyone in).
func (s *Server) ownerAdminEnabled() bool {
	return s.cfg.AdminOwners && (!s.cfg.DemoMode || s.p.EmailReal)
}
