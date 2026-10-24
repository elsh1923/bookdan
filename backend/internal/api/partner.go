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

var declineReasons = map[string]bool{"fully_booked": true, "room_unavailable": true, "maintenance": true, "other": true}

// staffHotels returns every hotel this staff member works for (an owner can have several), oldest assignment first.
func (s *Server) staffHotels(ctx context.Context, userID string) (ids, names []string, err error) {
	rows, err := s.pool.Query(ctx, `SELECT p.id, p.name FROM property_staff ps JOIN properties p ON p.id = ps.property_id
		WHERE ps.user_id = $1 ORDER BY ps.created_at, p.name`, userID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, nil, err
		}
		ids, names = append(ids, id), append(names, name)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	if len(ids) == 0 {
		return nil, nil, fail(http.StatusForbidden, codeNotStaff, "This account is not registered with a hotel")
	}
	return ids, names, nil
}

type partnerGuest struct {
	FullName    string `json:"full_name"`
	Contact     string `json:"contact"`
	Nationality string `json:"nationality"`
	IDType      string `json:"id_type"`
	IDLast4     string `json:"id_last4"`
	IDPurged    bool   `json:"id_purged"`
}

type partnerBooking struct {
	httpx.Meta
	ID        string       `json:"id"`
	Reference string       `json:"reference"`
	Status    string       `json:"status"`
	GuestName string       `json:"guest_name"`
	Guest     partnerGuest `json:"guest"`
	RoomName  string       `json:"room_name"`
	// PropertyName is set only when the account works for more than one hotel, so each booking says which one it is for.
	PropertyName  string  `json:"property_name,omitempty"`
	CheckIn       string  `json:"check_in"`
	CheckOut      string  `json:"check_out"`
	Nights        int     `json:"nights"`
	Guests        int     `json:"guests"`
	Rooms         int     `json:"rooms"`
	Total         int64   `json:"total_santim"`
	Currency      string  `json:"currency"`
	PaidAt        *string `json:"paid_at"`
	HotelDeadline *string `json:"hotel_deadline_at"`
	DeclineReason *string `json:"decline_reason"`
	// CheckedInAt is when the guest arrived; BalanceDue is what the guest still pays at the hotel (a pay-at-hotel booking: the whole total).
	CheckedInAt *string `json:"checked_in_at"`
	BalanceDue  int64   `json:"balance_due_santim"`
	// The room option the guest booked: the hotel serves breakfast when meal is breakfast; a non-refundable booking is not given back on cancelling.
	Meal       string `json:"meal"`
	Refundable bool   `json:"refundable"`
}

// A hotel never sees a room that is only held: the guest has not paid or asked yet.
const partnerSelect = `
	SELECT b.id, b.code, b.status, b.guest_name, g.full_name, g.contact, g.nationality, g.id_type, g.id_last4, g.purged_at IS NOT NULL,
	       rt.name, b.check_in, b.check_out, b.guests, b.rooms, b.total_santim, b.currency, b.paid_at, b.hotel_deadline_at, b.decline_reason,
	       p.name, b.checked_in_at, CASE WHEN b.payment_method = 'pay_at_hotel' THEN b.total_santim ELSE 0 END, b.meal, b.refundable
	FROM bookings b JOIN booking_guests g ON g.booking_id = b.id JOIN room_types rt ON rt.id = b.room_type_id
	JOIN properties p ON p.id = b.property_id`

func scanPartner(row pgx.Row, multiHotel bool) (partnerBooking, error) {
	var b partnerBooking
	var in, out time.Time
	var paid, deadline, checkedIn *time.Time
	var hotel string
	if err := row.Scan(&b.ID, &b.Reference, &b.Status, &b.GuestName, &b.Guest.FullName, &b.Guest.Contact, &b.Guest.Nationality, &b.Guest.IDType,
		&b.Guest.IDLast4, &b.Guest.IDPurged, &b.RoomName, &in, &out, &b.Guests, &b.Rooms, &b.Total, &b.Currency, &paid, &deadline, &b.DeclineReason, &hotel, &checkedIn, &b.BalanceDue, &b.Meal, &b.Refundable); err != nil {
		return b, err
	}
	if multiHotel {
		b.PropertyName = hotel
	}
	b.CheckIn, b.CheckOut, b.Meta = pgDate(in), pgDate(out), httpx.NowMeta()
	b.Nights = nightsBetween(b.CheckIn, b.CheckOut)
	if paid != nil {
		t := paid.UTC().Format(time.RFC3339)
		b.PaidAt = &t
	}
	if deadline != nil {
		t := deadline.UTC().Format(time.RFC3339)
		b.HotelDeadline = &t
	}
	if checkedIn != nil {
		t := checkedIn.UTC().Format(time.RFC3339)
		b.CheckedInAt = &t
	}
	return b, nil
}

func (s *Server) partnerBookings(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	propIDs, propNames, err := s.staffHotels(ctx, claimsFrom(r).Sub)
	if err != nil {
		return err
	}
	multi := len(propIDs) > 1
	propID, propName := propIDs[0], propNames[0]
	if multi {
		propName = fmt.Sprintf("%d hotels", len(propIDs))
	}
	s.sweep(ctx) // so a missed deadline is already closed when the hotel looks
	rows, err := s.pool.Query(ctx, partnerSelect+` WHERE b.property_id = ANY($1::uuid[]) AND b.status <> 'HELD'
		AND (b.check_out >= (now() AT TIME ZONE 'Africa/Addis_Ababa')::date - 14 OR b.status = 'PAID_AWAITING_HOTEL')
		ORDER BY b.created_at DESC LIMIT 200`, propIDs)
	if err != nil {
		return err
	}
	defer rows.Close()
	items := []partnerBooking{}
	for rows.Next() {
		b, err := scanPartner(rows, multi)
		if err != nil {
			return err
		}
		items = append(items, b)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	respond(w, http.StatusOK, struct {
		httpx.Meta
		Property struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"property"`
		Items []partnerBooking `json:"items"`
	}{httpx.NowMeta(), struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}{propID, propName}, items})
	return nil
}

// answer is the shared body of accept and decline: lock the booking, check it is this hotel's and still waiting, change it.
func (s *Server) answer(w http.ResponseWriter, r *http.Request, accept bool, reason string) error {
	ctx, id, uid := r.Context(), chi.URLParam(r, "id"), claimsFrom(r).Sub
	propIDs, _, err := s.staffHotels(ctx, uid)
	if err != nil {
		return err
	}
	s.sweep(ctx)
	err = s.tx(ctx, func(tx pgx.Tx) error {
		var status, roomType, cur string
		var in, out time.Time
		var rooms int
		var total int64
		var deadline *time.Time
		err := tx.QueryRow(ctx, `SELECT status, room_type_id, check_in, check_out, rooms, total_santim, currency, hotel_deadline_at
			FROM bookings WHERE id = $1 AND property_id = ANY($2::uuid[]) FOR UPDATE`, id, propIDs).Scan(&status, &roomType, &in, &out, &rooms, &total, &cur, &deadline)
		if err == pgx.ErrNoRows || isBadUUID(err) {
			return errNotFound
		}
		if err != nil {
			return err
		}
		if status == "DECLINED_REFUNDED" && s.declinedForNoAnswer(ctx, tx, id) {
			return fail(http.StatusConflict, codeDeadlinePassed, "The time to answer has passed and the guest was refunded")
		}
		if status != "PAID_AWAITING_HOTEL" {
			return fail(http.StatusConflict, codeInvalidState, "This booking is not waiting for an answer")
		}
		if deadline != nil && !deadline.After(s.now()) {
			return fail(http.StatusConflict, codeDeadlinePassed, "The time to answer has passed and the guest was refunded")
		}
		who := "staff:" + uid
		if accept {
			return s.confirm(ctx, tx, id, roomType, pgDate(in), pgDate(out), rooms, total, cur, who)
		}
		return s.decline(ctx, tx, id, roomType, pgDate(in), pgDate(out), rooms, reason, who)
	})
	if err != nil {
		return err
	}
	b, err := scanPartner(s.pool.QueryRow(ctx, partnerSelect+` WHERE b.id = $1`, id), len(propIDs) > 1)
	if err != nil {
		return err
	}
	s.runRefunds(ctx)
	if accept {
		s.notifyGuest(ctx, id, "confirmed")
	} else {
		s.notifyGuest(ctx, id, "declined")
	}
	respond(w, http.StatusOK, b)
	return nil
}

func (s *Server) declinedForNoAnswer(ctx context.Context, tx pgx.Tx, id string) bool {
	var reason *string
	_ = tx.QueryRow(ctx, `SELECT decline_reason FROM bookings WHERE id = $1`, id).Scan(&reason)
	return reason != nil && *reason == "no_response"
}

func (s *Server) partnerAccept(w http.ResponseWriter, r *http.Request) error {
	return s.answer(w, r, true, "")
}

func (s *Server) partnerDecline(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Reason string `json:"reason"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if !declineReasons[in.Reason] {
		return fail(http.StatusBadRequest, codeInvalidState, "A valid reason is required")
	}
	return s.answer(w, r, false, in.Reason)
}

// confirm: the rooms move from held to booked, and (for online payments) the hotel's share is recorded in the ledger.
func (s *Server) confirm(ctx context.Context, tx pgx.Tx, id, roomType, in, out string, rooms int, total int64, cur, who string) error {
	if err := actor(ctx, tx, who, "hotel accepted"); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE bookings SET status = 'CONFIRMED', hotel_deadline_at = NULL WHERE id = $1`, id); err != nil {
		return err
	}
	if err := confirmRooms(ctx, tx, roomType, in, out, rooms); err != nil {
		return err
	}
	var payID string
	var amount int64
	err := tx.QueryRow(ctx, `SELECT id, amount_santim FROM payments WHERE booking_id = $1 AND status = 'succeeded'`, id).Scan(&payID, &amount)
	if err == pgx.ErrNoRows {
		return nil // pay at hotel: no money has moved through us
	}
	if err != nil {
		return err
	}
	var commBP, commVatBP int64
	if err := tx.QueryRow(ctx, `SELECT commission_bps, commission_vat_bps FROM pricing_config WHERE id = 1`).Scan(&commBP, &commVatBP); err != nil {
		return err
	}
	// A coupon makes the guest pay less. The hotel still gets its share of the full price and BookDan pays for the discount, so the
	// share and the commission come from what the guest paid plus the discount, and the discount is booked as a coupon cost.
	var discount int64
	if err := tx.QueryRow(ctx, `SELECT discount_santim FROM bookings WHERE id = $1`, id).Scan(&discount); err != nil {
		return err
	}
	full := amount + discount
	commission := (full*commBP + 5000) / 10000
	commVAT := (commission*commVatBP + 5000) / 10000
	return ledger(ctx, tx, id, payID, "booking_confirmed", "hotel accepted: our commission and the hotel's share", cur,
		entry{"guest_deposits_held", amount}, entry{"coupon_expense", discount}, entry{"property_payable", -(full - commission - commVAT)},
		entry{"commission_revenue", -commission}, entry{"vat_on_commission_payable", -commVAT})
}

// decline releases the rooms and owes the guest their money back.
func (s *Server) decline(ctx context.Context, tx pgx.Tx, id, roomType, in, out string, rooms int, reason, who string) error {
	if err := actor(ctx, tx, who, "hotel declined: "+reason); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE bookings SET status = 'DECLINED_REFUNDED', decline_reason = $2, hotel_deadline_at = NULL WHERE id = $1`, id, reason); err != nil {
		return err
	}
	if err := releaseRooms(ctx, tx, roomType, in, out, rooms); err != nil {
		return err
	}
	var payID, cur string
	var amount int64
	err := tx.QueryRow(ctx, `SELECT id, currency, amount_santim FROM payments WHERE booking_id = $1 AND status = 'succeeded'`, id).Scan(&payID, &cur, &amount)
	if err == pgx.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	return accrueRefund(ctx, tx, id, payID, cur, amount)
}

// partnerReveal returns the full ID number of the guest on one booking. Every call is written to the audit log.
func (s *Server) partnerReveal(w http.ResponseWriter, r *http.Request) error {
	ctx, id := r.Context(), chi.URLParam(r, "id")
	propIDs, _, err := s.staffHotels(ctx, claimsFrom(r).Sub)
	if err != nil {
		return err
	}
	var enc []byte
	var purged bool
	err = s.pool.QueryRow(ctx, `SELECT g.id_number_enc, g.purged_at IS NOT NULL FROM booking_guests g
		JOIN bookings b ON b.id = g.booking_id WHERE g.booking_id = $1 AND b.property_id = ANY($2::uuid[]) AND b.status <> 'HELD'`, id, propIDs).Scan(&enc, &purged)
	if err == pgx.ErrNoRows || isBadUUID(err) {
		return errNotFound
	}
	if err != nil {
		return err
	}
	if purged || enc == nil {
		return fail(http.StatusGone, codeGone, "The ID number was erased after the retention period")
	}
	number, err := s.keys.Open(enc)
	if err != nil {
		return err
	}
	if err := s.tx(ctx, func(tx pgx.Tx) error {
		return s.audit(r, tx, "guest_id_revealed", map[string]any{"booking_id": id})
	}); err != nil {
		return err
	}
	respond(w, http.StatusOK, struct {
		httpx.Meta
		IDNumber string `json:"id_number"`
	}{httpx.NowMeta(), number})
	return nil
}

// registerPushToken saves where to send this phone's alerts (one token per install; a new one replaces the old). The same
// endpoint serves the hotel app (alerts for new requests) and the guest app (updates on their bookings).
func (s *Server) registerPushToken(app string) handler {
	return func(w http.ResponseWriter, r *http.Request) error { return s.savePushToken(app, w, r) }
}

func (s *Server) savePushToken(app string, w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Token      string `json:"token"`
		TokenType  string `json:"token_type"`
		DeviceID   string `json:"device_id"`
		Platform   string `json:"platform"`
		AppVersion string `json:"app_version"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if in.Token == "" || len(in.Token) > 4096 || in.TokenType != "fcm" || len(in.DeviceID) < 8 || (in.Platform != "android" && in.Platform != "ios") {
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "token, token_type fcm, device_id and platform are required")
	}
	c := claimsFrom(r)
	if _, err := s.pool.Exec(r.Context(), `INSERT INTO devices (user_id, device_id, app, platform, app_version, push_token)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (user_id, device_id, app) DO UPDATE SET push_token = EXCLUDED.push_token, platform = EXCLUDED.platform,
		  app_version = EXCLUDED.app_version, revoked_at = NULL, last_seen_at = now()`,
		c.Sub, in.DeviceID, app, in.Platform, in.AppVersion, in.Token); err != nil {
		return err
	}
	respond(w, http.StatusOK, httpx.NowMeta())
	return nil
}

// ---- background work

// sweep closes what time has overtaken: holds that ran out, and requests the hotel did not answer in time (the guest is
// refunded). It is cheap and idempotent, so it runs on a timer and also whenever a hotel looks at its bookings.
func (s *Server) sweep(ctx context.Context) {
	s.stepAlerts(ctx)
	var timedOut []string // hotels that did not answer: their guests are told after the commit
	if err := s.tx(ctx, func(tx pgx.Tx) error {
		timedOut = nil
		rows, err := tx.Query(ctx, `SELECT id, room_type_id, check_in, check_out, rooms, status FROM bookings
			WHERE (status = 'HELD' AND hold_expires_at <= now()) OR (status = 'PAID_AWAITING_HOTEL' AND hotel_deadline_at <= now())
			ORDER BY created_at LIMIT 100 FOR UPDATE SKIP LOCKED`)
		if err != nil {
			return err
		}
		type due struct {
			id, roomType, status string
			in, out              time.Time
			rooms                int
		}
		var list []due
		for rows.Next() {
			var d due
			if err := rows.Scan(&d.id, &d.roomType, &d.in, &d.out, &d.rooms, &d.status); err != nil {
				rows.Close()
				return err
			}
			list = append(list, d)
		}
		rows.Close()
		for _, d := range list {
			if d.status == "HELD" {
				if err := actor(ctx, tx, "system", "hold expired"); err != nil {
					return err
				}
				if _, err := tx.Exec(ctx, `UPDATE bookings SET status = 'EXPIRED' WHERE id = $1`, d.id); err != nil {
					return err
				}
				if err := releaseRooms(ctx, tx, d.roomType, pgDate(d.in), pgDate(d.out), d.rooms); err != nil {
					return err
				}
				continue
			}
			if err := s.decline(ctx, tx, d.id, d.roomType, pgDate(d.in), pgDate(d.out), d.rooms, "no_response", "system"); err != nil {
				return err
			}
			timedOut = append(timedOut, d.id)
		}
		return nil
	}); err != nil {
		slog.Error("sweep failed", "err", err)
		return
	}
	s.runRefunds(ctx)
	for _, id := range timedOut {
		s.notifyGuest(ctx, id, "no_response")
	}
}

// Sweep runs one pass (also used by tests).
func (s *Server) Sweep(ctx context.Context) { s.sweep(ctx) }

// RunWorkers sweeps every 20 seconds until ctx ends.
func (s *Server) RunWorkers(ctx context.Context) {
	t := time.NewTicker(20 * time.Second)
	defer t.Stop()
	for {
		s.sweep(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
