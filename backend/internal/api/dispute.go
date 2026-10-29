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

// Disputes: a guest challenges a charge or a stay. The booking becomes DISPUTED, which keeps its money from being released to the hotel
// (earnings count only checked-in, completed and no-show stays as payable), and a support ticket of kind "dispute" carries the conversation.
//
// A guest can dispute a booking once: after its stay has started (a stay to come is cancelled, not disputed), and until 14 days after check-out.
// The guest can withdraw it (the booking goes back to the status it had). An admin resolves it with one of three outcomes:
//   - rejected:               nothing changes; the booking goes back to the status it had;
//   - refunded_hotel_pays:    the guest gets everything back and the hotel's share and BookDan's commission are reversed (the booking is cancelled);
//   - refunded_platform_pays: the guest gets everything back from BookDan's own money (ledger expense dispute_expense); the hotel keeps its share.
// Two more outcomes refund only part of the payment while the booking stands (refunded_partial_hotel_pays and refunded_partial_platform_pays,
// with amount_santim): see partialrefund.go. If the hotel was already paid out, "hotel pays" leaves it owing that amount against its next payout.

const disputeWindowDays = 14

var disputeReasons = map[string]string{
	"not_as_described": "The stay was not as described",
	"refused_entry":    "The hotel refused me a room",
	"charged_wrong":    "I was charged the wrong amount",
	"no_show_wrong":    "I was marked as a no-show but I came",
	"other":            "Something else",
}

// disputable says whether a booking in this state can be disputed now.
func (s *Server) disputable(status, checkIn, checkOut string) bool {
	today := s.addisToday()
	switch status {
	case "CONFIRMED":
		if checkIn > today {
			return false
		}
	case "CHECKED_IN", "COMPLETED", "NO_SHOW":
	default:
		return false
	}
	out, err := time.Parse("2006-01-02", checkOut)
	if err != nil {
		return false
	}
	return !out.AddDate(0, 0, disputeWindowDays).Before(mustDate(today))
}

func mustDate(v string) time.Time {
	t, _ := time.Parse("2006-01-02", v)
	return t
}

func (s *Server) raiseDispute(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Reason  string `json:"reason"`
		Message string `json:"message"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	label, ok := disputeReasons[in.Reason]
	if !ok {
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "reason must be not_as_described, refused_entry, charged_wrong, no_show_wrong or other")
	}
	message, ok := cleanText(in.Message, 10, maxMessage)
	if !ok {
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, fmt.Sprintf("Tell us what happened (10 to %d characters)", maxMessage))
	}
	id, uid, ctx := chi.URLParam(r, "id"), claimsFrom(r).Sub, r.Context()
	var ticket, code string
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var status string
		var ci, co time.Time
		err := tx.QueryRow(ctx, `SELECT status, code, check_in, check_out FROM bookings WHERE id = $1 AND user_id = $2 FOR UPDATE`, id, uid).Scan(&status, &code, &ci, &co)
		if err == pgx.ErrNoRows || isBadUUID(err) {
			return errNotFound
		}
		if err != nil {
			return err
		}
		var earlier int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM support_tickets WHERE booking_id = $1 AND kind = 'dispute'`, id).Scan(&earlier); err != nil {
			return err
		}
		if earlier > 0 {
			return fail(http.StatusConflict, "DISPUTE_EXISTS", "A dispute was already opened for this booking. Write to support from Help and support.")
		}
		if !s.disputable(status, pgDate(ci), pgDate(co)) {
			if status == "CONFIRMED" && pgDate(ci) > s.addisToday() {
				return fail(http.StatusConflict, codeInvalidState, "The stay has not started. Cancel the booking, or write to support.")
			}
			if status == "CONFIRMED" || status == "CHECKED_IN" || status == "COMPLETED" || status == "NO_SHOW" {
				return fail(http.StatusConflict, "DISPUTE_WINDOW_CLOSED", fmt.Sprintf("A dispute can be opened until %d days after check-out. Write to support from Help and support.", disputeWindowDays))
			}
			return fail(http.StatusConflict, codeInvalidState, "This booking cannot be disputed")
		}
		if err := actor(ctx, tx, "guest:"+uid, "guest opened a dispute"); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE bookings SET status = 'DISPUTED', disputed_from = $2 WHERE id = $1`, id, status); err != nil {
			return err
		}
		subject := "Dispute: " + label + " (" + code + ")"
		if err := tx.QueryRow(ctx, `INSERT INTO support_tickets (user_id, booking_id, kind, subject, reason) VALUES ($1, $2, 'dispute', $3, $4) RETURNING id`,
			uid, id, subject, in.Reason).Scan(&ticket); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO support_messages (ticket_id, from_staff, author, body) VALUES ($1, false, 'guest', $2)`, ticket, message); err != nil {
			return err
		}
		return s.audit(r, tx, "dispute_opened", map[string]any{"booking_id": id, "ticket_id": ticket, "reason": in.Reason})
	})
	if err != nil {
		return err
	}
	s.mailSupport(ctx, ticket, "New dispute")
	s.alertHotel(ctx, id, "A guest opened a dispute", "Open booking "+code+" to give the hotel's account.", "booking_dispute")
	b, err := s.loadBooking(ctx, s.pool, id, uid)
	if err != nil {
		return err
	}
	respond(w, http.StatusOK, b)
	return nil
}

// withdrawDispute is what closing a dispute ticket means for the guest: the booking goes back to the status it had.
func (s *Server) withdrawDispute(ctx context.Context, r *http.Request, tx pgx.Tx, ticketID, bookingID string) error {
	var status string
	var from *string
	if err := tx.QueryRow(ctx, `SELECT status, disputed_from FROM bookings WHERE id = $1 FOR UPDATE`, bookingID).Scan(&status, &from); err != nil {
		return err
	}
	if status == "DISPUTED" && from != nil {
		if err := actor(ctx, tx, "guest:"+claimsFrom(r).Sub, "guest withdrew the dispute"); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE bookings SET status = $2, disputed_from = NULL WHERE id = $1`, bookingID, *from); err != nil {
			return err
		}
	}
	_, err := tx.Exec(ctx, `UPDATE support_tickets SET outcome = 'withdrawn' WHERE id = $1`, ticketID)
	return err
}

var disputeOutcomeText = map[string]string{
	"rejected":               "Decision: your dispute was not upheld. Your booking stays as it was.",
	"refunded_hotel_pays":    "Decision: your dispute was upheld. You are refunded in full.",
	"refunded_platform_pays": "Decision: your dispute was upheld. You are refunded in full.",
	// The amount is added when the decision is written.
	"refunded_partial_hotel_pays":    "Decision: your dispute was upheld in part.",
	"refunded_partial_platform_pays": "Decision: your dispute was upheld in part.",
}

// adminResolveDispute settles a dispute with one of the three outcomes above. The note is shown to the guest.
func (s *Server) adminResolveDispute(w http.ResponseWriter, r *http.Request) error {
	if err := requireAdmin(r); err != nil {
		return err
	}
	var in struct {
		Outcome string `json:"outcome"`
		Note    string `json:"note"`
		// Amount is the part of the payment to refund, in santim (for the two partial outcomes only).
		Amount int64 `json:"amount_santim"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if _, ok := disputeOutcomeText[in.Outcome]; !ok {
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "outcome must be rejected, refunded_hotel_pays, refunded_platform_pays, refunded_partial_hotel_pays or refunded_partial_platform_pays")
	}
	partial := strings.HasPrefix(in.Outcome, "refunded_partial_")
	if partial && in.Amount <= 0 {
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "amount_santim is the part of the payment to refund")
	}
	decision := disputeOutcomeText[in.Outcome]
	if partial {
		decision += " You are refunded " + formatETB(in.Amount) + "."
	}
	note, ok := cleanText(in.Note, 3, 1000)
	if !ok {
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "Write the reason for the decision (3 to 1000 characters)")
	}
	ctx, id := r.Context(), chi.URLParam(r, "id")
	var who string
	if err := s.pool.QueryRow(ctx, `SELECT coalesce(email, phone, id::text) FROM users WHERE id = $1`, claimsFrom(r).Sub).Scan(&who); err != nil {
		return err
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
		if kind != "dispute" || booking == nil {
			return fail(http.StatusConflict, codeInvalidState, "This is not a dispute")
		}
		if tStatus == "closed" {
			return fail(http.StatusConflict, codeInvalidState, "This dispute is already resolved")
		}
		bookingID = *booking
		var status, roomType, cur string
		var from *string
		var ci, co time.Time
		var rooms int
		if err := tx.QueryRow(ctx, `SELECT status, disputed_from, room_type_id, check_in, check_out, rooms, currency FROM bookings WHERE id = $1 FOR UPDATE`, bookingID).
			Scan(&status, &from, &roomType, &ci, &co, &rooms, &cur); err != nil {
			return err
		}
		if status != "DISPUTED" || from == nil {
			return fail(http.StatusConflict, codeInvalidState, "The booking is not in dispute")
		}

		if in.Outcome == "rejected" {
			if err := actor(ctx, tx, "admin:"+who, "dispute rejected"); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE bookings SET status = $2, disputed_from = NULL WHERE id = $1`, bookingID, *from); err != nil {
				return err
			}
		} else if partial {
			// Part of the payment goes back; the booking stands and returns to what it was.
			payer := "hotel"
			if in.Outcome == "refunded_partial_platform_pays" {
				payer = "platform"
			}
			if err := actor(ctx, tx, "admin:"+who, "dispute upheld in part: guest refunded part"); err != nil {
				return err
			}
			if err := s.partialRefund(ctx, tx, bookingID, &id, payer, in.Amount, note, who); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE bookings SET status = $2, disputed_from = NULL WHERE id = $1`, bookingID, *from); err != nil {
				return err
			}
		} else {
			rf, err := lockRefundable(ctx, tx, bookingID)
			if err == pgx.ErrNoRows {
				return fail(http.StatusBadRequest, "NO_PAYMENT", "Nothing was paid online for this booking, so there is nothing to refund.")
			}
			if err != nil {
				return err
			}
			if rf.Left <= 0 {
				return fail(http.StatusConflict, "NOTHING_LEFT", "Everything that was paid has already been refunded.")
			}
			payID, paid := rf.PaymentID, rf.Left // what is left after any partial refunds
			if err := actor(ctx, tx, "admin:"+who, "dispute upheld: guest refunded"); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE bookings SET status = 'CANCELLED_REFUNDED', refund_santim = $2, cancelled_at = coalesce(cancelled_at, now()), hotel_deadline_at = NULL, disputed_from = NULL WHERE id = $1`, bookingID, paid); err != nil {
				return err
			}
			// Nights still to come go back on sale (past nights stay as they were; a no-show already gave its rooms back).
			if *from == "CONFIRMED" || *from == "CHECKED_IN" {
				first := pgDate(ci)
				if today := s.addisToday(); today > first {
					first = today
				}
				if pgDate(co) > first {
					if err := unbookRooms(ctx, tx, roomType, first, pgDate(co), rooms); err != nil {
						return err
					}
				}
			}
			if in.Outcome == "refunded_hotel_pays" {
				err = reverseConfirmedAndOweRefund(ctx, tx, bookingID, payID, cur, paid)
			} else {
				err = ledger(ctx, tx, bookingID, payID, "refund", "dispute settled: refund paid by BookDan", cur, entry{"dispute_expense", paid}, entry{"refunds_payable", -paid})
			}
			if err != nil {
				return err
			}
		}

		if _, err := tx.Exec(ctx, `INSERT INTO support_messages (ticket_id, from_staff, author, body) VALUES ($1, true, $2, $3)`, id, who, decision+"\n\n"+note); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE support_tickets SET status = 'closed', closed_at = now(), closed_by = $2, outcome = $3, updated_at = now() WHERE id = $1`, id, who, in.Outcome); err != nil {
			return err
		}
		return s.audit(r, tx, "dispute_resolved", map[string]any{"ticket_id": id, "booking_id": bookingID, "outcome": in.Outcome, "amount_santim": in.Amount})
	})
	if err != nil {
		return err
	}
	s.notifyTicketOwner(ctx, id, decision+"\n\n"+note)
	if in.Outcome != "rejected" {
		s.runRefunds(ctx)
	}
	slog.Info("dispute resolved", "ticket", id, "booking", bookingID, "outcome", strings.ToLower(in.Outcome))
	respond(w, http.StatusOK, httpx.NowMeta())
	return nil
}
