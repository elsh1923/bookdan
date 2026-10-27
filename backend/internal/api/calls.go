package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/bookdan/backend/internal/httpx"
)

// Keeping guests and hotels on the platform until the stay is booked. The hotel's phone number is not on its page: a guest gets it once a booking is
// confirmed (the booking, the voucher, the confirmation email). When the guest taps "Call the hotel" the app tells the server (a call log, best effort),
// so the admin page can show how often guests and hotels talk outside the app. A tap is not a proof of a call, and a number once shown cannot be unseen:
// this is visibility, not a lock.

const maxCallLogsPerBooking = 50

// phoneVisible says whether the guest may see the hotel's number on a booking in this state.
func phoneVisible(status string) bool { return status == "CONFIRMED" || status == "CHECKED_IN" }

// logCall records that the guest tapped "Call the hotel" on one of their bookings.
func (s *Server) logCall(w http.ResponseWriter, r *http.Request) error {
	id, uid, ctx := chi.URLParam(r, "id"), claimsFrom(r).Sub, r.Context()
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var status, property string
		err := tx.QueryRow(ctx, `SELECT status, property_id::text FROM bookings WHERE id = $1 AND user_id = $2 FOR UPDATE`, id, uid).Scan(&status, &property)
		if err == pgx.ErrNoRows || isBadUUID(err) {
			return errNotFound
		}
		if err != nil {
			return err
		}
		if !phoneVisible(status) {
			return fail(http.StatusConflict, codeInvalidState, "The hotel's number is shown once the booking is confirmed")
		}
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM hotel_calls WHERE booking_id = $1`, id).Scan(&n); err != nil {
			return err
		}
		if n >= maxCallLogsPerBooking {
			return nil // enough is recorded; nothing is refused
		}
		_, err = tx.Exec(ctx, `INSERT INTO hotel_calls (booking_id, property_id, user_id) VALUES ($1, $2, $3)`, id, property, uid)
		return err
	})
	if err != nil {
		return err
	}
	respond(w, http.StatusOK, httpx.NowMeta())
	return nil
}

type callHotel struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Calls    int     `json:"calls"`
	Bookings int     `json:"bookings_with_calls"`
	Total    int     `json:"confirmed_bookings"`
	LastCall *string `json:"last_call_at"`
}

type callRow struct {
	At      string `json:"at"`
	Hotel   string `json:"hotel"`
	Booking string `json:"booking_code"`
	Guest   string `json:"guest_name"`
	Status  string `json:"status"`
}

// adminCalls shows, for the last N days (default 30, at most 365), how many calls were logged per hotel and the latest ones.
func (s *Server) adminCalls(w http.ResponseWriter, r *http.Request) error {
	if err := requireAdmin(r); err != nil {
		return err
	}
	days := atoiDefault(r.URL.Query().Get("days"), 30)
	if days < 1 || days > 365 {
		days = 30
	}
	ctx := r.Context()
	rows, err := s.pool.Query(ctx, `SELECT p.id, p.name, count(c.id)::int, count(DISTINCT c.booking_id)::int, max(c.created_at),
			(SELECT count(*)::int FROM bookings b WHERE b.property_id = p.id AND b.status IN ('CONFIRMED', 'CHECKED_IN', 'COMPLETED') AND b.paid_at > now() - make_interval(days => $1))
		FROM properties p JOIN hotel_calls c ON c.property_id = p.id AND c.created_at > now() - make_interval(days => $1)
		GROUP BY p.id ORDER BY count(c.id) DESC, p.name`, days)
	if err != nil {
		return err
	}
	defer rows.Close()
	hotels := []callHotel{}
	for rows.Next() {
		var h callHotel
		var last *time.Time
		if err := rows.Scan(&h.ID, &h.Name, &h.Calls, &h.Bookings, &last, &h.Total); err != nil {
			return err
		}
		if last != nil {
			t := last.UTC().Format(time.RFC3339)
			h.LastCall = &t
		}
		hotels = append(hotels, h)
	}
	rows.Close()
	rr, err := s.pool.Query(ctx, `SELECT c.created_at, p.name, b.code, b.guest_name, b.status FROM hotel_calls c JOIN bookings b ON b.id = c.booking_id JOIN properties p ON p.id = c.property_id
		WHERE c.created_at > now() - make_interval(days => $1) ORDER BY c.created_at DESC LIMIT 100`, days)
	if err != nil {
		return err
	}
	defer rr.Close()
	recent := []callRow{}
	for rr.Next() {
		var c callRow
		var at time.Time
		if err := rr.Scan(&at, &c.Hotel, &c.Booking, &c.Guest, &c.Status); err != nil {
			return err
		}
		c.At = at.UTC().Format(time.RFC3339)
		recent = append(recent, c)
	}
	respond(w, http.StatusOK, struct {
		httpx.Meta
		Days   int         `json:"days"`
		Hotels []callHotel `json:"hotels"`
		Recent []callRow   `json:"recent"`
	}{httpx.NowMeta(), days, hotels, recent})
	return rr.Err()
}
