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
			if _, err := tx.Exec(ctx, `UPDATE payments SET status = 'failed' WHERE booking_id = $1 AND status = 'pending'`, id); err != nil {
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
		err = tx.QueryRow(ctx, `SELECT id, coalesce(payment_url, '') FROM payments WHERE booking_id = $1 AND status = 'pending'`, id).Scan(&pid, &url)
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
	var notifyID string
	err := s.tx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `INSERT INTO payment_events (provider, event_id, payload) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, provider, eventID, json.RawMessage(raw))
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return nil // a repeat of an event we already have
		}
		var payID, bookingID, payStatus, bookingStatus, cur string
		var amount int64
		err = tx.QueryRow(ctx, `SELECT p.id, p.booking_id, p.status, b.status, p.amount_santim, p.currency
			FROM payments p JOIN bookings b ON b.id = p.booking_id WHERE p.provider = $1 AND p.provider_ref = $2 FOR UPDATE OF p, b`, provider, providerRef).
			Scan(&payID, &bookingID, &payStatus, &bookingStatus, &amount, &cur)
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
				if bookingStatus == "HELD" {
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

// runRefunds pays back every deposit that is owed: online-paid bookings the hotel declined, that timed out, or whose hold
// had ended when the payment arrived. A failed refund stays owed and is tried again by the sweeper.
func (s *Server) runRefunds(ctx context.Context) {
	rows, err := s.pool.Query(ctx, `SELECT p.id FROM payments p JOIN bookings b ON b.id = p.booking_id
		WHERE p.status = 'succeeded' AND (b.status IN ('DECLINED_REFUNDED', 'EXPIRED') OR (b.status = 'CANCELLED_REFUNDED' AND coalesce(b.refund_santim, 0) > 0)) LIMIT 50`)
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
		err := s.tx(ctx, func(tx pgx.Tx) error {
			var ref, cur, bookingID string
			var amount int64
			err := tx.QueryRow(ctx, `SELECT provider_ref, amount_santim, currency, booking_id FROM payments WHERE id = $1 AND status = 'succeeded' FOR UPDATE SKIP LOCKED`, id).
				Scan(&ref, &amount, &cur, &bookingID)
			if err == pgx.ErrNoRows {
				return nil // someone else is refunding it, or it is done
			}
			if err != nil {
				return err
			}
			if err := s.p.Gateway.Refund(ctx, ref, amount, cur); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE payments SET status = 'refunded' WHERE id = $1`, id); err != nil {
				return err
			}
			return ledger(ctx, tx, bookingID, id, "refund", "refund paid to the guest", cur,
				entry{"refunds_payable", amount}, entry{"gateway_clearing", -amount})
		})
		if err != nil {
			slog.Error("refund failed, will retry", "payment", id, "err", err)
		}
	}
}

// ---- alert to the hotel

// notifyHotel, escalate and remind live in alerts.go.

// push sends an alert and forgets any phone the push service says is gone. Failures are logged, never returned: an alert
// that cannot be sent must not undo the booking that caused it.
func (s *Server) push(ctx context.Context, tokens []string, title, body string, data map[string]string) {
	invalid, err := s.p.Push.Notify(ctx, tokens, title, body, data)
	if err != nil {
		slog.Error("push failed", "err", err)
	}
	if len(invalid) > 0 {
		if _, err := s.pool.Exec(ctx, `UPDATE devices SET push_token = NULL WHERE push_token = ANY($1::text[])`, invalid); err != nil {
			slog.Error("forget push tokens", "err", err)
		}
	}
}

// Guest messages. The text is English because the server does not know the guest's language yet; the data carries the
// booking and its new status so the app can open the right screen.
var guestMessages = map[string][2]string{
	"confirmed":   {"Booking confirmed", "%s has confirmed your room. Your voucher is ready."},
	"declined":    {"Booking not available", "%s could not accept your request. You will get a full refund."},
	"no_response": {"The hotel did not answer in time", "%s did not answer in time. You will get a full refund."},
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
