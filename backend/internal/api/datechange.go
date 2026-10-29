package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/bookdan/backend/internal/httpx"
)

// Changing the dates of a booking. A guest can move a CONFIRMED booking to other dates once, when all of this holds:
//   - the booking is refundable, has no coupon, and was not moved before;
//   - both the old and the new check-in are still more than 48 hours away (the same point where a cancellation stops being free), so the
//     booking stays cancellable under the usual rule;
//   - the same room type has the rooms free on every new night.
//
// What the new dates cost decides what happens (the guest sees it first, with POST .../change-dates/quote, and confirms the new total):
//   - same price:   the booking moves, nothing else changes;
//   - cheaper:      the booking moves, is repriced, and the difference goes back to the guest as a partial refund the hotel bears (its share, BookDan's
//     commission and the VAT shrink in proportion, as if the booking had been made at the new price);
//   - dearer:       the guest pays the difference (a second payment, kind date_change). The new nights are held for 15 minutes; when the payment arrives
//     the booking moves, is repriced, and the hotel's share and the commission on the extra money are booked. If the payment does not arrive in time the
//     request ends and the booking stays as it was; money that arrives later is refunded. The request can be cancelled at any time before it is paid;
//   - paid at the hotel, or a price that changes only the balance due at the hotel: the booking moves and is repriced, no money moves through BookDan.
//
// The hotel is alerted and the guest is emailed once the booking has moved; the first dates stay in original_check_in/out and the change is in the audit log.

const dateChangeTTL = 15 * time.Minute

// nightsOf lists the nights of a stay (check-in up to the night before check-out).
func nightsOf(in, out string) []string {
	a, _ := time.Parse("2006-01-02", in)
	b, _ := time.Parse("2006-01-02", out)
	var nights []string
	for d := a; d.Before(b); d = d.AddDate(0, 0, 1) {
		nights = append(nights, d.Format("2006-01-02"))
	}
	return nights
}

// except returns the nights in a that are not in b.
func except(a, b []string) []string {
	skip := map[string]bool{}
	for _, n := range b {
		skip[n] = true
	}
	var out []string
	for _, n := range a {
		if !skip[n] {
			out = append(out, n)
		}
	}
	return out
}

// newNights are the nights the booking does not have yet; oldNights the ones it gives up. Nights in both stay as they are, so a guest's own rooms are
// never counted twice when the new dates overlap the old ones.
func newNights(oldIn, oldOut, newIn, newOut string) []string {
	return except(nightsOf(newIn, newOut), nightsOf(oldIn, oldOut))
}
func oldNights(oldIn, oldOut, newIn, newOut string) []string {
	return except(nightsOf(oldIn, oldOut), nightsOf(newIn, newOut))
}

// takeNights adds rooms to every one of the given nights (column held or booked), or to none of them (SOLD_OUT).
func takeNights(ctx context.Context, tx pgx.Tx, column, roomTypeID string, nights []string, rooms int) error {
	if len(nights) == 0 {
		return nil
	}
	tag, err := tx.Exec(ctx, `UPDATE inventory SET `+column+` = `+column+` + $3
		WHERE room_type_id = $1 AND night = ANY($2::date[]) AND held + booked + $3 <= capacity`, roomTypeID, nights, rooms)
	if err != nil {
		return err
	}
	if int(tag.RowsAffected()) != len(nights) {
		return fail(http.StatusConflict, codeSoldOut, "The room is not free on all of those nights")
	}
	return nil
}

// moveRooms moves a confirmed booking's rooms from its old nights to its new ones, all or nothing. held says the new nights were already held
// (by a request waiting for its payment) and only need to become booked.
func moveRooms(ctx context.Context, tx pgx.Tx, roomTypeID, oldIn, oldOut, newIn, newOut string, rooms int, held bool) error {
	add, drop := newNights(oldIn, oldOut, newIn, newOut), oldNights(oldIn, oldOut, newIn, newOut)
	if len(add) > 0 {
		if held {
			if _, err := tx.Exec(ctx, `UPDATE inventory SET held = held - $3, booked = booked + $3 WHERE room_type_id = $1 AND night = ANY($2::date[])`, roomTypeID, add, rooms); err != nil {
				return err
			}
		} else {
			if err := ensureInventory(ctx, tx, roomTypeID, newIn, newOut); err != nil {
				return err
			}
			if err := takeNights(ctx, tx, "booked", roomTypeID, add, rooms); err != nil {
				return err
			}
		}
	}
	if len(drop) > 0 {
		if _, err := tx.Exec(ctx, `UPDATE inventory SET booked = booked - $3 WHERE room_type_id = $1 AND night = ANY($2::date[])`, roomTypeID, drop, rooms); err != nil {
			return err
		}
	}
	return nil
}

// repriceBooking moves a booking to its new dates and price. The database lets the amounts change only when this says so (app.reprice).
func repriceBooking(ctx context.Context, tx pgx.Tx, bookingID, oldIn, oldOut, newIn, newOut string, base, service, vat, total, deposit, balance int64) error {
	if _, err := tx.Exec(ctx, `SELECT set_config('app.reprice', 'on', true)`); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE bookings SET check_in = $2::date, check_out = $3::date, base_santim = $4, service_santim = $5, vat_santim = $6, total_santim = $7,
			deposit_santim = $8, balance_due_santim = $9, date_changes = date_changes + 1,
			original_check_in = coalesce(original_check_in, $10::date), original_check_out = coalesce(original_check_out, $11::date) WHERE id = $1`,
		bookingID, newIn, newOut, base, service, vat, total, deposit, balance, oldIn, oldOut); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `SELECT set_config('app.reprice', 'off', true)`)
	return err
}

// canChangeDates says whether a booking can be moved now (the price check happens when the guest picks dates).
func (s *Server) canChangeDates(status string, checkIn time.Time, refundable bool, coupon *string, changes int) bool {
	if status != "CONFIRMED" || !refundable || coupon != nil || changes >= 1 {
		return false
	}
	_, until := cancelWindow(checkIn)
	return !s.now().After(until)
}

// ---- looking at a change before it is made

// movingBooking is a booking that a guest wants to move, read under a lock.
type movingBooking struct {
	ID, Code, Status, RoomType, Meal, Currency     string
	OldIn, OldOut                                  time.Time
	Rooms, Changes                                 int
	Refundable                                     bool
	Coupon                                         *string
	Base, Service, VAT, Total, Deposit, BalanceDue int64
	Paid                                           int64 // paid online (all payments)
}

type changeRequest struct {
	CheckIn  string `json:"check_in"`
	CheckOut string `json:"check_out"`
	// ConfirmTotal is the new total the guest agreed to. Required when the price differs.
	ConfirmTotal int64  `json:"confirm_total_santim"`
	Method       string `json:"method"`
}

func (c changeRequest) valid(s *Server) error {
	bad := func(m string) error { return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, m) }
	if !isDate(c.CheckIn) || !isDate(c.CheckOut) {
		return bad("check_in and check_out must be dates (YYYY-MM-DD)")
	}
	if n := nightsBetween(c.CheckIn, c.CheckOut); n < 1 || n > maxNights {
		return bad(fmt.Sprintf("A stay is 1 to %d nights", maxNights))
	}
	today := s.addisToday()
	if c.CheckIn < today || nightsBetween(today, c.CheckIn) > maxAdvanceDays {
		return bad("Choose dates from today up to a year ahead")
	}
	return nil
}

// loadMoving reads and locks the guest's booking and checks that it may be moved at all (not yet: to which dates).
func (s *Server) loadMoving(ctx context.Context, tx pgx.Tx, id, uid string) (movingBooking, error) {
	var m movingBooking
	err := tx.QueryRow(ctx, `SELECT b.id, b.code, b.status, b.room_type_id, b.check_in, b.check_out, b.rooms, b.refundable, b.meal, b.currency, cp.code, b.date_changes,
			b.base_santim, b.service_santim, b.vat_santim, b.total_santim, b.deposit_santim, b.balance_due_santim,
			coalesce((SELECT sum(p.amount_santim) FROM payments p WHERE p.booking_id = b.id AND p.status = 'succeeded'), 0)::bigint
		FROM bookings b LEFT JOIN coupons cp ON cp.id = b.coupon_id WHERE b.id = $1 AND b.user_id = $2 FOR UPDATE OF b`, id, uid).
		Scan(&m.ID, &m.Code, &m.Status, &m.RoomType, &m.OldIn, &m.OldOut, &m.Rooms, &m.Refundable, &m.Meal, &m.Currency, &m.Coupon, &m.Changes,
			&m.Base, &m.Service, &m.VAT, &m.Total, &m.Deposit, &m.BalanceDue, &m.Paid)
	if err == pgx.ErrNoRows || isBadUUID(err) {
		return m, errNotFound
	}
	if err != nil {
		return m, err
	}
	switch {
	case m.Status != "CONFIRMED":
		return m, fail(http.StatusConflict, codeInvalidState, "Only a confirmed booking can be moved to other dates")
	case m.Changes >= 1:
		return m, fail(http.StatusConflict, "CHANGE_LIMIT", "This booking was already moved once. Cancel it and book again if the dates changed again.")
	case !m.Refundable:
		return m, fail(http.StatusConflict, "NON_REFUNDABLE", "A non-refundable booking cannot be moved to other dates")
	case m.Coupon != nil:
		return m, fail(http.StatusConflict, "COUPON_NO_CHANGE", "A booking with a coupon cannot be moved to other dates")
	}
	var pending bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM date_change_requests WHERE booking_id = $1 AND status = 'pending' AND expires_at > now())`, id).Scan(&pending); err != nil {
		return m, err
	}
	if pending {
		return m, fail(http.StatusConflict, "CHANGE_PENDING", "A change of dates is already waiting for its payment. Pay it, or cancel it first.")
	}
	if _, until := cancelWindow(m.OldIn); s.now().After(until) {
		return m, fail(http.StatusConflict, "CHANGE_TOO_LATE", "Dates can be changed until 48 hours before check-in. After that, contact the hotel.")
	}
	return m, nil
}

// dateChangePlan is what moving a booking to certain dates would do.
type dateChangePlan struct {
	Kind string // same_price | refund | pay | reprice
	In   string
	Out  string
	P    priced
	// Delta is the new deposit minus the old one: what the guest pays online now (if positive) or gets back (if negative).
	Delta int64
}

// planDateChange prices the new dates and says what would happen, or why it cannot.
func (s *Server) planDateChange(ctx context.Context, tx pgx.Tx, m movingBooking, in, out string) (dateChangePlan, error) {
	plan := dateChangePlan{In: in, Out: out}
	if in == pgDate(m.OldIn) && out == pgDate(m.OldOut) {
		return plan, fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "These are the dates you already have")
	}
	newIn, _ := time.Parse("2006-01-02", in)
	if _, until := cancelWindow(newIn); s.now().After(until) {
		return plan, fail(http.StatusConflict, "NEW_DATES_TOO_SOON", "The new check-in must be more than 48 hours away.")
	}
	p, err := s.priceStay(ctx, tx, m.RoomType, in, out, m.Rooms, rateOption{Meal: m.Meal, Refundable: m.Refundable}, nil)
	if err != nil {
		return plan, err
	}
	if p.Currency != m.Currency {
		return plan, fail(http.StatusConflict, codeInvalidState, "These dates are priced in another currency")
	}
	plan.P, plan.Delta = p, p.Deposit-m.Deposit
	switch {
	case p.Base == m.Base && p.Service == m.Service && p.VAT == m.VAT && p.Total == m.Total && p.Deposit == m.Deposit:
		plan.Kind = "same_price"
	case m.Paid <= 0 || plan.Delta == 0:
		plan.Kind = "reprice" // nothing is paid through BookDan, or the change only moves the balance due at the hotel
	case plan.Delta < 0:
		plan.Kind = "refund"
	default:
		plan.Kind = "pay"
	}
	// The same room type must have the rooms free on the nights the booking does not have yet.
	add := newNights(pgDate(m.OldIn), pgDate(m.OldOut), in, out)
	if len(add) > 0 {
		var free int
		if err := tx.QueryRow(ctx, `SELECT count(*)::int FROM unnest($2::date[]) AS n(night) JOIN room_types rt ON rt.id = $1
			LEFT JOIN inventory i ON i.room_type_id = rt.id AND i.night = n.night
			WHERE coalesce(i.capacity, rt.total_rooms) - coalesce(i.held, 0) - coalesce(i.booked, 0) >= $3`, m.RoomType, add, m.Rooms).Scan(&free); err != nil {
			return plan, err
		}
		if free != len(add) {
			return plan, fail(http.StatusConflict, codeSoldOut, "The room is not free on all of those nights")
		}
	}
	return plan, nil
}

type changeQuoteOut struct {
	httpx.Meta
	CheckIn         string `json:"check_in"`
	CheckOut        string `json:"check_out"`
	Nights          int    `json:"nights"`
	Kind            string `json:"kind"`
	Currency        string `json:"currency"`
	OldTotal        int64  `json:"old_total_santim"`
	NewTotal        int64  `json:"new_total_santim"`
	Difference      int64  `json:"difference_santim"`
	PayNow          int64  `json:"pay_now_santim"`
	RefundNow       int64  `json:"refund_now_santim"`
	OldBalanceDue   int64  `json:"old_balance_due_santim"`
	NewBalanceDue   int64  `json:"new_balance_due_santim"`
	PaysAtTheHotel  bool   `json:"difference_at_hotel"`
	HoldMinutesText string `json:"-"`
}

// changeDatesQuote shows what moving to the given dates would cost or return, and changes nothing.
func (s *Server) changeDatesQuote(w http.ResponseWriter, r *http.Request) error {
	var in changeRequest
	if err := decode(r, &in); err != nil {
		return err
	}
	if err := in.valid(s); err != nil {
		return err
	}
	id, uid, ctx := chi.URLParam(r, "id"), claimsFrom(r).Sub, r.Context()
	var out changeQuoteOut
	err := s.tx(ctx, func(tx pgx.Tx) error {
		m, err := s.loadMoving(ctx, tx, id, uid)
		if err != nil {
			return err
		}
		plan, err := s.planDateChange(ctx, tx, m, in.CheckIn, in.CheckOut)
		if err != nil {
			return err
		}
		out = changeQuoteOut{Meta: httpx.NowMeta(), CheckIn: in.CheckIn, CheckOut: in.CheckOut, Nights: nightsBetween(in.CheckIn, in.CheckOut), Kind: plan.Kind, Currency: m.Currency,
			OldTotal: m.Total, NewTotal: plan.P.Total, Difference: plan.P.Total - m.Total, OldBalanceDue: m.BalanceDue, NewBalanceDue: plan.P.Balance}
		switch plan.Kind {
		case "pay":
			out.PayNow = plan.Delta
		case "refund":
			out.RefundNow = -plan.Delta
		case "reprice":
			out.PaysAtTheHotel = true
		}
		return nil
	})
	if err != nil {
		return err
	}
	respond(w, http.StatusOK, out)
	return nil
}

// ---- making the change

func (s *Server) changeDates(w http.ResponseWriter, r *http.Request) error {
	var in changeRequest
	if err := decode(r, &in); err != nil {
		return err
	}
	if err := in.valid(s); err != nil {
		return err
	}
	if in.Method == "" {
		in.Method = "chapa"
	}
	if in.Method != "chapa" && in.Method != "telebirr" {
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "method must be chapa or telebirr")
	}
	id, uid, ctx := chi.URLParam(r, "id"), claimsFrom(r).Sub, r.Context()
	moved, refund := false, false
	err := s.tx(ctx, func(tx pgx.Tx) error {
		m, err := s.loadMoving(ctx, tx, id, uid)
		if err != nil {
			return err
		}
		plan, err := s.planDateChange(ctx, tx, m, in.CheckIn, in.CheckOut)
		if err != nil {
			return err
		}
		if plan.Kind != "same_price" && in.ConfirmTotal != plan.P.Total {
			return fail(http.StatusConflict, "PRICE_DIFFERS", fmt.Sprintf("The new dates cost %s instead of %s. Confirm the new price to move the booking.", formatETB(plan.P.Total), formatETB(m.Total)))
		}
		ci, co := pgDate(m.OldIn), pgDate(m.OldOut)
		p := plan.P

		if plan.Kind == "pay" {
			// The new nights are held while the difference is paid; the booking itself moves when the payment arrives.
			if err := ensureInventory(ctx, tx, m.RoomType, in.CheckIn, in.CheckOut); err != nil {
				return err
			}
			if err := takeNights(ctx, tx, "held", m.RoomType, newNights(ci, co, in.CheckIn, in.CheckOut), m.Rooms); err != nil {
				return err
			}
			var reqID string
			if err := tx.QueryRow(ctx, `INSERT INTO date_change_requests (booking_id, old_check_in, old_check_out, new_check_in, new_check_out, base_santim, service_santim, vat_santim,
					total_santim, deposit_santim, balance_due_santim, pay_santim, expires_at)
				VALUES ($1, $2::date, $3::date, $4::date, $5::date, $6, $7, $8, $9, $10, $11, $12, now() + $13::interval) RETURNING id`,
				id, ci, co, in.CheckIn, in.CheckOut, p.Base, p.Service, p.VAT, p.Total, p.Deposit, p.Balance, plan.Delta, fmt.Sprintf("%d seconds", int(dateChangeTTL.Seconds()))).Scan(&reqID); err != nil {
				return err
			}
			if err := s.startSupplement(ctx, tx, id, m.Code, reqID, in.Method, plan.Delta, m.Currency); err != nil {
				return err
			}
			return s.audit(r, tx, "booking_dates_change_requested", map[string]any{"booking_id": id, "from": ci + " to " + co, "to": in.CheckIn + " to " + in.CheckOut, "pay_santim": plan.Delta})
		}

		if err := moveRooms(ctx, tx, m.RoomType, ci, co, in.CheckIn, in.CheckOut, m.Rooms, false); err != nil {
			return err
		}
		if err := repriceBooking(ctx, tx, id, ci, co, in.CheckIn, in.CheckOut, p.Base, p.Service, p.VAT, p.Total, p.Deposit, p.Balance); err != nil {
			return err
		}
		if plan.Kind == "refund" {
			// The new dates cost less: the difference goes back to the guest and the hotel bears it, as if the booking had been made at the new price.
			if err := s.partialRefund(ctx, tx, id, nil, "hotel", -plan.Delta, "Dates changed to a cheaper stay", "guest:"+uid); err != nil {
				return err
			}
			refund = true
		}
		moved = true
		return s.audit(r, tx, "booking_dates_changed", map[string]any{"booking_id": id, "from": ci + " to " + co, "to": in.CheckIn + " to " + in.CheckOut, "kind": plan.Kind, "difference_santim": p.Total - m.Total})
	})
	if err != nil {
		return err
	}
	if refund {
		s.runRefunds(ctx)
	}
	if moved {
		s.alertHotelOfDateChange(ctx, id)
		s.emailGuest(ctx, id, "dates_changed")
	}
	b, err := s.loadBooking(ctx, s.pool, id, uid)
	if err != nil {
		return err
	}
	respond(w, http.StatusOK, b)
	return nil
}

// startSupplement opens the payment of the difference for a request. A payment still open for it is replaced.
func (s *Server) startSupplement(ctx context.Context, tx pgx.Tx, bookingID, code, reqID, method string, amount int64, cur string) error {
	if _, err := tx.Exec(ctx, `UPDATE payments SET status = 'failed' WHERE booking_id = $1 AND kind = 'date_change' AND status = 'pending'`, bookingID); err != nil {
		return err
	}
	var pid string
	if err := tx.QueryRow(ctx, `INSERT INTO payments (booking_id, provider, provider_ref, amount_santim, currency, kind) VALUES ($1, $2, $3, $4, $5, 'date_change') RETURNING id`,
		bookingID, s.p.Gateway.Name(), "tmp_"+randomToken(12), amount, cur).Scan(&pid); err != nil {
		return err
	}
	gs, err := s.p.Gateway.CreateSession(ctx, pid, code, method, amount, cur)
	if err != nil {
		return fail(http.StatusBadGateway, codePaymentFailed, "The payment service could not start the payment")
	}
	if _, err := tx.Exec(ctx, `UPDATE payments SET provider_ref = $2, payment_url = $3 WHERE id = $1`, pid, gs.ProviderRef, gs.PaymentURL); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE date_change_requests SET payment_id = $2 WHERE id = $1`, reqID, pid)
	return err
}

// payDateChange starts (or resumes) the payment of a waiting request, for a guest who closed the payment page.
func (s *Server) payDateChange(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Method string `json:"method"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if in.Method == "" {
		in.Method = "chapa"
	}
	if in.Method != "chapa" && in.Method != "telebirr" {
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "method must be chapa or telebirr")
	}
	id, uid, ctx := chi.URLParam(r, "id"), claimsFrom(r).Sub, r.Context()
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var code, cur string
		if err := tx.QueryRow(ctx, `SELECT code, currency FROM bookings WHERE id = $1 AND user_id = $2 AND status = 'CONFIRMED' FOR UPDATE`, id, uid).Scan(&code, &cur); err != nil {
			if err == pgx.ErrNoRows || isBadUUID(err) {
				return errNotFound
			}
			return err
		}
		var reqID string
		var amount int64
		var payID *string
		err := tx.QueryRow(ctx, `SELECT id, pay_santim, payment_id::text FROM date_change_requests WHERE booking_id = $1 AND status = 'pending' AND expires_at > now() FOR UPDATE`, id).Scan(&reqID, &amount, &payID)
		if err == pgx.ErrNoRows {
			return fail(http.StatusConflict, "NO_PENDING_CHANGE", "There is no change of dates waiting for a payment")
		}
		if err != nil {
			return err
		}
		if payID != nil {
			var open bool
			if err := tx.QueryRow(ctx, `SELECT status = 'pending' FROM payments WHERE id = $1`, *payID).Scan(&open); err != nil {
				return err
			}
			if open {
				return nil // the payment page is still good: the booking shows its address
			}
		}
		return s.startSupplement(ctx, tx, id, code, reqID, in.Method, amount, cur)
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

// cancelDateChange drops a change that is waiting for its payment: the new nights are released and the booking stays as it was.
func (s *Server) cancelDateChange(w http.ResponseWriter, r *http.Request) error {
	id, uid, ctx := chi.URLParam(r, "id"), claimsFrom(r).Sub, r.Context()
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var one int
		if err := tx.QueryRow(ctx, `SELECT 1 FROM bookings WHERE id = $1 AND user_id = $2 FOR UPDATE`, id, uid).Scan(&one); err != nil {
			if err == pgx.ErrNoRows || isBadUUID(err) {
				return errNotFound
			}
			return err
		}
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*)::int FROM date_change_requests WHERE booking_id = $1 AND status = 'pending'`, id).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			return fail(http.StatusConflict, "NO_PENDING_CHANGE", "There is no change of dates waiting for a payment")
		}
		if err := abandonDateChange(ctx, tx, id, "the guest cancelled the change"); err != nil {
			return err
		}
		return s.audit(r, tx, "booking_dates_change_cancelled", map[string]any{"booking_id": id})
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

// ---- the end of a request

// releaseHeld gives back the nights a request was holding.
func releaseHeld(ctx context.Context, tx pgx.Tx, roomTypeID, oldIn, oldOut, newIn, newOut string, rooms int) error {
	add := newNights(oldIn, oldOut, newIn, newOut)
	if len(add) == 0 {
		return nil
	}
	_, err := tx.Exec(ctx, `UPDATE inventory SET held = greatest(held - $3, 0) WHERE room_type_id = $1 AND night = ANY($2::date[])`, roomTypeID, add, rooms)
	return err
}

// abandonDateChange ends the booking's waiting request (if it has one) and releases its nights. The booking row must be locked by the caller. A payment that
// arrives afterwards is refunded.
func abandonDateChange(ctx context.Context, tx pgx.Tx, bookingID, why string) error {
	var reqID, roomType string
	var rooms int
	var oldIn, oldOut, newIn, newOut time.Time
	err := tx.QueryRow(ctx, `SELECT r.id, b.room_type_id, b.rooms, r.old_check_in, r.old_check_out, r.new_check_in, r.new_check_out
		FROM date_change_requests r JOIN bookings b ON b.id = r.booking_id WHERE r.booking_id = $1 AND r.status = 'pending' FOR UPDATE OF r`, bookingID).
		Scan(&reqID, &roomType, &rooms, &oldIn, &oldOut, &newIn, &newOut)
	if err == pgx.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	if err := releaseHeld(ctx, tx, roomType, pgDate(oldIn), pgDate(oldOut), pgDate(newIn), pgDate(newOut), rooms); err != nil {
		return err
	}
	slog.Info("date change request ended", "request", reqID, "why", why)
	_, err = tx.Exec(ctx, `UPDATE date_change_requests SET status = 'cancelled', resolved_at = now() WHERE id = $1`, reqID)
	return err
}

// expireDateChanges ends the requests whose payment did not arrive in time, and releases their nights. The payment itself is left alone: if the guest
// still pays, the money is refunded (settleDateChangePayment).
func (s *Server) expireDateChanges(ctx context.Context) {
	err := s.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT r.id, b.room_type_id, b.rooms, r.old_check_in, r.old_check_out, r.new_check_in, r.new_check_out
			FROM date_change_requests r JOIN bookings b ON b.id = r.booking_id WHERE r.status = 'pending' AND r.expires_at <= now()
			ORDER BY r.expires_at LIMIT 100 FOR UPDATE OF r SKIP LOCKED`)
		if err != nil {
			return err
		}
		type due struct {
			id, roomType                 string
			rooms                        int
			oldIn, oldOut, newIn, newOut time.Time
		}
		var list []due
		for rows.Next() {
			var d due
			if err := rows.Scan(&d.id, &d.roomType, &d.rooms, &d.oldIn, &d.oldOut, &d.newIn, &d.newOut); err != nil {
				rows.Close()
				return err
			}
			list = append(list, d)
		}
		rows.Close()
		sort.Slice(list, func(i, j int) bool { return list[i].id < list[j].id })
		for _, d := range list {
			if err := releaseHeld(ctx, tx, d.roomType, pgDate(d.oldIn), pgDate(d.oldOut), pgDate(d.newIn), pgDate(d.newOut), d.rooms); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE date_change_requests SET status = 'expired', resolved_at = now() WHERE id = $1`, d.id); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		slog.Error("expire date changes", "err", err)
	}
}

// settleDateChangePayment is what the arrival of the payment of a difference does. The payment and its booking are locked by the caller. It moves the booking
// when the request is still waiting and the booking is still as it was, and returns true; otherwise the money is refunded (the request ran out, the guest
// cancelled it, or the booking was cancelled meanwhile) and it returns false.
func (s *Server) settleDateChangePayment(ctx context.Context, tx pgx.Tx, payID, bookingID string, amount int64, cur string) (bool, error) {
	var reqID, reqStatus string
	var oldIn, oldOut, newIn, newOut time.Time
	var base, service, vat, total, deposit, balance int64
	err := tx.QueryRow(ctx, `SELECT id, status, old_check_in, old_check_out, new_check_in, new_check_out, base_santim, service_santim, vat_santim, total_santim, deposit_santim, balance_due_santim
		FROM date_change_requests WHERE payment_id = $1 FOR UPDATE`, payID).
		Scan(&reqID, &reqStatus, &oldIn, &oldOut, &newIn, &newOut, &base, &service, &vat, &total, &deposit, &balance)
	if err == pgx.ErrNoRows {
		return false, accrueRefund(ctx, tx, bookingID, payID, cur, amount)
	}
	if err != nil {
		return false, err
	}
	var status, roomType string
	var rooms, changes int
	if err := tx.QueryRow(ctx, `SELECT status, room_type_id, rooms, date_changes FROM bookings WHERE id = $1`, bookingID).Scan(&status, &roomType, &rooms, &changes); err != nil {
		return false, err
	}
	ci, co, ni, no := pgDate(oldIn), pgDate(oldOut), pgDate(newIn), pgDate(newOut)
	if reqStatus != "pending" || status != "CONFIRMED" || changes >= 1 {
		if reqStatus == "pending" {
			if err := releaseHeld(ctx, tx, roomType, ci, co, ni, no, rooms); err != nil {
				return false, err
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE date_change_requests SET status = 'refunded', resolved_at = coalesce(resolved_at, now()) WHERE id = $1`, reqID); err != nil {
			return false, err
		}
		return false, accrueRefund(ctx, tx, bookingID, payID, cur, amount)
	}
	if err := moveRooms(ctx, tx, roomType, ci, co, ni, no, rooms, true); err != nil {
		return false, err
	}
	if err := repriceBooking(ctx, tx, bookingID, ci, co, ni, no, base, service, vat, total, deposit, balance); err != nil {
		return false, err
	}
	// The hotel earns on the extra money like on any payment: its share, BookDan's commission and the VAT on it.
	if err := ledgerConfirmed(ctx, tx, bookingID, payID, cur, "dates changed: our commission and the hotel's share of the extra payment", amount, 0); err != nil {
		return false, err
	}
	if _, err := tx.Exec(ctx, `UPDATE date_change_requests SET status = 'applied', resolved_at = now() WHERE id = $1`, reqID); err != nil {
		return false, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO audit_log (action, details) VALUES ('booking_dates_changed', $1)`,
		map[string]any{"booking_id": bookingID, "from": ci + " to " + co, "to": ni + " to " + no, "kind": "pay", "paid_santim": amount})
	return true, err
}

// ---- what the booking shows while a change waits for its payment

type pendingChange struct {
	CheckIn  string `json:"check_in"`
	CheckOut string `json:"check_out"`
	NewTotal int64  `json:"new_total_santim"`
	PayNow   int64  `json:"pay_now_santim"`
	Expires  string `json:"expires_at"`
	Payment  *struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		URL    string `json:"payment_url"`
	} `json:"payment"`
}

// pendingChangeOf reads the booking's waiting change of dates, if it has one.
func pendingChangeOf(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, bookingID string) (*pendingChange, error) {
	var pc pendingChange
	var in, out, exp time.Time
	var pid, pstatus, purl *string
	err := q.QueryRow(ctx, `SELECT r.new_check_in, r.new_check_out, r.total_santim, r.pay_santim, r.expires_at, p.id::text, p.status, p.payment_url
		FROM date_change_requests r LEFT JOIN payments p ON p.id = r.payment_id WHERE r.booking_id = $1 AND r.status = 'pending' AND r.expires_at > now()`, bookingID).
		Scan(&in, &out, &pc.NewTotal, &pc.PayNow, &exp, &pid, &pstatus, &purl)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	pc.CheckIn, pc.CheckOut, pc.Expires = pgDate(in), pgDate(out), exp.UTC().Format(time.RFC3339)
	if pid != nil {
		pc.Payment = &struct {
			ID     string `json:"id"`
			Status string `json:"status"`
			URL    string `json:"payment_url"`
		}{*pid, deref(pstatus), deref(purl)}
	}
	return &pc, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
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
