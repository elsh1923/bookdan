package api

import (
	"context"
	"log/slog"
	"net/http"
	"slices"
	"sort"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/bookdan/backend/internal/httpx"
)

// Which alerts a person wants. New booking requests always reach the people on duty (their alerts switch and shift decide that). The other alerts are the
// person's own choice, per hotel, from the hotel app:
//
//	changes   a guest cancelled, or moved their dates
//	disputes  a guest opened a dispute
//	reviews   a guest reviewed the hotel (owners only)
//
// Anything muted here is simply left out of the people an alert goes to.

const (
	kindChanges  = "changes"
	kindDisputes = "disputes"
	kindReviews  = "reviews"
)

var alertKinds = []string{kindChanges, kindDisputes, kindReviews}

func validKinds(kinds []string) ([]string, bool) {
	out := []string{}
	for _, k := range kinds {
		if !slices.Contains(alertKinds, k) {
			return nil, false
		}
		if !slices.Contains(out, k) {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out, true
}

// unmuted drops the people who chose not to get this kind of alert.
func unmuted(rs []recipient, kind string) []recipient {
	if kind == "" {
		return rs
	}
	out := make([]recipient, 0, len(rs))
	for _, r := range rs {
		if !slices.Contains(r.muted, kind) {
			out = append(out, r)
		}
	}
	return out
}

// alertKindOf is the preference an alert type belongs to ("" for the ones nobody can turn off).
func alertKindOf(typ string) string {
	switch typ {
	case "booking_cancelled", "booking_dates_changed":
		return kindChanges
	case "booking_dispute":
		return kindDisputes
	case "new_review":
		return kindReviews
	}
	return ""
}

// alertHotelOfReview tells a hotel's owners that a guest reviewed it. Failure is logged, never returned.
func (s *Server) alertHotelOfReview(ctx context.Context, bookingID string, rating int) {
	q, err := s.loadRequest(ctx, bookingID)
	if err != nil {
		slog.Error("alert hotel of review", "err", err)
		return
	}
	var wave []recipient
	for _, r := range q.people {
		if isOwner(r) && r.token != "" && r.alertsOn {
			wave = append(wave, r)
		}
	}
	wave = unmuted(wave, kindReviews)
	if len(wave) == 0 {
		return
	}
	data := s.alertData(q, "new_review")
	s.push(ctx, tokensOf(wave), "A guest reviewed "+q.hotel, "They gave "+itoa(int64(rating))+" out of 5. Open Reviews to read it and reply.", data)
}

type alertPrefOut struct {
	PropertyID string   `json:"property_id"`
	Hotel      string   `json:"hotel"`
	Role       string   `json:"role"`
	AlertsOn   bool     `json:"alerts_on"`
	Muted      []string `json:"muted_kinds"`
	// Available are the optional alerts this person can choose about at this hotel (reviews are for owners).
	Available []string `json:"available_kinds"`
}

// partnerAlertPrefs lists the signed-in person's alert choices, one entry per hotel they work for.
func (s *Server) partnerAlertPrefs(w http.ResponseWriter, r *http.Request) error {
	rows, err := s.pool.Query(r.Context(), `SELECT p.id, p.name, ps.role, ps.alerts_on, ps.muted_kinds FROM property_staff ps JOIN properties p ON p.id = ps.property_id
		WHERE ps.user_id = $1 ORDER BY p.name`, claimsFrom(r).Sub)
	if err != nil {
		return err
	}
	defer rows.Close()
	out := []alertPrefOut{}
	for rows.Next() {
		var a alertPrefOut
		if err := rows.Scan(&a.PropertyID, &a.Hotel, &a.Role, &a.AlertsOn, &a.Muted); err != nil {
			return err
		}
		if a.Muted == nil {
			a.Muted = []string{}
		}
		a.Available = []string{kindChanges, kindDisputes}
		if a.Role == "owner" {
			a.Available = append(a.Available, kindReviews)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	respond(w, http.StatusOK, struct {
		httpx.Meta
		Hotels []alertPrefOut `json:"hotels"`
	}{httpx.NowMeta(), out})
	return nil
}

// partnerSetAlertPrefs saves which optional alerts the signed-in person does not want at one hotel.
func (s *Server) partnerSetAlertPrefs(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Muted []string `json:"muted_kinds"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	muted, ok := validKinds(in.Muted)
	if !ok {
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "muted_kinds can hold changes, disputes and reviews")
	}
	id := chi.URLParam(r, "id")
	err := s.tx(r.Context(), func(tx pgx.Tx) error {
		tag, err := tx.Exec(r.Context(), `UPDATE property_staff SET muted_kinds = $3 WHERE property_id = $1 AND user_id = $2`, id, claimsFrom(r).Sub, muted)
		if isBadUUID(err) {
			return errNotFound
		}
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return errNotFound
		}
		return s.audit(r, tx, "staff_alert_preferences", map[string]any{"property_id": id, "muted_kinds": muted})
	})
	if err != nil {
		return err
	}
	respond(w, http.StatusOK, struct {
		httpx.Meta
		Muted []string `json:"muted_kinds"`
	}{httpx.NowMeta(), muted})
	return nil
}
