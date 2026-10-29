package api

import (
	"context"
	"log/slog"
	"time"
)

// Emails to the guest about their booking, to the address they signed in with (a guest who signed in with a phone number has none, so
// they only get the app's own messages). The confirmation doubles as the receipt: the price, what they booked and the cancellation rule.
// Like every email here it can never fail the action that caused it.

func b2s(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

type guestBooking struct {
	email, code, stayName, hotel, address, phone, inTime, outTime, room, meal, coupon, method, currency string
	checkIn, checkOut                                                                                   time.Time
	rooms, guests, children                                                                             int
	refundable, forOther                                                                                bool
	total, discount, vat                                                                                int64
	refund                                                                                              *int64
}

func (s *Server) loadGuestBooking(ctx context.Context, bookingID string) (*guestBooking, error) {
	var g guestBooking
	err := s.pool.QueryRow(ctx, `SELECT u.email, b.code, b.guest_name, p.name, p.address, p.phone, to_char(p.check_in_time, 'HH24:MI'), to_char(p.check_out_time, 'HH24:MI'),
			rt.name, b.meal, coalesce(c.code, ''), coalesce(b.payment_method, ''), b.currency, b.check_in, b.check_out, b.rooms, b.guests, b.children,
			b.refundable, b.booked_for_other, b.total_santim, b.discount_santim, b.vat_santim, b.refund_santim
		FROM bookings b JOIN users u ON u.id = b.user_id JOIN properties p ON p.id = b.property_id JOIN room_types rt ON rt.id = b.room_type_id
		LEFT JOIN coupons c ON c.id = b.coupon_id WHERE b.id = $1 AND u.email IS NOT NULL`, bookingID).
		Scan(&g.email, &g.code, &g.stayName, &g.hotel, &g.address, &g.phone, &g.inTime, &g.outTime, &g.room, &g.meal, &g.coupon, &g.method, &g.currency,
			&g.checkIn, &g.checkOut, &g.rooms, &g.guests, &g.children, &g.refundable, &g.forOther, &g.total, &g.discount, &g.vat, &g.refund)
	if err != nil {
		return nil, err
	}
	return &g, nil
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return itoa(int64(n)) + " " + many
}

// summary is the block of facts every booking email carries.
func (g *guestBooking) summary() (text, html string) {
	in, out := pgDate(g.checkIn), pgDate(g.checkOut)
	room := g.room
	if g.rooms > 1 {
		room = itoa(int64(g.rooms)) + " × " + g.room
	}
	meal := "Room only"
	if g.meal == mealBreakfast {
		meal = "Breakfast included"
	}
	policy := "Free cancellation until 48 hours before check-in; after that there is no refund."
	if !g.refundable {
		policy = "Non-refundable: no refund once the hotel has confirmed. If the hotel cannot accept the booking, you get everything back."
	}
	lines := [][2]string{
		{"Reference", g.code},
		{"Hotel", g.hotel},
		{"Address", g.address},
		{"Hotel phone", g.phone},
		{"Check-in", in + " from " + g.inTime},
		{"Check-out", out + " until " + g.outTime},
		{"Room", room + " · " + meal},
		{"Guests", plural(g.guests, "guest", "guests")},
	}
	if g.forOther {
		lines = append(lines, [2]string{"Staying", g.stayName})
	}
	if g.coupon != "" && g.discount > 0 {
		lines = append(lines, [2]string{"Coupon " + g.coupon, "saved " + formatETB(g.discount)})
	}
	lines = append(lines, [2]string{"Total", formatETB(g.total) + " (VAT " + formatETB(g.vat) + " included)"}, [2]string{"Cancellation", policy})
	for _, l := range lines {
		text += l[0] + ": " + l[1] + "\n"
		html += `<tr><td style="padding:4px 12px 4px 0;color:#52617a;vertical-align:top">` + esc(l[0]) + `</td><td style="padding:4px 0">` + esc(l[1]) + `</td></tr>`
	}
	return text, `<table style="border-collapse:collapse;font-size:14px">` + html + `</table>`
}

// emailGuest sends the booking email for kind: requested, confirmed, declined, no_response or cancelled.
func (s *Server) emailGuest(ctx context.Context, bookingID, kind string) {
	g, err := s.loadGuestBooking(ctx, bookingID)
	if err != nil {
		return // no email address, or no such booking: nothing to send
	}
	text, table := g.summary()
	var subject, title, intro string
	switch kind {
	case "requested":
		subject, title = "Your booking request at "+g.hotel, "We sent your request to the hotel"
		intro = g.hotel + " answers within 15 minutes. If it cannot take you or does not answer in time, you get everything back automatically."
	case "confirmed":
		subject, title = "Booking confirmed at "+g.hotel, "Your booking is confirmed"
		intro = g.hotel + " has confirmed your room. This email is your receipt. Open the BookDan app to see your voucher with the QR code and PIN: it also works with no internet."
	case "declined":
		subject, title = "Booking not available at "+g.hotel, "The hotel could not accept your booking"
		intro = g.hotel + " could not accept your request. You get a full refund of " + formatETB(g.total) + " to the way you paid."
	case "no_response":
		subject, title = "Booking not confirmed at "+g.hotel, "The hotel did not answer in time"
		intro = g.hotel + " did not answer in time. You get a full refund of " + formatETB(g.total) + " to the way you paid."
	case "hotel_cancelled":
		subject, title = "Your booking at "+g.hotel+" was cancelled by the hotel", "The hotel cancelled your booking"
		intro = g.hotel + " had to cancel your booking and is sorry for it. You get a full refund of " + formatETB(g.total) + " to the way you paid. BookDan support can help you find another place: open Help and support in the app."
	case "needs_relocation":
		subject, title = g.hotel+" cannot host you", "Your hotel cannot host you"
		intro = g.hotel + " told us it cannot host you on your dates. BookDan support is finding you another place and will write to you in the app. You can also cancel the booking now and get a full refund."
	case "dates_changed":
		subject, title = "Dates changed at "+g.hotel, "Your booking has new dates"
		intro = "Your booking at " + g.hotel + " was moved to the new dates below. The total below is the price of the new dates; if it differs from before, the difference was paid or refunded. You can still cancel for free until 48 hours before check-in."
	case "cancelled":
		subject, title = "Booking cancelled at "+g.hotel, "Your booking was cancelled"
		switch {
		case g.refund != nil && *g.refund > 0:
			intro = "Your booking was cancelled. " + formatETB(*g.refund) + " is on its way back to you."
		case g.method == "pay_at_hotel":
			intro = "Your booking was cancelled. You had not paid anything online."
		default:
			intro = "Your booking was cancelled. It was too late for a refund."
		}
	default:
		return
	}
	body := para(intro) + table
	s.sendMail(ctx, g.email, emailContent{subject, intro + "\n\n" + text + "\nBookDan\n", frame(title, body)})
	slog.Debug("guest email", "kind", kind)
}
