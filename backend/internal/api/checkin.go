package api

import (
	"context"
	"crypto/hmac"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/bookdan/backend/internal/httpx"
)

// Check-in. A guest arrives and shows the voucher QR code (or says the booking reference and the 4-digit PIN from the same
// screen). The hotel app looks the booking up, shows who it is for, and the receptionist confirms. Nothing changes until they do.
//
// A booking can be checked in from the check-in date until 06:00 on the check-out date (Addis Ababa time), so a guest who arrives
// after midnight on the last night is still welcome. Checking in twice is harmless: the second time just says it already happened.

const (
	codeInvalidVoucher = "INVALID_VOUCHER"
	codeVoucherExpired = "VOUCHER_EXPIRED"
	codeWrongHotel     = "WRONG_HOTEL"
	codeInvalidPIN     = "INVALID_PIN"
	codePINLocked      = "PIN_LOCKED"

	pinTries = 5
	pinLock  = 30 * time.Minute
)

var pinRE = regexp.MustCompile(`^[0-9]{4}$`)

type querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// checkinState says whether a booking can be checked in now: ready, checked_in (already done), too_early, ended, cancelled,
// waiting (the hotel has not answered yet) or not_ready.
func (s *Server) checkinState(ctx context.Context, q querier, id string) (string, error) {
	var status string
	var early, late bool
	if err := q.QueryRow(ctx, `SELECT status, (now() AT TIME ZONE 'Africa/Addis_Ababa') < check_in::timestamp,
			(now() AT TIME ZONE 'Africa/Addis_Ababa') >= check_out::timestamp + interval '6 hours' FROM bookings WHERE id = $1`, id).Scan(&status, &early, &late); err != nil {
		return "", err
	}
	switch status {
	case "CONFIRMED":
		switch {
		case early:
			return "too_early", nil
		case late:
			return "ended", nil
		}
		return "ready", nil
	case "CHECKED_IN", "COMPLETED":
		return "checked_in", nil
	case "CANCELLED_REFUNDED", "DECLINED_REFUNDED", "EXPIRED":
		return "cancelled", nil
	case "PAID_AWAITING_HOTEL":
		return "waiting", nil
	}
	return "not_ready", nil
}

var checkinRefusals = map[string]string{
	"too_early": "This booking starts later: it cannot be checked in before the check-in date.",
	"ended":     "This stay is over: it can no longer be checked in.",
	"cancelled": "This booking was cancelled or declined.",
	"waiting":   "The hotel has not accepted this booking yet. Accept it first.",
	"not_ready": "This booking cannot be checked in.",
}

// checkPIN compares the PIN a guest says with the one on their voucher. After five wrong tries the booking is locked for 30
// minutes (the QR code still works): four digits are few, and the guard must live here, not in the app.
func (s *Server) checkPIN(ctx context.Context, id, pin string) error {
	var result error
	err := s.tx(ctx, func(tx pgx.Tx) error {
		result = nil
		if _, err := tx.Exec(ctx, `INSERT INTO vouchers (booking_id, kid, pin_hash) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
			id, s.keys.VoucherKID, s.keys.PINHash(id, s.keys.VoucherPIN(id))); err != nil {
			return err
		}
		var attempts int
		var locked *time.Time
		var hash []byte
		if err := tx.QueryRow(ctx, `SELECT pin_attempts, locked_until, pin_hash FROM vouchers WHERE booking_id = $1 FOR UPDATE`, id).Scan(&attempts, &locked, &hash); err != nil {
			return err
		}
		if locked != nil && locked.After(s.now()) {
			result = fail(http.StatusTooManyRequests, codePINLocked, "Too many wrong PINs. Scan the guest's QR code instead, or try again in a while.")
			return nil
		}
		if hmac.Equal(hash, s.keys.PINHash(id, pin)) {
			_, err := tx.Exec(ctx, `UPDATE vouchers SET pin_attempts = 0, locked_until = NULL WHERE booking_id = $1`, id)
			return err
		}
		attempts++
		if attempts >= pinTries {
			result = fail(http.StatusTooManyRequests, codePINLocked, "Too many wrong PINs. Scan the guest's QR code instead, or try again in 30 minutes.")
			_, err := tx.Exec(ctx, `UPDATE vouchers SET pin_attempts = 0, locked_until = now() + $2::interval WHERE booking_id = $1`, id, fmt.Sprintf("%d seconds", int(pinLock.Seconds())))
			return err
		}
		result = fail(http.StatusBadRequest, codeInvalidPIN, fmt.Sprintf("Wrong PIN. %d tries left.", pinTries-attempts))
		_, err := tx.Exec(ctx, `UPDATE vouchers SET pin_attempts = $2 WHERE booking_id = $1`, id, attempts)
		return err
	})
	if err != nil {
		return err
	}
	return result // a failed try is saved even though the answer is an error
}

type checkinResult struct {
	httpx.Meta
	// State is ready, checked_in, too_early, ended, cancelled, waiting or not_ready.
	State   string         `json:"state"`
	Booking partnerBooking `json:"booking"`
}

func (s *Server) partnerCheckinLookup(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	var in struct {
		Token     string `json:"token"`
		Reference string `json:"reference"`
		PIN       string `json:"pin"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	propIDs, _, err := s.staffHotels(ctx, claimsFrom(r).Sub)
	if err != nil {
		return err
	}
	var id string
	switch {
	case strings.TrimSpace(in.Token) != "":
		p, err := s.keys.VerifyVoucher(strings.TrimSpace(in.Token), s.now())
		if errors.Is(err, errVoucherExpired) {
			return fail(http.StatusConflict, codeVoucherExpired, "This voucher has expired.")
		}
		if err != nil {
			return fail(http.StatusBadRequest, codeInvalidVoucher, "This is not a BookDan voucher, or it was changed.")
		}
		mine := false
		for _, pid := range propIDs {
			mine = mine || pid == p.PID
		}
		if !mine {
			return fail(http.StatusConflict, codeWrongHotel, "This voucher is for a different hotel.")
		}
		if err := s.pool.QueryRow(ctx, `SELECT id FROM bookings WHERE code = $1 AND property_id = $2`, p.Code, p.PID).Scan(&id); err != nil {
			if err == pgx.ErrNoRows {
				return fail(http.StatusNotFound, httpx.CodeNotFound, "No booking was found for this voucher.")
			}
			return err
		}
	case in.Reference != "" && in.PIN != "":
		ref, pin := strings.ToUpper(strings.TrimSpace(in.Reference)), strings.TrimSpace(in.PIN)
		if !pinRE.MatchString(pin) {
			return fail(http.StatusBadRequest, codeInvalidPIN, "The PIN is 4 digits.")
		}
		// The same answer for an unknown reference and a wrong PIN, so the form cannot be used to find out which references exist.
		if err := s.pool.QueryRow(ctx, `SELECT id FROM bookings WHERE code = $1 AND property_id = ANY($2::uuid[]) AND status <> 'HELD'`, ref, propIDs).Scan(&id); err != nil {
			if err == pgx.ErrNoRows {
				return fail(http.StatusBadRequest, codeInvalidPIN, "Check the reference and the PIN.")
			}
			return err
		}
		if err := s.checkPIN(ctx, id, pin); err != nil {
			return err
		}
	default:
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "Send the voucher token, or the booking reference and PIN")
	}
	state, err := s.checkinState(ctx, s.pool, id)
	if err != nil {
		return err
	}
	b, err := scanPartner(s.pool.QueryRow(ctx, partnerSelect+` WHERE b.id = $1`, id), len(propIDs) > 1)
	if err != nil {
		return err
	}
	respond(w, http.StatusOK, checkinResult{httpx.NowMeta(), state, b})
	return nil
}

func (s *Server) partnerCheckin(w http.ResponseWriter, r *http.Request) error {
	ctx, id, uid := r.Context(), chi.URLParam(r, "id"), claimsFrom(r).Sub
	propIDs, _, err := s.staffHotels(ctx, uid)
	if err != nil {
		return err
	}
	err = s.tx(ctx, func(tx pgx.Tx) error {
		var locked string
		err := tx.QueryRow(ctx, `SELECT id FROM bookings WHERE id = $1 AND property_id = ANY($2::uuid[]) AND status <> 'HELD' FOR UPDATE`, id, propIDs).Scan(&locked)
		if err == pgx.ErrNoRows || isBadUUID(err) {
			return errNotFound
		}
		if err != nil {
			return err
		}
		state, err := s.checkinState(ctx, tx, id)
		if err != nil {
			return err
		}
		switch state {
		case "checked_in":
			return nil // already done: the same answer again
		case "ready":
		default:
			return fail(http.StatusConflict, codeInvalidState, checkinRefusals[state])
		}
		if err := actor(ctx, tx, "staff:"+uid, "guest checked in"); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE bookings SET status = 'CHECKED_IN', checked_in_at = now(), checked_in_by = $2 WHERE id = $1`, id, uid); err != nil {
			return err
		}
		return s.audit(r, tx, "guest_checked_in", map[string]any{"booking_id": id})
	})
	if err != nil {
		return err
	}
	b, err := scanPartner(s.pool.QueryRow(ctx, partnerSelect+` WHERE b.id = $1`, id), len(propIDs) > 1)
	if err != nil {
		return err
	}
	respond(w, http.StatusOK, b)
	return nil
}
