package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/bookdan/backend/internal/httpx"
)

// Support: a guest writes to the platform and the platform's staff (the admins) answer. One ticket is one conversation, optionally about a
// booking. A ticket is "open" while it waits for support, "answered" while it waits for the guest, and "closed" when finished. A guest's reply
// reopens an answered ticket. The admins and SUPPORT_EMAIL are emailed about every new ticket and every guest reply; the guest is emailed
// when support answers.

const (
	maxOpenTickets = 5 // a guest cannot flood the queue
	maxSubject     = 120
	maxMessage     = 2000
)

var ticketKinds = map[string]bool{"question": true, "problem": true}

type ticketMessage struct {
	FromStaff bool `json:"from_staff"`
	// Role says who wrote it: guest, hotel or support.
	Role      string `json:"role"`
	Author    string `json:"author"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
}

// ticketBooking is what an admin needs to settle a dispute about a booking.
type ticketBooking struct {
	Status       string  `json:"status"`
	DisputedFrom *string `json:"disputed_from"`
	Hotel        string  `json:"hotel"`
	CheckIn      string  `json:"check_in"`
	CheckOut     string  `json:"check_out"`
	PaidSantim   int64   `json:"paid_santim"`
	// RefundedSantim is what was already returned in part; LeftSantim is what can still be refunded.
	RefundedSantim int64  `json:"refunded_santim"`
	LeftSantim     int64  `json:"left_santim"`
	Currency       string `json:"currency"`
}

type ticketOut struct {
	ID          string          `json:"id"`
	Kind        string          `json:"kind"`
	Subject     string          `json:"subject"`
	Status      string          `json:"status"`
	BookingID   *string         `json:"booking_id"`
	BookingCode *string         `json:"booking_code"`
	CreatedAt   string          `json:"created_at"`
	UpdatedAt   string          `json:"updated_at"`
	Preview     string          `json:"preview,omitempty"`
	Guest       string          `json:"guest,omitempty"`   // admin view only
	Hotel       string          `json:"hotel,omitempty"`   // set when the request comes from a hotel's staff (admin view)
	Reason      *string         `json:"reason,omitempty"`  // disputes: why the guest opened it
	Outcome     *string         `json:"outcome,omitempty"` // disputes: how it ended
	Booking     *ticketBooking  `json:"booking,omitempty"` // admin view only: the booking a ticket is about
	Messages    []ticketMessage `json:"messages,omitempty"`
}

func cleanText(v string, min, max int) (string, bool) {
	v = strings.TrimSpace(v)
	n := utf8.RuneCountInString(v)
	return v, n >= min && n <= max
}

// ---- guest

func (s *Server) createTicket(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Kind      string `json:"kind"`
		Subject   string `json:"subject"`
		Message   string `json:"message"`
		BookingID string `json:"booking_id"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	bad := func(m string) error { return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, m) }
	if !ticketKinds[in.Kind] {
		return bad("kind must be question or problem")
	}
	subject, ok := cleanText(in.Subject, 3, maxSubject)
	if !ok {
		return bad(fmt.Sprintf("The subject must be 3 to %d characters", maxSubject))
	}
	message, ok := cleanText(in.Message, 1, maxMessage)
	if !ok {
		return bad(fmt.Sprintf("Write your message (at most %d characters)", maxMessage))
	}
	ctx, uid := r.Context(), claimsFrom(r).Sub
	id, err := s.openTicket(ctx, r, uid, in.Kind, subject, message, in.BookingID, claimsFrom(r).App == "partner")
	if err != nil {
		return err
	}
	respond(w, http.StatusOK, struct {
		httpx.Meta
		ID string `json:"id"`
	}{httpx.NowMeta(), id})
	return nil
}

// openTicket creates a ticket with its first message and tells support. bookingID may be empty; when it is not, it must be the guest's own.
// For hotel staff (asHotel) the booking must be one of their hotels' and the request is marked with that hotel (the first of theirs when it is not about a booking).
func (s *Server) openTicket(ctx context.Context, r *http.Request, uid, kind, subject, message, bookingID string, asHotel bool) (string, error) {
	var id string
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var booking, property any
		if asHotel {
			propIDs, _, err := s.staffHotels(ctx, uid)
			if err != nil {
				return err
			}
			property = propIDs[0]
			if bookingID != "" {
				var prop string
				if err := tx.QueryRow(ctx, `SELECT property_id::text FROM bookings WHERE id = $1 AND property_id = ANY($2::uuid[]) AND status <> 'HELD'`, bookingID, propIDs).Scan(&prop); err != nil {
					if err == pgx.ErrNoRows || isBadUUID(err) {
						return errNotFound
					}
					return err
				}
				booking, property = bookingID, prop
			}
		} else if bookingID != "" {
			var ok bool
			if err := tx.QueryRow(ctx, `SELECT true FROM bookings WHERE id = $1 AND user_id = $2`, bookingID, uid).Scan(&ok); err != nil {
				if err == pgx.ErrNoRows || isBadUUID(err) {
					return errNotFound
				}
				return err
			}
			booking = bookingID
		}
		// Serialise a guest's own tickets so two quick requests cannot both pass the limit.
		if _, err := tx.Exec(ctx, `SELECT 1 FROM users WHERE id = $1 FOR UPDATE`, uid); err != nil {
			return err
		}
		var open int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM support_tickets WHERE user_id = $1 AND status <> 'closed'`, uid).Scan(&open); err != nil {
			return err
		}
		if open >= maxOpenTickets {
			return fail(http.StatusConflict, "TOO_MANY_TICKETS", "You have several requests waiting for an answer. Wait for a reply, or close one you no longer need.")
		}
		if err := tx.QueryRow(ctx, `INSERT INTO support_tickets (user_id, booking_id, kind, subject, property_id) VALUES ($1, $2, $3, $4, $5) RETURNING id`, uid, booking, kind, subject, property).Scan(&id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO support_messages (ticket_id, from_staff, author, body) VALUES ($1, false, 'guest', $2)`, id, message); err != nil {
			return err
		}
		return s.audit(r, tx, "support_ticket_opened", map[string]any{"ticket_id": id, "kind": kind})
	})
	if err != nil {
		return "", err
	}
	s.mailSupport(ctx, id, "New support request")
	return id, nil
}

// mailSupport emails the admins and the support address a link-free summary of a ticket; the admin page has the conversation.
func (s *Server) mailSupport(ctx context.Context, ticketID, headline string) {
	var kind, subject, guest, body string
	var code *string
	err := s.pool.QueryRow(ctx, `SELECT t.kind, t.subject, coalesce(u.email, u.phone, ''), b.code,
		(SELECT body FROM support_messages m WHERE m.ticket_id = t.id ORDER BY m.created_at DESC LIMIT 1)
		FROM support_tickets t JOIN users u ON u.id = t.user_id LEFT JOIN bookings b ON b.id = t.booking_id WHERE t.id = $1`, ticketID).Scan(&kind, &subject, &guest, &code, &body)
	if err != nil {
		slog.Error("support mail: read ticket", "ticket", ticketID, "err", err)
		return
	}
	about := ""
	if code != nil {
		about = "Booking: " + *code + "\n"
	}
	text := fmt.Sprintf("%s (%s)\n\nFrom: %s\n%sSubject: %s\n\n%s\n\nAnswer it in the admin page, under Support.", headline, kind, guest, about, subject, body)
	for _, to := range s.supportRecipients() {
		if err := s.p.Mail.SendMail(ctx, to, headline+": "+subject, text, ""); err != nil {
			slog.Error("support mail: send", "to", to, "err", err)
		}
	}
}

func (s *Server) listTickets(w http.ResponseWriter, r *http.Request) error {
	rows, err := s.pool.Query(r.Context(), `SELECT t.id, t.kind, t.subject, t.status, t.booking_id, b.code, t.created_at, t.updated_at,
		coalesce((SELECT left(body, 140) FROM support_messages m WHERE m.ticket_id = t.id ORDER BY m.created_at DESC LIMIT 1), '')
		FROM support_tickets t LEFT JOIN bookings b ON b.id = t.booking_id WHERE t.user_id = $1 ORDER BY t.updated_at DESC LIMIT 50`, claimsFrom(r).Sub)
	if err != nil {
		return err
	}
	defer rows.Close()
	out := []ticketOut{}
	for rows.Next() {
		t, err := scanTicket(rows)
		if err != nil {
			return err
		}
		out = append(out, t)
	}
	respond(w, http.StatusOK, struct {
		httpx.Meta
		Tickets []ticketOut `json:"tickets"`
	}{httpx.NowMeta(), out})
	return rows.Err()
}

func scanTicket(rows pgx.Rows) (ticketOut, error) {
	var t ticketOut
	var created, updated time.Time
	if err := rows.Scan(&t.ID, &t.Kind, &t.Subject, &t.Status, &t.BookingID, &t.BookingCode, &created, &updated, &t.Preview); err != nil {
		return t, err
	}
	t.CreatedAt, t.UpdatedAt = created.UTC().Format(time.RFC3339), updated.UTC().Format(time.RFC3339)
	return t, nil
}

// readTicket loads one ticket with its messages. userID limits it to a guest's own; empty means any (admin).
func (s *Server) readTicket(ctx context.Context, id, userID string) (ticketOut, error) {
	var t ticketOut
	var created, updated time.Time
	err := s.pool.QueryRow(ctx, `SELECT t.id, t.kind, t.subject, t.status, t.booking_id, b.code, t.created_at, t.updated_at, coalesce(u.email, u.phone, ''), t.reason, t.outcome, coalesce(hp.name, '')
		FROM support_tickets t JOIN users u ON u.id = t.user_id LEFT JOIN bookings b ON b.id = t.booking_id LEFT JOIN properties hp ON hp.id = t.property_id
		WHERE t.id = $1 AND ($2 = '' OR t.user_id::text = $2)`, id, userID).Scan(&t.ID, &t.Kind, &t.Subject, &t.Status, &t.BookingID, &t.BookingCode, &created, &updated, &t.Guest, &t.Reason, &t.Outcome, &t.Hotel)
	if err == pgx.ErrNoRows || isBadUUID(err) {
		return t, errNotFound
	}
	if err != nil {
		return t, err
	}
	t.CreatedAt, t.UpdatedAt = created.UTC().Format(time.RFC3339), updated.UTC().Format(time.RFC3339)
	rows, err := s.pool.Query(ctx, `SELECT from_staff, author, body, created_at FROM support_messages WHERE ticket_id = $1 ORDER BY created_at, id`, id)
	if err != nil {
		return t, err
	}
	defer rows.Close()
	t.Messages = []ticketMessage{}
	for rows.Next() {
		var m ticketMessage
		var at time.Time
		if err := rows.Scan(&m.FromStaff, &m.Author, &m.Body, &at); err != nil {
			return t, err
		}
		m.CreatedAt = at.UTC().Format(time.RFC3339)
		m.Role = roleOf(m.Author)
		t.Messages = append(t.Messages, m)
	}
	return t, rows.Err()
}

func (s *Server) getTicket(w http.ResponseWriter, r *http.Request) error {
	t, err := s.readTicket(r.Context(), chi.URLParam(r, "id"), claimsFrom(r).Sub)
	if err != nil {
		return err
	}
	t.Guest = "" // a guest does not need their own address echoed
	// A staff member's address is not the guest's business: show "Support".
	for i := range t.Messages {
		switch t.Messages[i].Role {
		case "support":
			t.Messages[i].Author = "Support"
		case "hotel":
			t.Messages[i].Author = "Hotel"
		}
	}
	respond(w, http.StatusOK, struct {
		httpx.Meta
		Ticket ticketOut `json:"ticket"`
	}{httpx.NowMeta(), t})
	return nil
}

// addMessage appends a message to a ticket and moves its status. fromStaff picks who wrote it; the ticket must be open or answered.
func (s *Server) addMessage(ctx context.Context, r *http.Request, id, userID, author string, fromStaff bool, body string) error {
	return s.tx(ctx, func(tx pgx.Tx) error {
		var status string
		err := tx.QueryRow(ctx, `SELECT status FROM support_tickets WHERE id = $1 AND ($2 = '' OR user_id::text = $2) FOR UPDATE`, id, userID).Scan(&status)
		if err == pgx.ErrNoRows || isBadUUID(err) {
			return errNotFound
		}
		if err != nil {
			return err
		}
		if status == "closed" {
			return fail(http.StatusConflict, codeInvalidState, "This request is closed. Start a new one if you still need help.")
		}
		if _, err := tx.Exec(ctx, `INSERT INTO support_messages (ticket_id, from_staff, author, body) VALUES ($1, $2, $3, $4)`, id, fromStaff, author, body); err != nil {
			return err
		}
		next := "open"
		if fromStaff {
			next = "answered"
		}
		_, err = tx.Exec(ctx, `UPDATE support_tickets SET status = $2, updated_at = now() WHERE id = $1`, id, next)
		return err
	})
}

func (s *Server) replyTicket(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Message string `json:"message"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	message, ok := cleanText(in.Message, 1, maxMessage)
	if !ok {
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, fmt.Sprintf("Write your message (at most %d characters)", maxMessage))
	}
	id := chi.URLParam(r, "id")
	if err := s.addMessage(r.Context(), r, id, claimsFrom(r).Sub, "guest", false, message); err != nil {
		return err
	}
	s.mailSupport(r.Context(), id, "Guest replied")
	respond(w, http.StatusOK, httpx.NowMeta())
	return nil
}

func (s *Server) closeTicket(w http.ResponseWriter, r *http.Request) error {
	return s.finishTicket(w, r, claimsFrom(r).Sub, "guest")
}

// finishTicket closes a ticket. Closing twice is fine (it stays closed).
func (s *Server) finishTicket(w http.ResponseWriter, r *http.Request, userID, who string) error {
	id := chi.URLParam(r, "id")
	ctx := r.Context()
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var kind, status string
		var booking *string
		err := tx.QueryRow(ctx, `SELECT kind, status, booking_id FROM support_tickets WHERE id = $1 AND ($2 = '' OR user_id::text = $2) FOR UPDATE`, id, userID).Scan(&kind, &status, &booking)
		if err == pgx.ErrNoRows || isBadUUID(err) {
			return errNotFound
		}
		if err != nil {
			return err
		}
		// An open dispute is not just a conversation: the guest closing it withdraws it (the booking goes back to what it was), and
		// support settles it with a decision rather than by closing it.
		// A relocation is settled by support (the booking is restored or refunded), or ends when the guest cancels the booking.
		if kind == "relocation" && status != "closed" {
			return fail(http.StatusConflict, "RELOCATION_OPEN", "A relocation ends when support settles it or the guest cancels the booking")
		}
		if kind == "dispute" && status != "closed" {
			if who != "guest" {
				return fail(http.StatusConflict, "DISPUTE_OPEN", "Settle the dispute with a decision instead of closing it")
			}
			if booking != nil {
				if err := s.withdrawDispute(ctx, r, tx, id, *booking); err != nil {
					return err
				}
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE support_tickets SET status = 'closed', closed_at = coalesce(closed_at, now()), closed_by = coalesce(closed_by, $2), updated_at = now() WHERE id = $1`, id, who); err != nil {
			return err
		}
		return s.audit(r, tx, "support_ticket_closed", map[string]any{"ticket_id": id})
	})
	if err != nil {
		return err
	}
	respond(w, http.StatusOK, httpx.NowMeta())
	return nil
}

// ---- admin

func (s *Server) adminTickets(w http.ResponseWriter, r *http.Request) error {
	if err := requireAdmin(r); err != nil {
		return err
	}
	rows, err := s.pool.Query(r.Context(), `SELECT t.id, t.kind, t.subject, t.status, t.booking_id, b.code, t.created_at, t.updated_at,
		coalesce((SELECT left(body, 140) FROM support_messages m WHERE m.ticket_id = t.id ORDER BY m.created_at DESC LIMIT 1), ''), coalesce(u.email, u.phone, ''), coalesce(hp.name, '')
		FROM support_tickets t JOIN users u ON u.id = t.user_id LEFT JOIN bookings b ON b.id = t.booking_id LEFT JOIN properties hp ON hp.id = t.property_id
		ORDER BY (t.status = 'closed'), (t.status = 'answered'), t.updated_at DESC LIMIT 200`)
	if err != nil {
		return err
	}
	defer rows.Close()
	out := []ticketOut{}
	for rows.Next() {
		var t ticketOut
		var created, updated time.Time
		if err := rows.Scan(&t.ID, &t.Kind, &t.Subject, &t.Status, &t.BookingID, &t.BookingCode, &created, &updated, &t.Preview, &t.Guest, &t.Hotel); err != nil {
			return err
		}
		t.CreatedAt, t.UpdatedAt = created.UTC().Format(time.RFC3339), updated.UTC().Format(time.RFC3339)
		out = append(out, t)
	}
	respond(w, http.StatusOK, struct {
		httpx.Meta
		Tickets []ticketOut `json:"tickets"`
	}{httpx.NowMeta(), out})
	return rows.Err()
}

func (s *Server) adminGetTicket(w http.ResponseWriter, r *http.Request) error {
	if err := requireAdmin(r); err != nil {
		return err
	}
	t, err := s.readTicket(r.Context(), chi.URLParam(r, "id"), "")
	if err != nil {
		return err
	}
	if t.BookingID != nil {
		var b ticketBooking
		var ci, co time.Time
		err := s.pool.QueryRow(r.Context(), `SELECT b.status, b.disputed_from, p.name, b.check_in, b.check_out, b.currency,
			coalesce((SELECT sum(amount_santim) FROM payments pp WHERE pp.booking_id = b.id AND pp.status IN ('succeeded', 'refunded')), 0)::bigint,
			COALESCE((SELECT sum(r.amount_santim) FROM partial_refunds r WHERE r.booking_id = b.id), 0)::bigint,
			COALESCE((SELECT sum(amount_santim) FROM payments pp WHERE pp.booking_id = b.id AND pp.status = 'succeeded'), 0)::bigint
			FROM bookings b JOIN properties p ON p.id = b.property_id WHERE b.id = $1`, *t.BookingID).Scan(&b.Status, &b.DisputedFrom, &b.Hotel, &ci, &co, &b.Currency, &b.PaidSantim, &b.RefundedSantim, &b.LeftSantim)
		if err != nil {
			return err
		}
		b.CheckIn, b.CheckOut = pgDate(ci), pgDate(co)
		if b.LeftSantim -= b.RefundedSantim; b.LeftSantim < 0 {
			b.LeftSantim = 0
		}
		t.Booking = &b
	}
	respond(w, http.StatusOK, struct {
		httpx.Meta
		Ticket ticketOut `json:"ticket"`
	}{httpx.NowMeta(), t})
	return nil
}

func (s *Server) adminReplyTicket(w http.ResponseWriter, r *http.Request) error {
	if err := requireAdmin(r); err != nil {
		return err
	}
	var in struct {
		Message string `json:"message"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	message, ok := cleanText(in.Message, 1, maxMessage)
	if !ok {
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, fmt.Sprintf("Write the answer (at most %d characters)", maxMessage))
	}
	ctx, id := r.Context(), chi.URLParam(r, "id")
	var who string
	if err := s.pool.QueryRow(ctx, `SELECT coalesce(email, phone, id::text) FROM users WHERE id = $1`, claimsFrom(r).Sub).Scan(&who); err != nil {
		return err
	}
	if err := s.addMessage(ctx, r, id, "", who, true, message); err != nil {
		return err
	}
	s.notifyTicketOwner(ctx, id, message)
	respond(w, http.StatusOK, httpx.NowMeta())
	return nil
}

// notifyTicketOwner tells whoever wrote the ticket (a guest, or a hotel's staff member) that support answered: a push to their phone, and an email when they
// signed in with an address. A phone-only guest with no push set up sees the answer when they open the app. Failures are logged, never returned.
func (s *Server) notifyTicketOwner(ctx context.Context, ticketID, body string) {
	var email *string
	var subject string
	var hotel bool
	if err := s.pool.QueryRow(ctx, `SELECT u.email, t.subject, t.property_id IS NOT NULL FROM support_tickets t JOIN users u ON u.id = t.user_id WHERE t.id = $1`, ticketID).Scan(&email, &subject, &hotel); err != nil {
		slog.Error("support notify: read ticket owner", "ticket", ticketID, "err", err)
		return
	}
	s.pushTicketAnswer(ctx, ticketID, subject, hotel)
	s.mailTicketAnswer(ctx, email, subject, body, hotel)
}

// pushTicketAnswer alerts the phones of the person who wrote the ticket. The text names the request, never the answer, so nothing private shows on a lock screen.
// The data lets the app open the conversation when the alert is tapped.
func (s *Server) pushTicketAnswer(ctx context.Context, ticketID, subject string, hotel bool) {
	app := "guest"
	if hotel {
		app = "partner"
	}
	rows, err := s.pool.Query(ctx, `SELECT d.push_token FROM support_tickets t JOIN devices d ON d.user_id = t.user_id AND d.app = $2 AND d.push_token IS NOT NULL AND d.revoked_at IS NULL
		WHERE t.id = $1`, ticketID, app)
	if err != nil {
		slog.Error("support push: devices", "ticket", ticketID, "err", err)
		return
	}
	defer rows.Close()
	var tokens []string
	for rows.Next() {
		var t string
		if rows.Scan(&t) == nil {
			tokens = append(tokens, t)
		}
	}
	if len(tokens) == 0 {
		return
	}
	if len([]rune(subject)) > 80 {
		subject = string([]rune(subject)[:80]) + "..."
	}
	data := map[string]string{"type": "support_answer", "ticket_id": ticketID}
	if !hotel {
		data["channel"] = "booking_updates" // the guest app's Android channel
	}
	s.push(ctx, tokens, "BookDan support answered you", subject, data)
}

// mailTicketAnswer emails the answer. Nothing is sent to an account that has no address or was deleted.
func (s *Server) mailTicketAnswer(ctx context.Context, email *string, subject, body string, hotel bool) {
	if email == nil || strings.HasSuffix(*email, "@deleted.invalid") {
		return
	}
	where := "Open the BookDan app, Profile, Help and support, to reply."
	if hotel {
		where = "Open the BookDan Partner app, Settings, Help and support, to reply."
	}
	text := "BookDan support answered your request: " + subject + "\n\n" + body + "\n\n" + where
	if err := s.p.Mail.SendMail(ctx, *email, "BookDan support: "+subject, text, ""); err != nil {
		slog.Error("support mail: send to guest", "err", err)
	}
}

func (s *Server) adminCloseTicket(w http.ResponseWriter, r *http.Request) error {
	if err := requireAdmin(r); err != nil {
		return err
	}
	var who string
	if err := s.pool.QueryRow(r.Context(), `SELECT coalesce(email, phone, id::text) FROM users WHERE id = $1`, claimsFrom(r).Sub).Scan(&who); err != nil {
		return err
	}
	return s.finishTicket(w, r, "", who)
}
