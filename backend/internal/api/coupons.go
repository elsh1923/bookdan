package api

import (
	"context"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/bookdan/backend/internal/httpx"
)

// Coupons. The guest types a code at checkout; the server prices the stay with it and freezes the result in the quote.
// The discount comes off the room charges before service and VAT. BookDan pays for it: the ledger books the hotel's full share
// and the discount as a coupon cost (see confirm). A coupon works with online payment only, because nobody could fund a discount
// on money collected at the hotel.

const codeCouponInvalid = "COUPON_INVALID"

var couponCodeRE = regexp.MustCompile(`^[A-Z0-9]{3,20}$`)

type couponRule struct {
	id, code, kind, propertyID string
	value, minSubtotal         int64
	maxDiscount                *int64
	maxUses                    *int
	perUser                    int
	firstOnly                  bool
}

func couponProblem(msg string) error { return fail(http.StatusBadRequest, codeCouponInvalid, msg) }

// loadCoupon finds a coupon by code and checks everything that does not depend on the price: it is active, in its dates, for this hotel,
// not used up, not used too often by this guest, and (when it says so) the guest's first booking.
// For a booking (lock true) the coupon row is locked, so two guests cannot take the last use together.
func (s *Server) loadCoupon(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, userID, code, propertyID string, lock bool) (*couponRule, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if !couponCodeRE.MatchString(code) {
		return nil, couponProblem("That coupon code is not valid")
	}
	var c couponRule
	var prop *string
	var active bool
	var starts, ends *time.Time
	suffix := ""
	if lock {
		suffix = " FOR UPDATE"
	}
	err := q.QueryRow(ctx, `SELECT id, code, kind, value, max_discount_santim, min_subtotal_santim, starts_at, ends_at, max_uses, per_user_limit, first_booking_only, property_id::text, active
		FROM coupons WHERE code = $1`+suffix, code).
		Scan(&c.id, &c.code, &c.kind, &c.value, &c.maxDiscount, &c.minSubtotal, &starts, &ends, &c.maxUses, &c.perUser, &c.firstOnly, &prop, &active)
	if err == pgx.ErrNoRows {
		return nil, couponProblem("That coupon code is not valid")
	}
	if err != nil {
		return nil, err
	}
	now := s.now()
	switch {
	case !active:
		return nil, couponProblem("That coupon is no longer available")
	case starts != nil && now.Before(*starts):
		return nil, couponProblem("That coupon is not valid yet")
	case ends != nil && !now.Before(*ends):
		return nil, couponProblem("That coupon has expired")
	case prop != nil && *prop != propertyID:
		return nil, couponProblem("That coupon is not valid for this hotel")
	}
	// A use counts until its booking ends without the guest keeping the stay (expired hold, hotel declined, or cancelled with a refund).
	var total, mine int
	if err := q.QueryRow(ctx, `SELECT count(*)::int, count(*) FILTER (WHERE r.user_id = $2)::int
		FROM coupon_redemptions r JOIN bookings b ON b.id = r.booking_id
		WHERE r.coupon_id = $1 AND b.status NOT IN ('EXPIRED', 'DECLINED_REFUNDED', 'CANCELLED_REFUNDED')
		  AND NOT (b.status = 'HELD' AND b.hold_expires_at <= now())`, c.id, userID).Scan(&total, &mine); err != nil {
		return nil, err
	}
	if c.maxUses != nil && total >= *c.maxUses {
		return nil, couponProblem("That coupon has been used up")
	}
	if mine >= c.perUser {
		return nil, couponProblem("You have already used that coupon")
	}
	if c.firstOnly {
		var had bool
		if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM bookings WHERE user_id = $1
			AND status IN ('PAID_AWAITING_HOTEL', 'CONFIRMED', 'CHECKED_IN', 'COMPLETED', 'NO_SHOW', 'DISPUTED'))`, userID).Scan(&had); err != nil {
			return nil, err
		}
		if had {
			return nil, couponProblem("That coupon is for a first booking only")
		}
	}
	return &c, nil
}

// discount is how much comes off the room charges (before service and VAT).
func (c *couponRule) discount(base int64) (int64, error) {
	if base < c.minSubtotal {
		return 0, couponProblem("That coupon needs a larger booking")
	}
	var d int64
	if c.kind == "percent" {
		d = (base*c.value + 5000) / 10000
	} else {
		d = c.value
	}
	if c.maxDiscount != nil && d > *c.maxDiscount {
		d = *c.maxDiscount
	}
	if d > base {
		d = base
	}
	return d, nil
}

// ---- admin: coupons (platform admins only)

type couponIn struct {
	Code        string `json:"code"`
	Description string `json:"description"`
	Kind        string `json:"kind"`
	// Value is basis points for a percent coupon (1000 = 10%) and santim for a fixed one.
	Value       int64  `json:"value"`
	MaxDiscount *int64 `json:"max_discount_santim"`
	MinSubtotal int64  `json:"min_subtotal_santim"`
	StartsOn    string `json:"starts_on"` // YYYY-MM-DD, Addis Ababa; empty = no start
	EndsOn      string `json:"ends_on"`   // last day it works; empty = no end
	MaxUses     *int   `json:"max_uses"`
	PerUser     int    `json:"per_user_limit"`
	FirstOnly   bool   `json:"first_booking_only"`
	PropertyID  string `json:"property_id"` // empty = every hotel
	Active      bool   `json:"active"`
}

func (in *couponIn) validate(creating bool) error {
	bad := func(msg string) error { return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, msg) }
	in.Code = strings.ToUpper(strings.TrimSpace(in.Code))
	if creating && !couponCodeRE.MatchString(in.Code) {
		return bad("The code is 3 to 20 letters and digits")
	}
	if in.Kind != "percent" && in.Kind != "fixed" {
		return bad("Kind must be percent or fixed")
	}
	if in.Value <= 0 || (in.Kind == "percent" && in.Value > 10000) {
		return bad("A percent coupon is 1 to 10000 basis points (1000 = 10%); a fixed one is an amount in santim")
	}
	if in.MaxDiscount != nil && *in.MaxDiscount <= 0 {
		return bad("The maximum discount must be more than zero")
	}
	if in.MinSubtotal < 0 {
		return bad("The minimum booking cannot be negative")
	}
	if in.MaxUses != nil && *in.MaxUses <= 0 {
		return bad("Maximum uses must be more than zero")
	}
	if in.PerUser == 0 {
		in.PerUser = 1
	}
	if in.PerUser < 0 || in.PerUser > 100 {
		return bad("Uses per guest is 1 to 100")
	}
	for _, d := range []string{in.StartsOn, in.EndsOn} {
		if d != "" && !isDate(d) {
			return bad("Dates are YYYY-MM-DD")
		}
	}
	if in.StartsOn != "" && in.EndsOn != "" && in.EndsOn < in.StartsOn {
		return bad("The last day cannot be before the first day")
	}
	if len(in.Description) > 200 {
		return bad("The description is at most 200 characters")
	}
	in.Description = strings.TrimSpace(in.Description)
	return nil
}

type couponOut struct {
	ID          string  `json:"id"`
	Code        string  `json:"code"`
	Description string  `json:"description"`
	Kind        string  `json:"kind"`
	Value       int64   `json:"value"`
	MaxDiscount *int64  `json:"max_discount_santim"`
	MinSubtotal int64   `json:"min_subtotal_santim"`
	StartsOn    *string `json:"starts_on"`
	EndsOn      *string `json:"ends_on"`
	MaxUses     *int    `json:"max_uses"`
	PerUser     int     `json:"per_user_limit"`
	FirstOnly   bool    `json:"first_booking_only"`
	PropertyID  *string `json:"property_id"`
	Property    *string `json:"property_name"`
	Active      bool    `json:"active"`
	Uses        int     `json:"uses"`
	Discounted  int64   `json:"discounted_santim"`
}

func (s *Server) adminCoupons(w http.ResponseWriter, r *http.Request) error {
	if err := requireAdmin(r); err != nil {
		return err
	}
	rows, err := s.pool.Query(r.Context(), `
		SELECT c.id, c.code, c.description, c.kind, c.value, c.max_discount_santim, c.min_subtotal_santim,
		       to_char(c.starts_at AT TIME ZONE 'Africa/Addis_Ababa', 'YYYY-MM-DD'), to_char((c.ends_at - interval '1 day') AT TIME ZONE 'Africa/Addis_Ababa', 'YYYY-MM-DD'),
		       c.max_uses, c.per_user_limit, c.first_booking_only, c.property_id::text, p.name, c.active,
		       count(r.id) FILTER (WHERE b.status NOT IN ('EXPIRED', 'DECLINED_REFUNDED', 'CANCELLED_REFUNDED'))::int,
		       coalesce(sum(r.discount_santim) FILTER (WHERE b.status NOT IN ('EXPIRED', 'DECLINED_REFUNDED', 'CANCELLED_REFUNDED')), 0)::bigint
		FROM coupons c LEFT JOIN properties p ON p.id = c.property_id
		LEFT JOIN coupon_redemptions r ON r.coupon_id = c.id LEFT JOIN bookings b ON b.id = r.booking_id
		GROUP BY c.id, p.name ORDER BY c.created_at DESC`)
	if err != nil {
		return err
	}
	defer rows.Close()
	out := []couponOut{}
	for rows.Next() {
		var c couponOut
		if err := rows.Scan(&c.ID, &c.Code, &c.Description, &c.Kind, &c.Value, &c.MaxDiscount, &c.MinSubtotal, &c.StartsOn, &c.EndsOn,
			&c.MaxUses, &c.PerUser, &c.FirstOnly, &c.PropertyID, &c.Property, &c.Active, &c.Uses, &c.Discounted); err != nil {
			return err
		}
		out = append(out, c)
	}
	respond(w, http.StatusOK, struct {
		httpx.Meta
		Coupons []couponOut `json:"coupons"`
	}{httpx.NowMeta(), out})
	return rows.Err()
}

const couponWrite = `kind = $2, value = $3, max_discount_santim = $4, min_subtotal_santim = $5, description = $6,
	starts_at = CASE WHEN $7 = '' THEN NULL ELSE ($7::date)::timestamp AT TIME ZONE 'Africa/Addis_Ababa' END,
	ends_at = CASE WHEN $8 = '' THEN NULL ELSE (($8::date + 1))::timestamp AT TIME ZONE 'Africa/Addis_Ababa' END,
	max_uses = $9, per_user_limit = $10, first_booking_only = $11, property_id = NULLIF($12, '')::uuid, active = $13`

func (s *Server) adminCreateCoupon(w http.ResponseWriter, r *http.Request) error {
	if err := requireAdmin(r); err != nil {
		return err
	}
	var in couponIn
	if err := decode(r, &in); err != nil {
		return err
	}
	if err := in.validate(true); err != nil {
		return err
	}
	var id string
	err := s.tx(r.Context(), func(tx pgx.Tx) error {
		if err := tx.QueryRow(r.Context(), `INSERT INTO coupons (code, kind, value, max_discount_santim, min_subtotal_santim, description, starts_at, ends_at, max_uses, per_user_limit, first_booking_only, property_id, active)
			VALUES ($1, $2, $3, $4, $5, $6,
			  CASE WHEN $7 = '' THEN NULL ELSE ($7::date)::timestamp AT TIME ZONE 'Africa/Addis_Ababa' END,
			  CASE WHEN $8 = '' THEN NULL ELSE (($8::date + 1))::timestamp AT TIME ZONE 'Africa/Addis_Ababa' END,
			  $9, $10, $11, NULLIF($12, '')::uuid, $13) RETURNING id`,
			in.Code, in.Kind, in.Value, in.MaxDiscount, in.MinSubtotal, in.Description, in.StartsOn, in.EndsOn, in.MaxUses, in.PerUser, in.FirstOnly, in.PropertyID, in.Active).Scan(&id); err != nil {
			if strings.Contains(err.Error(), "coupons_code_key") {
				return fail(http.StatusConflict, codeInvalidState, "That code already exists")
			}
			if isBadUUID(err) {
				return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "Unknown hotel")
			}
			return err
		}
		return s.audit(r, tx, "admin_coupon_created", map[string]any{"coupon_id": id, "code": in.Code})
	})
	if err != nil {
		return err
	}
	respond(w, http.StatusOK, struct {
		httpx.Meta
		ID string `json:"id"`
	}{httpx.NowMeta(), id})
	return nil
}

// adminUpdateCoupon changes a coupon's rules. The code itself never changes, because guests and redemptions refer to it.
func (s *Server) adminUpdateCoupon(w http.ResponseWriter, r *http.Request) error {
	if err := requireAdmin(r); err != nil {
		return err
	}
	id := chi.URLParam(r, "id")
	var in couponIn
	if err := decode(r, &in); err != nil {
		return err
	}
	if err := in.validate(false); err != nil {
		return err
	}
	err := s.tx(r.Context(), func(tx pgx.Tx) error {
		tag, err := tx.Exec(r.Context(), `UPDATE coupons SET `+couponWrite+` WHERE id = $1`,
			id, in.Kind, in.Value, in.MaxDiscount, in.MinSubtotal, in.Description, in.StartsOn, in.EndsOn, in.MaxUses, in.PerUser, in.FirstOnly, in.PropertyID, in.Active)
		if isBadUUID(err) {
			return errNotFound
		}
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return errNotFound
		}
		return s.audit(r, tx, "admin_coupon_updated", map[string]any{"coupon_id": id, "active": in.Active})
	})
	if err != nil {
		return err
	}
	respond(w, http.StatusOK, httpx.NowMeta())
	return nil
}
