package api

import (
	"encoding/json"
	"net/http"
	"strings"

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
		       (SELECT count(*) FROM property_photos pp WHERE pp.property_id = p.id)::int
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
		ID          string     `json:"id"`
		Name        string     `json:"name"`
		Type        string     `json:"type"`
		City        string     `json:"city"`
		Area        string     `json:"area"`
		Address     string     `json:"address"`
		Phone       string     `json:"phone"`
		Description string     `json:"description"`
		Lat         float64    `json:"lat"`
		Lng         float64    `json:"lng"`
		Amenities   []string   `json:"amenities"`
		HouseRules  any        `json:"house_rules"`
		PayAtHotel  bool       `json:"pay_at_hotel"`
		Verified    bool       `json:"verified"`
		Status      string     `json:"status"`
		Rooms       []roomOut  `json:"rooms"`
		Photos      []photoOut `json:"photos"`
		Staff       []staffOut `json:"staff"`
	}
	var rules []byte
	if err := s.pool.QueryRow(ctx, `SELECT id, name, type, city, area, address, phone, description, `+lat+`, `+lng+`, amenities, house_rules, pay_at_hotel, verified, status
		FROM properties WHERE id = $1`, id).Scan(&h.ID, &h.Name, &h.Type, &h.City, &h.Area, &h.Address, &h.Phone, &h.Description, &h.Lat, &h.Lng,
		&h.Amenities, &rules, &h.PayAtHotel, &h.Verified, &h.Status); err != nil {
		return err
	}
	h.HouseRules = []string{}
	if len(rules) > 0 {
		h.HouseRules = json.RawMessage(rules)
	}
	if h.Amenities == nil {
		h.Amenities = []string{}
	}

	rr, err := s.pool.Query(ctx, `SELECT rt.id, rt.name, rt.capacity, rt.total_rooms, rt.amenities, rp.nightly_price_santim, coalesce(rp.currency, 'ETB')
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
		if err := rr.Scan(&ro.ID, &ro.Name, &ro.Capacity, &ro.TotalRooms, &ro.Amenities, &ro.PriceSantim, &ro.Currency); err != nil {
			return err
		}
		if ro.Amenities == nil {
			ro.Amenities = []string{}
		}
		h.Rooms = append(h.Rooms, ro)
	}
	rr.Close()

	pr, err := s.pool.Query(ctx, `SELECT id FROM property_photos WHERE property_id = $1 ORDER BY position, created_at`, id)
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

	sr, err := s.pool.Query(ctx, `SELECT u.id, coalesce(u.email, u.phone), ps.role FROM property_staff ps JOIN users u ON u.id = ps.user_id
		WHERE ps.property_id = $1 ORDER BY ps.role DESC, ps.created_at`, id)
	if err != nil {
		return err
	}
	defer sr.Close()
	h.Staff = []staffOut{}
	for sr.Next() {
		var so staffOut
		if err := sr.Scan(&so.UserID, &so.Contact, &so.Role); err != nil {
			return err
		}
		h.Staff = append(h.Staff, so)
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
		if err := tx.QueryRow(r.Context(), `INSERT INTO properties (name, type, city, area, address, location, phone, description, pay_at_hotel, amenities, house_rules, status, verified)
			VALUES ($1, $2, $3, $4, $5, `+s.pointSQL("$6", "$7")+`, $8, $9, $10, $11, $12::jsonb, 'draft', coalesce($13, false)) RETURNING id`,
			in.Name, in.Type, in.City, in.Area, in.Address, *in.Lng, *in.Lat, in.Phone, in.Description, in.PayAtHotel, in.Amenities, string(rules), in.Verified).Scan(&id); err != nil {
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
		if in.Status == "active" {
			var ready bool
			if err := tx.QueryRow(r.Context(), `SELECT EXISTS (SELECT 1 FROM room_types rt JOIN rate_plans rp ON rp.room_type_id = rt.id
				WHERE rt.property_id = $1 AND rt.total_rooms > 0 AND (now() AT TIME ZONE 'Africa/Addis_Ababa')::date BETWEEN rp.valid_from AND rp.valid_to)`, id).Scan(&ready); err != nil {
				return err
			}
			if !ready {
				return fail(http.StatusConflict, codeInvalidState, "Add a room with a price before publishing this hotel")
			}
		}
		// Only a platform admin can mark a hotel verified.
		verified := in.Verified
		if !isAdmin {
			verified = nil
		}
		if _, err := tx.Exec(r.Context(), `UPDATE properties SET name = $2, type = $3, city = $4, area = $5, address = $6, location = `+s.pointSQL("$7", "$8")+`,
			phone = $9, description = $10, pay_at_hotel = $11, amenities = $12, house_rules = $13::jsonb, status = $14, verified = coalesce($15, verified)
			WHERE id = $1`, id, in.Name, in.Type, in.City, in.Area, in.Address, *in.Lng, *in.Lat, in.Phone, in.Description, in.PayAtHotel, in.Amenities, string(rules), in.Status, verified); err != nil {
			return err
		}
		return s.adminAudit(r, tx, "admin_hotel_updated", id, map[string]any{"status": in.Status})
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
		if err := tx.QueryRow(r.Context(), `INSERT INTO room_types (property_id, name, capacity, total_rooms, amenities) VALUES ($1, $2, $3, $4, $5) RETURNING id`,
			id, in.Name, in.Capacity, in.TotalRooms, in.Amenities).Scan(&roomID); err != nil {
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
		if _, err := tx.Exec(r.Context(), `UPDATE room_types SET name = $2, capacity = $3, total_rooms = $4, amenities = $5 WHERE id = $1`,
			room, in.Name, in.Capacity, in.TotalRooms, in.Amenities); err != nil {
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

// ---- staff (platform admin only)

func (s *Server) adminAddStaff(w http.ResponseWriter, r *http.Request) error {
	if err := requireAdmin(r); err != nil {
		return err
	}
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
	col := "phone"
	if ident.Type == "email" {
		col = "email"
	}
	var uid string
	err = s.tx(r.Context(), func(tx pgx.Tx) error {
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
		case role == "guest":
			// A guest account that becomes hotel staff.
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
	respond(w, http.StatusOK, struct {
		httpx.Meta
		UserID string `json:"user_id"`
	}{httpx.NowMeta(), uid})
	return nil
}

func (s *Server) adminRemoveStaff(w http.ResponseWriter, r *http.Request) error {
	if err := requireAdmin(r); err != nil {
		return err
	}
	id, user := chi.URLParam(r, "id"), chi.URLParam(r, "user")
	if err := s.canManage(r, id); err != nil {
		return err
	}
	err := s.tx(r.Context(), func(tx pgx.Tx) error {
		tag, err := tx.Exec(r.Context(), `DELETE FROM property_staff WHERE property_id = $1 AND user_id = $2`, id, user)
		if err != nil {
			if isBadUUID(err) {
				return errNotFound
			}
			return err
		}
		if tag.RowsAffected() == 0 {
			return errNotFound
		}
		return s.adminAudit(r, tx, "admin_staff_removed", id, map[string]any{"user_id": user})
	})
	if err != nil {
		return err
	}
	respond(w, http.StatusOK, httpx.NowMeta())
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
