package api

import (
	"context"
	"sort"
	"time"
)

// Other dates that work. When a hotel cannot sell the whole stay the guest asked for, it may still sell part of it, or the same
// number of nights a day or two earlier or later. The server works these out from the same stock the booking uses, so a suggestion
// is never offered for a night the hotel cannot sell. A suggestion holds nothing: the quote and the hold check every night again.
//
// The rules, in order of preference:
//   - shorter stays inside the requested dates (the longest first, the ones nearest the requested dates first);
//   - the same number of nights, shifted by up to altShiftDays;
//   - never a night before today (Addis Ababa time), never a stay the party cannot fit into a room, never a night without a rate;
//   - no suggestion contained in another one we already show.

const (
	altShiftDays   = 2  // how far before and after the requested dates we look for the same number of nights
	altPerHotel    = 3  // at most this many suggestions for one hotel
	altHotelsShown = 10 // at most this many hotels in the "other dates" list
	altCandidates  = 40 // sold-out hotels we work out suggestions for, per search
)

// altWindow is a stay the guest could book instead.
type altWindow struct {
	CheckIn   string `json:"check_in"`
	CheckOut  string `json:"check_out"` // the day the guest leaves (not a night)
	Nights    int    `json:"nights"`
	FromPrice int64  `json:"from_price_santim"` // the cheapest nightly price of a room that fits, for the first night
	Currency  string `json:"currency"`
}

// dayStock is one room type on one night: how many rooms are free, and the nightly price. ok is false when the night cannot be sold
// (no rate covers it).
type dayStock struct {
	free  int
	price int64
	cur   string
	ok    bool
}

type roomStock struct {
	capacity int
	days     []dayStock // days[i] is the night base+i
}

// hotelStock is every room type of one hotel over the days we look at.
type hotelStock struct {
	base  time.Time // the date of days[0] of every room
	rooms []roomStock
}

// windowPrice reports the cheapest room that can take the party on every night of days[from:to], and its price for the first night.
func (h hotelStock) windowPrice(from, to, guests, rooms int) (int64, string, bool) {
	if from < 0 || from >= to {
		return 0, "", false
	}
	var best int64
	var cur string
	found := false
	for _, r := range h.rooms {
		if r.capacity*rooms < guests || to > len(r.days) {
			continue
		}
		fits := true
		for _, d := range r.days[from:to] {
			if !d.ok || d.free < rooms {
				fits = false
				break
			}
		}
		if !fits {
			continue
		}
		if p := r.days[from]; !found || p.price < best {
			best, cur, found = p.price, p.cur, true
		}
	}
	return best, cur, found
}

// findWindows returns the stays near the requested one that the hotel can sell. The requested stay is the nights [req, req+length) of
// the stock; todayIdx is the first night that can still be booked (it may be below zero, meaning every night we look at is open).
func findWindows(h hotelStock, req, length, todayIdx, guests, rooms int) []altWindow {
	var cands []windowCand
	// Shorter stays inside the request.
	for n := length - 1; n >= 1; n-- {
		for st := req; st+n <= req+length; st++ {
			cands = append(cands, windowCand{from: st, to: st + n, dist: abs(st - req)})
		}
	}
	// The same number of nights, a day or two before or after.
	for d := -altShiftDays; d <= altShiftDays; d++ {
		if d != 0 {
			cands = append(cands, windowCand{from: req + d, to: req + d + length, dist: abs(d)})
		}
	}

	// Keep only what can be sold, and tell the guest the price.
	lowest := todayIdx
	if lowest < 0 {
		lowest = 0
	}
	ok := cands[:0]
	for _, c := range cands {
		if c.from < lowest {
			continue
		}
		price, cur, sellable := h.windowPrice(c.from, c.to, guests, rooms)
		if !sellable {
			continue
		}
		c.price, c.cur = price, cur
		ok = append(ok, c)
	}
	sort.SliceStable(ok, func(i, j int) bool {
		a, b := ok[i], ok[j]
		if la, lb := a.to-a.from, b.to-b.from; la != lb {
			return la > lb // longer first
		}
		if a.dist != b.dist {
			return a.dist < b.dist // nearer the requested dates first
		}
		return a.from < b.from
	})

	var kept []windowCand
	for _, c := range ok {
		if len(kept) == altPerHotel {
			break
		}
		if containedInAny(c.from, c.to, kept) {
			continue
		}
		kept = append(kept, c)
	}

	out := make([]altWindow, 0, len(kept))
	for _, c := range kept {
		out = append(out, altWindow{
			CheckIn:   dayAt(c.from, h.base),
			CheckOut:  dayAt(c.to, h.base),
			Nights:    c.to - c.from,
			FromPrice: c.price,
			Currency:  c.cur,
		})
	}
	return out
}

// windowCand is a candidate stay, as day indexes [from, to).
type windowCand struct {
	from, to, dist int
	price          int64
	cur            string
}

func containedInAny(from, to int, kept []windowCand) bool {
	for _, k := range kept {
		if k.from <= from && to <= k.to {
			return true
		}
	}
	return false
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// dayAt is the calendar date of day index i, counted from base.
func dayAt(i int, base time.Time) string { return base.AddDate(0, 0, i).Format("2006-01-02") }

// dayIndex is how many days d is after base (negative when before).
func dayIndex(d, base time.Time) int { return int(d.Sub(base).Round(time.Hour).Hours() / 24) }

// hotelStocks loads the stock of the given hotels for the days [base, end): every room type, its rooms free on each night, and the
// nightly rate. A night with no inventory row has all rooms free (the same rule the search uses for rooms left).
func (s *Server) hotelStocks(ctx context.Context, propertyIDs []string, base, end time.Time) (map[string]hotelStock, error) {
	span := dayIndex(end, base)
	out := map[string]hotelStock{}
	roomProp := map[string]string{} // room type id -> property id
	roomIdx := map[string]int{}     // room type id -> index in its hotel's rooms

	rows, err := s.pool.Query(ctx, `SELECT rt.property_id, rt.id, rt.capacity, rt.total_rooms FROM room_types rt WHERE rt.property_id = ANY($1::uuid[]) ORDER BY rt.id`, propertyIDs)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var prop, id string
		var capacity, total int
		if err := rows.Scan(&prop, &id, &capacity, &total); err != nil {
			rows.Close()
			return nil, err
		}
		h := out[prop]
		days := make([]dayStock, span)
		for i := range days {
			days[i] = dayStock{free: total}
		}
		roomProp[id], roomIdx[id] = prop, len(h.rooms)
		h.rooms = append(h.rooms, roomStock{capacity: capacity, days: days})
		out[prop] = h
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	ids := make([]string, 0, len(roomProp))
	for id := range roomProp {
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return out, nil
	}

	inv, err := s.pool.Query(ctx, `SELECT i.room_type_id, i.night::text, i.capacity, i.held, i.booked FROM inventory i
		WHERE i.room_type_id = ANY($1::uuid[]) AND i.night >= $2::date AND i.night < $3::date`, ids, pgDate(base), pgDate(end))
	if err != nil {
		return nil, err
	}
	for inv.Next() {
		var id, night string
		var capacity, held, booked int
		if err := inv.Scan(&id, &night, &capacity, &held, &booked); err != nil {
			inv.Close()
			return nil, err
		}
		d, err := time.Parse("2006-01-02", night)
		if err != nil {
			continue
		}
		i := dayIndex(d, base)
		h := out[roomProp[id]]
		if i >= 0 && i < span {
			h.rooms[roomIdx[id]].days[i].free = capacity - held - booked
		}
	}
	inv.Close()
	if err := inv.Err(); err != nil {
		return nil, err
	}

	rates, err := s.pool.Query(ctx, `SELECT rp.room_type_id, rp.valid_from::text, rp.valid_to::text, rp.nightly_price_santim, rp.currency
		FROM rate_plans rp WHERE rp.room_type_id = ANY($1::uuid[]) AND rp.valid_to >= $2::date AND rp.valid_from < $3::date
		ORDER BY rp.valid_from, rp.id`, ids, pgDate(base), pgDate(end))
	if err != nil {
		return nil, err
	}
	for rates.Next() {
		var id, from, until, cur string
		var price int64
		if err := rates.Scan(&id, &from, &until, &price, &cur); err != nil {
			rates.Close()
			return nil, err
		}
		f, err1 := time.Parse("2006-01-02", from)
		t, err2 := time.Parse("2006-01-02", until)
		if err1 != nil || err2 != nil {
			continue
		}
		h := out[roomProp[id]]
		for i := dayIndex(f, base); i <= dayIndex(t, base); i++ {
			if i < 0 || i >= span {
				continue
			}
			day := &h.rooms[roomIdx[id]].days[i]
			if !day.ok { // the first rate plan that covers a night wins, as the search does
				day.price, day.cur, day.ok = price, cur, true
			}
		}
	}
	rates.Close()
	if err := rates.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// otherDates adds the stays a sold-out hotel can still offer to the candidates (the first altHotelsShown hotels that have one).
// Hotels that are free for the whole stay are not touched.
func (s *Server) otherDates(ctx context.Context, items []searchItem, checkIn, checkOut string, guests, rooms int) ([]searchItem, error) {
	in, err := time.Parse("2006-01-02", checkIn)
	if err != nil {
		return nil, nil
	}
	nights := nightsBetween(checkIn, checkOut)
	base := in.AddDate(0, 0, -altShiftDays)
	end := in.AddDate(0, 0, nights+altShiftDays)
	today, err := time.Parse("2006-01-02", s.addisToday())
	if err != nil {
		return nil, err
	}
	todayIdx := dayIndex(today, base)

	var ids []string
	var candidates []searchItem
	for _, it := range items {
		if it.Available {
			continue
		}
		candidates = append(candidates, it)
		ids = append(ids, it.ID)
		if len(candidates) == altCandidates {
			break
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	stocks, err := s.hotelStocks(ctx, ids, base, end)
	if err != nil {
		return nil, err
	}

	var out []searchItem
	for _, it := range candidates {
		stock := stocks[it.ID]
		stock.base = base
		windows := findWindows(stock, altShiftDays, nights, todayIdx, guests, rooms)
		if len(windows) == 0 {
			continue
		}
		it.Windows = windows
		out = append(out, it)
		if len(out) == altHotelsShown {
			break
		}
	}
	return out, nil
}
