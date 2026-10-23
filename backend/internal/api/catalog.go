package api

import (
	"context"
	"hash/fnv"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/bookdan/backend/internal/httpx"
)

const searchPageSize = 12

// Placeholder colours for hotels without photos (the apps tint the card while a photo loads).
var accents = []string{"#1E3A8A", "#0F766E", "#7C3AED", "#B45309", "#0E7490", "#9D174D", "#166534", "#374151"}

func accentFor(id string) string {
	h := fnv.New32a()
	h.Write([]byte(id))
	return accents[int(h.Sum32())%len(accents)]
}

// latLng returns SQL expressions for a property's latitude and longitude.
func (s *Server) latLng(col string) (lat, lng string) {
	if s.postgis {
		return "ST_Y(" + col + "::geometry)", "ST_X(" + col + "::geometry)"
	}
	// The test database stores the point as text: SRID=4326;POINT(lng lat)
	pair := "rtrim(split_part(" + col + ", '(', 2), ')')"
	return "split_part(" + pair + ", ' ', 2)::float8", "split_part(" + pair + ", ' ', 1)::float8"
}

type searchItem struct {
	ID                          string   `json:"id"`
	Name                        string   `json:"name"`
	Area                        string   `json:"area"`
	City                        string   `json:"city"`
	Type                        string   `json:"type"`
	Badge                       string   `json:"badge"`
	FromPrice                   int64    `json:"from_price_santim"`
	Currency                    string   `json:"currency"`
	Available                   bool     `json:"available"`
	Amenities                   []string `json:"amenities"`
	Rating                      *float64 `json:"rating"`
	ReviewCnt                   int      `json:"review_count"`
	PhotoURL                    *string  `json:"photo_url"`
	Accent                      string   `json:"accent"`
	Lat                         float64  `json:"lat"`
	Lng                         float64  `json:"lng"`
	description, phone, address string
	houseRules                  []byte
}

type roomType struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Capacity   int      `json:"capacity"`
	Price      int64    `json:"price_santim"`
	Currency   string   `json:"currency"`
	Amenities  []string `json:"amenities"`
	RoomsLeft  int      `json:"rooms_left"`
	propertyID string
}

// roomsFor returns the room types of the given properties with the live price for checkIn and the rooms still free
// on every night of the stay. A room type with no rate for check-in is not for sale and is left out.
func (s *Server) roomsFor(ctx context.Context, q interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, propertyIDs []string, checkIn, checkOut string) (map[string][]roomType, error) {
	rows, err := q.Query(ctx, `
		SELECT rt.property_id, rt.id, rt.name, rt.capacity, rt.amenities, rp.nightly_price_santim, rp.currency,
		  GREATEST(0, COALESCE((
		    SELECT min(COALESCE(i.capacity - i.held - i.booked, rt.total_rooms))
		    FROM generate_series($2::date, $3::date - 1, '1 day') AS n(night)
		    LEFT JOIN inventory i ON i.room_type_id = rt.id AND i.night = n.night::date
		  ), rt.total_rooms))::int AS rooms_left
		FROM room_types rt
		JOIN LATERAL (SELECT nightly_price_santim, currency FROM rate_plans
		              WHERE room_type_id = rt.id AND $2::date BETWEEN valid_from AND valid_to LIMIT 1) rp ON true
		WHERE rt.property_id = ANY($1::uuid[])
		ORDER BY rp.nightly_price_santim`, propertyIDs, checkIn, checkOut)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]roomType{}
	for rows.Next() {
		var r roomType
		if err := rows.Scan(&r.propertyID, &r.ID, &r.Name, &r.Capacity, &r.Amenities, &r.Price, &r.Currency, &r.RoomsLeft); err != nil {
			return nil, err
		}
		out[r.propertyID] = append(out[r.propertyID], r)
	}
	return out, rows.Err()
}

// stay validates the dates and party size that search and detail share.
func (s *Server) stay(r *http.Request) (checkIn, checkOut string, guests, rooms int, err error) {
	q := r.URL.Query()
	checkIn, checkOut = q.Get("check_in"), q.Get("check_out")
	if !isDate(checkIn) || !isDate(checkOut) {
		return "", "", 0, 0, fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "check_in and check_out must be dates (YYYY-MM-DD)")
	}
	if n := nightsBetween(checkIn, checkOut); n < 1 || n > maxNights {
		return "", "", 0, 0, fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "A stay is between 1 and 30 nights")
	}
	guests, rooms = atoiDefault(q.Get("guests"), 1), atoiDefault(q.Get("rooms"), 1)
	if guests < 1 || guests > 40 || rooms < 1 || rooms > maxRooms {
		return "", "", 0, 0, fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "Choose 1 to 9 rooms")
	}
	return checkIn, checkOut, guests, rooms, nil
}

func atoiDefault(v string, d int) int {
	if n, err := strconv.Atoi(v); err == nil {
		return n
	}
	return d
}

func (s *Server) search(w http.ResponseWriter, r *http.Request) error {
	checkIn, checkOut, guests, rooms, err := s.stay(r)
	if err != nil {
		return err
	}
	q := r.URL.Query()
	needle := strings.ToLower(strings.TrimSpace(q.Get("q")))
	types, amenities := []string{}, []string{}
	for _, t := range strings.Split(q.Get("types"), ",") {
		if t = strings.TrimSpace(t); t != "" {
			types = append(types, t)
		}
	}
	for _, a := range strings.Split(q.Get("amenities"), ",") {
		if a = strings.TrimSpace(a); a != "" {
			amenities = append(amenities, a)
		}
	}
	offset := atoiDefault(q.Get("cursor"), 0)
	if offset < 0 {
		offset = 0
	}

	// A hotel is shown when its name, area or city matches, or (with PostGIS) when it is within 3 km of a matching landmark.
	near := ""
	if s.postgis {
		near = ` OR EXISTS (SELECT 1 FROM landmarks l WHERE (lower(l.name) LIKE '%' || $1 || '%' OR lower(coalesce(l.name_am, '')) LIKE '%' || $1 || '%') AND ST_DWithin(l.location, p.location, 3000))`
	}
	lat, lng := s.latLng("p.location")
	rows, err := s.pool.Query(r.Context(), `
		SELECT p.id, p.name, p.area, p.city, p.type, p.verified, p.amenities, pr.rating, coalesce(pr.review_count, 0),
		       `+lat+`, `+lng+`, (SELECT pp.id FROM property_photos pp WHERE pp.property_id = p.id ORDER BY pp.position, pp.created_at LIMIT 1)
		FROM properties p LEFT JOIN property_ratings pr ON pr.property_id = p.id
		WHERE p.status = 'active'
		  AND ($1 = '' OR lower(p.name) LIKE '%' || $1 || '%' OR lower(p.area) LIKE '%' || $1 || '%' OR lower(p.city) LIKE '%' || $1 || '%'`+near+`)
		  AND (cardinality($2::text[]) = 0 OR p.type = ANY($2))
		  AND p.amenities @> $3::text[]
		ORDER BY (lower(p.name) LIKE '%' || $1 || '%' OR lower(p.area) LIKE '%' || $1 || '%' OR lower(p.city) LIKE '%' || $1 || '%') DESC, p.name
		LIMIT $4 OFFSET $5`, needle, types, amenities, searchPageSize+1, offset)
	if err != nil {
		return err
	}
	defer rows.Close()
	var items []searchItem
	for rows.Next() {
		var it searchItem
		var verified bool
		var rating *float64
		var coverID *string
		if err := rows.Scan(&it.ID, &it.Name, &it.Area, &it.City, &it.Type, &verified, &it.Amenities, &rating, &it.ReviewCnt, &it.Lat, &it.Lng, &coverID); err != nil {
			return err
		}
		if coverID != nil {
			u := s.photoURL(*coverID, "thumb") // the small copy: lists load fast on mobile data
			it.PhotoURL = &u
		}
		it.Badge = "none"
		if verified {
			it.Badge = "verified"
		}
		it.Rating, it.Accent = rating, accentFor(it.ID)
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()

	next := (*string)(nil)
	if len(items) > searchPageSize {
		items = items[:searchPageSize]
		n := strconv.Itoa(offset + searchPageSize)
		next = &n
	}
	ids := make([]string, len(items))
	for i, it := range items {
		ids[i] = it.ID
	}
	rts, err := s.roomsFor(r.Context(), s.pool, ids, checkIn, checkOut)
	if err != nil {
		return err
	}
	for i := range items {
		fillAvailability(&items[i], rts[items[i].ID], guests, rooms)
	}
	if items == nil {
		items = []searchItem{}
	}
	respond(w, http.StatusOK, struct {
		httpx.Meta
		Items      []searchItem `json:"items"`
		NextCursor *string      `json:"next_cursor"`
	}{httpx.NowMeta(), items, next})
	return nil
}

// fillAvailability sets the price shown on a card and whether the party fits: some room type has enough free rooms
// and the rooms together sleep the guests.
func fillAvailability(it *searchItem, rts []roomType, guests, rooms int) {
	it.Currency = "ETB"
	for i, rt := range rts {
		if i == 0 || rt.Price < it.FromPrice {
			it.FromPrice, it.Currency = rt.Price, rt.Currency
		}
		if rt.RoomsLeft >= rooms && rt.Capacity*rooms >= guests {
			it.Available = true
		}
	}
	if it.Amenities == nil {
		it.Amenities = []string{}
	}
}

func (s *Server) property(w http.ResponseWriter, r *http.Request) error {
	checkIn, checkOut, _, _, err := s.stay(r)
	if err != nil {
		return err
	}
	id := chi.URLParam(r, "id")
	ctx := r.Context()
	lat, lng := s.latLng("p.location")

	var it searchItem
	var verified bool
	var rating *float64
	var rules []byte
	var coverID *string
	err = s.pool.QueryRow(ctx, `
		SELECT p.id, p.name, p.area, p.city, p.type, p.verified, p.amenities, pr.rating, coalesce(pr.review_count, 0),
		       `+lat+`, `+lng+`, p.description, p.phone, p.address, p.house_rules, (SELECT pp.id FROM property_photos pp WHERE pp.property_id = p.id ORDER BY pp.position, pp.created_at LIMIT 1)
		FROM properties p LEFT JOIN property_ratings pr ON pr.property_id = p.id
		WHERE p.id = $1 AND p.status = 'active'`, id).
		Scan(&it.ID, &it.Name, &it.Area, &it.City, &it.Type, &verified, &it.Amenities, &rating, &it.ReviewCnt, &it.Lat, &it.Lng,
			&it.description, &it.phone, &it.address, &rules, &coverID)
	if err == pgx.ErrNoRows || (err != nil && strings.Contains(err.Error(), "invalid input syntax for type uuid")) {
		return errNotFound
	}
	if err != nil {
		return err
	}
	it.Badge = "none"
	if verified {
		it.Badge = "verified"
	}
	it.Rating, it.Accent = rating, accentFor(it.ID)
	if coverID != nil {
		u := s.photoURL(*coverID, "thumb")
		it.PhotoURL = &u
	}
	photos := []string{}
	pr, err := s.pool.Query(ctx, `SELECT id FROM property_photos WHERE property_id = $1 ORDER BY position, created_at`, it.ID)
	if err != nil {
		return err
	}
	defer pr.Close()
	for pr.Next() {
		var pid string
		if err := pr.Scan(&pid); err != nil {
			return err
		}
		photos = append(photos, s.photoURL(pid, "main"))
	}
	pr.Close()
	rts, err := s.roomsFor(ctx, s.pool, []string{it.ID}, checkIn, checkOut)
	if err != nil {
		return err
	}
	fillAvailability(&it, rts[it.ID], 1, 1)
	rooms := rts[it.ID]
	if rooms == nil {
		rooms = []roomType{}
	}
	for i := range rooms {
		if rooms[i].Amenities == nil {
			rooms[i].Amenities = []string{}
		}
	}

	type review struct {
		ID     string  `json:"id"`
		Author string  `json:"author"`
		Rating float64 `json:"rating"`
		Text   string  `json:"text"`
		Date   string  `json:"date"`
	}
	revRows, err := s.pool.Query(ctx, `SELECT r.id, coalesce(nullif(u.display_name, ''), 'Guest'), r.rating, r.text, r.created_at
		FROM reviews r JOIN users u ON u.id = r.user_id WHERE r.property_id = $1 ORDER BY r.created_at DESC LIMIT 20`, id)
	if err != nil {
		return err
	}
	defer revRows.Close()
	reviews := []review{}
	for revRows.Next() {
		var rv review
		var at time.Time
		if err := revRows.Scan(&rv.ID, &rv.Author, &rv.Rating, &rv.Text, &at); err != nil {
			return err
		}
		rv.Date = pgDate(at)
		reviews = append(reviews, rv)
	}

	type propertyOut struct {
		searchItem
		Description string   `json:"description"`
		Phone       string   `json:"phone"`
		Address     string   `json:"address"`
		HouseRules  any      `json:"house_rules"`
		Photos      []string `json:"photos"`
	}
	var houseRules any = []string{}
	if len(rules) > 0 {
		houseRules = jsonRaw(rules)
	}
	respond(w, http.StatusOK, struct {
		httpx.Meta
		Property  propertyOut `json:"property"`
		RoomTypes []roomType  `json:"room_types"`
		Reviews   []review    `json:"reviews"`
	}{httpx.NowMeta(), propertyOut{it, it.description, it.phone, it.address, houseRules, photos}, rooms, reviews})
	return nil
}
