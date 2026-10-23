package api

import (
	"context"
	"log/slog"
	"net/http"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/bookdan/backend/internal/httpx"
)

// Booking alerts: who is told, and when.
//
//	1. The moment a request arrives, the people on duty are alerted. In "everyone" mode that is every person on duty; in
//	   "front desk first" mode it is the receptionists on duty. If nobody in that group can be reached (nobody on duty, or no one
//	   has the app signed in), the next group is used instead: everyone on duty, then the owners.
//	2. "Front desk first" only: if the request is still unanswered after escalate_after_min minutes, the owners are alerted too.
//	   Owners are alerted whatever their shift, because they are the backstop. Switching a person's alerts off is the only way to
//	   keep them out.
//	3. Five minutes before the request runs out, everyone on duty and every owner gets a reminder, and the owners (and the
//	   business contact) get an email, so a missed push does not mean a refunded guest.
//
// Shifts use Addis Ababa time. A person with no shift is always on duty.

const (
	alertEveryone       = "everyone"
	alertFrontDeskFirst = "front_desk_first"
)

// recipient is one phone (or a person with no phone: token empty) that works for the hotel.
type recipient struct {
	role     string
	email    string
	token    string
	alertsOn bool
	onDuty   bool
}

func isOwner(r recipient) bool { return r.role == "owner" }

// firstWave picks who is alerted when a request arrives.
func firstWave(rs []recipient, mode string) []recipient {
	var duty, desk, owners []recipient
	for _, r := range rs {
		if r.token == "" || !r.alertsOn {
			continue
		}
		if r.onDuty {
			duty = append(duty, r)
			if !isOwner(r) {
				desk = append(desk, r)
			}
		}
		if isOwner(r) {
			owners = append(owners, r)
		}
	}
	switch {
	case mode == alertFrontDeskFirst && len(desk) > 0:
		return desk
	case len(duty) > 0:
		return duty
	default:
		return owners
	}
}

// escalationWave picks the owners who are alerted when the front desk has not answered.
func escalationWave(rs []recipient) []recipient {
	var out []recipient
	for _, r := range rs {
		if r.token != "" && r.alertsOn && isOwner(r) {
			out = append(out, r)
		}
	}
	return out
}

// reminderWave picks who is reminded when time is nearly up: everyone on duty and every owner.
func reminderWave(rs []recipient) []recipient {
	var out []recipient
	for _, r := range rs {
		if r.token != "" && r.alertsOn && (r.onDuty || isOwner(r)) {
			out = append(out, r)
		}
	}
	return out
}

func tokensOf(rs []recipient) []string {
	seen := map[string]bool{}
	var out []string
	for _, r := range rs {
		if !seen[r.token] {
			seen[r.token] = true
			out = append(out, r.token)
		}
	}
	return out
}

func hasOwner(rs []recipient) bool {
	for _, r := range rs {
		if isOwner(r) {
			return true
		}
	}
	return false
}

// request is what the alerts say about a booking.
type request struct {
	id, guest, code, hotel, mode, businessEmail string
	people                                      []recipient
}

// onDutySQL is true for a staff row (ps) whose shift covers the current time in Addis Ababa. A shift may run past midnight.
const onDutySQL = `(ps.shift_start IS NULL OR CASE WHEN ps.shift_start < ps.shift_end
		THEN (now() AT TIME ZONE 'Africa/Addis_Ababa')::time >= ps.shift_start AND (now() AT TIME ZONE 'Africa/Addis_Ababa')::time < ps.shift_end
		ELSE (now() AT TIME ZONE 'Africa/Addis_Ababa')::time >= ps.shift_start OR (now() AT TIME ZONE 'Africa/Addis_Ababa')::time < ps.shift_end END)`

func (s *Server) loadRequest(ctx context.Context, bookingID string) (request, error) {
	q := request{id: bookingID}
	if err := s.pool.QueryRow(ctx, `SELECT b.guest_name, b.code, p.name, p.alert_mode, coalesce(hb.contact_email, '')
		FROM bookings b JOIN properties p ON p.id = b.property_id LEFT JOIN hotel_business hb ON hb.property_id = p.id
		WHERE b.id = $1`, bookingID).Scan(&q.guest, &q.code, &q.hotel, &q.mode, &q.businessEmail); err != nil {
		return q, err
	}
	rows, err := s.pool.Query(ctx, `SELECT ps.role, coalesce(u.email, ''), coalesce(d.push_token, ''), ps.alerts_on, `+onDutySQL+`
		FROM bookings b JOIN property_staff ps ON ps.property_id = b.property_id JOIN users u ON u.id = ps.user_id
		LEFT JOIN devices d ON d.user_id = ps.user_id AND d.app = 'partner' AND d.push_token IS NOT NULL AND d.revoked_at IS NULL
		WHERE b.id = $1`, bookingID)
	if err != nil {
		return q, err
	}
	defer rows.Close()
	for rows.Next() {
		var r recipient
		if err := rows.Scan(&r.role, &r.email, &r.token, &r.alertsOn, &r.onDuty); err != nil {
			return q, err
		}
		q.people = append(q.people, r)
	}
	return q, rows.Err()
}

func (s *Server) alertData(q request, typ string) map[string]string {
	return map[string]string{"booking_id": q.id, "type": typ, "channel": "booking_alerts"}
}

// notifyHotel alerts the first wave when a request arrives.
func (s *Server) notifyHotel(ctx context.Context, bookingID string) {
	q, err := s.loadRequest(ctx, bookingID)
	if err != nil {
		slog.Error("notify hotel", "err", err)
		return
	}
	wave := firstWave(q.people, q.mode)
	if len(wave) == 0 {
		slog.Warn("a booking request reached nobody", "booking", bookingID)
		return
	}
	if hasOwner(wave) {
		// The owners are already alerted, so there is nobody left to escalate to.
		if _, err := s.pool.Exec(ctx, `UPDATE bookings SET hotel_escalated_at = now() WHERE id = $1 AND hotel_escalated_at IS NULL`, bookingID); err != nil {
			slog.Error("mark escalated", "err", err)
		}
	}
	s.push(ctx, tokensOf(wave), "New booking request", q.guest+" · "+q.code, s.alertData(q, "new_booking"))
}

// escalate alerts the owners that the front desk has not answered.
func (s *Server) escalate(ctx context.Context, bookingID string) {
	q, err := s.loadRequest(ctx, bookingID)
	if err != nil {
		slog.Error("escalate", "err", err)
		return
	}
	if wave := escalationWave(q.people); len(wave) > 0 {
		s.push(ctx, tokensOf(wave), "Nobody has answered yet", q.guest+" · "+q.code+" is waiting for an answer", s.alertData(q, "new_booking"))
	}
}

// remind warns that time is nearly up and emails the owners.
func (s *Server) remind(ctx context.Context, bookingID string) {
	q, err := s.loadRequest(ctx, bookingID)
	if err != nil {
		slog.Error("remind", "err", err)
		return
	}
	if wave := reminderWave(q.people); len(wave) > 0 {
		s.push(ctx, tokensOf(wave), "5 minutes left to answer", q.guest+" · "+q.code+" will be declined and refunded if nobody answers", s.alertData(q, "new_booking"))
	}
	sent := map[string]bool{}
	for _, to := range append(ownerEmails(q.people), q.businessEmail) {
		if to == "" || sent[to] {
			continue
		}
		sent[to] = true
		s.sendMail(ctx, to, s.requestWaitingEmail(q))
	}
}

func ownerEmails(rs []recipient) []string {
	var out []string
	for _, r := range rs {
		if isOwner(r) && r.alertsOn {
			out = append(out, r.email)
		}
	}
	return out
}

func (s *Server) requestWaitingEmail(q request) emailContent {
	text := "A booking request for " + q.hotel + " has had no answer for 10 minutes.\n\nGuest: " + q.guest + "\nReference: " + q.code +
		"\n\nOpen the BookDan Partner app and accept or decline it. In 5 minutes it is declined and the guest is refunded.\n\nBookDan\n"
	body := para("A booking request for "+q.hotel+" has had no answer for 10 minutes.") + para("Guest: "+q.guest) + para("Reference: "+q.code) +
		para("Open the BookDan Partner app and accept or decline it. In 5 minutes it is declined and the guest is refunded.")
	return emailContent{"Booking request waiting at " + q.hotel + " — 5 minutes left", text, frame("A booking request needs an answer", body)}
}

// stepAlerts finds the waiting requests that have reached an escalation or reminder time and sends each alert once.
// Claiming a request (setting its timestamp) happens before sending, so a slow send is never repeated.
func (s *Server) stepAlerts(ctx context.Context) {
	claim := func(query string) []string {
		rows, err := s.pool.Query(ctx, query)
		if err != nil {
			slog.Error("alert schedule", "err", err)
			return nil
		}
		defer rows.Close()
		var ids []string
		for rows.Next() {
			var id string
			if rows.Scan(&id) == nil {
				ids = append(ids, id)
			}
		}
		return ids
	}
	// A request has hotelWindow minutes (the deadline is paid time + 15 minutes); escalate_after_min counts from when it arrived.
	for _, id := range claim(`UPDATE bookings b SET hotel_escalated_at = now() FROM properties p
		WHERE p.id = b.property_id AND p.alert_mode = 'front_desk_first' AND b.status = 'PAID_AWAITING_HOTEL' AND b.hotel_escalated_at IS NULL
		  AND b.hotel_deadline_at > now() AND now() >= b.hotel_deadline_at - interval '15 minutes' + make_interval(mins => p.escalate_after_min)
		RETURNING b.id`) {
		s.escalate(ctx, id)
	}
	for _, id := range claim(`UPDATE bookings SET hotel_reminded_at = now(), hotel_escalated_at = coalesce(hotel_escalated_at, now())
		WHERE status = 'PAID_AWAITING_HOTEL' AND hotel_reminded_at IS NULL AND hotel_deadline_at > now()
		  AND now() >= hotel_deadline_at - interval '5 minutes'
		RETURNING id`) {
		s.remind(ctx, id)
	}
}

// ---- settings (admin page)

var shiftRE = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)

func (s *Server) adminSetAlertRules(w http.ResponseWriter, r *http.Request) error {
	id := chi.URLParam(r, "id")
	if err := s.canManage(r, id); err != nil {
		return err
	}
	var in struct {
		Mode          string `json:"alert_mode"`
		EscalateAfter int    `json:"escalate_after_min"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if in.Mode != alertEveryone && in.Mode != alertFrontDeskFirst {
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "Choose everyone or front desk first")
	}
	if in.EscalateAfter < 1 || in.EscalateAfter > 10 {
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "Escalate after 1 to 10 minutes")
	}
	err := s.tx(r.Context(), func(tx pgx.Tx) error {
		if _, err := tx.Exec(r.Context(), `UPDATE properties SET alert_mode = $2, escalate_after_min = $3 WHERE id = $1`, id, in.Mode, in.EscalateAfter); err != nil {
			return err
		}
		return s.adminAudit(r, tx, "admin_alert_rules", id, map[string]any{"alert_mode": in.Mode, "escalate_after_min": in.EscalateAfter})
	})
	if err != nil {
		return err
	}
	respond(w, http.StatusOK, httpx.NowMeta())
	return nil
}

// adminSetStaffAlerts switches one person's alerts on or off and sets their shift. A platform admin may change anyone; an owner
// may change their receptionists and themselves, but not another owner.
func (s *Server) adminSetStaffAlerts(w http.ResponseWriter, r *http.Request) error {
	id, user := chi.URLParam(r, "id"), chi.URLParam(r, "user")
	if err := s.canManage(r, id); err != nil {
		return err
	}
	var in struct {
		AlertsOn   bool   `json:"alerts_on"`
		ShiftStart string `json:"shift_start"`
		ShiftEnd   string `json:"shift_end"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	in.ShiftStart, in.ShiftEnd = strings.TrimSpace(in.ShiftStart), strings.TrimSpace(in.ShiftEnd)
	var start, end *string
	if in.ShiftStart != "" || in.ShiftEnd != "" {
		if !shiftRE.MatchString(in.ShiftStart) || !shiftRE.MatchString(in.ShiftEnd) {
			return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "Give both shift times as HH:MM, or leave both empty for always on duty")
		}
		if in.ShiftStart == in.ShiftEnd {
			return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "A shift cannot start and end at the same time")
		}
		start, end = &in.ShiftStart, &in.ShiftEnd
	}
	c := claimsFrom(r)
	err := s.tx(r.Context(), func(tx pgx.Tx) error {
		var role string
		err := tx.QueryRow(r.Context(), `SELECT role FROM property_staff WHERE property_id = $1 AND user_id = $2 FOR UPDATE`, id, user).Scan(&role)
		if err == pgx.ErrNoRows || isBadUUID(err) {
			return errNotFound
		}
		if err != nil {
			return err
		}
		if c.Role != "admin" && role == "owner" && user != c.Sub {
			return fail(http.StatusForbidden, codeForbidden, "Only a platform admin can change another owner")
		}
		if _, err := tx.Exec(r.Context(), `UPDATE property_staff SET alerts_on = $3, shift_start = $4::time, shift_end = $5::time WHERE property_id = $1 AND user_id = $2`,
			id, user, in.AlertsOn, start, end); err != nil {
			return err
		}
		return s.adminAudit(r, tx, "admin_staff_alerts", id, map[string]any{"user_id": user, "alerts_on": in.AlertsOn, "shift_start": in.ShiftStart, "shift_end": in.ShiftEnd})
	})
	if err != nil {
		return err
	}
	respond(w, http.StatusOK, httpx.NowMeta())
	return nil
}
