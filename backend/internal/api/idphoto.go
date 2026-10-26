package api

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/bookdan/backend/internal/httpx"
)

// A guest can add a photo of their ID: the front and, for a card, the back. The server stores an encrypted, re-drawn copy (see migration
// 00021). The guest sees only their own; a hotel sees the copy that travelled with a booking, through an audited view.

const maxIDPhotoJSON = 9 << 20 // the picture as base64 inside JSON

func idSide(r *http.Request) (string, error) {
	side := chi.URLParam(r, "side")
	if side != "front" && side != "back" {
		return "", fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "side must be front or back")
	}
	return side, nil
}

// putIDPhoto stores (or replaces) one side. Body: {"image": "<base64 of a JPEG, PNG or WebP>"}.
func (s *Server) putIDPhoto(w http.ResponseWriter, r *http.Request) error {
	side, err := idSide(r)
	if err != nil {
		return err
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxIDPhotoJSON+1))
	if err != nil || len(body) > maxIDPhotoJSON {
		return fail(http.StatusRequestEntityTooLarge, httpx.CodeInvalidRequest, "The photo is too big")
	}
	var in struct {
		Image string `json:"image"`
	}
	if err := json.Unmarshal(body, &in); err != nil || in.Image == "" {
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "image is required")
	}
	raw, err := base64.StdEncoding.DecodeString(stripDataURL(in.Image))
	if err != nil {
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "image must be base64")
	}
	// processPhoto draws the picture again as a plain JPEG (at most 1600 px: enough to read an ID), so it carries no location or camera data.
	jpg, _, _, _, err := processPhoto(raw)
	if err != nil {
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "That is not a usable photo (use a JPEG, PNG or WebP picture)")
	}
	uid := claimsFrom(r).Sub
	if err := s.tx(r.Context(), func(tx pgx.Tx) error {
		if _, err := tx.Exec(r.Context(), `INSERT INTO guest_id_photos (user_id, side, data_enc) VALUES ($1, $2, $3)
			ON CONFLICT (user_id, side) DO UPDATE SET data_enc = EXCLUDED.data_enc, updated_at = now()`, uid, side, s.keys.Seal(string(jpg))); err != nil {
			return err
		}
		return s.audit(r, tx, "guest_id_photo_saved", map[string]any{"side": side})
	}); err != nil {
		return err
	}
	respond(w, http.StatusOK, struct {
		httpx.Meta
		Side string `json:"side"`
	}{httpx.NowMeta(), side})
	return nil
}

func stripDataURL(s string) string {
	if i := bytes.IndexByte([]byte(s), ','); i >= 0 && len(s) > 5 && s[:5] == "data:" {
		return s[i+1:]
	}
	return s
}

// getIDPhoto returns the guest's own photo, so they can check what they added.
func (s *Server) getIDPhoto(w http.ResponseWriter, r *http.Request) error {
	side, err := idSide(r)
	if err != nil {
		return err
	}
	var enc []byte
	err = s.pool.QueryRow(r.Context(), `SELECT data_enc FROM guest_id_photos WHERE user_id = $1 AND side = $2`, claimsFrom(r).Sub, side).Scan(&enc)
	if err == pgx.ErrNoRows {
		return errNotFound
	}
	if err != nil {
		return err
	}
	return s.writeIDPhoto(w, enc)
}

func (s *Server) deleteIDPhoto(w http.ResponseWriter, r *http.Request) error {
	side, err := idSide(r)
	if err != nil {
		return err
	}
	if err := s.tx(r.Context(), func(tx pgx.Tx) error {
		if _, err := tx.Exec(r.Context(), `DELETE FROM guest_id_photos WHERE user_id = $1 AND side = $2`, claimsFrom(r).Sub, side); err != nil {
			return err
		}
		return s.audit(r, tx, "guest_id_photo_deleted", map[string]any{"side": side})
	}); err != nil {
		return err
	}
	respond(w, http.StatusOK, httpx.NowMeta())
	return nil
}

func (s *Server) writeIDPhoto(w http.ResponseWriter, enc []byte) error {
	plain, err := s.keys.Open(enc)
	if err != nil {
		return err
	}
	h := w.Header()
	h.Set("Content-Type", "image/jpeg")
	h.Set("Cache-Control", "private, no-store") // an ID must never sit in a shared cache or on disk
	h.Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write([]byte(plain))
	return nil
}

// partnerIDPhoto shows the hotel the ID photo that came with a booking. Only staff of that hotel, never for a booking still being held,
// and every view is written to the audit log (like revealing the ID number).
func (s *Server) partnerIDPhoto(w http.ResponseWriter, r *http.Request) error {
	ctx, id := r.Context(), chi.URLParam(r, "id")
	side, err := idSide(r)
	if err != nil {
		return err
	}
	propIDs, _, err := s.staffHotels(ctx, claimsFrom(r).Sub)
	if err != nil {
		return err
	}
	var enc []byte
	var purged bool
	err = s.pool.QueryRow(ctx, `SELECT p.data_enc, p.purged_at IS NOT NULL FROM booking_id_photos p
		JOIN bookings b ON b.id = p.booking_id WHERE p.booking_id = $1 AND p.side = $2 AND b.property_id = ANY($3::uuid[]) AND b.status <> 'HELD'`, id, side, propIDs).Scan(&enc, &purged)
	if err == pgx.ErrNoRows || isBadUUID(err) {
		return errNotFound
	}
	if err != nil {
		return err
	}
	if purged || enc == nil {
		return fail(http.StatusGone, codeGone, "The ID photo was erased after the retention period")
	}
	if err := s.tx(ctx, func(tx pgx.Tx) error {
		return s.audit(r, tx, "guest_id_photo_viewed", map[string]any{"booking_id": id, "side": side})
	}); err != nil {
		return err
	}
	return s.writeIDPhoto(w, enc)
}
