package api

import (
	"net/http"
	"time"

	"github.com/bookdan/backend/internal/httpx"
)

// The admin's numbers: how many guests searched-to-booked, the money, how fast hotels answer, who cancels, and what waits for a person.
// Everything is computed from the same tables the app runs on, for the last N days (Addis Ababa days), and refreshed on every read.

type dayPoint struct {
	Day       string `json:"day"`
	Bookings  int    `json:"bookings"`  // created (held) that day
	Confirmed int    `json:"confirmed"` // confirmed by the hotel that day
	GMV       int64  `json:"gmv_santim"`
}

type hotelStat struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Confirmed    int      `json:"confirmed"`
	GMV          int64    `json:"gmv_santim"`
	Requests     int      `json:"requests"`    // paid and waiting for an answer in the period
	Accepted     int      `json:"accepted"`    // ... answered yes
	Declined     int      `json:"declined"`    // ... answered no
	NoResponse   int      `json:"no_response"` // ... did not answer in time
	AvgReplyMin  *float64 `json:"avg_reply_minutes"`
	HotelCancels int      `json:"hotel_cancellations"`
}

type analyticsOut struct {
	httpx.Meta
	Days int    `json:"days"`
	From string `json:"from"`
	To   string `json:"to"`
	// The funnel: quotes asked for, rooms held, paid (or chosen to pay at the hotel), confirmed by the hotel, stays completed.
	Quotes    int `json:"quotes"`
	Holds     int `json:"holds"`
	Paid      int `json:"paid"`
	Confirmed int `json:"confirmed"`
	Completed int `json:"completed"`
	// Where bookings ended without a stay.
	Expired        int `json:"expired"`
	Declined       int `json:"declined"`
	NoResponse     int `json:"no_response"`
	CancelledGuest int `json:"cancelled_by_guest"`
	CancelledHotel int `json:"cancelled_by_hotel"`
	CancelledAdmin int `json:"cancelled_by_admin"`
	NoShows        int `json:"no_shows"`
	// Money, in santim: the value of what the hotels confirmed, BookDan's commission, refunds, and how much came through each way of paying.
	GMV           int64   `json:"gmv_santim"`
	Commission    int64   `json:"commission_santim"`
	CommissionVAT int64   `json:"commission_vat_santim"`
	Refunded      int64   `json:"refunded_santim"`
	PaidOnline    int     `json:"confirmed_paid_online"`
	PaidAtHotel   int     `json:"confirmed_pay_at_hotel"`
	Discounts     int64   `json:"coupon_discounts_santim"`
	AvgBooking    int64   `json:"avg_booking_santim"`
	AvgNights     float64 `json:"avg_nights"`
	NewGuests     int     `json:"new_guests"`
	RepeatGuests  int     `json:"repeat_guests"` // guests with more than one confirmed booking in the period
	// What waits for a person right now (not limited to the period).
	OpenSupport       int         `json:"open_support_requests"`
	OpenDisputes      int         `json:"open_disputes"`
	OpenRelocations   int         `json:"open_relocations"`
	OpenPaymentIssues int         `json:"open_payment_issues"`
	AwaitingHotel     int         `json:"awaiting_hotel_now"`
	Daily             []dayPoint  `json:"daily"`
	Hotels            []hotelStat `json:"hotels"`
}

func (s *Server) adminAnalytics(w http.ResponseWriter, r *http.Request) error {
	if err := requireAdmin(r); err != nil {
		return err
	}
	days := atoiDefault(r.URL.Query().Get("days"), 30)
	if days < 1 || days > 365 {
		days = 30
	}
	ctx := r.Context()
	today := s.addisToday()
	t0, _ := time.Parse("2006-01-02", today)
	from := t0.AddDate(0, 0, -(days - 1)).Format("2006-01-02")
	to := today
	a := analyticsOut{Meta: httpx.NowMeta(), Days: days, From: from, To: to}

	// All the period arguments are Addis Ababa dates: from (inclusive) and to (inclusive).
	day := func(col string) string {
		return `(` + col + ` AT TIME ZONE 'Africa/Addis_Ababa')::date BETWEEN $1::date AND $2::date`
	}
	one := func(dst any, sql string) error { return s.pool.QueryRow(ctx, sql, from, to).Scan(dst) }

	var firstErr error
	count := func(dst *int, sql string) {
		if firstErr == nil {
			firstErr = one(dst, sql)
		}
	}
	count(&a.Quotes, `SELECT count(*)::int FROM quotes WHERE `+day("created_at"))
	count(&a.Holds, `SELECT count(*)::int FROM bookings WHERE `+day("created_at"))
	count(&a.Paid, `SELECT count(*)::int FROM bookings WHERE paid_at IS NOT NULL AND `+day("paid_at"))
	count(&a.Confirmed, `SELECT count(DISTINCT booking_id)::int FROM booking_events WHERE to_status = 'CONFIRMED' AND `+day("created_at"))
	count(&a.Completed, `SELECT count(DISTINCT booking_id)::int FROM booking_events WHERE to_status = 'COMPLETED' AND `+day("created_at"))
	count(&a.Expired, `SELECT count(*)::int FROM bookings WHERE status = 'EXPIRED' AND `+day("created_at"))
	count(&a.Declined, `SELECT count(*)::int FROM bookings WHERE status = 'DECLINED_REFUNDED' AND coalesce(decline_reason, '') <> 'no_response' AND paid_at IS NOT NULL AND `+day("paid_at"))
	count(&a.NoResponse, `SELECT count(*)::int FROM bookings WHERE status = 'DECLINED_REFUNDED' AND decline_reason = 'no_response' AND paid_at IS NOT NULL AND `+day("paid_at"))
	count(&a.CancelledGuest, `SELECT count(*)::int FROM bookings WHERE cancelled_by = 'guest' AND `+day("cancelled_at"))
	count(&a.CancelledHotel, `SELECT count(*)::int FROM bookings WHERE cancelled_by = 'hotel' AND `+day("cancelled_at"))
	count(&a.CancelledAdmin, `SELECT count(*)::int FROM bookings WHERE cancelled_by = 'admin' AND `+day("cancelled_at"))
	count(&a.NoShows, `SELECT count(DISTINCT booking_id)::int FROM booking_events WHERE to_status = 'NO_SHOW' AND `+day("created_at"))
	count(&a.NewGuests, `SELECT count(*)::int FROM users WHERE role = 'guest' AND status = 'active' AND `+day("created_at"))
	count(&a.RepeatGuests, `SELECT count(*)::int FROM (SELECT b.user_id FROM bookings b JOIN booking_events e ON e.booking_id = b.id AND e.to_status = 'CONFIRMED' AND `+day("e.created_at")+` GROUP BY b.user_id HAVING count(DISTINCT b.id) > 1) x`)
	count(&a.PaidOnline, `SELECT count(DISTINCT e.booking_id)::int FROM booking_events e JOIN bookings b ON b.id = e.booking_id WHERE e.to_status = 'CONFIRMED' AND b.payment_method <> 'pay_at_hotel' AND `+day("e.created_at"))
	count(&a.PaidAtHotel, `SELECT count(DISTINCT e.booking_id)::int FROM booking_events e JOIN bookings b ON b.id = e.booking_id WHERE e.to_status = 'CONFIRMED' AND b.payment_method = 'pay_at_hotel' AND `+day("e.created_at"))
	count(&a.OpenSupport, `SELECT count(*)::int FROM support_tickets WHERE status <> 'closed' AND kind IN ('question', 'problem') AND $1::date IS NOT NULL AND $2::date IS NOT NULL`)
	count(&a.OpenDisputes, `SELECT count(*)::int FROM support_tickets WHERE status <> 'closed' AND kind = 'dispute' AND $1::date IS NOT NULL AND $2::date IS NOT NULL`)
	count(&a.OpenRelocations, `SELECT count(*)::int FROM support_tickets WHERE status <> 'closed' AND kind = 'relocation' AND $1::date IS NOT NULL AND $2::date IS NOT NULL`)
	count(&a.OpenPaymentIssues, `SELECT count(*)::int FROM payment_issues WHERE resolved_at IS NULL AND $1::date IS NOT NULL AND $2::date IS NOT NULL`)
	count(&a.AwaitingHotel, `SELECT count(*)::int FROM bookings WHERE status = 'PAID_AWAITING_HOTEL' AND $1::date IS NOT NULL AND $2::date IS NOT NULL`)
	if firstErr != nil {
		return firstErr
	}

	// Money. GMV is the total price of the bookings the hotels confirmed in the period.
	if err := s.pool.QueryRow(ctx, `SELECT coalesce(sum(b.total_santim), 0)::bigint, coalesce(sum(b.discount_santim), 0)::bigint, coalesce(avg(b.total_santim), 0)::bigint,
			coalesce(avg((b.check_out - b.check_in)::numeric), 0)::float8
		FROM bookings b WHERE b.id IN (SELECT booking_id FROM booking_events WHERE to_status = 'CONFIRMED' AND `+day("created_at")+`)`, from, to).Scan(&a.GMV, &a.Discounts, &a.AvgBooking, &a.AvgNights); err != nil {
		return err
	}
	if err := s.pool.QueryRow(ctx, `SELECT coalesce(-sum(e.amount_santim) FILTER (WHERE e.account_code = 'commission_revenue'), 0)::bigint,
			coalesce(-sum(e.amount_santim) FILTER (WHERE e.account_code = 'vat_on_commission_payable'), 0)::bigint
		FROM ledger_entries e JOIN ledger_transactions t ON t.id = e.transaction_id WHERE `+day("t.created_at"), from, to).Scan(&a.Commission, &a.CommissionVAT); err != nil {
		return err
	}
	if err := s.pool.QueryRow(ctx, `SELECT coalesce(sum(e.amount_santim), 0)::bigint FROM ledger_entries e JOIN ledger_transactions t ON t.id = e.transaction_id
		WHERE t.kind = 'refund' AND e.account_code = 'refunds_payable' AND e.amount_santim > 0 AND `+day("t.created_at"), from, to).Scan(&a.Refunded); err != nil {
		return err
	}

	// A point for every day of the period, also the quiet ones.
	rows, err := s.pool.Query(ctx, `SELECT d::date,
			(SELECT count(*)::int FROM bookings b WHERE (b.created_at AT TIME ZONE 'Africa/Addis_Ababa')::date = d::date),
			(SELECT count(DISTINCT e.booking_id)::int FROM booking_events e WHERE e.to_status = 'CONFIRMED' AND (e.created_at AT TIME ZONE 'Africa/Addis_Ababa')::date = d::date),
			(SELECT coalesce(sum(b.total_santim), 0)::bigint FROM bookings b WHERE b.id IN (SELECT booking_id FROM booking_events e WHERE e.to_status = 'CONFIRMED' AND (e.created_at AT TIME ZONE 'Africa/Addis_Ababa')::date = d::date))
		FROM generate_series($1::date, $2::date, '1 day') d ORDER BY d`, from, to)
	if err != nil {
		return err
	}
	a.Daily = []dayPoint{}
	for rows.Next() {
		var p dayPoint
		var d time.Time
		if err := rows.Scan(&d, &p.Bookings, &p.Confirmed, &p.GMV); err != nil {
			rows.Close()
			return err
		}
		p.Day = pgDate(d)
		a.Daily = append(a.Daily, p)
	}
	rows.Close()

	// Hotels: what they sold, and how they answer requests. A request is a booking that was paid and waited for the hotel in the period.
	hr, err := s.pool.Query(ctx, `
		WITH req AS (
		  SELECT b.property_id, b.status, b.decline_reason,
		         (SELECT min(e.created_at) FROM booking_events e WHERE e.booking_id = b.id AND e.from_status = 'PAID_AWAITING_HOTEL' AND e.to_status IN ('CONFIRMED', 'DECLINED_REFUNDED')) AS answered_at, b.paid_at,
		         EXISTS (SELECT 1 FROM booking_events e WHERE e.booking_id = b.id AND e.to_status = 'CONFIRMED') AS accepted
		  FROM bookings b WHERE b.paid_at IS NOT NULL AND `+day("b.paid_at")+`
		), conf AS (
		  SELECT b.property_id, count(*)::int AS n, coalesce(sum(b.total_santim), 0)::bigint AS gmv
		  FROM bookings b WHERE b.id IN (SELECT booking_id FROM booking_events WHERE to_status = 'CONFIRMED' AND `+day("created_at")+`) GROUP BY b.property_id
		)
		SELECT p.id, p.name, coalesce(c.n, 0), coalesce(c.gmv, 0),
		       count(r.status)::int,
		       count(r.status) FILTER (WHERE r.accepted)::int,
		       count(r.status) FILTER (WHERE r.status = 'DECLINED_REFUNDED' AND coalesce(r.decline_reason, '') <> 'no_response')::int,
		       count(r.status) FILTER (WHERE r.status = 'DECLINED_REFUNDED' AND r.decline_reason = 'no_response')::int,
		       (avg(extract(epoch FROM (r.answered_at - r.paid_at)) / 60) FILTER (WHERE r.answered_at IS NOT NULL AND coalesce(r.decline_reason, '') <> 'no_response'))::float8,
		       (SELECT count(*)::int FROM bookings hb WHERE hb.property_id = p.id AND hb.cancelled_by = 'hotel' AND `+day("hb.cancelled_at")+`)
		FROM properties p LEFT JOIN conf c ON c.property_id = p.id LEFT JOIN req r ON r.property_id = p.id
		WHERE p.status = 'active' GROUP BY p.id, p.name, c.n, c.gmv ORDER BY coalesce(c.gmv, 0) DESC, p.name LIMIT 50`, from, to)
	if err != nil {
		return err
	}
	defer hr.Close()
	a.Hotels = []hotelStat{}
	for hr.Next() {
		var h hotelStat
		if err := hr.Scan(&h.ID, &h.Name, &h.Confirmed, &h.GMV, &h.Requests, &h.Accepted, &h.Declined, &h.NoResponse, &h.AvgReplyMin, &h.HotelCancels); err != nil {
			return err
		}
		a.Hotels = append(a.Hotels, h)
	}
	if err := hr.Err(); err != nil {
		return err
	}
	respond(w, http.StatusOK, a)
	return nil
}
