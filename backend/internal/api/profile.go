package api

import (
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/bookdan/backend/internal/httpx"
)

var (
	idNumberRE = regexp.MustCompile(`^[A-Z0-9/-]{4,25}$`)
	spacesRE   = regexp.MustCompile(`\s+`)
	idTypes    = map[string]bool{"national_id": true, "passport": true, "driving_licence": true, "residence_permit": true, "other": true}
)

// normalizeID mirrors the apps: remove spaces, upper-case, and accept only letters, digits, dash and slash (4 to 25 characters).
// [verify] the exact national-ID formats with the issuing authority before tightening this.
func normalizeID(in string) (string, bool) {
	n := strings.ToUpper(spacesRE.ReplaceAllString(in, ""))
	return n, idNumberRE.MatchString(n)
}

func last4(n string) string {
	if len(n) <= 4 {
		return n
	}
	return n[len(n)-4:]
}

type profileOut struct {
	FullName    string `json:"full_name"`
	IDType      string `json:"id_type"`
	IDLast4     string `json:"id_last4"`
	Nationality string `json:"nationality"`
	UpdatedAt   string `json:"updated_at"`
	// IDPhotos says which sides the guest has added (the pictures themselves are fetched one at a time).
	IDPhotos idPhotoSides `json:"id_photos"`
	httpx.Meta
}

type idPhotoSides struct {
	Front bool `json:"front"`
	Back  bool `json:"back"`
}

type profileResponse struct {
	httpx.Meta
	Profile *profileOut `json:"profile"`
}

// readProfile never returns the ID number: the guest sees only its last four characters.
func (s *Server) readProfile(r *http.Request, userID string) (*profileOut, error) {
	var p profileOut
	var at time.Time
	err := s.pool.QueryRow(r.Context(), `SELECT full_name, id_type, id_last4, nationality, updated_at FROM guest_profiles WHERE user_id = $1`, userID).
		Scan(&p.FullName, &p.IDType, &p.IDLast4, &p.Nationality, &at)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p.UpdatedAt, p.Meta = at.UTC().Format(time.RFC3339), httpx.NowMeta()
	if err := s.pool.QueryRow(r.Context(), `SELECT coalesce(bool_or(side = 'front'), false), coalesce(bool_or(side = 'back'), false) FROM guest_id_photos WHERE user_id = $1`, userID).
		Scan(&p.IDPhotos.Front, &p.IDPhotos.Back); err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Server) getProfile(w http.ResponseWriter, r *http.Request) error {
	p, err := s.readProfile(r, claimsFrom(r).Sub)
	if err != nil {
		return err
	}
	respond(w, http.StatusOK, profileResponse{httpx.NowMeta(), p})
	return nil
}

func (s *Server) putProfile(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		FullName    string `json:"full_name"`
		IDType      string `json:"id_type"`
		IDNumber    string `json:"id_number"`
		Nationality string `json:"nationality"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	name, nat := strings.TrimSpace(in.FullName), strings.TrimSpace(in.Nationality)
	if len(name) < 2 || len(name) > 120 || len(nat) < 2 || len(nat) > 60 || !idTypes[in.IDType] {
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "Name, ID type and nationality are required")
	}
	number, ok := normalizeID(in.IDNumber)
	if !ok {
		return fail(http.StatusBadRequest, codeInvalidID, "That does not look like an ID number")
	}
	uid := claimsFrom(r).Sub
	err := s.tx(r.Context(), func(tx pgx.Tx) error {
		if _, err := tx.Exec(r.Context(), `
			INSERT INTO guest_profiles (user_id, full_name, id_type, id_number_enc, id_number_hash, id_last4, nationality)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (user_id) DO UPDATE SET full_name = EXCLUDED.full_name, id_type = EXCLUDED.id_type,
			  id_number_enc = EXCLUDED.id_number_enc, id_number_hash = EXCLUDED.id_number_hash,
			  id_last4 = EXCLUDED.id_last4, nationality = EXCLUDED.nationality`,
			uid, name, in.IDType, s.keys.Seal(number), s.keys.HashID(number), last4(number), nat); err != nil {
			return err
		}
		return s.audit(r, tx, "guest_profile_saved", nil) // never records the number itself
	})
	if err != nil {
		return err
	}
	p, err := s.readProfile(r, uid)
	if err != nil {
		return err
	}
	respond(w, http.StatusOK, profileResponse{httpx.NowMeta(), p})
	return nil
}

func (s *Server) deleteProfile(w http.ResponseWriter, r *http.Request) error {
	err := s.tx(r.Context(), func(tx pgx.Tx) error {
		if _, err := tx.Exec(r.Context(), `DELETE FROM guest_profiles WHERE user_id = $1`, claimsFrom(r).Sub); err != nil {
			return err
		}
		if _, err := tx.Exec(r.Context(), `DELETE FROM guest_id_photos WHERE user_id = $1`, claimsFrom(r).Sub); err != nil {
			return err
		}
		return s.audit(r, tx, "guest_profile_deleted", nil)
	})
	if err != nil {
		return err
	}
	respond(w, http.StatusOK, profileResponse{httpx.NowMeta(), nil})
	return nil
}

// audit appends to the append-only audit log.
func (s *Server) audit(r *http.Request, tx pgx.Tx, action string, details map[string]any) error {
	c := claimsFrom(r)
	if details == nil {
		details = map[string]any{}
	}
	var dev any
	if c.Dev != "" {
		dev = c.Dev
	}
	_, err := tx.Exec(r.Context(), `INSERT INTO audit_log (user_id, device_pk, action, ip, details) VALUES ($1, $2, $3, NULLIF($4, '')::inet, $5)`,
		c.Sub, dev, action, clientIP(r), details)
	return err
}
