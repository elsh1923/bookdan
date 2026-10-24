package api

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5"

	"github.com/bookdan/backend/internal/httpx"
)

// A room is sold as one of up to four options: room only or with breakfast, each with free cancellation or non-refundable.
// The hotel sets, per room type, what breakfast adds per night and how much cheaper the non-refundable option is.

const (
	mealRoomOnly  = "room_only"
	mealBreakfast = "breakfast"
	codeOptionOff = "OPTION_UNAVAILABLE"
)

type rateOption struct {
	Meal       string
	Refundable bool
}

func (o rateOption) valid() bool { return o.Meal == mealRoomOnly || o.Meal == mealBreakfast }

// roomOffer is what a room type offers besides the plain room.
type roomOffer struct {
	breakfast *int64 // santim per room per night
	nrBPS     *int   // discount for no refund, in basis points
}

func (s *Server) roomOffer(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, roomTypeID string) (roomOffer, error) {
	var o roomOffer
	err := q.QueryRow(ctx, `SELECT breakfast_santim, nonrefundable_bps FROM room_types WHERE id = $1`, roomTypeID).Scan(&o.breakfast, &o.nrBPS)
	if err == pgx.ErrNoRows || isBadUUID(err) {
		return o, errNotFound
	}
	return o, err
}

// night prices one night of the room under an option. The base price is the night's rate (with any boost).
func (o roomOffer) night(base int64, opt rateOption) (int64, error) {
	if !opt.valid() {
		return 0, fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "meal must be room_only or breakfast")
	}
	price := base
	if !opt.Refundable {
		if o.nrBPS == nil {
			return 0, fail(http.StatusConflict, codeOptionOff, "This room is not offered as non-refundable")
		}
		price = (base*int64(10000-*o.nrBPS) + 5000) / 10000
	}
	if opt.Meal == mealBreakfast {
		if o.breakfast == nil {
			return 0, fail(http.StatusConflict, codeOptionOff, "Breakfast is not offered with this room")
		}
		price += *o.breakfast
	}
	return price, nil
}

// roomOption is one way to book a room, as shown on the hotel page.
type roomOption struct {
	Meal       string `json:"meal"`
	Refundable bool   `json:"refundable"`
	Price      int64  `json:"price_santim"`
}

// options lists what the room offers, cheapest first within each meal, with the price per night for a base price.
func (o roomOffer) options(base int64) []roomOption {
	out := []roomOption{{mealRoomOnly, true, base}}
	add := func(opt rateOption) {
		if p, err := o.night(base, opt); err == nil {
			out = append(out, roomOption{opt.Meal, opt.Refundable, p})
		}
	}
	add(rateOption{mealRoomOnly, false})
	add(rateOption{mealBreakfast, true})
	add(rateOption{mealBreakfast, false})
	return out
}
