package api

import (
	"net/http"

	"github.com/bookdan/backend/internal/httpx"
)

// Bookings open this many days ahead: the room rates are published for a year, so a stay beyond it has no price.
const maxAdvanceDays = 365

// searchStay reads the stay for a search. The dates are optional: with none, hotels come back with a "from" price for tonight and no
// claim about rooms (datesKnown is false). Dates that are given must be today or later (Addis Ababa time) and within maxAdvanceDays.
func (s *Server) searchStay(r *http.Request) (checkIn, checkOut string, guests, rooms int, datesKnown bool, err error) {
	q := r.URL.Query()
	today := s.addisToday()
	if q.Get("check_in") == "" && q.Get("check_out") == "" {
		guests, rooms = atoiDefault(q.Get("guests"), 1), atoiDefault(q.Get("rooms"), 1)
		if guests < 1 || guests > 40 || rooms < 1 || rooms > maxRooms {
			return "", "", 0, 0, false, fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "Choose 1 to 9 rooms")
		}
		return today, addDaysISO(today, 1), guests, rooms, false, nil
	}
	checkIn, checkOut, guests, rooms, err = s.stay(r)
	if err != nil {
		return "", "", 0, 0, false, err
	}
	if checkIn < today {
		return "", "", 0, 0, false, fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "Check-in cannot be in the past")
	}
	if nightsBetween(today, checkIn) > maxAdvanceDays {
		return "", "", 0, 0, false, fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "Bookings open up to 365 days ahead")
	}
	return checkIn, checkOut, guests, rooms, true, nil
}
