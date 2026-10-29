package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/bookdan/backend/internal/httpx"
)

// Moderating reviews. A platform admin can take down a review that is abusive, spam, or shows someone's private details, and bring it back.
// A hidden review disappears from the hotel's page, from its rating and from the owner's list, and the owner can no longer answer it (the guest who
// wrote it still sees it on their own booking). The row and the reason stay, and every change is in the audit log.

type adminReview struct {
	ID       string  `json:"id"`
	Hotel    string  `json:"hotel"`
	HotelID  string  `json:"hotel_id"`
	Author   string  `json:"author"`
	BookCode string  `json:"booking_code"`
	Rating   int     `json:"rating"`
	Text     string  `json:"text"`
	Date     string  `json:"date"`
	Reply    *string `json:"reply"`
	Hidden   bool    `json:"hidden"`
	HiddenAt *string `json:"hidden_at"`
	Reason   *string `json:"hidden_reason"`
	HiddenBy *string `json:"hidden_by"`
}

// adminReviews lists reviews for moderation: ?status=visible (default) | hidden | all, ?hotel=<id>, ?q=<words in the review>, ?sort=latest (default) | lowest.
func (s *Server) adminReviews(w http.ResponseWriter, r *http.Request) error {
	if err := requireAdmin(r); err != nil {
		return err
	}
	q := r.URL.Query()
	status := q.Get("status")
	if status != "hidden" && status != "all" {
		status = "visible"
	}
	order := "v.created_at DESC"
	if q.Get("sort") == "lowest" {
		order = "v.rating, v.created_at DESC"
	}
	words := strings.TrimSpace(q.Get("q"))
	if len(words) > 100 {
		words = words[:100]
	}
	hotel := q.Get("hotel")
	rows, err := s.pool.Query(r.Context(), `SELECT v.id, p.id, p.name, coalesce(nullif(split_part(gp.full_name, ' ', 1), ''), nullif(u.display_name, ''), 'Guest'), b.code, v.rating, v.text,
			v.created_at, v.hotel_reply, v.hidden_at, v.hidden_reason, coalesce(a.email, '')
		FROM reviews v JOIN properties p ON p.id = v.property_id JOIN users u ON u.id = v.user_id JOIN bookings b ON b.id = v.booking_id
		LEFT JOIN guest_profiles gp ON gp.user_id = v.user_id LEFT JOIN users a ON a.id = v.hidden_by
		WHERE ($1 = 'all' OR ($1 = 'hidden') = (v.hidden_at IS NOT NULL))
		  AND ($2 = '' OR p.id::text = $2)
		  AND ($3 = '' OR v.text ILIKE '%' || $3 || '%' OR b.code ILIKE '%' || $3 || '%')
		ORDER BY `+order+` LIMIT 100`, status, hotel, words)
	if err != nil {
		return err
	}
	defer rows.Close()
	out := []adminReview{}
	for rows.Next() {
		var v adminReview
		var at time.Time
		var hiddenAt *time.Time
		var by string
		if err := rows.Scan(&v.ID, &v.HotelID, &v.Hotel, &v.Author, &v.BookCode, &v.Rating, &v.Text, &at, &v.Reply, &hiddenAt, &v.Reason, &by); err != nil {
			return err
		}
		v.Date = pgDate(at)
		if hiddenAt != nil {
			t := hiddenAt.UTC().Format(time.RFC3339)
			v.Hidden, v.HiddenAt = true, &t
			if by != "" {
				v.HiddenBy = &by
			}
		}
		out = append(out, v)
	}
	respond(w, http.StatusOK, struct {
		httpx.Meta
		Reviews []adminReview `json:"reviews"`
	}{httpx.NowMeta(), out})
	return rows.Err()
}

// adminHideReview takes a review down. The reason (3 to 300 characters) is kept with it.
func (s *Server) adminHideReview(w http.ResponseWriter, r *http.Request) error {
	if err := requireAdmin(r); err != nil {
		return err
	}
	var in struct {
		Reason string `json:"reason"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if len(in.Reason) < 3 || len(in.Reason) > 300 {
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "Give the reason for hiding this review (3 to 300 characters)")
	}
	return s.setReviewHidden(w, r, &in.Reason)
}

// adminRestoreReview brings a hidden review back.
func (s *Server) adminRestoreReview(w http.ResponseWriter, r *http.Request) error {
	if err := requireAdmin(r); err != nil {
		return err
	}
	return s.setReviewHidden(w, r, nil)
}

func (s *Server) setReviewHidden(w http.ResponseWriter, r *http.Request, reason *string) error {
	ctx, id, uid := r.Context(), chi.URLParam(r, "id"), claimsFrom(r).Sub
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var property string
		var hidden bool
		err := tx.QueryRow(ctx, `SELECT property_id, hidden_at IS NOT NULL FROM reviews WHERE id = $1 FOR UPDATE`, id).Scan(&property, &hidden)
		if err == pgx.ErrNoRows || isBadUUID(err) {
			return errNotFound
		}
		if err != nil {
			return err
		}
		action := "review_restored"
		if reason != nil {
			if hidden {
				return fail(http.StatusConflict, codeInvalidState, "This review is already hidden")
			}
			if _, err := tx.Exec(ctx, `UPDATE reviews SET hidden_at = now(), hidden_by = $2, hidden_reason = $3 WHERE id = $1`, id, uid, *reason); err != nil {
				return err
			}
			action = "review_hidden"
		} else {
			if !hidden {
				return fail(http.StatusConflict, codeInvalidState, "This review is not hidden")
			}
			if _, err := tx.Exec(ctx, `UPDATE reviews SET hidden_at = NULL, hidden_by = NULL, hidden_reason = NULL WHERE id = $1`, id); err != nil {
				return err
			}
		}
		d := map[string]any{"review_id": id}
		if reason != nil {
			d["reason"] = *reason
		}
		return s.adminAudit(r, tx, action, property, d)
	})
	if err != nil {
		return err
	}
	respond(w, http.StatusOK, httpx.NowMeta())
	return nil
}
