package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/bookdan/backend/internal/httpx"
)

// The hotel's rooms chart, and the rooms the hotel takes out of sale itself.
//
// A guest who walks in, or rooms blocked for repairs or an event, take rooms from the same inventory as bookings made in the app
// (inventory.booked). So a room can never be sold twice, and the guest app stops offering a room the moment the hotel takes it.
// room_blocks remembers why the rooms are taken, so the hotel can see it and give the rooms back.

const (
	maxChartDays   = 31
	maxBlockNights = 60
)

type chartNight struct {
	Date     string `json:"date"`
	Capacity int    `json:"capacity"`
	// Booked counts rooms of guests who booked in the app; Held rooms are being paid for right now; Blocked are the hotel's own walk-ins and blocks.
	Booked  int `json:"booked"`
	Held    int `json:"held"`
	Blocked int `json:"blocked"`
	Free    int `json:"free"`
}

type chartRoom struct {
	ID         string       `json:"id"`
	Name       string       `json:"name"`
	TotalRooms int          `json:"total_rooms"`
	Nights     []chartNight `json:"nights"`
}

type hotelRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// partnerHotel picks which of the staff member's hotels a request is about (the first one when none is named).
func (s *Server) partnerHotel(r *http.Request, named string) (id, name string, all []hotelRef, err error) {
	ids, names, err := s.staffHotels(r.Context(), claimsFrom(r).Sub)
	if err != nil {
		return "", "", nil, err
	}
	for i := range ids {
		all = append(all, hotelRef{ids[i], names[i]})
	}
	if named == "" {
		return ids[0], names[0], all, nil
	}
	for i := range ids {
		if ids[i] == named {
			return ids[i], names[i], all, nil
		}
	}
	return "", "", nil, errNotFound
}

func (s *Server) partnerRoomsChart(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	q := r.URL.Query()
	propID, propName, all, err := s.partnerHotel(r, q.Get("property_id"))
	if err != nil {
		return err
	}
	from := q.Get("from")
	if from == "" {
		from = s.addisToday()
	}
	days := atoiDefault(q.Get("days"), 14)
	if !isDate(from) || days < 1 || days > maxChartDays {
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "from is a date (YYYY-MM-DD) and days is 1 to 31")
	}
	to := addDaysISO(from, days)

	rows, err := s.pool.Query(ctx, `SELECT id, name, total_rooms FROM room_types WHERE property_id = $1 ORDER BY name`, propID)
	if err != nil {
		return err
	}
	var rooms []chartRoom
	idx := map[string]int{}
	for rows.Next() {
		var cr chartRoom
		if err := rows.Scan(&cr.ID, &cr.Name, &cr.TotalRooms); err != nil {
			rows.Close()
			return err
		}
		idx[cr.ID] = len(rooms)
		rooms = append(rooms, cr)
	}
	rows.Close()

	type inv struct{ capacity, held, booked int }
	invs := map[string]inv{} // room|night
	ir, err := s.pool.Query(ctx, `SELECT i.room_type_id::text, i.night, i.capacity, i.held, i.booked FROM inventory i JOIN room_types rt ON rt.id = i.room_type_id
		WHERE rt.property_id = $1 AND i.night >= $2::date AND i.night < $3::date`, propID, from, to)
	if err != nil {
		return err
	}
	for ir.Next() {
		var room string
		var night time.Time
		var v inv
		if err := ir.Scan(&room, &night, &v.capacity, &v.held, &v.booked); err != nil {
			ir.Close()
			return err
		}
		invs[room+"|"+pgDate(night)] = v
	}
	ir.Close()

	blocked := map[string]int{}
	br, err := s.pool.Query(ctx, `SELECT room_type_id::text, check_in, check_out, rooms FROM room_blocks
		WHERE property_id = $1 AND released_at IS NULL AND check_out > $2::date AND check_in < $3::date`, propID, from, to)
	if err != nil {
		return err
	}
	for br.Next() {
		var room string
		var in, out time.Time
		var n int
		if err := br.Scan(&room, &in, &out, &n); err != nil {
			br.Close()
			return err
		}
		for d := pgDate(in); d < pgDate(out); d = addDaysISO(d, 1) {
			blocked[room+"|"+d] += n
		}
	}
	br.Close()

	for i := range rooms {
		for d := 0; d < days; d++ {
			date := addDaysISO(from, d)
			key := rooms[i].ID + "|" + date
			v, ok := invs[key]
			if !ok { // no row yet: nothing has been sold that night
				v = inv{capacity: rooms[i].TotalRooms}
			}
			b := blocked[key]
			guests := v.booked - b
			if guests < 0 {
				guests = 0
			}
			rooms[i].Nights = append(rooms[i].Nights, chartNight{date, v.capacity, guests, v.held, b, v.capacity - v.held - v.booked})
		}
	}
	if rooms == nil {
		rooms = []chartRoom{}
	}
	respond(w, http.StatusOK, struct {
		httpx.Meta
		Hotels   []hotelRef  `json:"hotels"`
		Property hotelRef    `json:"property"`
		From     string      `json:"from"`
		Days     int         `json:"days"`
		Rooms    []chartRoom `json:"rooms"`
	}{httpx.NowMeta(), all, hotelRef{propID, propName}, from, days, rooms})
	return nil
}

func addDaysISO(iso string, n int) string {
	t, _ := time.Parse("2006-01-02", iso)
	return t.AddDate(0, 0, n).Format("2006-01-02")
}

type blockOut struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	RoomID    string `json:"room_type_id"`
	RoomName  string `json:"room_name"`
	CheckIn   string `json:"check_in"`
	CheckOut  string `json:"check_out"`
	Rooms     int    `json:"rooms"`
	GuestName string `json:"guest_name"`
	Phone     string `json:"guest_phone"`
	Note      string `json:"note"`
}

// partnerBlocks lists the walk-ins and blocks that are still on (not given back, not over).
func (s *Server) partnerBlocks(w http.ResponseWriter, r *http.Request) error {
	propID, _, _, err := s.partnerHotel(r, r.URL.Query().Get("property_id"))
	if err != nil {
		return err
	}
	rows, err := s.pool.Query(r.Context(), `SELECT b.id, b.kind, b.room_type_id, rt.name, b.check_in, b.check_out, b.rooms, coalesce(b.guest_name, ''), coalesce(b.guest_phone, ''), coalesce(b.note, '')
		FROM room_blocks b JOIN room_types rt ON rt.id = b.room_type_id
		WHERE b.property_id = $1 AND b.released_at IS NULL AND b.check_out >= $2::date ORDER BY b.check_in, b.created_at`, propID, s.addisToday())
	if err != nil {
		return err
	}
	defer rows.Close()
	out := []blockOut{}
	for rows.Next() {
		var b blockOut
		var in, o time.Time
		if err := rows.Scan(&b.ID, &b.Kind, &b.RoomID, &b.RoomName, &in, &o, &b.Rooms, &b.GuestName, &b.Phone, &b.Note); err != nil {
			return err
		}
		b.CheckIn, b.CheckOut = pgDate(in), pgDate(o)
		out = append(out, b)
	}
	respond(w, http.StatusOK, struct {
		httpx.Meta
		Blocks []blockOut `json:"blocks"`
	}{httpx.NowMeta(), out})
	return rows.Err()
}

// partnerCreateBlock records a walk-in guest or blocks rooms: the rooms are taken for every night, or none of them (SOLD_OUT).
func (s *Server) partnerCreateBlock(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		PropertyID string `json:"property_id"`
		RoomTypeID string `json:"room_type_id"`
		CheckIn    string `json:"check_in"`
		CheckOut   string `json:"check_out"`
		Rooms      int    `json:"rooms"`
		Kind       string `json:"kind"`
		GuestName  string `json:"guest_name"`
		GuestPhone string `json:"guest_phone"`
		Note       string `json:"note"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	propID, _, _, err := s.partnerHotel(r, in.PropertyID)
	if err != nil {
		return err
	}
	bad := func(msg string) error { return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, msg) }
	in.GuestName, in.GuestPhone, in.Note = strings.TrimSpace(in.GuestName), strings.TrimSpace(in.GuestPhone), strings.TrimSpace(in.Note)
	nights := 0
	if isDate(in.CheckIn) && isDate(in.CheckOut) {
		nights = nightsBetween(in.CheckIn, in.CheckOut)
	}
	switch {
	case in.Kind != "walk_in" && in.Kind != "block":
		return bad("kind must be walk_in or block")
	case !isDate(in.CheckIn) || !isDate(in.CheckOut) || nights < 1 || nights > maxBlockNights:
		return bad("Give the first and last night as dates, between 1 and 60 nights")
	case in.CheckIn < s.addisToday():
		return bad("The first night cannot be in the past")
	case in.Rooms < 1 || in.Rooms > 500:
		return bad("Choose how many rooms")
	case in.Kind == "walk_in" && (len(in.GuestName) < 2 || len(in.GuestName) > 120):
		return bad("Give the guest's name")
	case in.GuestPhone != "" && !phoneAnyRE.MatchString(in.GuestPhone):
		return bad("Give the phone number as digits")
	case len(in.Note) > 300:
		return bad("The note is at most 300 characters")
	}
	var id string
	err = s.tx(r.Context(), func(tx pgx.Tx) error {
		ctx := r.Context()
		var ok bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM room_types WHERE id = $1 AND property_id = $2)`, in.RoomTypeID, propID).Scan(&ok); err != nil {
			if isBadUUID(err) {
				return errNotFound
			}
			return err
		}
		if !ok {
			return errNotFound
		}
		if err := ensureInventory(ctx, tx, in.RoomTypeID, in.CheckIn, in.CheckOut); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `UPDATE inventory SET booked = booked + $4
			WHERE room_type_id = $1 AND night >= $2::date AND night < $3::date AND held + booked + $4 <= capacity`, in.RoomTypeID, in.CheckIn, in.CheckOut, in.Rooms)
		if err != nil {
			return err
		}
		if int(tag.RowsAffected()) != nights {
			return fail(http.StatusConflict, codeSoldOut, "There are not enough free rooms on every one of those nights")
		}
		if err := tx.QueryRow(ctx, `INSERT INTO room_blocks (property_id, room_type_id, check_in, check_out, rooms, kind, guest_name, guest_phone, note, created_by)
			VALUES ($1, $2, $3::date, $4::date, $5, $6, NULLIF($7, ''), NULLIF($8, ''), NULLIF($9, ''), $10) RETURNING id`,
			propID, in.RoomTypeID, in.CheckIn, in.CheckOut, in.Rooms, in.Kind, in.GuestName, in.GuestPhone, in.Note, claimsFrom(r).Sub).Scan(&id); err != nil {
			return err
		}
		return s.audit(r, tx, "rooms_"+in.Kind, map[string]any{"block_id": id, "property_id": propID, "room_type_id": in.RoomTypeID, "rooms": in.Rooms, "check_in": in.CheckIn, "check_out": in.CheckOut})
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

// partnerReleaseBlock gives the rooms back (a guest left early, or the repairs are done). Giving back twice is harmless.
func (s *Server) partnerReleaseBlock(w http.ResponseWriter, r *http.Request) error {
	id := chi.URLParam(r, "id")
	ids, _, err := s.staffHotels(r.Context(), claimsFrom(r).Sub)
	if err != nil {
		return err
	}
	err = s.tx(r.Context(), func(tx pgx.Tx) error {
		ctx := r.Context()
		var room string
		var in, out time.Time
		var rooms int
		var released *time.Time
		err := tx.QueryRow(ctx, `SELECT room_type_id::text, check_in, check_out, rooms, released_at FROM room_blocks WHERE id = $1 AND property_id = ANY($2::uuid[]) FOR UPDATE`, id, ids).
			Scan(&room, &in, &out, &rooms, &released)
		if err == pgx.ErrNoRows || isBadUUID(err) {
			return errNotFound
		}
		if err != nil {
			return err
		}
		if released != nil {
			return nil
		}
		if _, err := tx.Exec(ctx, `UPDATE inventory SET booked = greatest(booked - $4, 0) WHERE room_type_id = $1 AND night >= $2::date AND night < $3::date`, room, pgDate(in), pgDate(out), rooms); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE room_blocks SET released_at = now(), released_by = $2 WHERE id = $1`, id, claimsFrom(r).Sub); err != nil {
			return err
		}
		return s.audit(r, tx, "rooms_released", map[string]any{"block_id": id})
	})
	if err != nil {
		return err
	}
	respond(w, http.StatusOK, httpx.NowMeta())
	return nil
}
