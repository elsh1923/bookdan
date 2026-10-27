package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/bookdan/backend/internal/httpx"
)

// A payment the server cannot settle by itself goes on a list a person works through, and the admins and support are emailed once.
// Today the one kind is an amount mismatch: the gateway says the guest paid, but the amount or currency is not what we asked for. The
// booking is not confirmed (so no room is promised for money that does not match), and nothing is refunded or kept until a person decides.

// raisePaymentIssue records the issue (once per payment and kind) and, the first time, emails the admins and the support address.
func (s *Server) raisePaymentIssue(ctx context.Context, paymentID, bookingID, kind string, details map[string]any) {
	var id string
	err := s.pool.QueryRow(ctx, `INSERT INTO payment_issues (payment_id, booking_id, kind, details) VALUES ($1, $2, $3, $4)
		ON CONFLICT (payment_id, kind) DO NOTHING RETURNING id`, paymentID, bookingID, kind, details).Scan(&id)
	if err == pgx.ErrNoRows {
		return // already raised: nobody is emailed twice
	}
	if err != nil {
		slog.Error("could not record a payment issue", "payment", paymentID, "err", err)
		return
	}
	slog.Error("payment needs a person", "issue", id, "kind", kind, "booking", bookingID)

	var code, guest, hotel string
	_ = s.pool.QueryRow(ctx, `SELECT b.code, b.guest_name, p.name FROM bookings b JOIN properties p ON p.id = b.property_id WHERE b.id = $1`, bookingID).Scan(&code, &guest, &hotel)
	subject := "Payment needs attention: booking " + code
	text := fmt.Sprintf("A guest's payment did not match what was asked, so the booking was NOT confirmed.\n\nBooking: %s\nGuest: %s\nHotel: %s\nAsked: %s\nGateway reports: %s\n\n"+
		"Open the admin page, Payment issues, to see it and mark it resolved. Decide whether to refund the guest or ask for the difference.",
		code, guest, hotel, describeMoney(details["asked_santim"], details["asked_currency"]), describeMoney(details["paid_santim"], details["paid_currency"]))
	for _, to := range s.supportRecipients() {
		if err := s.p.Mail.SendMail(ctx, to, subject, text, ""); err != nil {
			slog.Error("could not email a payment issue", "to", to, "err", err)
		}
	}
}

// supportRecipients are the platform admins and the support address, without repeats.
func (s *Server) supportRecipients() []string {
	seen := map[string]bool{}
	var out []string
	for _, e := range append(append([]string{}, s.cfg.AdminEmails...), s.cfg.SupportEmail) {
		e = strings.ToLower(strings.TrimSpace(e))
		if e != "" && !seen[e] {
			seen[e] = true
			out = append(out, e)
		}
	}
	return out
}

func describeMoney(santim, currency any) string {
	n, _ := santim.(int64)
	if f, ok := santim.(float64); ok {
		n = int64(f)
	}
	cur, _ := currency.(string)
	return fmt.Sprintf("%d.%02d %s", n/100, n%100, cur)
}

type paymentIssueOut struct {
	ID         string         `json:"id"`
	Kind       string         `json:"kind"`
	BookingID  string         `json:"booking_id"`
	Code       string         `json:"booking_code"`
	Guest      string         `json:"guest_name"`
	Hotel      string         `json:"hotel"`
	Details    map[string]any `json:"details"`
	CreatedAt  string         `json:"created_at"`
	ResolvedAt *string        `json:"resolved_at"`
	ResolvedBy *string        `json:"resolved_by"`
	Note       *string        `json:"note"`
}

// adminPaymentIssues lists the open issues first, then the last 50 resolved ones.
func (s *Server) adminPaymentIssues(w http.ResponseWriter, r *http.Request) error {
	if err := requireAdmin(r); err != nil {
		return err
	}
	rows, err := s.pool.Query(r.Context(), `SELECT i.id, i.kind, i.booking_id, b.code, b.guest_name, p.name, i.details, i.created_at, i.resolved_at, i.resolved_by, i.note
		FROM payment_issues i JOIN bookings b ON b.id = i.booking_id JOIN properties p ON p.id = b.property_id
		ORDER BY (i.resolved_at IS NOT NULL), coalesce(i.resolved_at, i.created_at) DESC LIMIT 200`)
	if err != nil {
		return err
	}
	defer rows.Close()
	out := []paymentIssueOut{}
	resolved := 0
	for rows.Next() {
		var i paymentIssueOut
		var created time.Time
		var resolvedAt *time.Time
		if err := rows.Scan(&i.ID, &i.Kind, &i.BookingID, &i.Code, &i.Guest, &i.Hotel, &i.Details, &created, &resolvedAt, &i.ResolvedBy, &i.Note); err != nil {
			return err
		}
		i.CreatedAt = created.UTC().Format(time.RFC3339)
		if resolvedAt != nil {
			if resolved++; resolved > 50 {
				continue
			}
			t := resolvedAt.UTC().Format(time.RFC3339)
			i.ResolvedAt = &t
		}
		out = append(out, i)
	}
	respond(w, http.StatusOK, struct {
		httpx.Meta
		Issues []paymentIssueOut `json:"issues"`
	}{httpx.NowMeta(), out})
	return rows.Err()
}

// adminResolvePaymentIssue marks an issue handled, with a short note (what was decided: refunded, asked for the difference...).
func (s *Server) adminResolvePaymentIssue(w http.ResponseWriter, r *http.Request) error {
	if err := requireAdmin(r); err != nil {
		return err
	}
	var in struct {
		Note string `json:"note"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	note := strings.TrimSpace(in.Note)
	if len(note) < 3 || len(note) > 1000 {
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "Write what was decided (3 to 1000 characters)")
	}
	id, uid := chi.URLParam(r, "id"), claimsFrom(r).Sub
	err := s.tx(r.Context(), func(tx pgx.Tx) error {
		var who string
		if err := tx.QueryRow(r.Context(), `SELECT coalesce(email, phone, id::text) FROM users WHERE id = $1`, uid).Scan(&who); err != nil {
			return err
		}
		tag, err := tx.Exec(r.Context(), `UPDATE payment_issues SET resolved_at = now(), resolved_by = $2, note = $3 WHERE id = $1 AND resolved_at IS NULL`, id, who, note)
		if err != nil {
			if isBadUUID(err) {
				return errNotFound
			}
			return err
		}
		if tag.RowsAffected() == 0 {
			return fail(http.StatusConflict, codeInvalidState, "This issue is already resolved, or does not exist")
		}
		return s.audit(r, tx, "payment_issue_resolved", map[string]any{"issue_id": id})
	})
	if err != nil {
		return err
	}
	respond(w, http.StatusOK, httpx.NowMeta())
	return nil
}
