package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/bookdan/backend/internal/httpx"
)

// Changing the dates of a booking. A guest can move a CONFIRMED booking to other dates once, when all of this holds:
//   - the booking is refundable, has no coupon, and was not moved before;
//   - both the old and the new check-in are still more than 48 hours away (the same point where a cancellation stops being free), so the
//     booking stays cancellable under the usual rule;
//   - the same room type has the rooms free on every new night;
//   - the new dates cost exactly the same (base, service charge, VAT, total and deposit), so the payment and the ledger need no change.
// A different price is refused with PRICE_DIFFERS and the new price: the guest cancels (free) and books the new dates instead. Charging or
// refunding a difference needs partial refunds and a second payment, which are not built.
// The hotel is alerted and the guest is emailed; the first dates stay in original_check_in/out and the change is in the audit log.

// bookRooms takes confirmed rooms on every night of the stay, or none of them (SOLD_OUT).
func bookRooms(ctx context.Context, tx pgx.Tx, roomTypeID, in, out string, rooms int) error {
	if err := ensureInventory(ctx, tx, roomTypeID, in, out); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE inventory SET booked = booked + $4
		WHERE room_type_id = $1 AND night >= $2::date AND night < $3::date AND held + booked + $4 <= capacity`, roomTypeID, in, out, rooms)
	if err != nil {
		return err
	}
	if int(tag.RowsAffected()) != nightsBetween(in, out) {
		return fail(http.StatusConflict, codeSoldOut, "The room is not free on all of those nights")
	}
	return nil
}

// canChangeDates says whether a booking can be moved now (the price check happens when the guest picks dates).
func (s *Server) canChangeDates(status string, checkIn time.Time, refundable bool, coupon *string, changes int) bool {
	if status != "CONFIRMED" || !refundable || coupon != nil || changes >= 1 {
		return false
	}
	_, until := cancelWindow(checkIn)
	return !s.now().After(until)
}

func (s *Server) changeDates(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		CheckIn  string `json:"check_in"`
		CheckOut string `json:"check_out"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	bad := func(m string) error { return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, m) }
	if !isDate(in.CheckIn) || !isDate(in.CheckOut) {
		return bad("check_in and check_out must be dates (YYYY-MM-DD)")
	}
	nights := nightsBetween(in.CheckIn, in.CheckOut)
	if nights < 1 || nights > maxNights {
		return bad(fmt.Sprintf("A stay is 1 to %d nights", maxNights))
	}
	today := s.addisToday()
	if in.CheckIn < today || nightsBetween(today, in.CheckIn) > maxAdvanceDays {
		return bad("Choose dates from today up to a year ahead")
	}

	id, uid, ctx := chi.URLParam(r, "id"), claimsFrom(r).Sub, r.Context()
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var status, roomType, meal string
		var oldIn, oldOut time.Time
		var rooms, changes int
		var refundable bool
		var coupon *string
		var base, service, vat, total, deposit int64
		err := tx.QueryRow(ctx, `SELECT b.status, b.room_type_id, b.check_in, b.check_out, b.rooms, b.refundable, b.meal, cp.code, b.date_changes,
				b.base_santim, b.service_santim, b.vat_santim, b.total_santim, b.deposit_santim
			FROM bookings b LEFT JOIN coupons cp ON cp.id = b.coupon_id WHERE b.id = $1 AND b.user_id = $2 FOR UPDATE OF b`, id, uid).
			Scan(&status, &roomType, &oldIn, &oldOut, &rooms, &refundable, &meal, &coupon, &changes, &base, &service, &vat, &total, &deposit)
		if err == pgx.ErrNoRows || isBadUUID(err) {
			return errNotFound
		}
		if err != nil {
			return err
		}
		switch {
		case status != "CONFIRMED":
			return fail(http.StatusConflict, codeInvalidState, "Only a confirmed booking can be moved to other dates")
		case changes >= 1:
			return fail(http.StatusConflict, "CHANGE_LIMIT", "This booking was already moved once. Cancel it and book again if the dates changed again.")
		case !refundable:
			return fail(http.StatusConflict, "NON_REFUNDABLE", "A non-refundable booking cannot be moved to other dates")
		case coupon != nil:
			return fail(http.StatusConflict, "COUPON_NO_CHANGE", "A booking with a coupon cannot be moved to other dates")
		}
		ci, co := pgDate(oldIn), pgDate(oldOut)
		if in.CheckIn == ci && in.CheckOut == co {
			return bad("These are the dates you already have")
		}
		now := s.now()
		if _, until := cancelWindow(oldIn); now.After(until) {
			return fail(http.StatusConflict, "CHANGE_TOO_LATE", "Dates can be changed until 48 hours before check-in. After that, contact the hotel.")
		}
		newIn, _ := time.Parse("2006-01-02", in.CheckIn)
		if _, until := cancelWindow(newIn); now.After(until) {
			return fail(http.StatusConflict, "NEW_DATES_TOO_SOON", "The new check-in must be more than 48 hours away.")
		}

		p, err := s.priceStay(ctx, tx, roomType, in.CheckIn, in.CheckOut, rooms, rateOption{Meal: meal, Refundable: refundable}, nil)
		if err != nil {
			return err
		}
		if p.Base != base || p.Service != service || p.VAT != vat || p.Total != total || p.Deposit != deposit {
			return fail(http.StatusConflict, "PRICE_DIFFERS", fmt.Sprintf("The new dates cost %s instead of %s. Cancel this booking (free until 48 hours before check-in) and book the new dates instead.",
				formatETB(p.Total), formatETB(total)))
		}

		// Free the old nights first, so dates that overlap the old ones do not count the guest's own rooms twice; all or nothing.
		if err := unbookRooms(ctx, tx, roomType, ci, co, rooms); err != nil {
			return err
		}
		if err := bookRooms(ctx, tx, roomType, in.CheckIn, in.CheckOut, rooms); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE bookings SET check_in = $2::date, check_out = $3::date, date_changes = date_changes + 1,
			original_check_in = coalesce(original_check_in, $4::date), original_check_out = coalesce(original_check_out, $5::date) WHERE id = $1`,
			id, in.CheckIn, in.CheckOut, ci, co); err != nil {
			return err
		}
		return s.audit(r, tx, "booking_dates_changed", map[string]any{"booking_id": id, "from": ci + " to " + co, "to": in.CheckIn + " to " + in.CheckOut})
	})
	if err != nil {
		return err
	}
	s.alertHotelOfDateChange(ctx, id)
	s.emailGuest(ctx, id, "dates_changed")
	b, err := s.loadBooking(ctx, s.pool, id, uid)
	if err != nil {
		return err
	}
	respond(w, http.StatusOK, b)
	return nil
}

// alertHotelOfDateChange tells the hotel's phones that a confirmed booking moved. Failure is logged, never returned.
func (s *Server) alertHotelOfDateChange(ctx context.Context, bookingID string) {
	q, err := s.loadRequest(ctx, bookingID)
	if err != nil {
		slog.Error("alert hotel of date change", "err", err)
		return
	}
	var in, out time.Time
	if err := s.pool.QueryRow(ctx, `SELECT check_in, check_out FROM bookings WHERE id = $1`, bookingID).Scan(&in, &out); err != nil {
		slog.Error("alert hotel of date change", "err", err)
		return
	}
	var escalated bool
	_ = s.pool.QueryRow(ctx, `SELECT hotel_escalated_at IS NOT NULL FROM bookings WHERE id = $1`, bookingID).Scan(&escalated)
	wave := firstWave(q.people, q.mode)
	if escalated {
		wave = append(wave, escalationWave(q.people)...)
	}
	if len(wave) > 0 {
		s.push(ctx, tokensOf(wave), "Booking dates changed", fmt.Sprintf("%s · %s now %s to %s", q.guest, q.code, pgDate(in), pgDate(out)), s.alertData(q, "booking_dates_changed"))
	}
}
