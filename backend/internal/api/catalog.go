package api

import (
	"context"
	"hash/fnv"
	"math"
	"net/http"
	"sort"
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
	// Breakfast: some room here can be booked with breakfast. NonRefundable: a cheaper non-refundable rate is offered. ReplyMinutes: how long the
	// hotel usually takes to answer a request (nil until it has answered a few).
	Breakfast     bool     `json:"breakfast"`
	NonRefundable bool     `json:"nonrefundable"`
	ReplyMinutes  *int     `json:"reply_minutes"`
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Area          string   `json:"area"`
	City          string   `json:"city"`
	Type          string   `json:"type"`
	Badge         string   `json:"badge"`
	FromPrice     int64    `json:"from_price_santim"`
	Currency      string   `json:"currency"`
	Available     bool     `json:"available"`
	Amenities     []string `json:"amenities"`
	Rating        *float64 `json:"rating"`
	ReviewCnt     int      `json:"review_count"`
	PhotoURL      *string  `json:"photo_url"`
	Accent        string   `json:"accent"`
	Lat           float64  `json:"lat"`
	Lng           float64  `json:"lng"`
	// RoomsLeft is how many rooms of the cheapest room type that fits the party are still free (0 when none): the apps show "Only 2 left".
	RoomsLeft int `json:"rooms_left"`
	// DistanceM is the distance from the point the guest searched around (lat and lng in the request), when given.
	DistanceM *int `json:"distance_m"`
	// Windows are stays this hotel can still sell when it cannot sell the whole stay asked for (only in the "other dates" list).
	Windows []altWindow `json:"windows,omitempty"`
	// Landmark is the closest well-known place ("1.4 km to Bole Airport"), shown on the card.
	Landmark                    *placeRef `json:"landmark"`
	description, phone, address string
	houseRules                  []byte
	matched                     bool
}

// placeRef is a landmark and how far it is.
type placeRef struct {
	Name      string `json:"name"`
	NameAm    string `json:"name_am,omitempty"`
	DistanceM int    `json:"distance_m"`
}

type landmark struct {
	name, nameAm string
	lat, lng     float64
}

func (s *Server) landmarks(ctx context.Context) ([]landmark, error) {
	lat, lng := s.latLng("location")
	rows, err := s.pool.Query(ctx, `SELECT name, coalesce(name_am, ''), `+lat+`, `+lng+` FROM landmarks`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []landmark
	for rows.Next() {
		var l landmark
		if err := rows.Scan(&l.name, &l.nameAm, &l.lat, &l.lng); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// nearLandmarks returns the landmarks within maxM metres of a point, closest first (at most limit).
func nearLandmarks(ls []landmark, lat, lng float64, maxM, limit int) []placeRef {
	var out []placeRef
	for _, l := range ls {
		if d := distanceM(lat, lng, l.lat, l.lng); d <= maxM {
			out = append(out, placeRef{l.name, l.nameAm, d})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DistanceM < out[j].DistanceM })
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

type roomType struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Capacity  int      `json:"capacity"`
	Price     int64    `json:"price_santim"`
	Currency  string   `json:"currency"`
	Amenities []string `json:"amenities"`
	RoomsLeft int      `json:"rooms_left"`
	// Options are the ways to book this room (meal and cancellation), each with its price per night.
	Options []roomOption `json:"options"`
	// Photos are this room's own pictures (full size), cover first; Thumb is the first one, small, for the card.
	Photos     []string `json:"photos"`
	Thumb      *string  `json:"photo_thumb"`
	propertyID string
}

// roomsFor returns the room types of the given properties with the live price for checkIn and the rooms still free
// on every night of the stay. A room type with no rate for check-in is not for sale and is left out.
func (s *Server) roomsFor(ctx context.Context, q interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, propertyIDs []string, checkIn, checkOut string) (map[string][]roomType, error) {
	rows, err := q.Query(ctx, `
		SELECT rt.property_id, rt.id, rt.name, rt.capacity, rt.amenities, rp.nightly_price_santim, rp.currency, rt.breakfast_santim, rt.nonrefundable_bps,
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
		var offer roomOffer
		if err := rows.Scan(&r.propertyID, &r.ID, &r.Name, &r.Capacity, &r.Amenities, &r.Price, &r.Currency, &offer.breakfast, &offer.nrBPS, &r.RoomsLeft); err != nil {
			return nil, err
		}
		r.Options = offer.options(r.Price)
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

// searchPlan is what the guest asked for, read from the query string.
type searchPlan struct {
	breakfast, deals bool // only hotels that offer breakfast / a cheaper non-refundable rate
	needle           string
	types, amenities []string
	minPrice         int64 // santim per night, 0 = no limit
	maxPrice         int64
	minRating        float64
	sort             string
	lat, lng         float64
	hasPoint         bool
}

var searchSorts = map[string]bool{"recommended": true, "price_asc": true, "price_desc": true, "rating": true, "distance": true}

func csv(v string) []string {
	out := []string{}
	for _, x := range strings.Split(v, ",") {
		if x = strings.TrimSpace(x); x != "" {
			out = append(out, x)
		}
	}
	return out
}

func readSearchPlan(q map[string][]string) (searchPlan, error) {
	get := func(k string) string {
		if v := q[k]; len(v) > 0 {
			return v[0]
		}
		return ""
	}
	p := searchPlan{needle: strings.ToLower(strings.TrimSpace(get("q"))), types: csv(get("types")), amenities: csv(get("amenities")), sort: get("sort")}
	if p.sort == "" {
		p.sort = "recommended"
	}
	if !searchSorts[p.sort] {
		return p, fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "sort must be recommended, price_asc, price_desc, rating or distance")
	}
	p.breakfast, p.deals = get("breakfast") == "1", get("deals") == "1"
	var err error
	for k, dst := range map[string]*int64{"min_price": &p.minPrice, "max_price": &p.maxPrice} {
		if v := get(k); v != "" {
			if *dst, err = strconv.ParseInt(v, 10, 64); err != nil || *dst < 0 {
				return p, fail(http.StatusBadRequest, httpx.CodeInvalidRequest, k+" must be an amount in santim")
			}
		}
	}
	if v := get("min_rating"); v != "" {
		if p.minRating, err = strconv.ParseFloat(v, 64); err != nil || p.minRating < 0 || p.minRating > 5 {
			return p, fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "min_rating must be between 0 and 5")
		}
	}
	la, lo := get("lat"), get("lng")
	if la != "" && lo != "" {
		var e1, e2 error
		p.lat, e1 = strconv.ParseFloat(la, 64)
		p.lng, e2 = strconv.ParseFloat(lo, 64)
		if e1 != nil || e2 != nil || p.lat < -90 || p.lat > 90 || p.lng < -180 || p.lng > 180 {
			return p, fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "lat and lng must be coordinates")
		}
		p.hasPoint = true
	}
	if p.sort == "distance" && !p.hasPoint {
		return p, fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "sorting by distance needs lat and lng")
	}
	return p, nil
}

// distanceM is the straight-line distance in metres between two points.
func distanceM(lat1, lng1, lat2, lng2 float64) int {
	const r = 6371000.0
	rad := math.Pi / 180
	dLat, dLng := (lat2-lat1)*rad, (lng2-lng1)*rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return int(2 * r * math.Asin(math.Min(1, math.Sqrt(a))))
}

const maxSearchCandidates = 500

func (s *Server) search(w http.ResponseWriter, r *http.Request) error {
	checkIn, checkOut, guests, rooms, datesKnown, err := s.searchStay(r)
	if err != nil {
		return err
	}
	q := r.URL.Query()
	plan, err := readSearchPlan(q)
	if err != nil {
		return err
	}
	offset := atoiDefault(q.Get("cursor"), 0)
	if offset < 0 {
		offset = 0
	}

	// Every hotel that matches the words (name, area, city, and with PostGIS a landmark within 3 km). Type, amenities, price and
	// rating are applied below, after prices are known, so sorting and paging by price are exact and the filter counts can be shown.
	near := ""
	if s.postgis {
		near = ` OR EXISTS (SELECT 1 FROM landmarks l WHERE (lower(l.name) LIKE '%' || $1 || '%' OR lower(coalesce(l.name_am, '')) LIKE '%' || $1 || '%') AND ST_DWithin(l.location, p.location, 3000))`
	}
	lat, lng := s.latLng("p.location")
	rows, err := s.pool.Query(r.Context(), `
		SELECT p.id, p.name, p.area, p.city, p.type, p.verified, p.amenities, pr.rating::float8, coalesce(pr.review_count, 0),
		       `+lat+`, `+lng+`, (SELECT pp.id FROM property_photos pp WHERE pp.property_id = p.id AND pp.room_type_id IS NULL ORDER BY pp.position, pp.created_at LIMIT 1),
		       (lower(p.name) LIKE '%' || $1 || '%' OR lower(p.area) LIKE '%' || $1 || '%' OR lower(p.city) LIKE '%' || $1 || '%')
		FROM properties p LEFT JOIN property_ratings pr ON pr.property_id = p.id
		WHERE p.status = 'active'
		  AND ($1 = '' OR lower(p.name) LIKE '%' || $1 || '%' OR lower(p.area) LIKE '%' || $1 || '%' OR lower(p.city) LIKE '%' || $1 || '%'`+near+`)
		ORDER BY p.name
		LIMIT $2`, plan.needle, maxSearchCandidates)
	if err != nil {
		return err
	}
	defer rows.Close()
	var all []searchItem
	for rows.Next() {
		var it searchItem
		var verified bool
		var rating *float64
		var coverID *string
		if err := rows.Scan(&it.ID, &it.Name, &it.Area, &it.City, &it.Type, &verified, &it.Amenities, &rating, &it.ReviewCnt, &it.Lat, &it.Lng, &coverID, &it.matched); err != nil {
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
		all = append(all, it)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()

	ids := make([]string, len(all))
	for i, it := range all {
		ids[i] = it.ID
	}
	rts, err := s.roomsFor(r.Context(), s.pool, ids, checkIn, checkOut)
	if err != nil {
		return err
	}
	replies, err := s.replyMinutes(r.Context(), ids)
	if err != nil {
		return err
	}
	facets := struct {
		Types     map[string]int `json:"types"`
		Amenities map[string]int `json:"amenities"`
	}{map[string]int{}, map[string]int{}}
	var pmin, pmax int64
	var items []searchItem
	marks, err := s.landmarks(r.Context())
	if err != nil {
		return err
	}
	for i := range all {
		it := &all[i]
		fillAvailability(it, rts[it.ID], guests, rooms)
		if !datesKnown {
			// No dates: the price is a "from" price and the hotel makes no claim about rooms.
			it.Available, it.RoomsLeft = true, 0
		}
		it.ReplyMinutes = replies[it.ID]
		if plan.hasPoint {
			d := distanceM(plan.lat, plan.lng, it.Lat, it.Lng)
			it.DistanceM = &d
		}
		if near := nearLandmarks(marks, it.Lat, it.Lng, 15000, 1); len(near) > 0 {
			it.Landmark = &near[0]
		}
		// The counts next to each filter describe what the words found, so they stay put while the guest ticks boxes.
		facets.Types[it.Type]++
		for _, a := range it.Amenities {
			facets.Amenities[a]++
		}
		if it.Available && it.FromPrice > 0 {
			if pmin == 0 || it.FromPrice < pmin {
				pmin = it.FromPrice
			}
			if it.FromPrice > pmax {
				pmax = it.FromPrice
			}
		}
		if plan.keeps(*it) {
			items = append(items, *it)
		}
	}
	plan.order(items)

	// On the first page, the hotels with no room for the whole stay that can still offer part of it, or nearby dates.
	var other []searchItem
	if offset == 0 && datesKnown {
		if other, err = s.otherDates(r.Context(), items, checkIn, checkOut, guests, rooms); err != nil {
			return err
		}
	}
	if other == nil {
		other = []searchItem{}
	}

	total := len(items)
	next := (*string)(nil)
	if offset > len(items) {
		offset = len(items)
	}
	items = items[offset:]
	if len(items) > searchPageSize {
		items = items[:searchPageSize]
		n := strconv.Itoa(offset + searchPageSize)
		next = &n
	}
	if items == nil {
		items = []searchItem{}
	}
	respond(w, http.StatusOK, struct {
		httpx.Meta
		Items      []searchItem `json:"items"`
		NextCursor *string      `json:"next_cursor"`
		// Total is how many hotels match the filters (all pages). Prices are for one night; the bounds cover the hotels that have a room free.
		Total    int   `json:"total"`
		PriceMin int64 `json:"price_min_santim"`
		PriceMax int64 `json:"price_max_santim"`
		Facets   any   `json:"facets"`
		// OtherDates are sold-out hotels with stays that work nearby: the dates the hotel can sell, nothing held.
		OtherDates []searchItem `json:"other_dates"`
		// DatesKnown is false when the search had no dates: prices are "from" prices for tonight and nothing is claimed about rooms.
		DatesKnown bool `json:"dates_known"`
	}{httpx.NowMeta(), items, next, total, pmin, pmax, facets, other, datesKnown})
	return nil
}

// keeps reports whether a hotel passes the guest's type, amenity, price and rating filters.
func (p searchPlan) keeps(it searchItem) bool {
	if (p.breakfast && !it.Breakfast) || (p.deals && !it.NonRefundable) {
		return false
	}
	if len(p.types) > 0 {
		ok := false
		for _, t := range p.types {
			ok = ok || t == it.Type
		}
		if !ok {
			return false
		}
	}
	for _, want := range p.amenities {
		has := false
		for _, a := range it.Amenities {
			has = has || a == want
		}
		if !has {
			return false
		}
	}
	if p.minRating > 0 && (it.Rating == nil || *it.Rating < p.minRating) {
		return false
	}
	// A price filter speaks about the price a guest would pay, so hotels with no price for these dates cannot pass it.
	if (p.minPrice > 0 || p.maxPrice > 0) && it.FromPrice == 0 {
		return false
	}
	if p.minPrice > 0 && it.FromPrice < p.minPrice {
		return false
	}
	if p.maxPrice > 0 && it.FromPrice > p.maxPrice {
		return false
	}
	return true
}

// order sorts the hotels. Whatever the sort, hotels with no room for the party come last.
func (p searchPlan) order(items []searchItem) {
	less := func(a, b searchItem) bool {
		if a.Available != b.Available {
			return a.Available
		}
		switch p.sort {
		case "price_asc":
			if a.FromPrice != b.FromPrice {
				return a.FromPrice < b.FromPrice
			}
		case "price_desc":
			if a.FromPrice != b.FromPrice {
				return a.FromPrice > b.FromPrice
			}
		case "rating":
			if ra, rb := ratingOr0(a), ratingOr0(b); ra != rb {
				return ra > rb
			}
			if a.ReviewCnt != b.ReviewCnt {
				return a.ReviewCnt > b.ReviewCnt
			}
		case "distance":
			if a.DistanceM != nil && b.DistanceM != nil && *a.DistanceM != *b.DistanceM {
				return *a.DistanceM < *b.DistanceM
			}
		default: // recommended: the ones the words name first, then the best rated
			if a.matched != b.matched {
				return a.matched
			}
			if ra, rb := ratingOr0(a), ratingOr0(b); ra != rb {
				return ra > rb
			}
		}
		return a.Name < b.Name
	}
	sort.SliceStable(items, func(i, j int) bool { return less(items[i], items[j]) })
}

func ratingOr0(it searchItem) float64 {
	if it.Rating == nil {
		return 0
	}
	return *it.Rating
}

// fillAvailability sets the price shown on a card and whether the party fits: some room type has enough free rooms
// and the rooms together sleep the guests.
func fillAvailability(it *searchItem, rts []roomType, guests, rooms int) {
	it.Currency = "ETB"
	for i, rt := range rts { // cheapest first
		for _, o := range rt.Options {
			if o.Meal == mealBreakfast {
				it.Breakfast = true
			}
			if !o.Refundable {
				it.NonRefundable = true
			}
		}
		if i == 0 || rt.Price < it.FromPrice {
			it.FromPrice, it.Currency = rt.Price, rt.Currency
		}
		if rt.RoomsLeft >= rooms && rt.Capacity*rooms >= guests {
			if !it.Available {
				it.RoomsLeft = rt.RoomsLeft
			}
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
	var checkInTime, checkOutTime, descAm, directions, directionsAm string
	err = s.pool.QueryRow(ctx, `
		SELECT p.id, p.name, p.area, p.city, p.type, p.verified, p.amenities, pr.rating::float8, coalesce(pr.review_count, 0),
		       `+lat+`, `+lng+`, p.description, p.phone, p.address, p.house_rules, (SELECT pp.id FROM property_photos pp WHERE pp.property_id = p.id AND pp.room_type_id IS NULL ORDER BY pp.position, pp.created_at LIMIT 1),
		       to_char(p.check_in_time, 'HH24:MI'), to_char(p.check_out_time, 'HH24:MI'), p.description_am, p.directions, p.directions_am
		FROM properties p LEFT JOIN property_ratings pr ON pr.property_id = p.id
		WHERE p.id = $1 AND p.status = 'active'`, id).
		Scan(&it.ID, &it.Name, &it.Area, &it.City, &it.Type, &verified, &it.Amenities, &rating, &it.ReviewCnt, &it.Lat, &it.Lng,
			&it.description, &it.phone, &it.address, &rules, &coverID, &checkInTime, &checkOutTime, &descAm, &directions, &directionsAm)
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
	pr, err := s.pool.Query(ctx, `SELECT id FROM property_photos WHERE property_id = $1 AND room_type_id IS NULL ORDER BY position, created_at`, it.ID)
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
	if rep, err := s.replyMinutes(ctx, []string{it.ID}); err == nil {
		it.ReplyMinutes = rep[it.ID]
	}
	rooms := rts[it.ID]
	if rooms == nil {
		rooms = []roomType{}
	}
	for i := range rooms {
		if rooms[i].Amenities == nil {
			rooms[i].Amenities = []string{}
		}
		rooms[i].Photos = []string{}
	}
	if err := s.attachRoomPhotos(ctx, rooms); err != nil {
		return err
	}

	type review struct {
		ID     string  `json:"id"`
		Author string  `json:"author"`
		Rating float64 `json:"rating"`
		Text   string  `json:"text"`
		Date   string  `json:"date"`
		// Verified: every review comes from a guest whose stay was booked and finished on BookDan.
		Verified bool `json:"verified"`
		// Reply is the hotel's public answer, if it wrote one.
		Reply *replyOut `json:"reply"`
	}
	revRows, err := s.pool.Query(ctx, `SELECT r.id, coalesce(nullif(split_part(gp.full_name, ' ', 1), ''), nullif(u.display_name, ''), 'Guest'), r.rating, r.text, r.created_at, r.hotel_reply, r.hotel_reply_at
		FROM reviews r JOIN users u ON u.id = r.user_id LEFT JOIN guest_profiles gp ON gp.user_id = r.user_id WHERE r.property_id = $1 ORDER BY r.created_at DESC LIMIT 20`, id)
	if err != nil {
		return err
	}
	defer revRows.Close()
	reviews := []review{}
	for revRows.Next() {
		var rv review
		var at time.Time
		var reply *string
		var replyAt *time.Time
		if err := revRows.Scan(&rv.ID, &rv.Author, &rv.Rating, &rv.Text, &at, &reply, &replyAt); err != nil {
			return err
		}
		rv.Date, rv.Verified = pgDate(at), true
		if reply != nil && replyAt != nil {
			rv.Reply = &replyOut{*reply, pgDate(*replyAt)}
		}
		reviews = append(reviews, rv)
	}

	summary, err := s.ratingSummary(ctx, id)
	if err != nil {
		return err
	}
	marks, err := s.landmarks(ctx)
	if err != nil {
		return err
	}
	nearby := nearLandmarks(marks, it.Lat, it.Lng, 5000, 6)
	if nearby == nil {
		nearby = []placeRef{}
	}

	type propertyOut struct {
		searchItem
		Description  string   `json:"description"`
		Phone        string   `json:"phone"`
		Address      string   `json:"address"`
		HouseRules   any      `json:"house_rules"`
		Photos       []string `json:"photos"`
		CheckInTime  string   `json:"check_in_time"`
		CheckOutTime string   `json:"check_out_time"`
		// The description in Amharic and how to find the hotel in words, in both languages (empty when the hotel wrote none).
		DescriptionAm string `json:"description_am"`
		Directions    string `json:"directions"`
		DirectionsAm  string `json:"directions_am"`
	}
	var houseRules any = []string{}
	if len(rules) > 0 {
		houseRules = jsonRaw(rules)
	}
	// The hotel's number is not on its page: a guest gets it once a booking is confirmed (see calls.go).
	phone := ""
	respond(w, http.StatusOK, struct {
		httpx.Meta
		Property  propertyOut `json:"property"`
		RoomTypes []roomType  `json:"room_types"`
		Reviews   []review    `json:"reviews"`
		// RatingSummary is the average, the count per star, and the averages of the four sub-ratings; Nearby lists landmarks within 5 km.
		RatingSummary ratingSummary `json:"rating_summary"`
		Nearby        []placeRef    `json:"nearby"`
	}{httpx.NowMeta(), propertyOut{it, it.description, phone, it.address, houseRules, photos, checkInTime, checkOutTime, descAm, directions, directionsAm}, rooms, reviews, summary, nearby})
	return nil
}
