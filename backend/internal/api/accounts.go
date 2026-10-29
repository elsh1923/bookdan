package api

import (
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/bookdan/backend/internal/httpx"
)

// A guest can take their data (export) and delete their account.
//
// Deleting removes what identifies the guest and is theirs to remove: the saved profile and full ID number, the phone numbers and notes on
// their bookings, the text of their reviews, their sign-in sessions and phones (revoked), and the sign-in codes. The account itself is closed and
// its email address is freed (a later sign-in with the same address starts a new, empty account).
//
// What stays, because records of money and stays must be kept: the bookings and the ledger, and the hotel's own copy of the guest's name
// (booking_guests: the database lets only the ID number be erased). How long a hotel keeps that copy is the retention decision.
// An account with a stay still to come, or money in play, cannot be deleted: the guest cancels first.

// activeStatuses are the bookings that still involve a stay or money.
const activeBookingStatuses = `'HELD', 'PAID_AWAITING_HOTEL', 'CONFIRMED', 'CHECKED_IN', 'NEEDS_RELOCATION', 'DISPUTED'`

func (s *Server) deleteAccount(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Confirm string `json:"confirm"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if in.Confirm != "DELETE" {
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, `Send {"confirm": "DELETE"} to delete the account`)
	}
	ctx, uid := r.Context(), claimsFrom(r).Sub
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var active int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM bookings WHERE user_id = $1 AND status IN (`+activeBookingStatuses+`)`, uid).Scan(&active); err != nil {
			return err
		}
		if active > 0 {
			return fail(http.StatusConflict, "ACTIVE_BOOKINGS", "You have a booking that is not finished. Cancel it, or wait until the stay is over, then delete your account.")
		}
		var email, phone *string
		if err := tx.QueryRow(ctx, `SELECT email, phone FROM users WHERE id = $1 FOR UPDATE`, uid).Scan(&email, &phone); err != nil {
			return err
		}
		steps := []struct {
			sql  string
			args []any
		}{
			{`DELETE FROM guest_profiles WHERE user_id = $1`, []any{uid}},
			{`DELETE FROM saved_properties WHERE user_id = $1`, []any{uid}},
			{`UPDATE booking_guests g SET id_number_enc = NULL, purged_at = now() FROM bookings b WHERE b.id = g.booking_id AND b.user_id = $1 AND g.purged_at IS NULL`, []any{uid}},
			{`UPDATE bookings SET guest_phone = NULL, special_requests = NULL WHERE user_id = $1`, []any{uid}},
			{`UPDATE reviews SET text = '' WHERE user_id = $1`, []any{uid}},
			// Devices are revoked, not deleted: the append-only audit log refers to them. No phone can be alerted and no session continues.
			{`UPDATE devices SET revoked_at = coalesce(revoked_at, now()), push_token = NULL WHERE user_id = $1`, []any{uid}},
			{`UPDATE refresh_tokens SET revoked_at = coalesce(revoked_at, now()) WHERE user_id = $1`, []any{uid}},
			// Support conversations: the words are the guest's own, so they go; the emptied tickets stay, closed.
			{`UPDATE support_messages SET body = '' WHERE ticket_id IN (SELECT id FROM support_tickets WHERE user_id = $1) AND NOT from_staff`, []any{uid}},
			{`UPDATE support_tickets SET subject = '[deleted]', status = 'closed', closed_at = coalesce(closed_at, now()), closed_by = coalesce(closed_by, 'account deleted') WHERE user_id = $1`, []any{uid}},
			{`DELETE FROM otp_challenges WHERE identifier IN (coalesce($1, ''), coalesce($2, ''))`, []any{email, phone}},
			{`DELETE FROM idempotency_keys WHERE user_id = $1`, []any{uid}},
			// Closed, and the address freed. The placeholder keeps the row valid (an account needs an email or a phone).
			{`UPDATE users SET email = 'deleted-' || id::text || '@deleted.invalid', phone = NULL, display_name = NULL, status = 'suspended' WHERE id = $1`, []any{uid}},
		}
		for _, st := range steps {
			if _, err := tx.Exec(ctx, st.sql, st.args...); err != nil {
				return err
			}
		}
		return s.audit(r, tx, "account_deleted", nil)
	})
	if err != nil {
		return err
	}
	respond(w, http.StatusOK, httpx.NowMeta())
	return nil
}

type exportBooking struct {
	Code            string  `json:"code"`
	Status          string  `json:"status"`
	Hotel           string  `json:"hotel"`
	CheckIn         string  `json:"check_in"`
	CheckOut        string  `json:"check_out"`
	Guests          int     `json:"guests"`
	Rooms           int     `json:"rooms"`
	GuestName       string  `json:"guest_name"`
	TotalSantim     int64   `json:"total_santim"`
	Currency        string  `json:"currency"`
	PaymentMethod   *string `json:"payment_method"`
	SpecialRequests *string `json:"special_requests"`
	CreatedAt       string  `json:"created_at"`
}

type exportReview struct {
	BookingCode string `json:"booking_code"`
	Rating      int    `json:"rating"`
	Text        string `json:"text"`
	CreatedAt   string `json:"created_at"`
}

// exportAccount returns everything the app holds about the guest, as one JSON document. The full ID number is never included (it is stored
// encrypted and the app itself only ever shows its last four characters).
func (s *Server) exportAccount(w http.ResponseWriter, r *http.Request) error {
	ctx, uid := r.Context(), claimsFrom(r).Sub
	var email, phone *string
	if err := s.pool.QueryRow(ctx, `SELECT email, phone FROM users WHERE id = $1`, uid).Scan(&email, &phone); err != nil {
		return err
	}
	profile, err := s.readProfile(r, uid)
	if err != nil {
		return err
	}

	bookings := []exportBooking{}
	rows, err := s.pool.Query(ctx, `SELECT b.code, b.status, p.name, b.check_in, b.check_out, b.guests, b.rooms, b.guest_name, b.total_santim, b.currency, b.payment_method, b.special_requests, b.created_at
		FROM bookings b JOIN properties p ON p.id = b.property_id WHERE b.user_id = $1 ORDER BY b.created_at`, uid)
	if err != nil {
		return err
	}
	for rows.Next() {
		var b exportBooking
		var in, out, created time.Time
		if err := rows.Scan(&b.Code, &b.Status, &b.Hotel, &in, &out, &b.Guests, &b.Rooms, &b.GuestName, &b.TotalSantim, &b.Currency, &b.PaymentMethod, &b.SpecialRequests, &created); err != nil {
			rows.Close()
			return err
		}
		b.CheckIn, b.CheckOut, b.CreatedAt = pgDate(in), pgDate(out), created.UTC().Format(time.RFC3339)
		bookings = append(bookings, b)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	reviews := []exportReview{}
	rr, err := s.pool.Query(ctx, `SELECT b.code, v.rating, v.text, v.created_at FROM reviews v JOIN bookings b ON b.id = v.booking_id WHERE v.user_id = $1 ORDER BY v.created_at`, uid)
	if err != nil {
		return err
	}
	defer rr.Close()
	for rr.Next() {
		var v exportReview
		var created time.Time
		if err := rr.Scan(&v.BookingCode, &v.Rating, &v.Text, &created); err != nil {
			return err
		}
		v.CreatedAt = created.UTC().Format(time.RFC3339)
		reviews = append(reviews, v)
	}
	saved, err := s.savedList(r, uid, nil)
	if err != nil {
		return err
	}
	respond(w, http.StatusOK, struct {
		httpx.Meta
		ExportedAt string          `json:"exported_at"`
		Email      *string         `json:"email"`
		Phone      *string         `json:"phone"`
		Profile    *profileOut     `json:"profile"`
		Bookings   []exportBooking `json:"bookings"`
		Reviews    []exportReview  `json:"reviews"`
		Saved      []savedOut      `json:"saved_hotels"`
	}{httpx.NowMeta(), time.Now().UTC().Format(time.RFC3339), email, phone, profile, bookings, reviews, saved})
	return rr.Err()
}
