package api

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/bookdan/backend/internal/httpx"
)

type hotelSummary struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	City   string `json:"city"`
	Type   string `json:"type"`
	Status string `json:"status"`
	Rooms  int    `json:"room_types"`
	Photos int    `json:"photos"`
}

func (s *Server) adminListHotels(w http.ResponseWriter, r *http.Request) error {
	c := claimsFrom(r)
	rows, err := s.pool.Query(r.Context(), `
		SELECT p.id, p.name, p.city, p.type, p.status,
		       (SELECT count(*) FROM room_types rt WHERE rt.property_id = p.id)::int,
		       (SELECT count(*) FROM property_photos pp WHERE pp.property_id = p.id AND pp.room_type_id IS NULL)::int
		FROM properties p
		WHERE $1 = 'admin' OR EXISTS (SELECT 1 FROM property_staff ps WHERE ps.property_id = p.id AND ps.user_id = $2::uuid AND ps.role = 'owner')
		ORDER BY p.name`, c.Role, c.Sub)
	if err != nil {
		return err
	}
	defer rows.Close()
	items := []hotelSummary{}
	for rows.Next() {
		var h hotelSummary
		if err := rows.Scan(&h.ID, &h.Name, &h.City, &h.Type, &h.Status, &h.Rooms, &h.Photos); err != nil {
			return err
		}
		items = append(items, h)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	respond(w, http.StatusOK, struct {
		httpx.Meta
		Items []hotelSummary `json:"items"`
	}{httpx.NowMeta(), items})
	return nil
}

// ---- hotel details

type hotelInput struct {
	// ConfirmHide says the admin has seen the bookings still to come and wants the hotel taken off the list anyway.
	ConfirmHide bool     `json:"confirm_hide"`
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	City        string   `json:"city"`
	Area        string   `json:"area"`
	Address     string   `json:"address"`
	Phone       string   `json:"phone"`
	Description string   `json:"description"`
	Lat         *float64 `json:"lat"`
	Lng         *float64 `json:"lng"`
	Amenities   []string `json:"amenities"`
	HouseRules  []string `json:"house_rules"`
	PayAtHotel  bool     `json:"pay_at_hotel"`
	Verified    *bool    `json:"verified"`
	Status      string   `json:"status"`
	// CheckInTime and CheckOutTime are HH:MM (Addis Ababa time). Empty keeps the current ones (14:00 and 12:00 for a new hotel).
	// The description in Amharic, and how to find the hotel in words (English and Amharic).
	DescriptionAm string `json:"description_am"`
	Directions    string `json:"directions"`
	DirectionsAm  string `json:"directions_am"`
	CheckInTime   string `json:"check_in_time"`
	CheckOutTime  string `json:"check_out_time"`
}

func trimLen(v string, min, max int) (string, bool) {
	v = strings.TrimSpace(v)
	return v, len(v) >= min && len(v) <= max
}

// validate checks and cleans a hotel form. Returned strings are safe to store.
func (in *hotelInput) validate(creating bool) error {
	bad := func(msg string) error { return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, msg) }
	var ok bool
	if in.Name, ok = trimLen(in.Name, 2, 120); !ok {
		return bad("The name must be 2 to 120 characters")
	}
	if !hotelTypes[in.Type] {
		return bad("Type must be hotel, guesthouse, lodge or resort")
	}
	if in.City, ok = trimLen(in.City, 2, 80); !ok {
		return bad("City is required")
	}
	if in.Area, ok = trimLen(in.Area, 2, 120); !ok {
		return bad("Area is required")
	}
	if in.Address, ok = trimLen(in.Address, 2, 200); !ok {
		return bad("Address is required")
	}
	in.Phone = strings.TrimSpace(in.Phone)
	if !phoneAnyRE.MatchString(in.Phone) {
		return bad("Enter a phone number")
	}
	if in.Description, ok = trimLen(in.Description, 0, 2000); !ok {
		return bad("The description is at most 2000 characters")
	}
	if in.Lat == nil || in.Lng == nil || *in.Lat < -90 || *in.Lat > 90 || *in.Lng < -180 || *in.Lng > 180 {
		return bad("Latitude and longitude are required (for example 9.0108 and 38.7613)")
	}
	seen := map[string]bool{}
	clean := []string{}
	for _, a := range in.Amenities {
		if !amenityList[a] {
			return bad("Unknown amenity: " + a)
		}
		if !seen[a] {
			seen[a] = true
			clean = append(clean, a)
		}
	}
	in.Amenities = clean
	if len(in.HouseRules) > 12 {
		return bad("At most 12 house rules")
	}
	rules := []string{}
	for _, v := range in.HouseRules {
		if v = strings.TrimSpace(v); v != "" {
			if len(v) > 200 {
				return bad("A house rule is at most 200 characters")
			}
			rules = append(rules, v)
		}
	}
	in.HouseRules = rules
	in.CheckInTime, in.CheckOutTime = strings.TrimSpace(in.CheckInTime), strings.TrimSpace(in.CheckOutTime)
	in.DescriptionAm, in.Directions, in.DirectionsAm = strings.TrimSpace(in.DescriptionAm), strings.TrimSpace(in.Directions), strings.TrimSpace(in.DirectionsAm)
	if len(in.DescriptionAm) > 2000 || len(in.Directions) > 600 || len(in.DirectionsAm) > 600 {
		return bad("The Amharic description is at most 2000 characters; directions at most 600")
	}
	for _, v := range []string{in.CheckInTime, in.CheckOutTime} {
		if v != "" && !shiftRE.MatchString(v) {
			return bad("Check-in and check-out times are HH:MM, for example 14:00")
		}
	}
	if in.Status == "" {
		in.Status = "draft"
	}
	if !hotelStatuses[in.Status] || (creating && in.Status != "draft") {
		return bad("Status must be draft, active or hidden")
	}
	return nil
}

// pointSQL builds the location value from two bound parameters (longitude first).
func (s *Server) pointSQL(lng, lat string) string {
	if s.postgis {
		return "ST_SetSRID(ST_MakePoint(" + lng + "::float8, " + lat + "::float8), 4326)::geography"
	}
	return "'SRID=4326;POINT(' || " + lng + "::float8::text || ' ' || " + lat + "::float8::text || ')'"
}

type roomOut struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Capacity    int      `json:"capacity"`
	TotalRooms  int      `json:"total_rooms"`
	Amenities   []string `json:"amenities"`
	PriceSantim *int64   `json:"price_santim"`
	Currency    string   `json:"currency"`
	// What breakfast adds per night, and the non-refundable discount (basis points); empty when not offered.
	BreakfastSantim  *int64 `json:"breakfast_santim"`
	NonRefundableBPS *int   `json:"nonrefundable_bps"`
	// The room's own photos, cover first.
	Photos []photoOut `json:"photos"`
}

type photoOut struct {
	ID    string `json:"id"`
	Main  string `json:"main"`
	Thumb string `json:"thumb"`
	Cover bool   `json:"cover"`
}

type staffOut struct {
	UserID  string `json:"user_id"`
	Contact string `json:"contact"`
	Role    string `json:"role"`
	// Status is "invited" until the person first signs in, then "joined".
	Status    string  `json:"status"`
	InvitedAt string  `json:"invited_at"`
	JoinedAt  *string `json:"joined_at"`
	// Alerts: switched on or off, the shift (HH:MM in Addis Ababa time, both null = always), and whether any phone is signed in to receive them.
	AlertsOn   bool    `json:"alerts_on"`
	ShiftStart *string `json:"shift_start"`
	ShiftEnd   *string `json:"shift_end"`
	Reachable  bool    `json:"reachable"`
}

type businessOut struct {
	LegalName   string  `json:"legal_name"`
	License     string  `json:"license_number"`
	TIN         *string `json:"tin"`
	ContactName string  `json:"contact_name"`
	ContactTel  string  `json:"contact_phone"`
	ContactMail string  `json:"contact_email"`
	Method      string  `json:"payout_method"`
	AccountName string  `json:"payout_account_name"`
	// Last4 is all the page shows of the payout account; the full number is revealed on request (and logged).
	Last4 string `json:"payout_last4"`
}

func (s *Server) photoURL(id, size string) string {
	return s.cfg.PublicURL + "/v1/photos/" + id + "/" + size
}

func (s *Server) adminGetHotel(w http.ResponseWriter, r *http.Request) error {
	id := chi.URLParam(r, "id")
	if err := s.canManage(r, id); err != nil {
		return err
	}
	ctx := r.Context()
	lat, lng := s.latLng("location")
	var h struct {
		httpx.Meta
		ID            string       `json:"id"`
		Name          string       `json:"name"`
		Type          string       `json:"type"`
		City          string       `json:"city"`
		Area          string       `json:"area"`
		Address       string       `json:"address"`
		Phone         string       `json:"phone"`
		Description   string       `json:"description"`
		Lat           float64      `json:"lat"`
		Lng           float64      `json:"lng"`
		Amenities     []string     `json:"amenities"`
		HouseRules    any          `json:"house_rules"`
		PayAtHotel    bool         `json:"pay_at_hotel"`
		Verified      bool         `json:"verified"`
		Status        string       `json:"status"`
		DescriptionAm string       `json:"description_am"`
		Directions    string       `json:"directions"`
		DirectionsAm  string       `json:"directions_am"`
		CheckIn       string       `json:"check_in_time"`
		CheckOut      string       `json:"check_out_time"`
		AlertMode     string       `json:"alert_mode"`
		EscalateMin   int          `json:"escalate_after_min"`
		Rooms         []roomOut    `json:"rooms"`
		Photos        []photoOut   `json:"photos"`
		Staff         []staffOut   `json:"staff"`
		Business      *businessOut `json:"business"`
	}
	var rules []byte
	if err := s.pool.QueryRow(ctx, `SELECT id, name, type, city, area, address, phone, description, `+lat+`, `+lng+`, amenities, house_rules, pay_at_hotel, verified, status, alert_mode, escalate_after_min,
		to_char(check_in_time, 'HH24:MI'), to_char(check_out_time, 'HH24:MI'), description_am, directions, directions_am
		FROM properties WHERE id = $1`, id).Scan(&h.ID, &h.Name, &h.Type, &h.City, &h.Area, &h.Address, &h.Phone, &h.Description, &h.Lat, &h.Lng,
		&h.Amenities, &rules, &h.PayAtHotel, &h.Verified, &h.Status, &h.AlertMode, &h.EscalateMin, &h.CheckIn, &h.CheckOut, &h.DescriptionAm, &h.Directions, &h.DirectionsAm); err != nil {
		return err
	}
	h.HouseRules = []string{}
	if len(rules) > 0 {
		h.HouseRules = json.RawMessage(rules)
	}
	if h.Amenities == nil {
		h.Amenities = []string{}
	}

	rr, err := s.pool.Query(ctx, `SELECT rt.id, rt.name, rt.capacity, rt.total_rooms, rt.amenities, rp.nightly_price_santim, coalesce(rp.currency, 'ETB'), rt.breakfast_santim, rt.nonrefundable_bps
		FROM room_types rt LEFT JOIN LATERAL (SELECT nightly_price_santim, currency FROM rate_plans
		  WHERE room_type_id = rt.id AND (now() AT TIME ZONE 'Africa/Addis_Ababa')::date BETWEEN valid_from AND valid_to LIMIT 1) rp ON true
		WHERE rt.property_id = $1 ORDER BY rt.name`, id)
	if err != nil {
		return err
	}
	defer rr.Close()
	h.Rooms = []roomOut{}
	for rr.Next() {
		var ro roomOut
		if err := rr.Scan(&ro.ID, &ro.Name, &ro.Capacity, &ro.TotalRooms, &ro.Amenities, &ro.PriceSantim, &ro.Currency, &ro.BreakfastSantim, &ro.NonRefundableBPS); err != nil {
			return err
		}
		if ro.Amenities == nil {
			ro.Amenities = []string{}
		}
		ro.Photos = []photoOut{}
		h.Rooms = append(h.Rooms, ro)
	}
	rr.Close()
	roomPics, err := s.pool.Query(ctx, `SELECT room_type_id::text, id FROM property_photos WHERE property_id = $1 AND room_type_id IS NOT NULL ORDER BY position, created_at`, id)
	if err != nil {
		return err
	}
	defer roomPics.Close()
	for roomPics.Next() {
		var room, photo string
		if err := roomPics.Scan(&room, &photo); err != nil {
			return err
		}
		for i := range h.Rooms {
			if h.Rooms[i].ID == room {
				h.Rooms[i].Photos = append(h.Rooms[i].Photos, photoOut{photo, s.photoURL(photo, "main"), s.photoURL(photo, "thumb"), len(h.Rooms[i].Photos) == 0})
			}
		}
	}
	roomPics.Close()

	pr, err := s.pool.Query(ctx, `SELECT id FROM property_photos WHERE property_id = $1 AND room_type_id IS NULL ORDER BY position, created_at`, id)
	if err != nil {
		return err
	}
	defer pr.Close()
	h.Photos = []photoOut{}
	for pr.Next() {
		var pid string
		if err := pr.Scan(&pid); err != nil {
			return err
		}
		h.Photos = append(h.Photos, photoOut{pid, s.photoURL(pid, "main"), s.photoURL(pid, "thumb"), len(h.Photos) == 0})
	}
	pr.Close()

	sr, err := s.pool.Query(ctx, `SELECT u.id, coalesce(u.email, u.phone), ps.role, ps.invited_at, ps.joined_at, ps.alerts_on,
			to_char(ps.shift_start, 'HH24:MI'), to_char(ps.shift_end, 'HH24:MI'),
			EXISTS (SELECT 1 FROM devices d WHERE d.user_id = u.id AND d.app = 'partner' AND d.push_token IS NOT NULL AND d.revoked_at IS NULL)
		FROM property_staff ps JOIN users u ON u.id = ps.user_id
		WHERE ps.property_id = $1 ORDER BY ps.role DESC, ps.created_at`, id)
	if err != nil {
		return err
	}
	defer sr.Close()
	h.Staff = []staffOut{}
	for sr.Next() {
		var so staffOut
		var invited time.Time
		var joined *time.Time
		if err := sr.Scan(&so.UserID, &so.Contact, &so.Role, &invited, &joined, &so.AlertsOn, &so.ShiftStart, &so.ShiftEnd, &so.Reachable); err != nil {
			return err
		}
		so.InvitedAt, so.Status = invited.UTC().Format(time.RFC3339), "invited"
		if joined != nil {
			t := joined.UTC().Format(time.RFC3339)
			so.JoinedAt, so.Status = &t, "joined"
		}
		h.Staff = append(h.Staff, so)
	}
	sr.Close()

	// The business record is for platform admins only (it holds a payout account).
	if claimsFrom(r).Role == "admin" {
		var bz businessOut
		err := s.pool.QueryRow(ctx, `SELECT legal_name, license_number, tin, contact_name, contact_phone, contact_email, payout_method, payout_account_name, payout_last4
			FROM hotel_business WHERE property_id = $1`, id).Scan(&bz.LegalName, &bz.License, &bz.TIN, &bz.ContactName, &bz.ContactTel, &bz.ContactMail, &bz.Method, &bz.AccountName, &bz.Last4)
		if err == nil {
			h.Business = &bz
		} else if err != pgx.ErrNoRows {
			return err
		}
	}
	h.Meta = httpx.NowMeta()
	respond(w, http.StatusOK, h)
	return nil
}

func (s *Server) adminCreateHotel(w http.ResponseWriter, r *http.Request) error {
	if err := requireAdmin(r); err != nil {
		return err
	}
	var in hotelInput
	if err := decode(r, &in); err != nil {
		return err
	}
	if err := in.validate(true); err != nil {
		return err
	}
	rules, _ := json.Marshal(in.HouseRules)
	var id string
	err := s.tx(r.Context(), func(tx pgx.Tx) error {
		if err := tx.QueryRow(r.Context(), `INSERT INTO properties (name, type, city, area, address, location, phone, description, pay_at_hotel, amenities, house_rules, status, verified, check_in_time, check_out_time)
			VALUES ($1, $2, $3, $4, $5, `+s.pointSQL("$6", "$7")+`, $8, $9, $10, $11, $12::jsonb, 'draft', coalesce($13, false), coalesce(nullif($14, '')::time, '14:00'), coalesce(nullif($15, '')::time, '12:00')) RETURNING id`,
			in.Name, in.Type, in.City, in.Area, in.Address, *in.Lng, *in.Lat, in.Phone, in.Description, in.PayAtHotel, in.Amenities, string(rules), in.Verified, in.CheckInTime, in.CheckOutTime).Scan(&id); err != nil {
			return err
		}
		if _, err := tx.Exec(r.Context(), `UPDATE properties SET description_am = $2, directions = $3, directions_am = $4 WHERE id = $1`, id, in.DescriptionAm, in.Directions, in.DirectionsAm); err != nil {
			return err
		}
		return s.adminAudit(r, tx, "admin_hotel_created", id, map[string]any{"name": in.Name})
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

// warnUpcomingBookings refuses to take a listed hotel off the list while guests have bookings that are not finished, unless the admin
// confirms. The bookings stay valid either way; the refusal names them so the admin can call the hotel first.
func (s *Server) warnUpcomingBookings(ctx context.Context, tx pgx.Tx, id string) error {
	var current string
	if err := tx.QueryRow(ctx, `SELECT status FROM properties WHERE id = $1`, id).Scan(&current); err != nil {
		return err
	}
	if current != "active" {
		return nil
	}
	rows, err := tx.Query(ctx, `SELECT code FROM bookings WHERE property_id = $1 AND status IN (`+activeBookingStatuses+`)
		AND check_out >= `+"(now() AT TIME ZONE 'Africa/Addis_Ababa')::date"+` ORDER BY check_in LIMIT 6`, id)
	if err != nil {
		return err
	}
	var codes []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			rows.Close()
			return err
		}
		codes = append(codes, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil || len(codes) == 0 {
		return err
	}
	list := strings.Join(codes, ", ")
	if len(codes) == 6 {
		list = strings.Join(codes[:5], ", ") + " and more"
	}
	return fail(http.StatusConflict, "HAS_UPCOMING_BOOKINGS", "This hotel has bookings that are not finished ("+list+"). Guests keep their bookings if you take it off the list; tell the hotel first.")
}

func (s *Server) adminUpdateHotel(w http.ResponseWriter, r *http.Request) error {
	id := chi.URLParam(r, "id")
	if err := s.canManage(r, id); err != nil {
		return err
	}
	var in hotelInput
	if err := decode(r, &in); err != nil {
		return err
	}
	if err := in.validate(false); err != nil {
		return err
	}
	isAdmin := claimsFrom(r).Role == "admin"
	rules, _ := json.Marshal(in.HouseRules)
	err := s.tx(r.Context(), func(tx pgx.Tx) error {
		if !isAdmin {
			// Whether a hotel is visible to guests is BookDan's decision: an owner edits the hotel without changing its status.
			if err := tx.QueryRow(r.Context(), `SELECT status FROM properties WHERE id = $1`, id).Scan(&in.Status); err != nil {
				return err
			}
		}
		if in.Status == "active" {
			var current string
			var hasBusiness bool
			if err := tx.QueryRow(r.Context(), `SELECT status, EXISTS (SELECT 1 FROM hotel_business WHERE property_id = $1) FROM properties WHERE id = $1`, id).Scan(&current, &hasBusiness); err != nil {
				return err
			}
			if current != "active" && !hasBusiness {
				return fail(http.StatusConflict, codeInvalidState, "Add the business and payout details before publishing this hotel")
			}
			var ready bool
			if err := tx.QueryRow(r.Context(), `SELECT EXISTS (SELECT 1 FROM room_types rt JOIN rate_plans rp ON rp.room_type_id = rt.id
				WHERE rt.property_id = $1 AND rt.total_rooms > 0 AND (now() AT TIME ZONE 'Africa/Addis_Ababa')::date BETWEEN rp.valid_from AND rp.valid_to)`, id).Scan(&ready); err != nil {
				return err
			}
			if !ready {
				return fail(http.StatusConflict, codeInvalidState, "Add a room with a price before publishing this hotel")
			}
		}
		if in.Status != "active" && !in.ConfirmHide {
			if err := s.warnUpcomingBookings(r.Context(), tx, id); err != nil {
				return err
			}
		}
		// Only a platform admin can mark a hotel verified.
		verified := in.Verified
		if !isAdmin {
			verified = nil
		}
		if _, err := tx.Exec(r.Context(), `UPDATE properties SET name = $2, type = $3, city = $4, area = $5, address = $6, location = `+s.pointSQL("$7", "$8")+`,
			phone = $9, description = $10, pay_at_hotel = $11, amenities = $12, house_rules = $13::jsonb, status = $14, verified = coalesce($15, verified),
			check_in_time = coalesce(nullif($16, '')::time, check_in_time), check_out_time = coalesce(nullif($17, '')::time, check_out_time)
			WHERE id = $1`, id, in.Name, in.Type, in.City, in.Area, in.Address, *in.Lng, *in.Lat, in.Phone, in.Description, in.PayAtHotel, in.Amenities, string(rules), in.Status, verified, in.CheckInTime, in.CheckOutTime); err != nil {
			return err
		}
		if _, err := tx.Exec(r.Context(), `UPDATE properties SET description_am = $2, directions = $3, directions_am = $4 WHERE id = $1`, id, in.DescriptionAm, in.Directions, in.DirectionsAm); err != nil {
			return err
		}
		return s.adminAudit(r, tx, "admin_hotel_updated", id, map[string]any{"status": in.Status, "by": claimsFrom(r).Role, "name": in.Name})
	})
	if err != nil {
		return err
	}
	respond(w, http.StatusOK, httpx.NowMeta())
	return nil
}

// ---- rooms and prices

type roomInput struct {
	Name       string   `json:"name"`
	Capacity   int      `json:"capacity"`
	TotalRooms int      `json:"total_rooms"`
	Amenities  []string `json:"amenities"`
	// Optional extras: what breakfast adds per room per night (santim), and how much cheaper a non-refundable booking is (basis points: 500 = 5%).
	BreakfastSantim  *int64 `json:"breakfast_santim"`
	NonRefundableBPS *int   `json:"nonrefundable_bps"`
}

func (in *roomInput) validate() error {
	bad := func(msg string) error { return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, msg) }
	var ok bool
	if in.Name, ok = trimLen(in.Name, 2, 80); !ok {
		return bad("The room name must be 2 to 80 characters")
	}
	if in.Capacity < 1 || in.Capacity > 20 {
		return bad("Guests per room must be between 1 and 20")
	}
	if in.TotalRooms < 0 || in.TotalRooms > 500 {
		return bad("Number of rooms must be between 0 and 500")
	}
	clean := []string{}
	for _, a := range in.Amenities {
		if a = strings.TrimSpace(a); a != "" {
			if len(a) > 40 || len(clean) >= 12 {
				return bad("Room features: at most 12, each up to 40 characters")
			}
			clean = append(clean, a)
		}
	}
	in.Amenities = clean
	if in.BreakfastSantim != nil && (*in.BreakfastSantim <= 0 || *in.BreakfastSantim > 100_000_000) {
		return bad("Breakfast must cost more than zero (or leave it empty if you do not offer it)")
	}
	if in.NonRefundableBPS != nil && (*in.NonRefundableBPS < 100 || *in.NonRefundableBPS > 5000) {
		return bad("The non-refundable discount is 1% to 50% (or leave it empty if you do not offer it)")
	}
	return nil
}

func (s *Server) adminCreateRoom(w http.ResponseWriter, r *http.Request) error {
	id := chi.URLParam(r, "id")
	if err := s.canManage(r, id); err != nil {
		return err
	}
	var in roomInput
	if err := decode(r, &in); err != nil {
		return err
	}
	if err := in.validate(); err != nil {
		return err
	}
	var roomID string
	err := s.tx(r.Context(), func(tx pgx.Tx) error {
		if err := tx.QueryRow(r.Context(), `INSERT INTO room_types (property_id, name, capacity, total_rooms, amenities, breakfast_santim, nonrefundable_bps) VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
			id, in.Name, in.Capacity, in.TotalRooms, in.Amenities, in.BreakfastSantim, in.NonRefundableBPS).Scan(&roomID); err != nil {
			return err
		}
		return s.adminAudit(r, tx, "admin_room_created", id, map[string]any{"room_type_id": roomID, "name": in.Name})
	})
	if isUnique(err) {
		return fail(http.StatusConflict, codeInvalidState, "This hotel already has a room with that name")
	}
	if err != nil {
		return err
	}
	respond(w, http.StatusOK, struct {
		httpx.Meta
		ID string `json:"id"`
	}{httpx.NowMeta(), roomID})
	return nil
}

func (s *Server) adminUpdateRoom(w http.ResponseWriter, r *http.Request) error {
	id, room := chi.URLParam(r, "id"), chi.URLParam(r, "room")
	if err := s.canManage(r, id); err != nil {
		return err
	}
	var in roomInput
	if err := decode(r, &in); err != nil {
		return err
	}
	if err := in.validate(); err != nil {
		return err
	}
	err := s.tx(r.Context(), func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(r.Context(), `SELECT EXISTS (SELECT 1 FROM room_types WHERE id = $1 AND property_id = $2)`, room, id).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return errNotFound
		}
		// You cannot shrink below what is already held or sold on any coming night.
		var busiest int
		if err := tx.QueryRow(r.Context(), `SELECT coalesce(max(held + booked), 0) FROM inventory WHERE room_type_id = $1 AND night >= $2::date`, room, s.addisToday()).Scan(&busiest); err != nil {
			return err
		}
		if in.TotalRooms < busiest {
			return fail(http.StatusConflict, codeInvalidState, "Up to "+itoa(int64(busiest))+" of these rooms are already held or booked on a coming night, so you cannot go below that")
		}
		if _, err := tx.Exec(r.Context(), `UPDATE room_types SET name = $2, capacity = $3, total_rooms = $4, amenities = $5, breakfast_santim = $6, nonrefundable_bps = $7 WHERE id = $1`,
			room, in.Name, in.Capacity, in.TotalRooms, in.Amenities, in.BreakfastSantim, in.NonRefundableBPS); err != nil {
			return err
		}
		// Coming nights that already exist follow the new number of rooms; later nights are created from it on demand.
		if _, err := tx.Exec(r.Context(), `UPDATE inventory SET capacity = $2 WHERE room_type_id = $1 AND night >= $3::date`, room, in.TotalRooms, s.addisToday()); err != nil {
			return err
		}
		return s.adminAudit(r, tx, "admin_room_updated", id, map[string]any{"room_type_id": room})
	})
	if isUnique(err) {
		return fail(http.StatusConflict, codeInvalidState, "This hotel already has a room with that name")
	}
	if err != nil {
		return err
	}
	respond(w, http.StatusOK, httpx.NowMeta())
	return nil
}

// adminSetPrice sets one nightly price for the next two years. (Seasonal prices come later.) Existing bookings keep the price
// they were sold at.
func (s *Server) adminSetPrice(w http.ResponseWriter, r *http.Request) error {
	id, room := chi.URLParam(r, "id"), chi.URLParam(r, "room")
	if err := s.canManage(r, id); err != nil {
		return err
	}
	var in struct {
		PriceSantim int64 `json:"price_santim"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if in.PriceSantim < 10000 || in.PriceSantim > 100_000_000 || in.PriceSantim%100 != 0 {
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "The price must be between 100 and 1,000,000 ETB, in whole birr")
	}
	err := s.tx(r.Context(), func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(r.Context(), `SELECT EXISTS (SELECT 1 FROM room_types WHERE id = $1 AND property_id = $2)`, room, id).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return errNotFound
		}
		if _, err := tx.Exec(r.Context(), `DELETE FROM rate_plans WHERE room_type_id = $1`, room); err != nil {
			return err
		}
		if _, err := tx.Exec(r.Context(), `INSERT INTO rate_plans (room_type_id, currency, nightly_price_santim, valid_from, valid_to)
			VALUES ($1, 'ETB', $2, $3::date, $3::date + 730)`, room, in.PriceSantim, s.addisToday()); err != nil {
			return err
		}
		return s.adminAudit(r, tx, "admin_price_set", id, map[string]any{"room_type_id": room, "price_santim": in.PriceSantim})
	})
	if err != nil {
		return err
	}
	respond(w, http.StatusOK, httpx.NowMeta())
	return nil
}

// ---- staff

// A platform admin adds and removes anyone. A hotel owner adds and removes only the receptionists of their own hotel: owners are
// always chosen by the platform, so a hotel can never hand ownership (and the admin page) to someone BookDan has not approved.

func (s *Server) adminAddStaff(w http.ResponseWriter, r *http.Request) error {
	id := chi.URLParam(r, "id")
	if err := s.canManage(r, id); err != nil {
		return err
	}
	var in struct {
		Contact string `json:"contact"`
		Role    string `json:"role"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	contact := strings.TrimSpace(in.Contact)
	typ := "phone"
	if strings.Contains(contact, "@") {
		typ = "email"
	}
	ident, _, err := identity{Type: typ, Value: contact}.clean()
	if err != nil {
		return err
	}
	if in.Role != "owner" && in.Role != "receptionist" {
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "Role must be owner or receptionist")
	}
	isAdmin := claimsFrom(r).Role == "admin"
	if !isAdmin && in.Role != "receptionist" {
		return fail(http.StatusForbidden, codeForbidden, "Only a platform admin can add an owner")
	}
	col := "phone"
	if ident.Type == "email" {
		col = "email"
	}
	var uid, hotel string
	err = s.tx(r.Context(), func(tx pgx.Tx) error {
		if err := tx.QueryRow(r.Context(), `SELECT name FROM properties WHERE id = $1`, id).Scan(&hotel); err != nil {
			return err
		}
		var role string
		err := tx.QueryRow(r.Context(), `SELECT id, role FROM users WHERE `+col+` = $1`, ident.Value).Scan(&uid, &role)
		switch {
		case err == pgx.ErrNoRows:
			if err := tx.QueryRow(r.Context(), `INSERT INTO users (`+col+`, role) VALUES ($1, $2) RETURNING id`, ident.Value, in.Role).Scan(&uid); err != nil {
				return err
			}
		case err != nil:
			return err
		case role == "admin":
			return fail(http.StatusConflict, codeInvalidState, "That person is a platform admin already")
		}
		// Whoever is already on this hotel keeps their role unless a platform admin changes it.
		var existing string
		err = tx.QueryRow(r.Context(), `SELECT role FROM property_staff WHERE property_id = $1 AND user_id = $2`, id, uid).Scan(&existing)
		if err != nil && err != pgx.ErrNoRows {
			return err
		}
		if err == nil && !isAdmin && existing == "owner" {
			return fail(http.StatusForbidden, codeForbidden, "That person is an owner of this hotel")
		}
		// A guest account that becomes hotel staff; an owner somewhere stays an owner.
		if role == "guest" {
			if _, err := tx.Exec(r.Context(), `UPDATE users SET role = $2 WHERE id = $1`, uid, in.Role); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(r.Context(), `INSERT INTO property_staff (property_id, user_id, role) VALUES ($1, $2, $3)
			ON CONFLICT (property_id, user_id) DO UPDATE SET role = EXCLUDED.role`, id, uid, in.Role); err != nil {
			return err
		}
		return s.adminAudit(r, tx, "admin_staff_added", id, map[string]any{"user_id": uid, "role": in.Role})
	})
	if err != nil {
		return err
	}
	// Tell them (an email address only; a phone number has no inbox).
	if ident.Type == "email" {
		s.sendMail(r.Context(), ident.Value, s.staffInvitedEmail(in.Role, hotel, ident.Value))
	}
	respond(w, http.StatusOK, struct {
		httpx.Meta
		UserID string `json:"user_id"`
	}{httpx.NowMeta(), uid})
	return nil
}

func (s *Server) adminRemoveStaff(w http.ResponseWriter, r *http.Request) error {
	id, user := chi.URLParam(r, "id"), chi.URLParam(r, "user")
	if err := s.canManage(r, id); err != nil {
		return err
	}
	isAdmin := claimsFrom(r).Role == "admin"
	err := s.tx(r.Context(), func(tx pgx.Tx) error {
		var role string
		err := tx.QueryRow(r.Context(), `SELECT role FROM property_staff WHERE property_id = $1 AND user_id = $2 FOR UPDATE`, id, user).Scan(&role)
		if err == pgx.ErrNoRows || isBadUUID(err) {
			return errNotFound
		}
		if err != nil {
			return err
		}
		if !isAdmin && role == "owner" {
			return fail(http.StatusForbidden, codeForbidden, "Only a platform admin can remove an owner")
		}
		if _, err := tx.Exec(r.Context(), `DELETE FROM property_staff WHERE property_id = $1 AND user_id = $2`, id, user); err != nil {
			return err
		}
		// They stop receiving this hotel's alerts at once (their phone stays registered for any other hotel they work for).
		return s.adminAudit(r, tx, "admin_staff_removed", id, map[string]any{"user_id": user, "role": role})
	})
	if err != nil {
		return err
	}
	respond(w, http.StatusOK, httpx.NowMeta())
	return nil
}

// ---- the business behind a hotel (platform admin only)

type businessIn struct {
	LegalName   string `json:"legal_name"`
	License     string `json:"license_number"`
	TIN         string `json:"tin"`
	ContactName string `json:"contact_name"`
	ContactTel  string `json:"contact_phone"`
	ContactMail string `json:"contact_email"`
	Method      string `json:"payout_method"`
	AccountName string `json:"payout_account_name"`
	// AccountNumber is where the hotel is paid. Leave it empty when editing to keep the number already saved.
	AccountNumber string `json:"payout_account_number"`
}

var payoutMethods = map[string]bool{"telebirr": true, "cbe_birr": true, "bank_transfer": true}
var accountNumberRE = regexp.MustCompile(`^[0-9A-Za-z+ -]{4,40}$`)

func (s *Server) adminSetBusiness(w http.ResponseWriter, r *http.Request) error {
	if err := requireAdmin(r); err != nil {
		return err
	}
	id := chi.URLParam(r, "id")
	if err := s.canManage(r, id); err != nil {
		return err
	}
	var in businessIn
	if err := decode(r, &in); err != nil {
		return err
	}
	bad := func(msg string) error { return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, msg) }
	var ok bool
	if in.LegalName, ok = trimLen(in.LegalName, 2, 150); !ok {
		return bad("Legal name of the business is required")
	}
	if in.License, ok = trimLen(in.License, 2, 60); !ok {
		return bad("The trade licence number is required")
	}
	if in.TIN, ok = trimLen(in.TIN, 0, 30); !ok {
		return bad("The tax number is too long")
	}
	if in.ContactName, ok = trimLen(in.ContactName, 2, 80); !ok {
		return bad("A contact person is required")
	}
	if !phoneAnyRE.MatchString(strings.TrimSpace(in.ContactTel)) {
		return bad("Enter the contact person's phone number")
	}
	in.ContactTel = strings.TrimSpace(in.ContactTel)
	in.ContactMail = strings.ToLower(strings.TrimSpace(in.ContactMail))
	if _, _, err := (identity{Type: "email", Value: in.ContactMail}).clean(); err != nil {
		return bad("Enter the contact person's email address")
	}
	if !payoutMethods[in.Method] {
		return bad("Payout method must be telebirr, cbe_birr or bank_transfer")
	}
	if in.AccountName, ok = trimLen(in.AccountName, 2, 100); !ok {
		return bad("The name on the payout account is required")
	}
	number := strings.TrimSpace(in.AccountNumber)
	if number != "" && !accountNumberRE.MatchString(number) {
		return bad("The payout account number looks wrong")
	}
	err := s.tx(r.Context(), func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(r.Context(), `SELECT EXISTS (SELECT 1 FROM hotel_business WHERE property_id = $1)`, id).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			if number == "" {
				return bad("The payout account number is required")
			}
			if _, err := tx.Exec(r.Context(), `INSERT INTO hotel_business (property_id, legal_name, license_number, tin, contact_name, contact_phone, contact_email,
				payout_method, payout_account_name, payout_enc, payout_last4) VALUES ($1, $2, $3, NULLIF($4, ''), $5, $6, $7, $8, $9, $10, $11)`,
				id, in.LegalName, in.License, in.TIN, in.ContactName, in.ContactTel, in.ContactMail, in.Method, in.AccountName,
				s.keys.Seal(number), last4(strings.ReplaceAll(number, " ", ""))); err != nil {
				return err
			}
		} else {
			if _, err := tx.Exec(r.Context(), `UPDATE hotel_business SET legal_name = $2, license_number = $3, tin = NULLIF($4, ''), contact_name = $5,
				contact_phone = $6, contact_email = $7, payout_method = $8, payout_account_name = $9 WHERE property_id = $1`,
				id, in.LegalName, in.License, in.TIN, in.ContactName, in.ContactTel, in.ContactMail, in.Method, in.AccountName); err != nil {
				return err
			}
			if number != "" {
				if _, err := tx.Exec(r.Context(), `UPDATE hotel_business SET payout_enc = $2, payout_last4 = $3 WHERE property_id = $1`,
					id, s.keys.Seal(number), last4(strings.ReplaceAll(number, " ", ""))); err != nil {
					return err
				}
			}
		}
		// The audit entry says the record changed, never what the account number is.
		return s.adminAudit(r, tx, "admin_business_saved", id, map[string]any{"payout_changed": number != ""})
	})
	if err != nil {
		return err
	}
	respond(w, http.StatusOK, httpx.NowMeta())
	return nil
}

// adminRevealPayout shows the full payout account number of a hotel (to make a payment). Every call is written to the audit log.
func (s *Server) adminRevealPayout(w http.ResponseWriter, r *http.Request) error {
	if err := requireAdmin(r); err != nil {
		return err
	}
	id := chi.URLParam(r, "id")
	if err := s.canManage(r, id); err != nil {
		return err
	}
	var enc []byte
	if err := s.pool.QueryRow(r.Context(), `SELECT payout_enc FROM hotel_business WHERE property_id = $1`, id).Scan(&enc); err != nil {
		if err == pgx.ErrNoRows {
			return errNotFound
		}
		return err
	}
	number, err := s.keys.Open(enc)
	if err != nil {
		return err
	}
	if err := s.tx(r.Context(), func(tx pgx.Tx) error { return s.adminAudit(r, tx, "admin_payout_revealed", id, nil) }); err != nil {
		return err
	}
	respond(w, http.StatusOK, struct {
		httpx.Meta
		AccountNumber string `json:"account_number"`
	}{httpx.NowMeta(), number})
	return nil
}

func isUnique(err error) bool {
	var pe interface{ SQLState() string }
	for e := err; e != nil; {
		if p, ok := e.(interface{ SQLState() string }); ok {
			pe = p
			break
		}
		u, ok := e.(interface{ Unwrap() error })
		if !ok {
			break
		}
		e = u.Unwrap()
	}
	return pe != nil && pe.SQLState() == "23505"
}
