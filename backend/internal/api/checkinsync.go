package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/bookdan/backend/internal/httpx"
)

// Offline check-in. A hotel with no internet can still check guests in: the partner app checks the Ed25519 signature of the guest's voucher QR code on
// the phone itself (the public key is cached; GET /v1/keys/voucher), records the check-in and the moment it happened, and sends it here once it is online.
// The server does not trust the phone: it verifies the voucher again, finds the booking, and applies the check-in only if the booking is still valid and the
// moment falls inside the stay. Each item gets its own answer, a repeat is harmless, and what could not be applied is reported so the hotel can sort it out
// (for example a guest checked in offline whose booking had been cancelled meanwhile).

const (
	maxSyncItems     = 100
	syncMaxAge       = 30 * 24 * time.Hour
	syncClockSkew    = 5 * time.Minute
	checkoutGraceFor = 6 * time.Hour // as for an online check-in: a stay can be checked in until 6 hours after the check-out day starts
)

type syncItem struct {
	Token string `json:"token"`
	At    string `json:"at"` // when the guest was checked in (RFC 3339), by the phone's clock
}

type syncAnswer struct {
	Reference string `json:"reference"`
	// Result: checked_in, already, cancelled, wrong_hotel, invalid, not_found, too_early, ended, not_ready, bad_time.
	Result string `json:"result"`
}

func (s *Server) partnerCheckinSync(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Items []syncItem `json:"items"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if len(in.Items) == 0 || len(in.Items) > maxSyncItems {
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "Send 1 to 100 check-ins at a time")
	}
	ctx, uid := r.Context(), claimsFrom(r).Sub
	propIDs, _, err := s.staffHotels(ctx, uid)
	if err != nil {
		return err
	}
	out := make([]syncAnswer, 0, len(in.Items))
	for _, it := range in.Items {
		out = append(out, s.applyOfflineCheckin(r, uid, propIDs, it))
	}
	respond(w, http.StatusOK, struct {
		httpx.Meta
		Results []syncAnswer `json:"results"`
	}{httpx.NowMeta(), out})
	return nil
}

func (s *Server) applyOfflineCheckin(r *http.Request, uid string, propIDs []string, it syncItem) syncAnswer {
	ctx := r.Context()
	at, err := time.Parse(time.RFC3339, strings.TrimSpace(it.At))
	now := s.now()
	if err != nil || at.After(now.Add(syncClockSkew)) || at.Before(now.Add(-syncMaxAge)) {
		return syncAnswer{"", "bad_time"}
	}
	if at.After(now) {
		at = now
	}
	// The voucher is checked as of the moment of the check-in (a voucher that has expired since is fine).
	p, err := s.keys.VerifyVoucher(strings.TrimSpace(it.Token), at)
	if err != nil && !errors.Is(err, errVoucherExpired) {
		return syncAnswer{"", "invalid"}
	}
	if errors.Is(err, errVoucherExpired) {
		return syncAnswer{p.Code, "ended"}
	}
	mine := false
	for _, pid := range propIDs {
		mine = mine || pid == p.PID
	}
	if !mine {
		return syncAnswer{p.Code, "wrong_hotel"}
	}
	result := "not_found"
	err = s.tx(ctx, func(tx pgx.Tx) error {
		var id, status string
		var in, out time.Time
		err := tx.QueryRow(ctx, `SELECT id, status, check_in, check_out FROM bookings WHERE code = $1 AND property_id = $2 FOR UPDATE`, p.Code, p.PID).Scan(&id, &status, &in, &out)
		if err == pgx.ErrNoRows {
			return nil
		}
		if err != nil {
			return err
		}
		switch status {
		case "CHECKED_IN", "COMPLETED":
			result = "already"
			return nil
		case "CONFIRMED":
		case "CANCELLED_REFUNDED", "DECLINED_REFUNDED", "EXPIRED":
			result = "cancelled"
			return nil
		default:
			result = "not_ready"
			return nil
		}
		// The moment must fall in the stay: from the check-in day (Addis Ababa) to 6 hours after the check-out day begins.
		start := time.Date(in.Year(), in.Month(), in.Day(), 0, 0, 0, 0, time.UTC).Add(-3 * time.Hour)
		end := time.Date(out.Year(), out.Month(), out.Day(), 0, 0, 0, 0, time.UTC).Add(-3*time.Hour + checkoutGraceFor)
		switch {
		case at.Before(start):
			result = "too_early"
			return nil
		case !at.Before(end):
			result = "ended"
			return nil
		}
		if _, err := tx.Exec(ctx, `SELECT set_config('app.actor', $1, true), set_config('app.reason', $2, true)`, "staff:"+uid, "guest checked in (offline, sent later)"); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE bookings SET status = 'CHECKED_IN', checked_in_at = $3, checked_in_by = $2 WHERE id = $1`, id, uid, at); err != nil {
			return err
		}
		result = "checked_in"
		return s.audit(r, tx, "guest_checked_in_offline", map[string]any{"booking_id": id, "at": at.UTC().Format(time.RFC3339), "sent_at": now.UTC().Format(time.RFC3339)})
	})
	if err != nil {
		return syncAnswer{p.Code, "not_ready"}
	}
	return syncAnswer{p.Code, result}
}
