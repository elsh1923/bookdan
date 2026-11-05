package api

import (
	"context"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5"

	"github.com/bookdan/backend/internal/httpx"
)

// One person, one account. A guest who signed in with an email can add their phone number (and the other way round), proving they own it with a code sent
// to it. After that either one signs in to the same account.
//
// If the number or address is already the sign-in of another guest account (the person once used the other method, and has bookings there), the code they
// just entered proves they own both, and the app offers to MERGE: the other account's bookings, reviews, support requests, saved hotels and details move
// into this account and the other one is closed. Hotel staff and admin accounts are never merged, and neither is an account with a payment in progress.
//
// The code is checked twice for a merge (first to find out, then with the person's yes) but spent only once.

const (
	codeAlreadyHas    = "ALREADY_HAS_IDENTITY"
	codeIdentityInUse = "IDENTITY_IN_USE"
	codeNotMergeable  = "IDENTITY_NOT_MERGEABLE"
)

// identityColumn is the users column for an identity type (the type was validated by clean).
func identityColumn(t string) string {
	if t == "email" {
		return "email"
	}
	return "phone"
}

type myIdentitiesOut struct {
	httpx.Meta
	Email *string `json:"email"`
	Phone *string `json:"phone"`
	// PhoneAvailable is false when the server cannot send text messages (email-only launch): a phone number cannot be verified then.
	PhoneAvailable bool `json:"phone_available"`
}

func (s *Server) myIdentities(w http.ResponseWriter, r *http.Request) error {
	out := myIdentitiesOut{Meta: httpx.NowMeta(), PhoneAvailable: s.cfg.OTPProvider != "off"}
	if err := s.pool.QueryRow(r.Context(), `SELECT email, phone FROM users WHERE id = $1`, claimsFrom(r).Sub).Scan(&out.Email, &out.Phone); err != nil {
		return err
	}
	respond(w, http.StatusOK, out)
	return nil
}

// linkCode sends a code to the new sign-in method. Whether it already belongs to someone is not revealed here: only a person who holds the code learns it.
func (s *Server) linkCode(w http.ResponseWriter, r *http.Request) error {
	var in identity
	if err := decode(r, &in); err != nil {
		return err
	}
	id, channel, err := in.clean()
	if err != nil {
		return err
	}
	var email, phone *string
	if err := s.pool.QueryRow(r.Context(), `SELECT email, phone FROM users WHERE id = $1`, claimsFrom(r).Sub).Scan(&email, &phone); err != nil {
		return err
	}
	if (id.Type == "email" && email != nil) || (id.Type == "phone" && phone != nil) {
		return fail(http.StatusConflict, codeAlreadyHas, "Your account already has "+map[string]string{"email": "an email address", "phone": "a phone number"}[id.Type])
	}
	if err := s.sendCode(r, id, channel); err != nil {
		return err
	}
	respond(w, http.StatusOK, httpx.NowMeta())
	return nil
}

type mergeFacts struct{ bookings, saved int }

// mergeCheck decides whether the other account may be merged into this one, and what the person is told they are merging.
func (s *Server) mergeCheck(ctx context.Context, tx pgx.Tx, drop string) (mergeFacts, error) {
	var f mergeFacts
	var role, status string
	if err := tx.QueryRow(ctx, `SELECT role, status FROM users WHERE id = $1`, drop).Scan(&role, &status); err != nil {
		return f, err
	}
	var staff, admin, busy bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM property_staff WHERE user_id = $1), EXISTS (SELECT 1 FROM admin_accounts WHERE user_id = $1),
			EXISTS (SELECT 1 FROM bookings WHERE user_id = $1 AND status = 'HELD')
			  OR EXISTS (SELECT 1 FROM date_change_requests c JOIN bookings b ON b.id = c.booking_id WHERE b.user_id = $1 AND c.status = 'pending')`, drop).Scan(&staff, &admin, &busy); err != nil {
		return f, err
	}
	switch {
	case role != "guest" || staff || admin:
		return f, fail(http.StatusForbidden, codeNotMergeable, "That belongs to a hotel or BookDan team account, which cannot be merged with a guest account.")
	case status != "active":
		return f, fail(http.StatusForbidden, codeNotMergeable, "That account is suspended, so it cannot be merged.")
	case busy:
		return f, fail(http.StatusConflict, codeInvalidState, "A payment is in progress on that account. Finish or wait for it, then try again.")
	}
	if err := tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM bookings WHERE user_id = $1 AND status <> 'HELD')::int, (SELECT count(*) FROM saved_properties WHERE user_id = $1)::int`, drop).Scan(&f.bookings, &f.saved); err != nil {
		return f, err
	}
	return f, nil
}

// mergeGuests moves everything of the account `drop` into `keep`, closes `drop`, and gives `keep` the sign-in method that was just proven.
func (s *Server) mergeGuests(r *http.Request, tx pgx.Tx, keep, drop string, id identity) error {
	ctx := r.Context()
	var dropEmail, dropPhone, dropName *string
	if err := tx.QueryRow(ctx, `SELECT email, phone, display_name FROM users WHERE id = $1`, drop).Scan(&dropEmail, &dropPhone, &dropName); err != nil {
		return err
	}
	// What belongs to the person moves over. (The columns that point at hotel staff and admins cannot be on a guest account: mergeCheck refused those.)
	for _, table := range []string{"quotes", "bookings", "coupon_redemptions", "hotel_calls", "reviews", "support_tickets"} {
		if _, err := tx.Exec(ctx, `UPDATE `+table+` SET user_id = $1 WHERE user_id = $2`, keep, drop); err != nil {
			return err
		}
	}
	steps := []struct {
		sql  string
		args []any
	}{
		// Saved hotels: the union. Details and ID photos: this account's own win; the other's are taken only where this account has none.
		{`INSERT INTO saved_properties (user_id, property_id, saved_at) SELECT $1, property_id, saved_at FROM saved_properties WHERE user_id = $2 ON CONFLICT DO NOTHING`, []any{keep, drop}},
		{`DELETE FROM saved_properties WHERE user_id = $1`, []any{drop}},
		{`UPDATE guest_profiles SET user_id = $1 WHERE user_id = $2 AND NOT EXISTS (SELECT 1 FROM guest_profiles WHERE user_id = $1)`, []any{keep, drop}},
		{`DELETE FROM guest_profiles WHERE user_id = $1`, []any{drop}},
		{`UPDATE guest_id_photos g SET user_id = $1 WHERE user_id = $2 AND NOT EXISTS (SELECT 1 FROM guest_id_photos k WHERE k.user_id = $1 AND k.side = g.side)`, []any{keep, drop}},
		{`DELETE FROM guest_id_photos WHERE user_id = $1`, []any{drop}},
		{`DELETE FROM idempotency_keys WHERE user_id = $1`, []any{drop}},
		{`DELETE FROM otp_challenges WHERE identifier IN (coalesce($1, ''), coalesce($2, ''))`, []any{dropEmail, dropPhone}},
	}
	for _, st := range steps {
		if _, err := tx.Exec(ctx, st.sql, st.args...); err != nil {
			return err
		}
	}
	if err := endSessions(r, tx, drop); err != nil {
		return err
	}
	// Close the other account and free its sign-in methods, then give this account the one that was just proven (and the other one it had, if this
	// account has no such method of its own).
	if _, err := tx.Exec(ctx, `UPDATE users SET email = 'deleted-' || id::text || '@deleted.invalid', phone = NULL, display_name = NULL, status = 'suspended',
			suspended_at = now(), suspended_reason = $2 WHERE id = $1`, drop, "Merged into account "+keep+" by its owner"); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE users SET `+identityColumn(id.Type)+` = $2, display_name = coalesce(display_name, $3) WHERE id = $1`, keep, id.Value, dropName); err != nil {
		return err
	}
	if id.Type == "phone" && dropEmail != nil {
		if _, err := tx.Exec(ctx, `UPDATE users SET email = $2 WHERE id = $1 AND email IS NULL`, keep, *dropEmail); err != nil {
			return err
		}
	}
	if id.Type == "email" && dropPhone != nil {
		if _, err := tx.Exec(ctx, `UPDATE users SET phone = $2 WHERE id = $1 AND phone IS NULL`, keep, *dropPhone); err != nil {
			return err
		}
	}
	return nil
}

// linkConfirm checks the code and attaches the sign-in method to this account, merging the other account first when the person agrees.
func (s *Server) linkConfirm(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		identity
		Code  string `json:"code"`
		Merge bool   `json:"merge"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	id, _, err := in.identity.clean()
	if err != nil {
		return err
	}
	ctx, uid := r.Context(), claimsFrom(r).Sub
	col := identityColumn(id.Type)
	merged := false
	err = s.tx(ctx, func(tx pgx.Tx) error {
		// Who has this sign-in method now? Then lock both accounts in one fixed order, so two people merging at once cannot wait for each other.
		var other string
		if err := tx.QueryRow(ctx, `SELECT id FROM users WHERE `+col+` = $1`, id.Value).Scan(&other); err != nil && err != pgx.ErrNoRows {
			return err
		}
		ids := []string{uid}
		if other != "" && other != uid {
			ids = append(ids, other)
		}
		rows, err := tx.Query(ctx, `SELECT id FROM users WHERE id = ANY($1::uuid[]) ORDER BY id FOR UPDATE`, ids)
		if err != nil {
			return err
		}
		rows.Close()
		var email, phone *string
		var role string
		if err := tx.QueryRow(ctx, `SELECT email, phone, role FROM users WHERE id = $1`, uid).Scan(&email, &phone, &role); err != nil {
			return err
		}
		var again string
		if err := tx.QueryRow(ctx, `SELECT coalesce((SELECT id::text FROM users WHERE `+col+` = $1), '')`, id.Value).Scan(&again); err != nil {
			return err
		}
		if again != other {
			return fail(http.StatusConflict, codeInvalidState, "That changed while you were confirming. Try again.")
		}
		if role != "guest" {
			return fail(http.StatusForbidden, codeForbidden, "Only guest accounts can add a sign-in method")
		}
		if (id.Type == "email" && email != nil) || (id.Type == "phone" && phone != nil) {
			return fail(http.StatusConflict, codeAlreadyHas, "Your account already has "+map[string]string{"email": "an email address", "phone": "a phone number"}[id.Type])
		}

		// A code is spent when it attaches something new or merges; asking whether a merge is wanted only confirms it.
		spend := other == "" || in.Merge
		if err := s.checkOTPMode(ctx, tx, id, in.Code, spend); err != nil {
			return err
		}
		if other == "" {
			if _, err := tx.Exec(ctx, `UPDATE users SET `+col+` = $2 WHERE id = $1`, uid, id.Value); err != nil {
				return err
			}
			return s.audit(r, tx, "identity_linked", map[string]any{"type": id.Type})
		}
		facts, err := s.mergeCheck(ctx, tx, other)
		if err != nil {
			return err
		}
		if !in.Merge {
			label := map[string]string{"email": "address", "phone": "number"}[id.Type]
			return fail(http.StatusConflict, codeIdentityInUse, fmt.Sprintf("That %s already has a BookDan account with %s. Merge it into this account? Its bookings, saved hotels and details move here, and the other account is closed.",
				label, plural(facts.bookings, "booking", "bookings")))
		}
		if err := s.mergeGuests(r, tx, uid, other, id); err != nil {
			return err
		}
		merged = true
		return s.audit(r, tx, "accounts_merged", map[string]any{"kept": uid, "closed": other, "bookings_moved": facts.bookings, "type": id.Type})
	})
	var wc *wrongCode
	if asWrong(err, &wc) {
		_, _ = s.pool.Exec(ctx, `UPDATE otp_challenges SET attempts = attempts + 1 WHERE id = $1 AND attempts < max_attempts`, wc.id)
		return fail(http.StatusUnauthorized, codeInvalidOTP, "That code is wrong or has expired")
	}
	if err != nil {
		return err
	}
	out := myIdentitiesOut{Meta: httpx.NowMeta(), PhoneAvailable: s.cfg.OTPProvider != "off"}
	if err := s.pool.QueryRow(ctx, `SELECT email, phone FROM users WHERE id = $1`, uid).Scan(&out.Email, &out.Phone); err != nil {
		return err
	}
	respond(w, http.StatusOK, struct {
		myIdentitiesOut
		Merged bool `json:"merged"`
	}{out, merged})
	return nil
}
