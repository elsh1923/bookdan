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

// When the hotel cannot keep its promise, and the hotel's side of a guest's dispute.
//
//   - Cancel (hotel owners only): a confirmed booking is cancelled by the hotel. The guest is refunded in full and the hotel bears it (its share
//     and BookDan's commission are reversed), the rooms of nights still to come go back on sale, the guest is told and so is support.
//   - Cannot host (any hotel staff): the booking becomes NEEDS_RELOCATION and a support request of kind relocation is opened for the guest. The
//     guest can cancel at once for a full refund; support can find another hotel, then settle it as "restored" (the booking is confirmed
//     again) or "refunded" (a full refund, the hotel bears it).
//   - Disputes: the hotel's phones are alerted when a guest opens a dispute about its booking; the hotel reads the guest's account and adds its
//     own statement, which support and the guest see as the hotel's.

var hotelCancelReasons = map[string]string{
	"overbooked":  "The hotel has no room for you",
	"maintenance": "A problem with the room or the building",
	"emergency":   "An emergency at the hotel",
	"other":       "Another reason",
}

// ownerHotels are the hotels the user owns (cancelling a guest's booking is an owner's decision).
func (s *Server) ownerHotels(ctx context.Context, userID string) ([]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT property_id FROM property_staff WHERE user_id = $1 AND role = 'owner'`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// hotelSideCancel cancels a booking the hotel cannot honour: the guest is refunded in full (what is left after partial refunds) and the
// hotel bears it. The booking row must be locked by the caller. by is hotel or admin.
func (s *Server) hotelSideCancel(ctx context.Context, tx pgx.Tx, bookingID, who, by, reason string) error {
	var status, roomType, cur string
	var in, out time.Time
	var rooms int
	if err := tx.QueryRow(ctx, `SELECT status, room_type_id, check_in, check_out, rooms, currency FROM bookings WHERE id = $1`, bookingID).Scan(&status, &roomType, &in, &out, &rooms, &cur); err != nil {
		return err
	}
	var refund int64
	var payID string
	rf, err := lockRefundable(ctx, tx, bookingID)
	if err != nil && err != pgx.ErrNoRows {
		return err
	}
	if err == nil {
		payID, refund = rf.PaymentID, rf.Left
	}
	if err := actor(ctx, tx, who, "cancelled by the "+by+": "+reason); err != nil {
		return err
	}
	if err := abandonDateChange(ctx, tx, bookingID, "the booking was cancelled"); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE bookings SET status = 'CANCELLED_REFUNDED', refund_santim = $2, cancelled_at = coalesce(cancelled_at, now()), cancelled_by = $3, cancel_reason = $4,
		hotel_deadline_at = NULL, disputed_from = NULL WHERE id = $1`, bookingID, refund, by, reason); err != nil {
		return err
	}
	// Nights still to come go back on sale; nights already past stay as they were.
	first := pgDate(in)
	if today := s.addisToday(); today > first {
		first = today
	}
	if pgDate(out) > first {
		if err := unbookRooms(ctx, tx, roomType, first, pgDate(out), rooms); err != nil {
			return err
		}
	}
	if payID == "" || refund <= 0 {
		return nil // paid at the hotel, or everything was already returned
	}
	return reverseConfirmedAndOweRefund(ctx, tx, bookingID, payID, cur, refund)
}

func (s *Server) partnerCancel(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Reason string `json:"reason"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if _, ok := hotelCancelReasons[in.Reason]; !ok {
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "reason must be overbooked, maintenance, emergency or other")
	}
	ctx, id, uid := r.Context(), chi.URLParam(r, "id"), claimsFrom(r).Sub
	if _, _, err := s.staffHotels(ctx, uid); err != nil {
		return err
	}
	owned, err := s.ownerHotels(ctx, uid)
	if err != nil {
		return err
	}
	if len(owned) == 0 {
		return fail(http.StatusForbidden, codeForbidden, "Only a hotel owner can cancel a booking")
	}
	err = s.tx(ctx, func(tx pgx.Tx) error {
		var status string
		var out time.Time
		err := tx.QueryRow(ctx, `SELECT status, check_out FROM bookings WHERE id = $1 AND property_id = ANY($2::uuid[]) FOR UPDATE`, id, owned).Scan(&status, &out)
		if err == pgx.ErrNoRows || isBadUUID(err) {
			return errNotFound
		}
		if err != nil {
			return err
		}
		if status != "CONFIRMED" && status != "NEEDS_RELOCATION" {
			return fail(http.StatusConflict, codeInvalidState, "Only a confirmed booking that has not been checked in can be cancelled by the hotel")
		}
		if pgDate(out) <= s.addisToday() {
			return fail(http.StatusConflict, codeInvalidState, "This stay is already over")
		}
		if err := s.hotelSideCancel(ctx, tx, id, "staff:"+uid, "hotel", in.Reason); err != nil {
			return err
		}
		// A relocation request that was open is finished by this.
		if _, err := tx.Exec(ctx, `INSERT INTO support_messages (ticket_id, from_staff, author, body)
			SELECT id, true, 'BookDan', 'The hotel cancelled your booking. You are refunded in full.' FROM support_tickets WHERE booking_id = $1 AND kind = 'relocation' AND status <> 'closed'`, id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE support_tickets SET status = 'closed', closed_at = now(), closed_by = 'hotel cancelled', outcome = 'relocation_refunded', updated_at = now()
			WHERE booking_id = $1 AND kind = 'relocation' AND status <> 'closed'`, id); err != nil {
			return err
		}
		return s.audit(r, tx, "hotel_cancelled_booking", map[string]any{"booking_id": id, "reason": in.Reason})
	})
	if err != nil {
		return err
	}
	s.notifyGuest(ctx, id, "hotel_cancelled")
	s.mailSupportAbout(ctx, id, "A hotel cancelled a confirmed booking", "Reason: "+hotelCancelReasons[in.Reason])
	s.runRefunds(ctx)
	propIDs, _, _ := s.staffHotels(ctx, uid)
	b, err := scanPartner(s.pool.QueryRow(ctx, partnerSelect+` WHERE b.id = $1`, id), len(propIDs) > 1)
	if err != nil {
		return err
	}
	respond(w, http.StatusOK, b)
	return nil
}

func (s *Server) partnerRelocate(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Reason string `json:"reason"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	label, ok := hotelCancelReasons[in.Reason]
	if !ok {
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "reason must be overbooked, maintenance, emergency or other")
	}
	ctx, id, uid := r.Context(), chi.URLParam(r, "id"), claimsFrom(r).Sub
	propIDs, _, err := s.staffHotels(ctx, uid)
	if err != nil {
		return err
	}
	var ticket string
	err = s.tx(ctx, func(tx pgx.Tx) error {
		var status, hotel, code, guestID string
		var out time.Time
		err := tx.QueryRow(ctx, `SELECT b.status, p.name, b.code, b.user_id::text, b.check_out FROM bookings b JOIN properties p ON p.id = b.property_id
			WHERE b.id = $1 AND b.property_id = ANY($2::uuid[]) FOR UPDATE OF b`, id, propIDs).Scan(&status, &hotel, &code, &guestID, &out)
		if err == pgx.ErrNoRows || isBadUUID(err) {
			return errNotFound
		}
		if err != nil {
			return err
		}
		if status != "CONFIRMED" {
			return fail(http.StatusConflict, codeInvalidState, "Only a confirmed booking that has not been checked in can be moved to another hotel")
		}
		if pgDate(out) <= s.addisToday() {
			return fail(http.StatusConflict, codeInvalidState, "This stay is already over")
		}
		if err := actor(ctx, tx, "staff:"+uid, "hotel cannot host the guest: "+in.Reason); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE bookings SET status = 'NEEDS_RELOCATION' WHERE id = $1`, id); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO support_tickets (user_id, booking_id, kind, subject, status) VALUES ($1, $2, 'relocation', $3, 'answered') RETURNING id`,
			guestID, id, "Your stay needs a new hotel ("+code+")").Scan(&ticket); err != nil {
			return err
		}
		body := hotel + " told us it cannot host you on your dates (" + strings.ToLower(label) + "). We are sorry. BookDan support is looking for another place for you and will write here. " +
			"If you prefer, you can cancel the booking now from the booking screen and you are refunded in full."
		if _, err := tx.Exec(ctx, `INSERT INTO support_messages (ticket_id, from_staff, author, body) VALUES ($1, true, 'BookDan', $2)`, ticket, body); err != nil {
			return err
		}
		return s.audit(r, tx, "hotel_cannot_host", map[string]any{"booking_id": id, "ticket_id": ticket, "reason": in.Reason})
	})
	if err != nil {
		return err
	}
	s.notifyGuest(ctx, id, "needs_relocation")
	s.mailSupportAbout(ctx, id, "A hotel cannot host a guest: find another hotel", "Reason: "+label+"\nSupport request: "+ticket)
	b, err := scanPartner(s.pool.QueryRow(ctx, partnerSelect+` WHERE b.id = $1`, id), len(propIDs) > 1)
	if err != nil {
		return err
	}
	respond(w, http.StatusOK, b)
	return nil
}

// mailSupportAbout tells the admins and the support address about a booking event, with the booking and hotel named.
func (s *Server) mailSupportAbout(ctx context.Context, bookingID, headline, extra string) {
	var code, guest, hotel string
	_ = s.pool.QueryRow(ctx, `SELECT b.code, b.guest_name, p.name FROM bookings b JOIN properties p ON p.id = b.property_id WHERE b.id = $1`, bookingID).Scan(&code, &guest, &hotel)
	text := fmt.Sprintf("%s\n\nBooking: %s\nGuest: %s\nHotel: %s\n%s\n\nOpen the admin page to follow it up.", headline, code, guest, hotel, extra)
	for _, to := range s.supportRecipients() {
		if err := s.p.Mail.SendMail(ctx, to, headline+": "+code, text, ""); err != nil {
			slog.Error("support mail about a booking", "to", to, "err", err)
		}
	}
}

// adminResolveRelocation settles a relocation: restored (the hotel can host after all) or refunded (a full refund, the hotel bears it).
func (s *Server) adminResolveRelocation(w http.ResponseWriter, r *http.Request) error {
	if err := requireAdmin(r); err != nil {
		return err
	}
	var in struct {
		Outcome string `json:"outcome"`
		Note    string `json:"note"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if in.Outcome != "restored" && in.Outcome != "refunded" {
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "outcome must be restored or refunded")
	}
	note, ok := cleanText(in.Note, 3, 1000)
	if !ok {
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "Write what was decided (3 to 1000 characters); the guest reads it")
	}
	ctx, id := r.Context(), chi.URLParam(r, "id")
	var who string
	if err := s.pool.QueryRow(ctx, `SELECT coalesce(email, phone, id::text) FROM users WHERE id = $1`, claimsFrom(r).Sub).Scan(&who); err != nil {
		return err
	}
	text := "Your booking is confirmed again at the hotel."
	outcome := "relocation_restored"
	if in.Outcome == "refunded" {
		text, outcome = "Your booking was cancelled and you are refunded in full.", "relocation_refunded"
	}
	var bookingID string
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var kind, tStatus string
		var booking *string
		err := tx.QueryRow(ctx, `SELECT kind, status, booking_id FROM support_tickets WHERE id = $1 FOR UPDATE`, id).Scan(&kind, &tStatus, &booking)
		if err == pgx.ErrNoRows || isBadUUID(err) {
			return errNotFound
		}
		if err != nil {
			return err
		}
		if kind != "relocation" || booking == nil {
			return fail(http.StatusConflict, codeInvalidState, "This is not a relocation request")
		}
		if tStatus == "closed" {
			return fail(http.StatusConflict, codeInvalidState, "This relocation is already settled")
		}
		bookingID = *booking
		var status string
		if err := tx.QueryRow(ctx, `SELECT status FROM bookings WHERE id = $1 FOR UPDATE`, bookingID).Scan(&status); err != nil {
			return err
		}
		if status != "NEEDS_RELOCATION" {
			return fail(http.StatusConflict, codeInvalidState, "The booking is no longer waiting to be moved")
		}
		if in.Outcome == "restored" {
			if err := actor(ctx, tx, "admin:"+who, "relocation settled: the hotel hosts the guest"); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE bookings SET status = 'CONFIRMED' WHERE id = $1`, bookingID); err != nil {
				return err
			}
		} else if err := s.hotelSideCancel(ctx, tx, bookingID, "admin:"+who, "admin", "relocation: no other place found"); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO support_messages (ticket_id, from_staff, author, body) VALUES ($1, true, $2, $3)`, id, who, text+"\n\n"+note); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE support_tickets SET status = 'closed', closed_at = now(), closed_by = $2, outcome = $3, updated_at = now() WHERE id = $1`, id, who, outcome); err != nil {
			return err
		}
		return s.audit(r, tx, "relocation_resolved", map[string]any{"ticket_id": id, "booking_id": bookingID, "outcome": in.Outcome})
	})
	if err != nil {
		return err
	}
	s.notifyTicketOwner(ctx, id, text+"\n\n"+note)
	if in.Outcome == "refunded" {
		s.runRefunds(ctx)
	}
	respond(w, http.StatusOK, httpx.NowMeta())
	return nil
}

// ---- the hotel's side of a dispute

// alertHotel pushes a short alert about a booking to the hotel's phones (the first wave, and the owners).
func (s *Server) alertHotel(ctx context.Context, bookingID, title, body, typ string) {
	q, err := s.loadRequest(ctx, bookingID)
	if err != nil {
		slog.Error("alert hotel", "type", typ, "err", err)
		return
	}
	wave := append(firstWave(q.people, q.mode), escalationWave(q.people)...)
	if len(wave) > 0 {
		s.push(ctx, tokensOf(wave), title, body, s.alertData(q, typ))
	}
}

type hotelDisputeOut struct {
	TicketID string          `json:"ticket_id"`
	Status   string          `json:"status"`
	Reason   string          `json:"reason"`
	Messages []ticketMessage `json:"messages"`
}

// partnerDispute shows the hotel what the guest said and what the hotel has said: never what support wrote to the guest.
func (s *Server) partnerDispute(w http.ResponseWriter, r *http.Request) error {
	ctx, id := r.Context(), chi.URLParam(r, "id")
	propIDs, _, err := s.staffHotels(ctx, claimsFrom(r).Sub)
	if err != nil {
		return err
	}
	var out hotelDisputeOut
	err = s.pool.QueryRow(ctx, `SELECT t.id, t.status, coalesce(t.reason, '') FROM support_tickets t JOIN bookings b ON b.id = t.booking_id
		WHERE t.booking_id = $1 AND t.kind = 'dispute' AND b.property_id = ANY($2::uuid[]) ORDER BY t.created_at DESC LIMIT 1`, id, propIDs).Scan(&out.TicketID, &out.Status, &out.Reason)
	if err == pgx.ErrNoRows || isBadUUID(err) {
		return errNotFound
	}
	if err != nil {
		return err
	}
	if label, ok := disputeReasons[out.Reason]; ok {
		out.Reason = label
	}
	rows, err := s.pool.Query(ctx, `SELECT from_staff, author, body, created_at FROM support_messages WHERE ticket_id = $1 AND (author = 'guest' OR author LIKE 'hotel:%') ORDER BY created_at, id`, out.TicketID)
	if err != nil {
		return err
	}
	defer rows.Close()
	out.Messages = []ticketMessage{}
	for rows.Next() {
		var m ticketMessage
		var at time.Time
		var from bool
		if err := rows.Scan(&from, &m.Author, &m.Body, &at); err != nil {
			return err
		}
		m.Role = roleOf(m.Author)
		m.Author = map[string]string{"guest": "Guest", "hotel": "Hotel"}[m.Role]
		m.CreatedAt = at.UTC().Format(time.RFC3339)
		out.Messages = append(out.Messages, m)
	}
	respond(w, http.StatusOK, struct {
		httpx.Meta
		Dispute hotelDisputeOut `json:"dispute"`
	}{httpx.NowMeta(), out})
	return rows.Err()
}

// partnerDisputeStatement adds the hotel's statement to an open dispute about one of its bookings.
func (s *Server) partnerDisputeStatement(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Message string `json:"message"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	msg, ok := cleanText(in.Message, 10, maxMessage)
	if !ok {
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, fmt.Sprintf("Write the hotel's account (10 to %d characters)", maxMessage))
	}
	ctx, id, uid := r.Context(), chi.URLParam(r, "id"), claimsFrom(r).Sub
	propIDs, _, err := s.staffHotels(ctx, uid)
	if err != nil {
		return err
	}
	var ticket string
	err = s.tx(ctx, func(tx pgx.Tx) error {
		var hotel string
		err := tx.QueryRow(ctx, `SELECT t.id, p.name FROM support_tickets t JOIN bookings b ON b.id = t.booking_id JOIN properties p ON p.id = b.property_id
			WHERE t.booking_id = $1 AND t.kind = 'dispute' AND t.status <> 'closed' AND b.property_id = ANY($2::uuid[]) FOR UPDATE OF t`, id, propIDs).Scan(&ticket, &hotel)
		if err == pgx.ErrNoRows || isBadUUID(err) {
			return fail(http.StatusConflict, codeInvalidState, "There is no open dispute about this booking")
		}
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO support_messages (ticket_id, from_staff, author, body) VALUES ($1, false, $2, $3)`, ticket, "hotel:"+hotel, msg); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE support_tickets SET status = 'open', updated_at = now() WHERE id = $1`, ticket); err != nil {
			return err
		}
		return s.audit(r, tx, "hotel_dispute_statement", map[string]any{"booking_id": id, "ticket_id": ticket})
	})
	if err != nil {
		return err
	}
	s.mailSupport(ctx, ticket, "The hotel answered a dispute")
	respond(w, http.StatusOK, httpx.NowMeta())
	return nil
}

// roleOf says who wrote a support message: the guest, the hotel, or BookDan's support.
func roleOf(author string) string {
	switch {
	case author == "guest":
		return "guest"
	case strings.HasPrefix(author, "hotel:"):
		return "hotel"
	}
	return "support"
}
