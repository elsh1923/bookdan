package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/bookdan/backend/internal/httpx"
)

// Users: BookDan's support and super admins can look a person up, suspend them (with a reason the audit log keeps), reinstate them, or sign them out
// everywhere. A suspended account cannot sign in or refresh (the server re-checks the account on every request), its phones stop getting alerts, and its
// bookings and money records stay as they were. Other admins are managed on the Admins page, so they are not touched here, and a person cannot act on
// their own account.

type adminUserOut struct {
	ID        string  `json:"id"`
	Email     *string `json:"email"`
	Phone     *string `json:"phone"`
	Name      *string `json:"name"`
	Role      string  `json:"role"`
	Status    string  `json:"status"`
	Deleted   bool    `json:"deleted"`
	CreatedAt string  `json:"created_at"`
	LastSeen  *string `json:"last_seen_at"`
	Bookings  int     `json:"bookings"`
	Phones    int     `json:"signed_in_devices"`
	Hotels    string  `json:"hotels"`
	Suspended *string `json:"suspended_at"`
	Reason    *string `json:"suspended_reason"`
}

const deletedAccountSuffix = "@deleted.invalid"

func (s *Server) adminUsers(w http.ResponseWriter, r *http.Request) error {
	if err := requireAdmin(r); err != nil {
		return err
	}
	q := r.URL.Query()
	limit := 30
	if q.Get("limit") != "" {
		n, err := pageLimit(r, 100)
		if err != nil {
			return err
		}
		limit = n
	}
	status, role := q.Get("status"), q.Get("role")
	if status != "" && status != "active" && status != "suspended" {
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "status must be active or suspended")
	}
	if role != "" && role != "guest" && role != "owner" && role != "receptionist" && role != "admin" {
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "role must be guest, owner, receptionist or admin")
	}
	after, afterID, err := decodeCursor(q.Get("cursor"))
	if err != nil {
		return err
	}
	word := likeEscape(strings.TrimSpace(q.Get("q")))
	rows, err := s.pool.Query(r.Context(), `
		SELECT u.id, u.email, u.phone, u.display_name, u.role, u.status, u.created_at,
		       (SELECT max(d.last_seen_at) FROM devices d WHERE d.user_id = u.id),
		       (SELECT count(*) FROM bookings b WHERE b.user_id = u.id)::int,
		       (SELECT count(*) FROM devices d WHERE d.user_id = u.id AND d.revoked_at IS NULL)::int,
		       coalesce((SELECT string_agg(p.name, ', ' ORDER BY p.name) FROM property_staff ps JOIN properties p ON p.id = ps.property_id WHERE ps.user_id = u.id), ''),
		       u.suspended_at, u.suspended_reason
		FROM users u
		WHERE ($1 = '' OR u.email ILIKE '%' || $1 || '%' OR u.phone ILIKE '%' || $1 || '%' OR u.display_name ILIKE '%' || $1 || '%')
		  AND ($2 = '' OR u.status = $2) AND ($3 = '' OR u.role = $3)
		  AND ($4::timestamptz IS NULL OR (u.created_at, u.id) < ($4, $5::uuid))
		ORDER BY u.created_at DESC, u.id DESC LIMIT $6`, word, status, role, after, nilIfEmpty(afterID), limit+1)
	if err != nil {
		return err
	}
	defer rows.Close()
	items := []adminUserOut{}
	for rows.Next() {
		var u adminUserOut
		var created time.Time
		var seen, susp *time.Time
		if err := rows.Scan(&u.ID, &u.Email, &u.Phone, &u.Name, &u.Role, &u.Status, &created, &seen, &u.Bookings, &u.Phones, &u.Hotels, &susp, &u.Reason); err != nil {
			return err
		}
		u.CreatedAt = created.UTC().Format(time.RFC3339Nano)
		u.Deleted = u.Email != nil && strings.HasSuffix(*u.Email, deletedAccountSuffix)
		if seen != nil {
			t := seen.UTC().Format(time.RFC3339)
			u.LastSeen = &t
		}
		if susp != nil {
			t := susp.UTC().Format(time.RFC3339)
			u.Suspended = &t
		}
		items = append(items, u)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	var next *string
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1]
		at, _ := time.Parse(time.RFC3339Nano, last.CreatedAt)
		c := encodeCursor(at, last.ID)
		next = &c
	}
	respond(w, http.StatusOK, struct {
		httpx.Meta
		Items      []adminUserOut `json:"items"`
		NextCursor *string        `json:"next_cursor"`
	}{httpx.NowMeta(), items, next})
	return nil
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// userForAction loads the account an admin is about to act on, locked, and refuses the ones that must not be touched here.
func (s *Server) userForAction(r *http.Request, tx pgx.Tx, id string) (status string, err error) {
	var role string
	var email *string
	err = tx.QueryRow(r.Context(), `SELECT status, role, email FROM users WHERE id = $1 FOR UPDATE`, id).Scan(&status, &role, &email)
	if err == pgx.ErrNoRows || isBadUUID(err) {
		return "", errNotFound
	}
	if err != nil {
		return "", err
	}
	if role == "admin" {
		return "", fail(http.StatusForbidden, codeForbidden, "BookDan admins are managed on the Admins page")
	}
	if id == claimsFrom(r).Sub {
		return "", fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "You cannot do this to your own account")
	}
	if email != nil && strings.HasSuffix(*email, deletedAccountSuffix) {
		return "", fail(http.StatusConflict, codeInvalidState, "This account was deleted by its owner")
	}
	return status, nil
}

// endSessions signs a person out of every phone and browser: refresh tokens and devices are revoked and the phones no longer get alerts.
func endSessions(r *http.Request, tx pgx.Tx, id string) error {
	ctx := r.Context()
	if _, err := tx.Exec(ctx, `UPDATE refresh_tokens SET revoked_at = coalesce(revoked_at, now()) WHERE user_id = $1`, id); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `UPDATE devices SET revoked_at = coalesce(revoked_at, now()), push_token = NULL WHERE user_id = $1`, id)
	return err
}

func (s *Server) adminSuspendUser(w http.ResponseWriter, r *http.Request) error {
	if err := requireAdmin(r); err != nil {
		return err
	}
	var in struct {
		Reason string `json:"reason"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	reason, ok := cleanText(in.Reason, 3, 500)
	if !ok {
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "Write the reason (3 to 500 characters); it is kept in the audit log")
	}
	id := chi.URLParam(r, "id")
	err := s.tx(r.Context(), func(tx pgx.Tx) error {
		status, err := s.userForAction(r, tx, id)
		if err != nil {
			return err
		}
		if status == "suspended" {
			return nil // already done: the same answer again
		}
		if _, err := tx.Exec(r.Context(), `UPDATE users SET status = 'suspended', suspended_at = now(), suspended_reason = $2, suspended_by = $3 WHERE id = $1`, id, reason, claimsFrom(r).Sub); err != nil {
			return err
		}
		if err := endSessions(r, tx, id); err != nil {
			return err
		}
		return s.audit(r, tx, "user_suspended", map[string]any{"user_id": id, "reason": reason})
	})
	if err != nil {
		return err
	}
	respond(w, http.StatusOK, httpx.NowMeta())
	return nil
}

func (s *Server) adminReinstateUser(w http.ResponseWriter, r *http.Request) error {
	if err := requireAdmin(r); err != nil {
		return err
	}
	id := chi.URLParam(r, "id")
	err := s.tx(r.Context(), func(tx pgx.Tx) error {
		status, err := s.userForAction(r, tx, id)
		if err != nil {
			return err
		}
		if status == "active" {
			return nil
		}
		if _, err := tx.Exec(r.Context(), `UPDATE users SET status = 'active', suspended_at = NULL, suspended_reason = NULL, suspended_by = NULL WHERE id = $1`, id); err != nil {
			return err
		}
		return s.audit(r, tx, "user_reinstated", map[string]any{"user_id": id})
	})
	if err != nil {
		return err
	}
	respond(w, http.StatusOK, httpx.NowMeta())
	return nil
}

// adminSignOutUser ends every session of a person without suspending them (a lost phone, a shared device).
func (s *Server) adminSignOutUser(w http.ResponseWriter, r *http.Request) error {
	if err := requireAdmin(r); err != nil {
		return err
	}
	id := chi.URLParam(r, "id")
	err := s.tx(r.Context(), func(tx pgx.Tx) error {
		if _, err := s.userForAction(r, tx, id); err != nil {
			return err
		}
		if err := endSessions(r, tx, id); err != nil {
			return err
		}
		return s.audit(r, tx, "user_signed_out", map[string]any{"user_id": id})
	})
	if err != nil {
		return err
	}
	respond(w, http.StatusOK, httpx.NowMeta())
	return nil
}
