package api

import (
	"net/http"
	"regexp"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/bookdan/backend/internal/httpx"
)

// Saved hotels (the wishlist). The app keeps the list on the phone so it works offline, and syncs it here so it follows the guest to a new phone.
//   - GET    /me/saved                  the guest's list, newest first;
//   - PUT    /me/saved/{propertyId}     save one (saving twice is fine);
//   - DELETE /me/saved/{propertyId}     remove one (removing twice is fine);
//   - POST   /me/saved/sync             {add: [...], remove: [...]}: what changed on the phone since the last sync. The answer is the whole list, which
//     the phone then keeps. Unknown hotels in add are ignored (a hotel that was hidden since); a hotel in both lists is removed.
// A guest can save at most maxSaved hotels. Deleting the account deletes the list.

const maxSaved = 200

var uuidRE = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type savedOut struct {
	PropertyID string `json:"property_id"`
	SavedAt    string `json:"saved_at"`
}

func (s *Server) listSaved(w http.ResponseWriter, r *http.Request) error {
	items, err := s.savedList(r, claimsFrom(r).Sub, nil)
	if err != nil {
		return err
	}
	respond(w, http.StatusOK, struct {
		httpx.Meta
		Items []savedOut `json:"items"`
	}{httpx.NowMeta(), items})
	return nil
}

// savedList reads the guest's list. With tx it reads inside that transaction.
func (s *Server) savedList(r *http.Request, uid string, tx pgx.Tx) ([]savedOut, error) {
	const q = `SELECT property_id, saved_at FROM saved_properties WHERE user_id = $1 ORDER BY saved_at DESC, property_id`
	var rows pgx.Rows
	var err error
	if tx != nil {
		rows, err = tx.Query(r.Context(), q, uid)
	} else {
		rows, err = s.pool.Query(r.Context(), q, uid)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []savedOut{}
	for rows.Next() {
		var x savedOut
		var at time.Time
		if err := rows.Scan(&x.PropertyID, &at); err != nil {
			return nil, err
		}
		x.SavedAt = at.UTC().Format(time.RFC3339)
		out = append(out, x)
	}
	return out, rows.Err()
}

// addSaved saves hotels that exist and are listed (not a draft), up to the limit. It returns without error when some were skipped.
func addSaved(r *http.Request, tx pgx.Tx, uid string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	var have int
	if err := tx.QueryRow(r.Context(), `SELECT count(*)::int FROM saved_properties WHERE user_id = $1`, uid).Scan(&have); err != nil {
		return err
	}
	room := maxSaved - have
	if room <= 0 {
		return fail(http.StatusConflict, "SAVED_FULL", "You can save up to 200 hotels. Remove one first.")
	}
	_, err := tx.Exec(r.Context(), `INSERT INTO saved_properties (user_id, property_id)
		SELECT $1, p.id FROM properties p WHERE p.id = ANY($2::uuid[]) AND p.status = 'active'
		ORDER BY p.id LIMIT $3 ON CONFLICT DO NOTHING`, uid, ids, room)
	return err
}

func (s *Server) saveProperty(w http.ResponseWriter, r *http.Request) error {
	uid, id := claimsFrom(r).Sub, chi.URLParam(r, "propertyId")
	err := s.tx(r.Context(), func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(r.Context(), `SELECT EXISTS (SELECT 1 FROM properties WHERE id = $1 AND status = 'active')`, id).Scan(&exists); err != nil {
			if isBadUUID(err) {
				return errNotFound
			}
			return err
		}
		if !exists {
			return errNotFound
		}
		var already bool
		if err := tx.QueryRow(r.Context(), `SELECT EXISTS (SELECT 1 FROM saved_properties WHERE user_id = $1 AND property_id = $2)`, uid, id).Scan(&already); err != nil {
			return err
		}
		if already {
			return nil
		}
		return addSaved(r, tx, uid, []string{id})
	})
	if err != nil {
		return err
	}
	respond(w, http.StatusOK, httpx.NowMeta())
	return nil
}

func (s *Server) unsaveProperty(w http.ResponseWriter, r *http.Request) error {
	if _, err := s.pool.Exec(r.Context(), `DELETE FROM saved_properties WHERE user_id = $1 AND property_id::text = $2`, claimsFrom(r).Sub, chi.URLParam(r, "propertyId")); err != nil {
		return err
	}
	respond(w, http.StatusOK, httpx.NowMeta())
	return nil
}

// syncSaved applies what changed on the phone and returns the whole list.
func (s *Server) syncSaved(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Add    []string `json:"add"`
		Remove []string `json:"remove"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if len(in.Add) > 500 || len(in.Remove) > 500 {
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "Too many hotels in one sync")
	}
	for _, id := range append(append([]string{}, in.Add...), in.Remove...) {
		if !uuidRE.MatchString(id) {
			return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "A hotel id is not valid")
		}
	}
	uid := claimsFrom(r).Sub
	removed := map[string]bool{}
	for _, id := range in.Remove {
		removed[id] = true
	}
	var add []string
	for _, id := range in.Add {
		if !removed[id] {
			add = append(add, id)
		}
	}
	var items []savedOut
	err := s.tx(r.Context(), func(tx pgx.Tx) error {
		if len(in.Remove) > 0 {
			if _, err := tx.Exec(r.Context(), `DELETE FROM saved_properties WHERE user_id = $1 AND property_id = ANY($2::uuid[])`, uid, in.Remove); err != nil {
				return err
			}
		}
		if err := addSaved(r, tx, uid, add); err != nil {
			// A full list is not an error for a sync: the phone gets the list the server holds.
			if ae, ok := err.(*apiError); !ok || ae.code != "SAVED_FULL" {
				return err
			}
		}
		var err error
		items, err = s.savedList(r, uid, tx)
		return err
	})
	if err != nil {
		return err
	}
	respond(w, http.StatusOK, struct {
		httpx.Meta
		Items []savedOut `json:"items"`
	}{httpx.NowMeta(), items})
	return nil
}
