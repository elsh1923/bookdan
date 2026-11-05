package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/bookdan/backend/internal/httpx"
	"github.com/bookdan/backend/internal/ops"
)

// ---- ledger. Every money movement is a balanced, append-only transaction (the database refuses an unbalanced one).

type entry struct {
	account string
	santim  int64
}

func ledger(ctx context.Context, tx pgx.Tx, bookingID, paymentID, kind, memo, currency string, entries ...entry) error {
	var txID string
	var pay any
	if paymentID != "" {
		pay = paymentID
	}
	if err := tx.QueryRow(ctx, `INSERT INTO ledger_transactions (booking_id, payment_id, kind, memo) VALUES ($1, $2, $3, $4) RETURNING id`,
		bookingID, pay, kind, memo).Scan(&txID); err != nil {
		return err
	}
	for _, e := range entries {
		if e.santim == 0 {
			continue
		}
		if _, err := tx.Exec(ctx, `INSERT INTO ledger_entries (transaction_id, account_code, amount_santim, currency) VALUES ($1, $2, $3, $4)`,
			txID, e.account, e.santim, currency); err != nil {
			return err
		}
	}
	return nil
}

// accrueRefund records that a guest is owed their deposit back. The money itself moves in runRefunds.
func accrueRefund(ctx context.Context, tx pgx.Tx, bookingID, paymentID, currency string, amount int64) error {
	return ledger(ctx, tx, bookingID, paymentID, "refund", "refund owed to the guest", currency,
		entry{"guest_deposits_held", amount}, entry{"refunds_payable", -amount})
}

// ---- starting a payment

type sessionOut struct {
	ID        string `json:"id"`
	BookingID string `json:"booking_id"`
	Method    string `json:"method"`
	URL       string `json:"payment_url"`
}

func (s *Server) payBooking(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Method string `json:"method"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if in.Method != "telebirr" && in.Method != "chapa" && in.Method != "pay_at_hotel" {
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "method must be telebirr, chapa or pay_at_hotel")
	}
	id, uid, ctx := chi.URLParam(r, "id"), claimsFrom(r).Sub, r.Context()

	var session *sessionOut
	requested := false
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var status, cur, code string
		var hold *time.Time
		var deposit int64
		var payAtHotel bool
		var chosen *string
		var discount int64
		err := tx.QueryRow(ctx, `SELECT b.status, b.hold_expires_at, b.deposit_santim, b.currency, b.code, p.pay_at_hotel, b.payment_method, b.discount_santim
			FROM bookings b JOIN properties p ON p.id = b.property_id WHERE b.id = $1 AND b.user_id = $2 FOR UPDATE OF b`, id, uid).
			Scan(&status, &hold, &deposit, &cur, &code, &payAtHotel, &chosen, &discount)
		if err == pgx.ErrNoRows || isBadUUID(err) {
			return errNotFound
		}
		if err != nil {
			return err
		}
		if status != "HELD" || hold == nil || !hold.After(s.now()) {
			return fail(http.StatusConflict, codeHoldExpired, "The hold has ended")
		}

		if in.Method == "pay_at_hotel" {
			if !payAtHotel {
				return fail(http.StatusBadRequest, codePaymentFailed, "This hotel does not take pay-at-hotel bookings")
			}
			if discount > 0 {
				return fail(http.StatusConflict, "COUPON_ONLINE_ONLY", "A booking with a coupon is paid online")
			}
			if _, err := tx.Exec(ctx, `UPDATE payments SET status = 'failed' WHERE booking_id = $1 AND status = 'pending' AND kind = 'booking'`, id); err != nil {
				return err
			}
			if err := actor(ctx, tx, "guest:"+uid, "chose to pay at the hotel"); err != nil {
				return err
			}
			requested = true
			_, err := tx.Exec(ctx, `UPDATE bookings SET status = 'PAID_AWAITING_HOTEL', payment_method = 'pay_at_hotel', hold_expires_at = NULL,
				hotel_deadline_at = now() + $2::interval, paid_at = now() WHERE id = $1`, id, "900 seconds")
			return err
		}

		if deposit <= 0 {
			return fail(http.StatusConflict, codeInvalidState, "Nothing is payable online for this booking")
		}
		// Asking again while a payment of the same kind is pending returns the same page: never a second charge.
		var pid, url string
		err = tx.QueryRow(ctx, `SELECT id, coalesce(payment_url, '') FROM payments WHERE booking_id = $1 AND status = 'pending' AND kind = 'booking'`, id).Scan(&pid, &url)
		if err == nil && chosen != nil && *chosen == in.Method {
			session = &sessionOut{pid, id, in.Method, url}
			return nil
		}
		if err == nil {
			if _, err := tx.Exec(ctx, `UPDATE payments SET status = 'failed' WHERE id = $1`, pid); err != nil {
				return err
			}
		} else if err != pgx.ErrNoRows {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO payments (booking_id, provider, provider_ref, amount_santim, currency) VALUES ($1, $2, $3, $4, $5) RETURNING id`,
			id, s.p.Gateway.Name(), "tmp_"+randomToken(12), deposit, cur).Scan(&pid); err != nil {
			return err
		}
		gs, err := s.p.Gateway.CreateSession(ctx, pid, code, in.Method, deposit, cur)
		if err != nil {
			return fail(http.StatusBadGateway, codePaymentFailed, "The payment service could not start the payment")
		}
		if _, err := tx.Exec(ctx, `UPDATE payments SET provider_ref = $2, payment_url = $3 WHERE id = $1`, pid, gs.ProviderRef, gs.PaymentURL); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE bookings SET payment_method = $2 WHERE id = $1`, id, in.Method); err != nil {
			return err
		}
		session = &sessionOut{pid, id, in.Method, gs.PaymentURL}
		return nil
	})
	if err != nil {
		return err
	}
	if requested {
		s.notifyHotel(ctx, id)
		s.emailGuest(ctx, id, "requested")
	}
	b, err := s.loadBooking(ctx, s.pool, id, uid)
	if err != nil {
		return err
	}
	respond(w, http.StatusOK, struct {
		httpx.Meta
		Booking bookingOut  `json:"booking"`
		Session *sessionOut `json:"session"`
	}{httpx.NowMeta(), b, session})
	return nil
}

// ---- payment results

// processPaymentEvent applies one gateway result. It is safe to call twice with the same event: the second call changes nothing.
func (s *Server) processPaymentEvent(ctx context.Context, provider, eventID, providerRef, status string, raw []byte) error {
	var notifyID, movedID string
	err := s.tx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `INSERT INTO payment_events (provider, event_id, payload) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, provider, eventID, json.RawMessage(raw))
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return nil // a repeat of an event we already have
		}
		var payID, bookingID, payStatus, bookingStatus, cur, kind string
		var amount int64
		err = tx.QueryRow(ctx, `SELECT p.id, p.booking_id, p.status, b.status, p.amount_santim, p.currency, p.kind
			FROM payments p JOIN bookings b ON b.id = p.booking_id WHERE p.provider = $1 AND p.provider_ref = $2 FOR UPDATE OF p, b`, provider, providerRef).
			Scan(&payID, &bookingID, &payStatus, &bookingStatus, &amount, &cur, &kind)
		if err == pgx.ErrNoRows {
			return errNotFound
		}
		if err != nil {
			return err
		}
		// A "failed" payment can still turn out to have been paid (results arrive late or out of order, or we closed it when the guest
		// switched method). Money received is never ignored: it confirms a booking that is still held, otherwise it is refunded.
		if payStatus == "failed" && status == "succeeded" {
			slog.Warn("payment succeeded after it was recorded as failed", "payment", payID, "booking", bookingID, "booking_status", bookingStatus)
		}
		if payStatus == "pending" || (payStatus == "failed" && status == "succeeded") {
			switch status {
			case "failed":
				if _, err := tx.Exec(ctx, `UPDATE payments SET status = 'failed' WHERE id = $1`, payID); err != nil {
					return err
				}
			case "succeeded":
				if _, err := tx.Exec(ctx, `UPDATE payments SET status = 'succeeded' WHERE id = $1`, payID); err != nil {
					return err
				}
				if err := ledger(ctx, tx, bookingID, payID, "deposit_received", "guest paid", cur,
					entry{"gateway_clearing", amount}, entry{"guest_deposits_held", -amount}); err != nil {
					return err
				}
				if kind == "date_change" {
					// The payment of the difference for new dates: it moves the booking, or (when the request ran out) it is refunded.
					applied, err := s.settleDateChangePayment(ctx, tx, payID, bookingID, amount, cur)
					if err != nil {
						return err
					}
					if applied {
						movedID = bookingID
					}
				} else if bookingStatus == "HELD" {
					// The rooms are still reserved for us (even if the clock ran out a moment ago), so honour the payment.
					if err := actor(ctx, tx, "gateway:"+provider, "payment succeeded"); err != nil {
						return err
					}
					if _, err := tx.Exec(ctx, `UPDATE bookings SET status = 'PAID_AWAITING_HOTEL', paid_at = now(), hold_expires_at = NULL,
						hotel_deadline_at = now() + $2::interval WHERE id = $1`, bookingID, "900 seconds"); err != nil {
						return err
					}
					notifyID = bookingID
				} else if err := accrueRefund(ctx, tx, bookingID, payID, cur, amount); err != nil {
					// The hold had already ended and the rooms were released: the money goes back (see runRefunds).
					return err
				}
			}
		}
		_, err = tx.Exec(ctx, `UPDATE payment_events SET processed_at = now() WHERE provider = $1 AND event_id = $2`, provider, eventID)
		return err
	})
	if err != nil {
		return err
	}
	if notifyID != "" {
		s.notifyHotel(ctx, notifyID)
		s.emailGuest(ctx, notifyID, "requested")
	}
	if movedID != "" {
		s.alertHotelOfDateChange(ctx, movedID)
		s.emailGuest(ctx, movedID, "dates_changed")
	}
	s.runRefunds(ctx)
	return nil
}

// paymentWebhook receives a gateway callback. It is trusted only when the body carries a valid HMAC signature.
func (s *Server) paymentWebhook(w http.ResponseWriter, r *http.Request) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, 64<<10))
	if err != nil {
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "Could not read the request")
	}
	if s.cfg.PaymentWebhookSecret == "" {
		return fail(http.StatusServiceUnavailable, httpx.CodeUnavailable, "Payment webhooks are not configured")
	}
	mac := hmac.New(sha256.New, []byte(s.cfg.PaymentWebhookSecret))
	mac.Write(body)
	sig, err := hex.DecodeString(r.Header.Get("X-Signature"))
	if err != nil || !hmac.Equal(sig, mac.Sum(nil)) {
		return errAuth
	}
	var ev struct {
		EventID    string `json:"event_id"`
		PaymentRef string `json:"payment_ref"`
		Status     string `json:"status"`
	}
	if err := json.Unmarshal(body, &ev); err != nil || ev.EventID == "" || ev.PaymentRef == "" || (ev.Status != "succeeded" && ev.Status != "failed") {
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "event_id, payment_ref and status are required")
	}
	if err := s.processPaymentEvent(r.Context(), chi.URLParam(r, "provider"), ev.EventID, ev.PaymentRef, ev.Status, body); err != nil {
		return err
	}
	respond(w, http.StatusOK, httpx.NowMeta())
	return nil
}

// Refunds. A deposit is owed back when the hotel declined, the request timed out, the hold had ended when the payment arrived, or the guest cancelled with
// a refund due. Paying it back has three steps, and none holds a database transaction open while it waits for the payment gateway:
//
//  1. claim: in a short transaction the refund is marked as started, its amount is fixed (what is left after partial refunds), and it is given a few minutes;
//  2. the gateway is asked to return the money, with a reference that makes asking twice safe (nothing is held while waiting);
//  3. in another short transaction the payment becomes 'refunded' and the ledger records it.
//
// If the server stops between 1 and 3, the claim runs out and the next pass asks the gateway again with the same reference and amount, finishes the record, and
// the guest is paid once. A refund the gateway refuses is tried again with a growing pause, and the operators are told. Once a refund has started, no partial
// refund can be added to that payment (see lockRefundable).
const (
	refundClaim    = 2 * time.Minute
	refundMaxPause = 10 * time.Minute
	refundTimeout  = 30 * time.Second
)

// refundPause is how long to wait before asking again after the nth try failed.
func refundPause(attempts int) time.Duration {
	p := time.Duration(attempts) * 20 * time.Second
	if p < 20*time.Second {
		p = 20 * time.Second
	}
	if p > refundMaxPause {
		p = refundMaxPause
	}
	return p
}

// runRefunds is called after something that makes a refund owed (a decline, a cancellation, a payment that arrived too late). It pays the first few that are due, so
// the person who caused it sees the money on its way; the sweeper pays the rest (sweepRefunds).
func (s *Server) runRefunds(ctx context.Context) { s.payRefunds(ctx, 5) }

// sweepRefunds is the timer's pass: every refund that is due.
func (s *Server) sweepRefunds(ctx context.Context) { s.payRefunds(ctx, 50) }

func (s *Server) payRefunds(ctx context.Context, limit int) {
	rows, err := s.pool.Query(ctx, `SELECT p.id FROM payments p JOIN bookings b ON b.id = p.booking_id
		WHERE p.status = 'succeeded' AND (p.refund_claimed_until IS NULL OR p.refund_claimed_until < now())
		  AND (b.status IN ('DECLINED_REFUNDED', 'EXPIRED') OR (b.status = 'CANCELLED_REFUNDED' AND coalesce(b.refund_santim, 0) > 0)
			OR (p.kind = 'date_change' AND NOT EXISTS (SELECT 1 FROM date_change_requests r WHERE r.payment_id = p.id AND r.status IN ('pending', 'applied'))))
		ORDER BY p.created_at LIMIT $1`, limit)
	if err != nil {
		slog.Error("refunds: list", "err", err)
		return
	}
	var ids []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	for _, id := range ids {
		s.refundPayment(ctx, id)
	}
	s.payPartialRefunds(ctx, limit)
}

// refundPayment pays back what is left of one payment (steps 1 to 3 above).
func (s *Server) refundPayment(ctx context.Context, id string) {
	// The work must finish even if the request that started it is closed.
	ctx = context.WithoutCancel(ctx)
	var ref, cur, bookingID string
	var amount int64
	var attempts int
	claimed := false
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var frozen *int64
		// What is left of the payment after partial refunds (which are refunded by their own gateway calls). A retry pays the amount decided at the first claim.
		err := tx.QueryRow(ctx, `SELECT provider_ref, amount_santim - coalesce((SELECT sum(r.amount_santim) FROM partial_refunds r WHERE r.payment_id = payments.id), 0)::bigint,
				currency, booking_id, refund_amount_santim, refund_attempts
			FROM payments WHERE id = $1 AND status = 'succeeded' AND (refund_claimed_until IS NULL OR refund_claimed_until < now()) FOR UPDATE SKIP LOCKED`, id).
			Scan(&ref, &amount, &cur, &bookingID, &frozen, &attempts)
		if err == pgx.ErrNoRows {
			return nil // someone else is refunding it, or it is done
		}
		if err != nil {
			return err
		}
		if frozen != nil {
			amount = *frozen
		}
		if amount <= 0 {
			// Everything was already returned by partial refunds.
			_, err := tx.Exec(ctx, `UPDATE payments SET status = 'refunded' WHERE id = $1`, id)
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE payments SET refund_amount_santim = $2, refund_started_at = coalesce(refund_started_at, now()),
				refund_claimed_until = now() + $3::interval, refund_attempts = refund_attempts + 1 WHERE id = $1`, id, amount, fmt.Sprintf("%d seconds", int(refundClaim.Seconds()))); err != nil {
			return err
		}
		attempts++
		claimed = true
		return nil
	})
	if err != nil {
		slog.Error("refund: claim", "payment", id, "err", err)
		return
	}
	if !claimed {
		return
	}

	// Step 2: the gateway, with nothing held.
	gwCtx, cancel := context.WithTimeout(ctx, refundTimeout)
	err = s.p.Gateway.Refund(gwCtx, ref, "", amount, cur)
	cancel()
	if err != nil {
		if _, e := s.pool.Exec(ctx, `UPDATE payments SET refund_claimed_until = now() + $2::interval, refund_last_error = $3 WHERE id = $1`,
			id, fmt.Sprintf("%d seconds", int(refundPause(attempts).Seconds())), err.Error()); e != nil {
			slog.Error("refund: schedule retry", "payment", id, "err", e)
		}
		slog.Error("refund failed, will retry", "payment", id, "attempt", attempts, "err", err)
		ops.Report("refund-failed", "A refund could not be paid", "Payment "+id+": "+err.Error()+"\nIt is tried again every few minutes. If it keeps failing, refund the guest by hand.")
		return
	}

	// Step 3: record it.
	err = s.tx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE payments SET status = 'refunded', refund_claimed_until = NULL, refund_last_error = NULL WHERE id = $1 AND status = 'succeeded'`, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return nil // already recorded by another pass
		}
		return ledger(ctx, tx, bookingID, id, "refund", "refund paid to the guest", cur,
			entry{"refunds_payable", amount}, entry{"gateway_clearing", -amount})
	})
	if err != nil {
		// The gateway has the money moving. The claim runs out and the next pass asks again (answered once) and records it.
		slog.Error("refund paid but not recorded, will finish on the next pass", "payment", id, "err", err)
		ops.Report("refund-not-recorded", "A refund was paid but not recorded", "Payment "+id+": "+err.Error()+"\nThe next pass finishes the record. No action is needed unless this repeats.")
	}
}

// ---- alert to the hotel

// notifyHotel, escalate and remind live in alerts.go.

// push sends an alert and forgets any phone the push service says is gone. Failures are logged, never returned: an alert
// that cannot be sent must not undo the booking that caused it.
func (s *Server) push(ctx context.Context, tokens []string, title, body string, data map[string]string) {
	s.QueuePush(ctx, tokens, title, body, data) // written to the outbox, sent at once, and tried again if it fails (outbox.go)
}

// deliverPush is the actual send, for the outbox: it asks the push service, and forgets any phone the service says is gone. A failure is returned so the outbox
// tries again.
func (s *Server) deliverPush(ctx context.Context, p pushPayload) error {
	invalid, err := s.p.Push.Notify(ctx, p.Tokens, p.Title, p.Body, p.Data)
	if len(invalid) > 0 {
		if _, e := s.pool.Exec(ctx, `UPDATE devices SET push_token = NULL WHERE push_token = ANY($1::text[])`, invalid); e != nil {
			slog.Error("forget push tokens", "err", e)
		}
	}
	return err
}

// Guest messages. The text is English because the server does not know the guest's language yet; the data carries the
// booking and its new status so the app can open the right screen.
var guestMessages = map[string][2]string{
	"confirmed":        {"Booking confirmed", "%s has confirmed your room. Your voucher is ready."},
	"declined":         {"Booking not available", "%s could not accept your request. You will get a full refund."},
	"no_response":      {"The hotel did not answer in time", "%s did not answer in time. You will get a full refund."},
	"hotel_cancelled":  {"The hotel cancelled your booking", "%s had to cancel your booking. You are refunded in full."},
	"needs_relocation": {"Your hotel cannot host you", "%s cannot host you. BookDan support is finding you another place, or you can cancel for a full refund."},
}

// notifyGuest tells the guest what the hotel decided. kind is confirmed, declined or no_response.
func (s *Server) notifyGuest(ctx context.Context, bookingID, kind string) {
	msg, ok := guestMessages[kind]
	if !ok {
		return
	}
	s.emailGuest(ctx, bookingID, kind) // the same news by email, with the receipt, for guests who signed in with an address
	rows, err := s.pool.Query(ctx, `SELECT d.push_token, p.name FROM bookings b JOIN properties p ON p.id = b.property_id
		JOIN devices d ON d.user_id = b.user_id AND d.app = 'guest' AND d.push_token IS NOT NULL AND d.revoked_at IS NULL
		WHERE b.id = $1`, bookingID)
	if err != nil {
		slog.Error("notify guest", "err", err)
		return
	}
	defer rows.Close()
	var tokens []string
	var hotel string
	for rows.Next() {
		var t string
		if rows.Scan(&t, &hotel) == nil {
			tokens = append(tokens, t)
		}
	}
	if len(tokens) == 0 {
		return
	}
	s.push(ctx, tokens, msg[0], fmt.Sprintf(msg[1], hotel), map[string]string{"booking_id": bookingID, "type": "booking_update", "status": kind, "channel": "booking_updates"})
}

// ---- voucher

func (s *Server) voucher(w http.ResponseWriter, r *http.Request) error {
	id, uid, ctx := chi.URLParam(r, "id"), claimsFrom(r).Sub, r.Context()
	var out struct {
		httpx.Meta
		BookingID     string  `json:"booking_id"`
		Reference     string  `json:"reference"`
		Token         string  `json:"token"`
		PIN           string  `json:"pin"`
		ExpiresAt     string  `json:"expires_at"`
		PropertyID    string  `json:"property_id"`
		PropertyName  string  `json:"property_name"`
		PropertyPhone string  `json:"property_phone"`
		Address       string  `json:"address"`
		Lat           float64 `json:"lat"`
		Lng           float64 `json:"lng"`
		CheckIn       string  `json:"check_in"`
		CheckOut      string  `json:"check_out"`
		Nights        int     `json:"nights"`
		Guests        int     `json:"guests"`
		Rooms         int     `json:"rooms"`
		GuestName     string  `json:"guest_name"`
		BalanceDue    int64   `json:"balance_due_santim"`
		Currency      string  `json:"currency"`
	}
	lat, lng := s.latLng("p.location")
	var status string
	var pm *string
	var in, o time.Time
	var total int64
	err := s.pool.QueryRow(ctx, `SELECT b.id, b.code, b.status, b.property_id, p.name, p.phone, p.address, `+lat+`, `+lng+`,
			b.check_in, b.check_out, b.guests, b.rooms, b.guest_name, b.total_santim, b.currency, b.payment_method
		FROM bookings b JOIN properties p ON p.id = b.property_id WHERE b.id = $1 AND b.user_id = $2`, id, uid).
		Scan(&out.BookingID, &out.Reference, &status, &out.PropertyID, &out.PropertyName, &out.PropertyPhone, &out.Address, &out.Lat, &out.Lng,
			&in, &o, &out.Guests, &out.Rooms, &out.GuestName, &total, &out.Currency, &pm)
	if err == pgx.ErrNoRows || isBadUUID(err) {
		return errNotFound
	}
	if err != nil {
		return err
	}
	if status != "CONFIRMED" && status != "CHECKED_IN" {
		return fail(http.StatusConflict, codeVoucherNotYet, "The voucher is ready once the hotel confirms")
	}
	out.CheckIn, out.CheckOut = pgDate(in), pgDate(o)
	out.Nights = nightsBetween(out.CheckIn, out.CheckOut)
	// Valid until the end of the check-out day (Addis Ababa) plus a day of grace.
	exp := o.Add(24*time.Hour - time.Second - 3*time.Hour).Add(24 * time.Hour)
	out.PIN = s.keys.VoucherPIN(out.BookingID)
	out.Token = s.keys.SignVoucher(out.PropertyID, out.Reference, out.CheckIn, out.CheckOut, exp)
	out.ExpiresAt = exp.UTC().Format(time.RFC3339)
	if pm != nil && *pm == "pay_at_hotel" {
		out.BalanceDue = total
	}
	if _, err := s.pool.Exec(ctx, `INSERT INTO vouchers (booking_id, kid, pin_hash) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
		out.BookingID, s.keys.VoucherKID, s.keys.PINHash(out.BookingID, out.PIN)); err != nil {
		return err
	}
	out.Meta = httpx.NowMeta()
	respond(w, http.StatusOK, out)
	return nil
}
