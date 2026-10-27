package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/bookdan/backend/internal/httpx"
)

// Hotel self-signup. A hotel applies on the /apply page (or any client of POST /v1/hotel-applications); an admin reviews the application.
// Approving creates a DRAFT hotel (nothing is public) and invites the applicant by email as its owner; the admin then completes the hotel (map
// pin, rooms, prices, photos, business and payout details) and publishes it with the usual checks. Rejecting emails the applicant the reason.
//
// The endpoint is open to anyone, so it is guarded: a hidden field that only bots fill (the application is dropped, the bot is told it worked), at most
// 5 applications per address per hour, and at most 2 waiting applications per email.

const (
	applicationsPerIPPerHour = 5
	pendingPerEmail          = 2
	// A hotel the admin approves starts in Addis Ababa's centre until its owner or the admin moves the pin.
	defaultLat, defaultLng = 9.0108, 38.7613
)

type applicationIn struct {
	HotelName     string `json:"hotel_name"`
	HotelType     string `json:"hotel_type"`
	City          string `json:"city"`
	Area          string `json:"area"`
	Address       string `json:"address"`
	HotelPhone    string `json:"hotel_phone"`
	Description   string `json:"description"`
	RoomsCount    int    `json:"rooms_count"`
	PayAtHotel    bool   `json:"pay_at_hotel"`
	OwnerName     string `json:"owner_name"`
	OwnerEmail    string `json:"owner_email"`
	OwnerPhone    string `json:"owner_phone"`
	LegalName     string `json:"legal_name"`
	LicenseNumber string `json:"license_number"`
	// Website is a trap: people never see it, bots fill every field.
	Website string `json:"website"`
}

func appReference(id string) string {
	return "HA-" + strings.ToUpper(strings.ReplaceAll(id, "-", "")[:8])
}

func (s *Server) submitApplication(w http.ResponseWriter, r *http.Request) error {
	var in applicationIn
	if err := decode(r, &in); err != nil {
		return err
	}
	if strings.TrimSpace(in.Website) != "" {
		// A bot: say it worked and keep nothing.
		respond(w, http.StatusOK, struct {
			httpx.Meta
			Reference string `json:"reference"`
		}{httpx.NowMeta(), "HA-00000000"})
		return nil
	}
	bad := func(m string) error { return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, m) }
	var ok bool
	if in.HotelName, ok = trimLen(in.HotelName, 2, 120); !ok {
		return bad("The hotel's name is required (2 to 120 characters)")
	}
	if !hotelTypes[in.HotelType] {
		return bad("Type must be hotel, guesthouse, lodge or resort")
	}
	if in.City, ok = trimLen(in.City, 2, 80); !ok {
		return bad("The city is required")
	}
	if in.Area, ok = trimLen(in.Area, 2, 120); !ok {
		return bad("The area or neighbourhood is required")
	}
	if in.Address, ok = trimLen(in.Address, 2, 200); !ok {
		return bad("The street address is required")
	}
	in.HotelPhone = strings.TrimSpace(in.HotelPhone)
	if !phoneAnyRE.MatchString(in.HotelPhone) {
		return bad("Enter the hotel's phone number")
	}
	if in.Description, ok = trimLen(in.Description, 0, 2000); !ok {
		return bad("The description is at most 2000 characters")
	}
	if in.RoomsCount < 1 || in.RoomsCount > 1000 {
		return bad("Say how many rooms the hotel has (1 to 1000)")
	}
	if in.OwnerName, ok = trimLen(in.OwnerName, 2, 120); !ok {
		return bad("The owner's name is required")
	}
	email, _, err := identity{Type: "email", Value: strings.TrimSpace(in.OwnerEmail)}.clean()
	if err != nil {
		return bad("Enter the owner's email address: that is how they sign in")
	}
	in.OwnerPhone = strings.TrimSpace(in.OwnerPhone)
	if !phoneAnyRE.MatchString(in.OwnerPhone) {
		return bad("Enter the owner's phone number")
	}
	if in.LegalName, ok = trimLen(in.LegalName, 2, 160); !ok {
		return bad("The legal name of the business is required")
	}
	if in.LicenseNumber, ok = trimLen(in.LicenseNumber, 2, 80); !ok {
		return bad("The tourism licence number is required")
	}

	ctx, ip := r.Context(), clientIP(r)
	var id string
	err = s.tx(ctx, func(tx pgx.Tx) error {
		var recent, waiting int
		if err := tx.QueryRow(ctx, `SELECT count(*)::int FROM hotel_applications WHERE ip = NULLIF($1, '')::inet AND created_at > now() - interval '1 hour'`, ip).Scan(&recent); err != nil {
			return err
		}
		if recent >= applicationsPerIPPerHour {
			return fail(http.StatusTooManyRequests, codeTooMany, "Too many applications from this connection. Try again in an hour.")
		}
		if err := tx.QueryRow(ctx, `SELECT count(*)::int FROM hotel_applications WHERE lower(owner_email) = $1 AND status = 'pending'`, email.Value).Scan(&waiting); err != nil {
			return err
		}
		if waiting >= pendingPerEmail {
			return fail(http.StatusConflict, "APPLICATION_WAITING", "You already have applications waiting for a decision. We will email you.")
		}
		return tx.QueryRow(ctx, `INSERT INTO hotel_applications (hotel_name, hotel_type, city, area, address, hotel_phone, description, rooms_count, pay_at_hotel,
				owner_name, owner_email, owner_phone, legal_name, license_number, ip)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, NULLIF($15, '')::inet) RETURNING id`,
			in.HotelName, in.HotelType, in.City, in.Area, in.Address, in.HotelPhone, in.Description, in.RoomsCount, in.PayAtHotel,
			in.OwnerName, email.Value, in.OwnerPhone, in.LegalName, in.LicenseNumber, ip).Scan(&id)
	})
	if err != nil {
		return err
	}
	ref := appReference(id)
	text := fmt.Sprintf("Thank you, %s. We received the application of %s to be listed on BookDan (reference %s).\n\nWe review every application, usually within 3 working days, and will email you at this address with our decision. "+
		"If we approve it, you get an invitation to the BookDan Partner app and we help you add your rooms, prices and photos.\n\nBookDan\n", in.OwnerName, in.HotelName, ref)
	s.sendMail(ctx, email.Value, emailContent{"We received your application: " + in.HotelName, text,
		frame("We received your application", para("Thank you, "+in.OwnerName+". We received the application of "+in.HotelName+" to be listed on BookDan (reference "+ref+").")+
			para("We review every application, usually within 3 working days, and will email you at this address with our decision. If we approve it, you get an invitation to the BookDan Partner app and we help you add your rooms, prices and photos."))})
	for _, to := range s.supportRecipients() {
		_ = s.p.Mail.SendMail(ctx, to, "New hotel application: "+in.HotelName, fmt.Sprintf("%s (%s) in %s, %s applied to be listed.\nOwner: %s <%s>\nReference %s\n\nOpen the admin page, Applications, to review it.", in.HotelName, in.HotelType, in.Area, in.City, in.OwnerName, email.Value, ref), "")
	}
	respond(w, http.StatusOK, struct {
		httpx.Meta
		Reference string `json:"reference"`
	}{httpx.NowMeta(), ref})
	return nil
}

type applicationOut struct {
	ID            string  `json:"id"`
	Reference     string  `json:"reference"`
	Status        string  `json:"status"`
	HotelName     string  `json:"hotel_name"`
	HotelType     string  `json:"hotel_type"`
	City          string  `json:"city"`
	Area          string  `json:"area"`
	Address       string  `json:"address"`
	HotelPhone    string  `json:"hotel_phone"`
	Description   string  `json:"description"`
	RoomsCount    int     `json:"rooms_count"`
	PayAtHotel    bool    `json:"pay_at_hotel"`
	OwnerName     string  `json:"owner_name"`
	OwnerEmail    string  `json:"owner_email"`
	OwnerPhone    string  `json:"owner_phone"`
	LegalName     string  `json:"legal_name"`
	LicenseNumber string  `json:"license_number"`
	CreatedAt     string  `json:"created_at"`
	DecidedAt     *string `json:"decided_at"`
	DecidedBy     *string `json:"decided_by"`
	DecisionNote  *string `json:"decision_note"`
	PropertyID    *string `json:"property_id"`
}

const applicationCols = `id, status, hotel_name, hotel_type, city, area, address, hotel_phone, description, rooms_count, pay_at_hotel, owner_name, owner_email, owner_phone,
	legal_name, license_number, created_at, decided_at, decided_by, decision_note, property_id::text`

func scanApplication(row pgx.Row) (applicationOut, error) {
	var a applicationOut
	var created time.Time
	var decided *time.Time
	if err := row.Scan(&a.ID, &a.Status, &a.HotelName, &a.HotelType, &a.City, &a.Area, &a.Address, &a.HotelPhone, &a.Description, &a.RoomsCount, &a.PayAtHotel,
		&a.OwnerName, &a.OwnerEmail, &a.OwnerPhone, &a.LegalName, &a.LicenseNumber, &created, &decided, &a.DecidedBy, &a.DecisionNote, &a.PropertyID); err != nil {
		return a, err
	}
	a.Reference = appReference(a.ID)
	a.CreatedAt = created.UTC().Format(time.RFC3339)
	if decided != nil {
		t := decided.UTC().Format(time.RFC3339)
		a.DecidedAt = &t
	}
	return a, nil
}

func (s *Server) adminApplications(w http.ResponseWriter, r *http.Request) error {
	if err := requireAdmin(r); err != nil {
		return err
	}
	rows, err := s.pool.Query(r.Context(), `SELECT `+applicationCols+` FROM hotel_applications ORDER BY (status <> 'pending'), created_at DESC LIMIT 200`)
	if err != nil {
		return err
	}
	defer rows.Close()
	out := []applicationOut{}
	for rows.Next() {
		a, err := scanApplication(rows)
		if err != nil {
			return err
		}
		out = append(out, a)
	}
	respond(w, http.StatusOK, struct {
		httpx.Meta
		Applications []applicationOut `json:"applications"`
	}{httpx.NowMeta(), out})
	return rows.Err()
}

// adminApproveApplication creates the draft hotel and invites the owner.
func (s *Server) adminApproveApplication(w http.ResponseWriter, r *http.Request) error {
	if err := requireAdmin(r); err != nil {
		return err
	}
	ctx, id := r.Context(), chi.URLParam(r, "id")
	var who string
	if err := s.pool.QueryRow(ctx, `SELECT coalesce(email, phone, id::text) FROM users WHERE id = $1`, claimsFrom(r).Sub).Scan(&who); err != nil {
		return err
	}
	var propertyID string
	var app applicationOut
	err := s.tx(ctx, func(tx pgx.Tx) error {
		a, err := scanApplication(tx.QueryRow(ctx, `SELECT `+applicationCols+` FROM hotel_applications WHERE id = $1 FOR UPDATE`, id))
		if err == pgx.ErrNoRows || isBadUUID(err) {
			return errNotFound
		}
		if err != nil {
			return err
		}
		if a.Status != "pending" {
			return fail(http.StatusConflict, codeInvalidState, "This application was already decided")
		}
		app = a
		// The owner's account: a guest account that becomes the owner; an owner or receptionist elsewhere keeps working there too. A platform admin cannot be a hotel owner.
		var uid, role string
		err = tx.QueryRow(ctx, `SELECT id, role FROM users WHERE email = $1`, a.OwnerEmail).Scan(&uid, &role)
		switch {
		case err == pgx.ErrNoRows:
			if err := tx.QueryRow(ctx, `INSERT INTO users (email, role, display_name) VALUES ($1, 'owner', $2) RETURNING id`, a.OwnerEmail, a.OwnerName).Scan(&uid); err != nil {
				return err
			}
		case err != nil:
			return err
		case role == "admin":
			return fail(http.StatusConflict, codeInvalidState, "The applicant's email belongs to a platform admin")
		case role == "guest":
			if _, err := tx.Exec(ctx, `UPDATE users SET role = 'owner' WHERE id = $1`, uid); err != nil {
				return err
			}
		}
		rules, _ := json.Marshal([]string{})
		if err := tx.QueryRow(ctx, `INSERT INTO properties (name, type, city, area, address, location, phone, description, pay_at_hotel, amenities, house_rules, status, verified)
			VALUES ($1, $2, $3, $4, $5, `+s.pointSQL("$6", "$7")+`, $8, $9, $10, '{}', $11::jsonb, 'draft', false) RETURNING id`,
			a.HotelName, a.HotelType, a.City, a.Area, a.Address, defaultLng, defaultLat, a.HotelPhone, a.Description, a.PayAtHotel, string(rules)).Scan(&propertyID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO property_staff (property_id, user_id, role) VALUES ($1, $2, 'owner') ON CONFLICT (property_id, user_id) DO UPDATE SET role = 'owner'`, propertyID, uid); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE hotel_applications SET status = 'approved', decided_at = now(), decided_by = $2, property_id = $3 WHERE id = $1`, id, who, propertyID); err != nil {
			return err
		}
		return s.adminAudit(r, tx, "admin_application_approved", propertyID, map[string]any{"application_id": id})
	})
	if err != nil {
		return err
	}
	s.sendMail(ctx, app.OwnerEmail, s.staffInvitedEmail("owner", app.HotelName, app.OwnerEmail))
	text := "Good news: " + app.HotelName + " is approved to be listed on BookDan (application " + app.Reference + ").\n\nYou received a second email with how to sign in to the BookDan Partner app. " +
		"Our team will now set up your hotel with you: the exact location, your rooms and prices, photos, and your business and payout details. Your hotel goes live when that is complete.\n\nBookDan\n"
	s.sendMail(ctx, app.OwnerEmail, emailContent{"Your hotel is approved: " + app.HotelName, text, frame("Your hotel is approved", para("Good news: "+app.HotelName+" is approved to be listed on BookDan (application "+app.Reference+"). You received a second email with how to sign in to the BookDan Partner app.")+
		para("Our team will now set up your hotel with you: the exact location, your rooms and prices, photos, and your business and payout details. Your hotel goes live when that is complete."))})
	respond(w, http.StatusOK, struct {
		httpx.Meta
		PropertyID string `json:"property_id"`
	}{httpx.NowMeta(), propertyID})
	return nil
}

func (s *Server) adminRejectApplication(w http.ResponseWriter, r *http.Request) error {
	if err := requireAdmin(r); err != nil {
		return err
	}
	var in struct {
		Note string `json:"note"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	note, ok := cleanText(in.Note, 3, 1000)
	if !ok {
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "Write the reason (3 to 1000 characters); the applicant reads it")
	}
	ctx, id := r.Context(), chi.URLParam(r, "id")
	var who string
	if err := s.pool.QueryRow(ctx, `SELECT coalesce(email, phone, id::text) FROM users WHERE id = $1`, claimsFrom(r).Sub).Scan(&who); err != nil {
		return err
	}
	var app applicationOut
	err := s.tx(ctx, func(tx pgx.Tx) error {
		a, err := scanApplication(tx.QueryRow(ctx, `SELECT `+applicationCols+` FROM hotel_applications WHERE id = $1 FOR UPDATE`, id))
		if err == pgx.ErrNoRows || isBadUUID(err) {
			return errNotFound
		}
		if err != nil {
			return err
		}
		if a.Status != "pending" {
			return fail(http.StatusConflict, codeInvalidState, "This application was already decided")
		}
		app = a
		if _, err := tx.Exec(ctx, `UPDATE hotel_applications SET status = 'rejected', decided_at = now(), decided_by = $2, decision_note = $3 WHERE id = $1`, id, who, note); err != nil {
			return err
		}
		return s.adminAudit(r, tx, "admin_application_rejected", "", map[string]any{"application_id": id})
	})
	if err != nil {
		return err
	}
	text := "Thank you for applying to list " + app.HotelName + " on BookDan (application " + app.Reference + ").\n\nWe cannot list it at this time.\n\n" + note + "\n\nYou are welcome to apply again when this is sorted out.\n\nBookDan\n"
	s.sendMail(ctx, app.OwnerEmail, emailContent{"About your application: " + app.HotelName, text, frame("About your application", para("Thank you for applying to list "+app.HotelName+" on BookDan (application "+app.Reference+"). We cannot list it at this time.")+para(note)+para("You are welcome to apply again when this is sorted out."))})
	respond(w, http.StatusOK, httpx.NowMeta())
	return nil
}
