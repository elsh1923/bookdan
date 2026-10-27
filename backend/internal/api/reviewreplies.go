package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/bookdan/backend/internal/httpx"
)

// A hotel's public reply to a guest's review. Hotel owners only (a reply speaks for the hotel); one reply per review, which the owner can change
// or take away. The reply is shown under the review on the hotel's page and on the guest's own booking.

type replyOut struct {
	Text string `json:"text"`
	Date string `json:"date"`
}

type partnerReview struct {
	ID       string    `json:"id"`
	Hotel    string    `json:"hotel"`
	Author   string    `json:"author"`
	Rating   int       `json:"rating"`
	Text     string    `json:"text"`
	Date     string    `json:"date"`
	Reply    *replyOut `json:"reply"`
	BookCode string    `json:"booking_code"`
}

// partnerReviews lists the latest reviews of the owner's hotels, those without a reply first.
func (s *Server) partnerReviews(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	owned, err := s.ownerHotels(ctx, claimsFrom(r).Sub)
	if err != nil {
		return err
	}
	if len(owned) == 0 {
		return fail(http.StatusForbidden, codeForbidden, "Only a hotel owner can read and answer reviews")
	}
	rows, err := s.pool.Query(ctx, `SELECT v.id, p.name, coalesce(nullif(split_part(gp.full_name, ' ', 1), ''), nullif(u.display_name, ''), 'Guest'), v.rating, v.text, v.created_at,
			v.hotel_reply, v.hotel_reply_at, b.code
		FROM reviews v JOIN properties p ON p.id = v.property_id JOIN users u ON u.id = v.user_id JOIN bookings b ON b.id = v.booking_id
		LEFT JOIN guest_profiles gp ON gp.user_id = v.user_id
		WHERE v.property_id = ANY($1::uuid[]) ORDER BY (v.hotel_reply IS NOT NULL), v.created_at DESC LIMIT 100`, owned)
	if err != nil {
		return err
	}
	defer rows.Close()
	out := []partnerReview{}
	for rows.Next() {
		var v partnerReview
		var at time.Time
		var reply *string
		var replyAt *time.Time
		if err := rows.Scan(&v.ID, &v.Hotel, &v.Author, &v.Rating, &v.Text, &at, &reply, &replyAt, &v.BookCode); err != nil {
			return err
		}
		v.Date = pgDate(at)
		if reply != nil && replyAt != nil {
			v.Reply = &replyOut{*reply, pgDate(*replyAt)}
		}
		out = append(out, v)
	}
	respond(w, http.StatusOK, struct {
		httpx.Meta
		Reviews []partnerReview `json:"reviews"`
	}{httpx.NowMeta(), out})
	return rows.Err()
}

func (s *Server) partnerReplyReview(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Text string `json:"text"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	text, ok := cleanText(in.Text, 3, 1000)
	if !ok {
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, fmt.Sprintf("The reply must be 3 to 1000 characters"))
	}
	return s.setReply(w, r, &text)
}

func (s *Server) partnerDeleteReply(w http.ResponseWriter, r *http.Request) error {
	return s.setReply(w, r, nil)
}

// setReply writes (or clears, when text is nil) the reply on a review of a hotel the user owns.
func (s *Server) setReply(w http.ResponseWriter, r *http.Request, text *string) error {
	ctx, id, uid := r.Context(), chi.URLParam(r, "id"), claimsFrom(r).Sub
	owned, err := s.ownerHotels(ctx, uid)
	if err != nil {
		return err
	}
	if len(owned) == 0 {
		return fail(http.StatusForbidden, codeForbidden, "Only a hotel owner can answer reviews")
	}
	err = s.tx(ctx, func(tx pgx.Tx) error {
		var tag int64
		var err error
		if text == nil {
			t, e := tx.Exec(ctx, `UPDATE reviews SET hotel_reply = NULL, hotel_reply_at = NULL, hotel_reply_by = NULL WHERE id = $1 AND property_id = ANY($2::uuid[])`, id, owned)
			tag, err = t.RowsAffected(), e
		} else {
			t, e := tx.Exec(ctx, `UPDATE reviews SET hotel_reply = $3, hotel_reply_at = now(), hotel_reply_by = $4 WHERE id = $1 AND property_id = ANY($2::uuid[])`, id, owned, *text, uid)
			tag, err = t.RowsAffected(), e
		}
		if err != nil {
			if isBadUUID(err) {
				return errNotFound
			}
			return err
		}
		if tag == 0 {
			return errNotFound
		}
		action := "review_reply_set"
		if text == nil {
			action = "review_reply_removed"
		}
		return s.audit(r, tx, action, map[string]any{"review_id": id})
	})
	if err != nil {
		return err
	}
	respond(w, http.StatusOK, httpx.NowMeta())
	return nil
}
