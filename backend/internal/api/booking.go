package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/bookdan/backend/internal/httpx"
)

// ---- pricing

type nightPrice struct {
	Night       string `json:"night"`
	RateSantim  int64  `json:"rate_santim"`
	BoostBP     int    `json:"boost_bp"`
	PriceSantim int64  `json:"price_santim"`
}

type priced struct {
	Currency                  string
	Nightly                   int64 // one room, first night
	Base, Service, VAT, Total int64
	Deposit, Balance          int64
	// Discount is how much cheaper Total is because of a coupon (VAT included): what the price would have been, minus Total.
	Discount int64
	Nights   []nightPrice
}

// priceStay prices every night (rate plus any boost, then the chosen room option), multiplies by the number of rooms, takes off a coupon
// if there is one (disc gets the room charges and returns the discount), then adds the service charge and VAT from pricing_config.
// Whole santim, rounded half up, never floats. The result is frozen into the quote.
func (s *Server) priceStay(ctx context.Context, q interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}, roomTypeID, checkIn, checkOut string, rooms int, opt rateOption, disc func(base int64) (int64, error)) (priced, error) {
	offer, err := s.roomOffer(ctx, q, roomTypeID)
	if err != nil {
		return priced{}, err
	}
	rows, err := q.Query(ctx, `
		SELECT n.night::date, rp.nightly_price_santim, rp.currency,
		  COALESCE((SELECT sum(b.percent_bp) FROM rate_boosts b WHERE b.property_id = rt.property_id AND n.night::date BETWEEN b.starts_on AND b.ends_on), 0)::int
		FROM room_types rt
		CROSS JOIN generate_series($2::date, $3::date - 1, '1 day') AS n(night)
		LEFT JOIN rate_plans rp ON rp.room_type_id = rt.id AND n.night::date BETWEEN rp.valid_from AND rp.valid_to
		WHERE rt.id = $1 ORDER BY n.night`, roomTypeID, checkIn, checkOut)
	if err != nil {
		return priced{}, err
	}
	defer rows.Close()
	var p priced
	var sum int64
	for rows.Next() {
		var night time.Time
		var rate *int64
		var cur *string
		var boost int
		if err := rows.Scan(&night, &rate, &cur, &boost); err != nil {
			return priced{}, err
		}
		if rate == nil {
			return priced{}, fail(http.StatusConflict, codeInvalidState, "This room has no price for those dates")
		}
		price, err := offer.night((*rate*int64(10000+boost)+5000)/10000, opt)
		if err != nil {
			return priced{}, err
		}
		if len(p.Nights) == 0 {
			p.Nightly, p.Currency = price, *cur
		}
		p.Nights = append(p.Nights, nightPrice{pgDate(night), *rate, boost, price})
		sum += price
	}
	if err := rows.Err(); err != nil {
		return priced{}, err
	}
	rows.Close()

	var serviceBP, vatBP, depositBP int64
	var confirmed bool
	if err := q.QueryRow(ctx, `SELECT service_bps, vat_bps, deposit_bps, confirmed FROM pricing_config WHERE id = 1`).Scan(&serviceBP, &vatBP, &depositBP, &confirmed); err != nil {
		return priced{}, err
	}
	if !confirmed && s.cfg.IsProduction() {
		// The rates in pricing_config are placeholders until the owner confirms them (design.md, open decisions).
		return priced{}, fail(http.StatusServiceUnavailable, codePricingPending, "Prices are not confirmed yet")
	}
	list := sum * int64(rooms)
	listService := (list*serviceBP + 5000) / 10000
	listTotal := list + listService + ((list+listService)*vatBP+5000)/10000
	p.Base = list
	if disc != nil {
		d, err := disc(list)
		if err != nil {
			return priced{}, err
		}
		p.Base = list - d
	}
	p.Service = (p.Base*serviceBP + 5000) / 10000
	p.VAT = ((p.Base+p.Service)*vatBP + 5000) / 10000
	p.Total = p.Base + p.Service + p.VAT
	p.Discount = listTotal - p.Total
	p.Deposit = (p.Total*depositBP + 5000) / 10000
	p.Balance = p.Total - p.Deposit
	return p, nil
}

// ---- quotes

type quoteOut struct {
	httpx.Meta
	Children       int    `json:"children"`
	ID             string `json:"id"`
	PropertyID     string `json:"property_id"`
	PropertyName   string `json:"property_name"`
	RoomTypeID     string `json:"room_type_id"`
	RoomName       string `json:"room_name"`
	CheckIn        string `json:"check_in"`
	CheckOut       string `json:"check_out"`
	Nights         int    `json:"nights"`
	Guests         int    `json:"guests"`
	Rooms          int    `json:"rooms"`
	RoomsAvailable int    `json:"rooms_available"`
	Nightly        int64  `json:"nightly_santim"`
	Subtotal       int64  `json:"subtotal_santim"`
	VAT            int64  `json:"vat_santim"`
	Total          int64  `json:"total_santim"`
	Currency       string `json:"currency"`
	PayAtHotel     bool   `json:"pay_at_hotel"`
	// The room option chosen, the coupon applied and how much it took off the total (VAT included); ListTotal is the price without the coupon.
	Meal       string  `json:"meal"`
	Refundable bool    `json:"refundable"`
	CouponCode *string `json:"coupon_code"`
	Discount   int64   `json:"discount_santim"`
	ListTotal  int64   `json:"list_total_santim"`
	// The hotel's first photo (a small copy, empty when it has none) and its check-in and check-out times, for the review screen.
	PhotoURL     *string `json:"photo_url"`
	CheckInTime  string  `json:"check_in_time"`
	CheckOutTime string  `json:"check_out_time"`
}

func (s *Server) createQuote(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		PropertyID string `json:"property_id"`
		RoomTypeID string `json:"room_type_id"`
		CheckIn    string `json:"check_in"`
		CheckOut   string `json:"check_out"`
		Guests     int    `json:"guests"`
		Rooms      int    `json:"rooms"`
		// The room option (meal room_only or breakfast; refundable defaults to true) and an optional coupon code.
		Meal       string `json:"meal"`
		Refundable *bool  `json:"refundable"`
		CouponCode string `json:"coupon_code"`
		Children   int    `json:"children"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if in.Rooms == 0 {
		in.Rooms = 1
	}
	opt := rateOption{Meal: in.Meal, Refundable: in.Refundable == nil || *in.Refundable}
	if opt.Meal == "" {
		opt.Meal = mealRoomOnly
	}
	bad := func(msg string) error { return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, msg) }
	switch {
	case !isDate(in.CheckIn) || !isDate(in.CheckOut):
		return bad("check_in and check_out must be dates (YYYY-MM-DD)")
	case in.CheckIn < s.addisToday():
		return bad("The check-in date has already passed")
	case nightsBetween(in.CheckIn, in.CheckOut) < 1 || nightsBetween(in.CheckIn, in.CheckOut) > maxNights:
		return bad("A stay is between 1 and 30 nights")
	case in.Rooms < 1 || in.Rooms > maxRooms:
		return bad("Choose between 1 and 9 rooms")
	case in.Guests < in.Rooms:
		return bad("Each room needs at least one guest")
	case in.Children < 0 || in.Children > in.Guests-in.Rooms:
		return bad("Each room needs at least one adult")
	}
	ctx := r.Context()

	var propName, roomName string
	var capacity int
	var payAtHotel bool
	var coverID *string
	var checkInTime, checkOutTime string
	err := s.pool.QueryRow(ctx, `SELECT p.name, rt.name, rt.capacity, p.pay_at_hotel,
		(SELECT pp.id FROM property_photos pp WHERE pp.property_id = p.id AND pp.room_type_id IS NULL ORDER BY pp.position, pp.created_at LIMIT 1),
		to_char(p.check_in_time, 'HH24:MI'), to_char(p.check_out_time, 'HH24:MI')
		FROM room_types rt JOIN properties p ON p.id = rt.property_id
		WHERE rt.id = $1 AND p.id = $2 AND p.status = 'active'`, in.RoomTypeID, in.PropertyID).
		Scan(&propName, &roomName, &capacity, &payAtHotel, &coverID, &checkInTime, &checkOutTime)
	if err == pgx.ErrNoRows || isBadUUID(err) {
		return errNotFound
	}
	if err != nil {
		return err
	}
	if in.Guests > capacity*in.Rooms {
		return fail(http.StatusConflict, codeInvalidState, "Too many guests for these rooms")
	}

	rts, err := s.roomsFor(ctx, s.pool, []string{in.PropertyID}, in.CheckIn, in.CheckOut)
	if err != nil {
		return err
	}
	left := -1
	for _, rt := range rts[in.PropertyID] {
		if rt.ID == in.RoomTypeID {
			left = rt.RoomsLeft
		}
	}
	if left < in.Rooms {
		return fail(http.StatusConflict, codeSoldOut, "Not enough rooms are left for those dates")
	}
	var coupon *couponRule
	var disc func(int64) (int64, error)
	if strings.TrimSpace(in.CouponCode) != "" {
		if coupon, err = s.loadCoupon(ctx, s.pool, claimsFrom(r).Sub, in.CouponCode, in.PropertyID, false); err != nil {
			return err
		}
		disc = coupon.discount
	}
	price, err := s.priceStay(ctx, s.pool, in.RoomTypeID, in.CheckIn, in.CheckOut, in.Rooms, opt, disc)
	if err != nil {
		return err
	}
	var couponID, couponCode *string
	if coupon != nil {
		couponID, couponCode = &coupon.id, &coupon.code
	}

	var id string
	if err := s.pool.QueryRow(ctx, `INSERT INTO quotes (user_id, property_id, room_type_id, check_in, check_out, guests, rooms, currency,
			base_santim, service_santim, vat_santim, total_santim, deposit_santim, balance_due_santim, price_snapshot, expires_at, meal, refundable, coupon_id, discount_santim)
		VALUES ($1, $2, $3, $4::date, $5::date, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, now() + $16::interval, $17, $18, $19, $20) RETURNING id`,
		claimsFrom(r).Sub, in.PropertyID, in.RoomTypeID, in.CheckIn, in.CheckOut, in.Guests, in.Rooms, price.Currency,
		price.Base, price.Service, price.VAT, price.Total, price.Deposit, price.Balance, price.Nights, "600 seconds", opt.Meal, opt.Refundable, couponID, price.Discount).Scan(&id); err != nil {
		return err
	}
	if in.Children > 0 {
		if _, err := s.pool.Exec(ctx, `UPDATE quotes SET children = $2 WHERE id = $1`, id, in.Children); err != nil {
			return err
		}
	}
	respond(w, http.StatusOK, quoteOut{
		Meta: httpx.NowMeta(), ID: id, PropertyID: in.PropertyID, PropertyName: propName, RoomTypeID: in.RoomTypeID, RoomName: roomName,
		CheckIn: in.CheckIn, CheckOut: in.CheckOut, Nights: len(price.Nights), Guests: in.Guests, Rooms: in.Rooms, RoomsAvailable: left,
		Nightly: price.Nightly, Subtotal: price.Base + price.Service, VAT: price.VAT, Total: price.Total, Currency: price.Currency,
		// A booking with a coupon is paid online: nobody could fund the discount on money collected at the hotel.
		PayAtHotel: payAtHotel && coupon == nil,
		Meal:       opt.Meal, Refundable: opt.Refundable, CouponCode: couponCode, Discount: price.Discount, ListTotal: price.Total + price.Discount, Children: in.Children,
		PhotoURL: photoLink(s, coverID), CheckInTime: checkInTime, CheckOutTime: checkOutTime,
	})
	return nil
}

func isBadUUID(err error) bool {
	var pe *pgconn.PgError
	if asPg(err, &pe) {
		return pe.Code == "22P02" // invalid_text_representation
	}
	return err != nil && strings.Contains(err.Error(), "invalid input syntax for type uuid")
}

func asPg(err error, target **pgconn.PgError) bool {
	for err != nil {
		if pe, ok := err.(*pgconn.PgError); ok {
			*target = pe
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}

// ---- booking views

type bookingOut struct {
	httpx.Meta
	Children        int     `json:"children"`
	SpecialRequests *string `json:"special_requests"`
	BookedForOther  bool    `json:"booked_for_other"`
	GuestPhone      *string `json:"guest_phone"`
	ID              string  `json:"id"`
	Reference       string  `json:"reference"`
	Status          string  `json:"status"`
	PropertyID      string  `json:"property_id"`
	PropertyName    string  `json:"property_name"`
	PropertyPhone   string  `json:"property_phone"`
	Address         string  `json:"address"`
	RoomName        string  `json:"room_name"`
	CheckIn         string  `json:"check_in"`
	CheckOut        string  `json:"check_out"`
	Nights          int     `json:"nights"`
	Guests          int     `json:"guests"`
	Rooms           int     `json:"rooms"`
	GuestName       string  `json:"guest_name"`
	Total           int64   `json:"total_santim"`
	Currency        string  `json:"currency"`
	PaymentMethod   *string `json:"payment_method"`
	PayAtHotel      bool    `json:"pay_at_hotel"`
	Payment         string  `json:"payment"` // none | pending | failed
	HoldExpiresAt   *string `json:"hold_expires_at"`
	// Cancellation: whether it is possible now, what the guest would get back, and until when a confirmed booking is refunded in full.
	CanCancel         bool    `json:"can_cancel"`
	RefundIfCancelled int64   `json:"refund_if_cancelled_santim"`
	FullRefundUntil   *string `json:"full_refund_until"`
	RefundSantim      *int64  `json:"refund_santim"` // what a cancellation refunded (0 when too late for a refund)
	// Review: whether the guest may write one now, and the one they wrote.
	CanReview bool       `json:"can_review"`
	Review    *reviewOut `json:"review"`
	// Dispute: whether the guest may open one now (once per booking, from the start of the stay until 14 days after check-out), and the
	// guest's dispute on this booking, if any.
	CanDispute      bool    `json:"can_dispute"`
	DisputeTicketID *string `json:"dispute_ticket_id"`
	// Dates: whether the guest may move the booking to other dates now (once, same price: see datechange.go), and the dates it first had.
	// PartiallyRefunded is the part of the payment already returned while the booking stands (a dispute settled in part, or a goodwill credit).
	PartiallyRefunded int64   `json:"partially_refunded_santim"`
	CanChangeDates    bool    `json:"can_change_dates"`
	OriginalCheckIn   *string `json:"original_check_in"`
	OriginalCheckOut  *string `json:"original_check_out"`
	// CheckInTime and CheckOutTime are the hotel's times (HH:MM, Addis Ababa).
	CheckInTime  string `json:"check_in_time"`
	CheckOutTime string `json:"check_out_time"`
	// Timeline lists what happened to the booking and when (only on the single-booking view).
	Timeline []timelineEvent `json:"timeline,omitempty"`
	// The room option booked (meal room_only or breakfast; refundable false = no refund once confirmed) and the coupon, if any.
	Meal           string  `json:"meal"`
	Refundable     bool    `json:"refundable"`
	CouponCode     *string `json:"coupon_code"`
	DiscountSantim int64   `json:"discount_santim"`
}

type timelineEvent struct {
	Status string `json:"status"`
	At     string `json:"at"`
}

const bookingSelect = `
	SELECT b.id, b.code, b.status, b.property_id, p.name, p.phone, p.address, rt.name, b.check_in, b.check_out, b.guests, b.rooms,
	       b.guest_name, b.total_santim, b.currency, b.payment_method, p.pay_at_hotel, b.hold_expires_at,
	       COALESCE((SELECT CASE pay.status WHEN 'pending' THEN 'pending' WHEN 'failed' THEN 'failed' ELSE 'none' END
	                 FROM payments pay WHERE pay.booking_id = b.id ORDER BY pay.created_at DESC LIMIT 1), 'none'),
	       greatest(COALESCE((SELECT sum(amount_santim) FROM payments pp WHERE pp.booking_id = b.id AND pp.status = 'succeeded'), 0)
	         - COALESCE((SELECT sum(r.amount_santim) FROM partial_refunds r WHERE r.booking_id = b.id), 0), 0)::bigint,
	       b.refund_santim, rv.rating, rv.text, rv.hotel_reply, rv.hotel_reply_at, to_char(p.check_in_time, 'HH24:MI'), to_char(p.check_out_time, 'HH24:MI'),
	       b.meal, b.refundable, cp.code, b.discount_santim, b.children, b.special_requests, b.booked_for_other, b.guest_phone,
	       (SELECT dt.id FROM support_tickets dt WHERE dt.booking_id = b.id AND dt.kind = 'dispute' ORDER BY dt.created_at DESC LIMIT 1),
	       b.date_changes, b.original_check_in, b.original_check_out,
	       COALESCE((SELECT sum(r.amount_santim) FROM partial_refunds r WHERE r.booking_id = b.id), 0)::bigint
	FROM bookings b JOIN properties p ON p.id = b.property_id JOIN room_types rt ON rt.id = b.room_type_id
	LEFT JOIN reviews rv ON rv.booking_id = b.id LEFT JOIN coupons cp ON cp.id = b.coupon_id`

func (s *Server) scanBooking(row pgx.Row) (bookingOut, error) {
	var b bookingOut
	var in, out time.Time
	var hold *time.Time
	var paid int64
	var reviewRating *int
	var reviewText *string
	var replyText *string
	var replyAt *time.Time
	var changes int
	var origIn, origOut *time.Time
	if err := row.Scan(&b.ID, &b.Reference, &b.Status, &b.PropertyID, &b.PropertyName, &b.PropertyPhone, &b.Address, &b.RoomName,
		&in, &out, &b.Guests, &b.Rooms, &b.GuestName, &b.Total, &b.Currency, &b.PaymentMethod, &b.PayAtHotel, &hold, &b.Payment,
		&paid, &b.RefundSantim, &reviewRating, &reviewText, &replyText, &replyAt, &b.CheckInTime, &b.CheckOutTime,
		&b.Meal, &b.Refundable, &b.CouponCode, &b.DiscountSantim, &b.Children, &b.SpecialRequests, &b.BookedForOther, &b.GuestPhone, &b.DisputeTicketID, &changes, &origIn, &origOut, &b.PartiallyRefunded); err != nil {
		return b, err
	}
	if !phoneVisible(b.Status) {
		b.PropertyPhone = "" // shown once the hotel has confirmed the booking
	}
	b.CheckIn, b.CheckOut = pgDate(in), pgDate(out)
	b.CanChangeDates = s.canChangeDates(b.Status, in, b.Refundable, b.CouponCode, changes)
	if origIn != nil && origOut != nil {
		oi, oo := pgDate(*origIn), pgDate(*origOut)
		b.OriginalCheckIn, b.OriginalCheckOut = &oi, &oo
	}
	b.CanDispute = b.DisputeTicketID == nil && s.disputable(b.Status, b.CheckIn, b.CheckOut)
	b.Nights = nightsBetween(b.CheckIn, b.CheckOut)
	b.Meta = httpx.NowMeta()
	// The status is derived from time as well, so a hold that ran out reads EXPIRED even before the sweeper has run.
	if b.Status == "HELD" {
		if hold != nil && !hold.After(s.now()) {
			b.Status = "EXPIRED"
		} else if hold != nil {
			t := hold.UTC().Format(time.RFC3339)
			b.HoldExpiresAt = &t
		}
	}
	if b.Status == "HELD" || b.Status == "EXPIRED" {
		b.PaymentMethod = nil // the chosen method becomes the paid method only once paid
	}
	if b.Status != "HELD" {
		b.Payment = "none"
	}
	// A guest whose hotel cannot host them can always cancel, for a full refund.
	if b.Status == "NEEDS_RELOCATION" {
		b.CanCancel, b.RefundIfCancelled = true, paid
	}
	// What cancelling would do right now (the server decides; the app only shows it).
	checkInAt, until := cancelWindow(in)
	if open := s.now().Before(checkInAt); open && (b.Status == "PAID_AWAITING_HOTEL" || b.Status == "CONFIRMED") {
		b.CanCancel = true
		// A non-refundable booking gets nothing back once the hotel has confirmed it (before that, everything).
		if b.Status == "PAID_AWAITING_HOTEL" || (b.Refundable && !s.now().After(until)) {
			b.RefundIfCancelled = paid
		}
		if b.Status == "CONFIRMED" && paid > 0 && b.Refundable {
			u := until.UTC().Format(time.RFC3339)
			b.FullRefundUntil = &u
		}
	}
	if reviewRating != nil {
		text := ""
		if reviewText != nil {
			text = *reviewText
		}
		b.Review = &reviewOut{Rating: *reviewRating, Text: text}
		if replyText != nil && replyAt != nil {
			b.Review.Reply = &replyOut{*replyText, pgDate(*replyAt)}
		}
	} else if (b.Status == "CONFIRMED" || b.Status == "CHECKED_IN" || b.Status == "COMPLETED") && b.CheckOut <= s.addisToday() {
		b.CanReview = true
	}
	return b, nil
}

func (s *Server) loadBooking(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, id, userID string) (bookingOut, error) {
	b, err := s.scanBooking(q.QueryRow(ctx, bookingSelect+` WHERE b.id = $1 AND b.user_id = $2`, id, userID))
	if err == pgx.ErrNoRows || isBadUUID(err) {
		return b, errNotFound
	}
	return b, err
}

func (s *Server) getBooking(w http.ResponseWriter, r *http.Request) error {
	s.settleChapaPending(r.Context(), chi.URLParam(r, "id"), claimsFrom(r).Sub)
	b, err := s.loadBooking(r.Context(), s.pool, chi.URLParam(r, "id"), claimsFrom(r).Sub)
	if err != nil {
		return err
	}
	if b.Timeline, err = s.timeline(r.Context(), b.ID); err != nil {
		return err
	}
	respond(w, http.StatusOK, b)
	return nil
}

func (s *Server) listBookings(w http.ResponseWriter, r *http.Request) error {
	limit, err := pageLimit(r, 100)
	if err != nil {
		return err
	}
	after, afterID, err := decodeCursor(r.URL.Query().Get("cursor"))
	if err != nil {
		return err
	}
	// One more than asked for tells whether there is a next page. Newest first; the id breaks ties so no booking is skipped or repeated.
	rows, err := s.pool.Query(r.Context(), bookingSelect+` WHERE b.user_id = $1 AND ($2::timestamptz IS NULL OR (b.created_at, b.id) < ($2::timestamptz, $3::uuid))
		ORDER BY b.created_at DESC, b.id DESC LIMIT $4`, claimsFrom(r).Sub, after, nullIfEmpty(afterID), limit+1)
	if err != nil {
		return err
	}
	defer rows.Close()
	items := []bookingOut{}
	for rows.Next() {
		b, err := s.scanBooking(rows)
		if err != nil {
			return err
		}
		items = append(items, b)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()
	var next *string
	if len(items) > limit {
		items = items[:limit]
		var at time.Time
		last := items[len(items)-1].ID
		if err := s.pool.QueryRow(r.Context(), `SELECT created_at FROM bookings WHERE id = $1`, last).Scan(&at); err != nil {
			return err
		}
		c := encodeCursor(at, last)
		next = &c
	}
	respond(w, http.StatusOK, struct {
		httpx.Meta
		Items      []bookingOut `json:"items"`
		NextCursor *string      `json:"next_cursor"`
	}{httpx.NowMeta(), items, next})
	return nil
}

func nullIfEmpty(v string) any {
	if v == "" {
		return nil
	}
	return v
}

// ---- inventory helpers (always inside a transaction)

func ensureInventory(ctx context.Context, tx pgx.Tx, roomTypeID, in, out string) error {
	_, err := tx.Exec(ctx, `INSERT INTO inventory (room_type_id, night, capacity)
		SELECT rt.id, d::date, rt.total_rooms FROM room_types rt, generate_series($2::date, $3::date - 1, '1 day') d WHERE rt.id = $1
		ON CONFLICT DO NOTHING`, roomTypeID, in, out)
	return err
}

// holdRooms takes rooms on every night of the stay, or none of them (SOLD_OUT).
func holdRooms(ctx context.Context, tx pgx.Tx, roomTypeID, in, out string, rooms int) error {
	if err := ensureInventory(ctx, tx, roomTypeID, in, out); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE inventory SET held = held + $4
		WHERE room_type_id = $1 AND night >= $2::date AND night < $3::date AND held + booked + $4 <= capacity`, roomTypeID, in, out, rooms)
	if err != nil {
		return err
	}
	if int(tag.RowsAffected()) != nightsBetween(in, out) {
		return fail(http.StatusConflict, codeSoldOut, "Those rooms were just taken")
	}
	return nil
}

func releaseRooms(ctx context.Context, tx pgx.Tx, roomTypeID, in, out string, rooms int) error {
	_, err := tx.Exec(ctx, `UPDATE inventory SET held = held - $4
		WHERE room_type_id = $1 AND night >= $2::date AND night < $3::date`, roomTypeID, in, out, rooms)
	return err
}

func confirmRooms(ctx context.Context, tx pgx.Tx, roomTypeID, in, out string, rooms int) error {
	_, err := tx.Exec(ctx, `UPDATE inventory SET held = held - $4, booked = booked + $4
		WHERE room_type_id = $1 AND night >= $2::date AND night < $3::date`, roomTypeID, in, out, rooms)
	return err
}

// ---- hold

const codeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // no 0/O/1/I: read aloud at a front desk

func newCode() string {
	b := make([]byte, 6)
	for i := range b {
		b[i] = codeAlphabet[rand.IntN(len(codeAlphabet))]
	}
	return "BD-" + string(b)
}

func (s *Server) createBooking(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		QuoteID         string `json:"quote_id"`
		GuestName       string `json:"guest_name"`
		IdempotencyKey  string `json:"idempotency_key"`
		SpecialRequests string `json:"special_requests"`
		BookedForOther  bool   `json:"booked_for_other"`
		GuestPhone      string `json:"guest_phone"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	key := r.Header.Get("Idempotency-Key")
	if key == "" {
		key = in.IdempotencyKey
	}
	name := strings.TrimSpace(in.GuestName)
	if len(key) < 8 || len(key) > 255 || len(name) < 2 || len(name) > 120 || in.QuoteID == "" {
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "quote_id, guest_name and an idempotency key are required")
	}
	in.SpecialRequests, in.GuestPhone = strings.TrimSpace(in.SpecialRequests), strings.TrimSpace(in.GuestPhone)
	if len(in.SpecialRequests) > 400 || (in.GuestPhone != "" && (!phoneAnyRE.MatchString(in.GuestPhone) || len(in.GuestPhone) > 30)) {
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "Special requests are at most 400 characters; give the guest's phone number as digits")
	}
	uid := claimsFrom(r).Sub
	hash := sha256.Sum256([]byte(in.QuoteID + "|" + name + "|" + in.SpecialRequests + "|" + b2s(in.BookedForOther) + "|" + in.GuestPhone))
	ctx := r.Context()

	var bookingID string
	err := s.tx(ctx, func(tx pgx.Tx) error {
		// 1. Safe retries: the same key and request returns the same booking; the same key for a different request is an error.
		tag, err := tx.Exec(ctx, `INSERT INTO idempotency_keys (user_id, key, endpoint, request_hash) VALUES ($1, $2, 'POST /v1/bookings', $3)
			ON CONFLICT DO NOTHING`, uid, key, hash[:])
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			var prior []byte
			var body *string
			if err := tx.QueryRow(ctx, `SELECT request_hash, response_body->>'booking_id' FROM idempotency_keys WHERE user_id = $1 AND key = $2`, uid, key).Scan(&prior, &body); err != nil {
				return err
			}
			if hex.EncodeToString(prior) != hex.EncodeToString(hash[:]) {
				return fail(http.StatusConflict, codeIdemMismatch, "This key was used for a different booking")
			}
			if body == nil {
				return fail(http.StatusConflict, codeInvalidState, "This request is still being processed")
			}
			bookingID = *body
			return nil
		}

		// 2. The price must still be valid and belong to this guest.
		var q struct {
			propertyID, roomTypeID, currency    string
			checkIn, checkOut                   time.Time
			guests, rooms                       int
			base, service, vat, total, dep, bal int64
			expired                             bool
			meal                                string
			refundable                          bool
			couponID                            *string
			discount                            int64
		}
		err = tx.QueryRow(ctx, `SELECT property_id, room_type_id, check_in, check_out, guests, rooms, currency, base_santim, service_santim,
				vat_santim, total_santim, deposit_santim, balance_due_santim, expires_at <= now(), meal, refundable, coupon_id::text, discount_santim
			FROM quotes WHERE id = $1 AND user_id = $2 FOR UPDATE`, in.QuoteID, uid).
			Scan(&q.propertyID, &q.roomTypeID, &q.checkIn, &q.checkOut, &q.guests, &q.rooms, &q.currency, &q.base, &q.service, &q.vat, &q.total, &q.dep, &q.bal, &q.expired, &q.meal, &q.refundable, &q.couponID, &q.discount)
		if err == pgx.ErrNoRows || isBadUUID(err) {
			return errNotFound
		}
		if err != nil {
			return err
		}
		if q.expired {
			return fail(http.StatusConflict, codeQuoteExpired, "The price has expired. Check it again.")
		}
		var used bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM bookings WHERE quote_id = $1)`, in.QuoteID).Scan(&used); err != nil {
			return err
		}
		if used {
			return fail(http.StatusConflict, codeQuoteExpired, "This price was already used")
		}

		// 3. The hotel receives the guest's profile with the request, so there must be one.
		var contact string
		var hasProfile bool
		if err := tx.QueryRow(ctx, `SELECT coalesce(u.phone, u.email), EXISTS (SELECT 1 FROM guest_profiles g WHERE g.user_id = u.id) FROM users u WHERE u.id = $1`, uid).Scan(&contact, &hasProfile); err != nil {
			return err
		}
		if !hasProfile {
			return fail(http.StatusConflict, codeProfileMissing, "Add your name and ID before booking")
		}

		// A coupon is checked again now, with its row locked: it may have run out or expired since the price was shown.
		if q.couponID != nil {
			var code string
			if err := tx.QueryRow(ctx, `SELECT code FROM coupons WHERE id = $1`, *q.couponID).Scan(&code); err != nil {
				return err
			}
			if _, err := s.loadCoupon(ctx, tx, uid, code, q.propertyID, true); err != nil {
				return err
			}
		}

		// 4. Take the rooms, all nights or none.
		ci, co := pgDate(q.checkIn), pgDate(q.checkOut)
		if err := holdRooms(ctx, tx, q.roomTypeID, ci, co, q.rooms); err != nil {
			return err
		}
		if err := actor(ctx, tx, "guest:"+uid, "hold placed"); err != nil {
			return err
		}
		for attempt := 0; ; attempt++ {
			err = tx.QueryRow(ctx, `INSERT INTO bookings (code, quote_id, user_id, property_id, room_type_id, check_in, check_out, guests, rooms, guest_name,
					currency, base_santim, service_santim, vat_santim, total_santim, deposit_santim, balance_due_santim, hold_expires_at, meal, refundable, coupon_id, discount_santim)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, now() + $18::interval, $19, $20, $21::uuid, $22)
				ON CONFLICT (code) DO NOTHING RETURNING id`,
				newCode(), in.QuoteID, uid, q.propertyID, q.roomTypeID, q.checkIn, q.checkOut, q.guests, q.rooms, name,
				q.currency, q.base, q.service, q.vat, q.total, q.dep, q.bal, "900 seconds", q.meal, q.refundable, q.couponID, q.discount).Scan(&bookingID)
			if err != pgx.ErrNoRows || attempt >= 5 {
				break
			}
		}
		if err != nil {
			return err
		}
		// Who is travelling and what they ask the hotel for.
		if _, err := tx.Exec(ctx, `UPDATE bookings SET children = (SELECT children FROM quotes WHERE id = $2), special_requests = NULLIF($3, ''), booked_for_other = $4, guest_phone = NULLIF($5, '') WHERE id = $1`,
			bookingID, in.QuoteID, in.SpecialRequests, in.BookedForOther, in.GuestPhone); err != nil {
			return err
		}
		if q.couponID != nil {
			if _, err := tx.Exec(ctx, `INSERT INTO coupon_redemptions (coupon_id, user_id, booking_id, discount_santim) VALUES ($1, $2, $3, $4)`, *q.couponID, uid, bookingID, q.discount); err != nil {
				return err
			}
		}
		// 5. A frozen copy of the profile for the hotel. It is never changed by later edits to the guest's profile.
		if _, err := tx.Exec(ctx, `INSERT INTO booking_guests (booking_id, property_id, full_name, contact, nationality, id_type, id_number_enc, id_number_hash, id_last4)
			SELECT $1, $2, g.full_name, $3, g.nationality, g.id_type, g.id_number_enc, g.id_number_hash, g.id_last4 FROM guest_profiles g WHERE g.user_id = $4`,
			bookingID, q.propertyID, contact, uid); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE idempotency_keys SET response_status = 200, response_body = jsonb_build_object('booking_id', $3::text) WHERE user_id = $1 AND key = $2`, uid, key, bookingID)
		return err
	})
	if err != nil {
		return err
	}
	b, err := s.loadBooking(ctx, s.pool, bookingID, uid)
	if err != nil {
		return err
	}
	respond(w, http.StatusOK, b)
	return nil
}

// settleChapaPending asks Chapa about this guest\x27s pending card or bank payment, so a booking moves on as soon as the guest is
// back in the app even if Chapa\x27s own notice is late or never arrives. Only the guest\x27s own booking is checked. Errors are logged,
// not returned: the booking still shows its current state.
func (s *Server) settleChapaPending(ctx context.Context, id, userID string) {
	rows, err := s.pool.Query(ctx, `SELECT p.provider_ref FROM payments p JOIN bookings b ON b.id = p.booking_id
		WHERE p.booking_id = $1 AND b.user_id = $2 AND p.provider = 'chapa' AND p.status = 'pending' AND p.provider_ref <> ''`, id, userID)
	if err != nil {
		return
	}
	var refs []string
	for rows.Next() {
		var ref string
		if rows.Scan(&ref) == nil {
			refs = append(refs, ref)
		}
	}
	rows.Close()
	for _, ref := range refs {
		if err := s.chapaSettle(ctx, ref); err != nil && err != errNotFound {
			slog.Warn("chapa check from the app failed", "ref", ref, "err", err)
		}
	}
}

// photoLink is the address of a hotel's photo (the small copy), or nil when the hotel has none.
func photoLink(s *Server, id *string) *string {
	if id == nil {
		return nil
	}
	u := s.photoURL(*id, "thumb")
	return &u
}
