package api

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/bookdan/backend/internal/httpx"
)

// Cancellation rules (decided by the owner):
//   - before the hotel has answered, the guest can cancel and gets everything back;
//   - once the hotel has confirmed, a full refund until 48 hours before check-in, nothing after that;
//   - check-in time is 12:00 Addis Ababa time on the check-in date; from then on a booking can no longer be cancelled;
//   - a booking paid at the hotel has no deposit, so there is nothing to refund.
const (
	freeCancelBeforeCheckIn = 48 * time.Hour
	checkInHour             = 12 // Addis Ababa time
)

// cancelWindow returns the moment check-in starts (UTC) and the last moment a cancellation is still refunded.
func cancelWindow(checkIn time.Time) (checkInAt, fullRefundUntil time.Time) {
	checkInAt = time.Date(checkIn.Year(), checkIn.Month(), checkIn.Day(), checkInHour, 0, 0, 0, time.UTC).Add(-3 * time.Hour)
	return checkInAt, checkInAt.Add(-freeCancelBeforeCheckIn)
}

// unbookRooms gives back rooms that were already confirmed (booked), as opposed to held.
func unbookRooms(ctx context.Context, tx pgx.Tx, roomTypeID, in, out string, rooms int) error {
	_, err := tx.Exec(ctx, `UPDATE inventory SET booked = booked - $4
		WHERE room_type_id = $1 AND night >= $2::date AND night < $3::date`, roomTypeID, in, out, rooms)
	return err
}

func (s *Server) cancelBooking(w http.ResponseWriter, r *http.Request) error {
	id, uid, ctx := chi.URLParam(r, "id"), claimsFrom(r).Sub, r.Context()
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var status, roomType, cur string
		var in, out time.Time
		var rooms int
		var refundable bool
		err := tx.QueryRow(ctx, `SELECT status, room_type_id, check_in, check_out, rooms, currency, refundable FROM bookings WHERE id = $1 AND user_id = $2 FOR UPDATE`, id, uid).
			Scan(&status, &roomType, &in, &out, &rooms, &cur, &refundable)
		if err == pgx.ErrNoRows || isBadUUID(err) {
			return errNotFound
		}
		if err != nil {
			return err
		}
		if status != "PAID_AWAITING_HOTEL" && status != "CONFIRMED" {
			return fail(http.StatusConflict, codeInvalidState, "This booking cannot be cancelled")
		}
		now := s.now()
		checkInAt, until := cancelWindow(in)
		if !now.Before(checkInAt) {
			return fail(http.StatusConflict, codeCancelTooLate, "The stay has started, so the booking can no longer be cancelled here. Contact the hotel.")
		}

		// How much comes back: everything while the hotel has not answered; after it confirmed, everything until 48 hours before check-in.
		var payID string
		var paid int64
		err = tx.QueryRow(ctx, `SELECT id, amount_santim FROM payments WHERE booking_id = $1 AND status = 'succeeded'`, id).Scan(&payID, &paid)
		hasPayment := err == nil
		if err != nil && err != pgx.ErrNoRows {
			return err
		}
		var refund int64
		// Non-refundable: nothing back once the hotel has confirmed (before that the hotel has not accepted anything yet).
		if hasPayment && (status == "PAID_AWAITING_HOTEL" || (refundable && !now.After(until))) {
			refund = paid
		}

		if err := actor(ctx, tx, "guest:"+uid, "guest cancelled"); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE bookings SET status = 'CANCELLED_REFUNDED', refund_santim = $2, cancelled_at = now(), hotel_deadline_at = NULL WHERE id = $1`, id, refund); err != nil {
			return err
		}
		ci, co := pgDate(in), pgDate(out)
		if status == "CONFIRMED" {
			err = unbookRooms(ctx, tx, roomType, ci, co, rooms)
		} else {
			err = releaseRooms(ctx, tx, roomType, ci, co, rooms)
		}
		if err != nil {
			return err
		}
		if refund == 0 {
			return nil // late cancellation (or pay at hotel): the hotel keeps what it was owed, no money moves
		}
		if status == "PAID_AWAITING_HOTEL" {
			return accrueRefund(ctx, tx, id, payID, cur, refund)
		}
		return reverseConfirmedAndOweRefund(ctx, tx, id, payID, cur, refund)
	})
	if err != nil {
		return err
	}
	s.alertHotelOfCancellation(ctx, id)
	s.runRefunds(ctx)
	b, err := s.loadBooking(ctx, s.pool, id, uid)
	if err != nil {
		return err
	}
	respond(w, http.StatusOK, b)
	return nil
}

// reverseConfirmedAndOweRefund undoes what accepting the booking recorded (the hotel's share and our commission) and records that
// the guest is owed their money, as one balanced ledger transaction. It reverses the exact amounts that were booked.
func reverseConfirmedAndOweRefund(ctx context.Context, tx pgx.Tx, bookingID, paymentID, currency string, refund int64) error {
	rows, err := tx.Query(ctx, `SELECT e.account_code, e.amount_santim FROM ledger_entries e JOIN ledger_transactions t ON t.id = e.transaction_id
		WHERE t.booking_id = $1 AND t.kind = 'booking_confirmed'`, bookingID)
	if err != nil {
		return err
	}
	var entries []entry
	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.account, &e.santim); err != nil {
			rows.Close()
			return err
		}
		e.santim = -e.santim
		entries = append(entries, e)
	}
	rows.Close()
	if len(entries) == 0 {
		return accrueRefund(ctx, tx, bookingID, paymentID, currency, refund)
	}
	entries = append(entries, entry{"guest_deposits_held", refund}, entry{"refunds_payable", -refund})
	return ledger(ctx, tx, bookingID, paymentID, "refund", "guest cancelled: the hotel's share is reversed and the refund is owed", currency, entries...)
}

// alertHotelOfCancellation tells the hotel's phones. Failure is logged, never returned.
func (s *Server) alertHotelOfCancellation(ctx context.Context, bookingID string) {
	q, err := s.loadRequest(ctx, bookingID)
	if err != nil {
		slog.Error("alert hotel of cancellation", "err", err)
		return
	}
	// Whoever would have been alerted to the request is told it is gone (owners too, if the request had reached them).
	var escalated bool
	_ = s.pool.QueryRow(ctx, `SELECT hotel_escalated_at IS NOT NULL FROM bookings WHERE id = $1`, bookingID).Scan(&escalated)
	wave := firstWave(q.people, q.mode)
	if escalated {
		wave = append(wave, escalationWave(q.people)...)
	}
	if len(wave) > 0 {
		s.push(ctx, tokensOf(wave), "Booking cancelled", q.guest+" · "+q.code+" was cancelled by the guest", s.alertData(q, "booking_cancelled"))
	}
}

// ---- reviews

type reviewOut struct {
	Rating int    `json:"rating"`
	Text   string `json:"text"`
}

// submitReview saves a guest's review of a finished stay: only the guest of that booking, only once the check-out date has come,
// one review per booking.
func (s *Server) submitReview(w http.ResponseWriter, r *http.Request) error {
	id, uid, ctx := chi.URLParam(r, "id"), claimsFrom(r).Sub, r.Context()
	var in struct {
		Rating int    `json:"rating"`
		Text   string `json:"text"`
		// Optional sub-ratings, 1 to 5 each (0 or left out = not given).
		Cleanliness int `json:"cleanliness"`
		Location    int `json:"location"`
		Staff       int `json:"staff"`
		Value       int `json:"value"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	text := strings.TrimSpace(in.Text)
	if in.Rating < 1 || in.Rating > 5 || len(text) > 1000 {
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "Give 1 to 5 stars and at most 1000 characters of text")
	}
	for _, sub := range []int{in.Cleanliness, in.Location, in.Staff, in.Value} {
		if sub < 0 || sub > 5 {
			return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "Sub-ratings are 1 to 5 stars")
		}
	}
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var status, property string
		var out time.Time
		err := tx.QueryRow(ctx, `SELECT status, property_id, check_out FROM bookings WHERE id = $1 AND user_id = $2 FOR UPDATE`, id, uid).Scan(&status, &property, &out)
		if err == pgx.ErrNoRows || isBadUUID(err) {
			return errNotFound
		}
		if err != nil {
			return err
		}
		if (status != "CONFIRMED" && status != "CHECKED_IN" && status != "COMPLETED") || pgDate(out) > s.addisToday() {
			return fail(http.StatusConflict, codeReviewNotReady, "You can review a stay after check-out")
		}
		tag, err := tx.Exec(ctx, `INSERT INTO reviews (booking_id, property_id, user_id, rating, text, rating_cleanliness, rating_location, rating_staff, rating_value)
			VALUES ($1, $2, $3, $4, $5, NULLIF($6, 0), NULLIF($7, 0), NULLIF($8, 0), NULLIF($9, 0)) ON CONFLICT (booking_id) DO NOTHING`,
			id, property, uid, in.Rating, text, in.Cleanliness, in.Location, in.Staff, in.Value)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return fail(http.StatusConflict, codeAlreadyReviewed, "You already reviewed this stay")
		}
		return nil
	})
	if err != nil {
		return err
	}
	b, err := s.loadBooking(ctx, s.pool, id, uid)
	if err != nil {
		return err
	}
	respond(w, http.StatusOK, b)
	return nil
}
