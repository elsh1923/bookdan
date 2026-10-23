package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/bookdan/backend/internal/httpx"
)

// The admin web page lives at /admin and talks to /v1/admin/*. Two kinds of people use it:
//   - platform admins (the emails in ADMIN_EMAILS) sign in with ADMIN_PASSWORD and manage every hotel;
//   - hotel owners (only when ADMIN_OWNERS=true) sign in with a code and manage only their own hotels.
// Receptionists never use it. The token lasts 8 hours and has no refresh: sign in again.

const adminTokenTTL = 8 * time.Hour

var (
	hotelTypes    = map[string]bool{"hotel": true, "guesthouse": true, "lodge": true, "resort": true}
	hotelStatuses = map[string]bool{"draft": true, "active": true, "hidden": true}
	amenityList   = map[string]bool{"generator": true, "hot_water": true, "wifi": true, "parking": true, "pay_at_hotel": true}
	phoneAnyRE    = regexp.MustCompile(`^\+?[0-9 ()-]{7,20}$`)
)

// loginLimiter slows down password guessing: 8 tries per 10 minutes per address and email.
type loginLimiter struct {
	mu sync.Mutex
	m  map[string][]time.Time
}

func (l *loginLimiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.m == nil {
		l.m = map[string][]time.Time{}
	}
	recent := l.m[key][:0]
	for _, t := range l.m[key] {
		if now.Sub(t) < 10*time.Minute {
			recent = append(recent, t)
		}
	}
	if len(recent) >= 8 {
		l.m[key] = recent
		return false
	}
	l.m[key] = append(recent, now)
	return true
}

func (s *Server) adminEnabled() bool { return s.cfg.AdminPassword != "" || s.cfg.AdminOwners }

func (s *Server) adminToken(userID, role string) string {
	now := s.now()
	return s.keys.SignJWT(claims{Sub: userID, Role: role, App: "admin", Iat: now.Unix(), Exp: now.Add(adminTokenTTL).Unix()})
}

// adminConfig tells the page which sign-in forms to offer.
func (s *Server) adminConfig(w http.ResponseWriter, _ *http.Request) error {
	respond(w, http.StatusOK, struct {
		httpx.Meta
		Password bool `json:"password"`
		Owners   bool `json:"owners"`
	}{httpx.NowMeta(), s.cfg.AdminPassword != "", s.cfg.AdminOwners})
	return nil
}

func (s *Server) adminLogin(w http.ResponseWriter, r *http.Request) error {
	if s.cfg.AdminPassword == "" {
		return errNotFound
	}
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if !s.adminLimit.allow(clientIP(r)+"|"+email, s.now()) {
		return fail(http.StatusTooManyRequests, codeTooMany, "Too many attempts. Try again in a few minutes.")
	}
	// Both checks always run, so the response time does not reveal whether the email was on the list.
	want := sha256.Sum256([]byte(s.cfg.AdminPassword))
	got := sha256.Sum256([]byte(in.Password))
	passOK := hmac.Equal(want[:], got[:])
	listed := false
	for _, e := range s.cfg.AdminEmails {
		if e == email {
			listed = true
		}
	}
	if !passOK || !listed {
		return fail(http.StatusUnauthorized, httpx.CodeUnauthenticated, "Wrong email or password")
	}
	var uid string
	if err := s.pool.QueryRow(r.Context(), `INSERT INTO users (email, role) VALUES ($1, 'admin')
		ON CONFLICT (email) DO UPDATE SET role = 'admin' RETURNING id`, email).Scan(&uid); err != nil {
		return err
	}
	if _, err := s.pool.Exec(r.Context(), `INSERT INTO audit_log (user_id, action, ip, details) VALUES ($1, 'admin_login', NULLIF($2, '')::inet, '{}')`, uid, clientIP(r)); err != nil {
		return err
	}
	respond(w, http.StatusOK, struct {
		httpx.Meta
		Token string `json:"token"`
		Role  string `json:"role"`
	}{httpx.NowMeta(), s.adminToken(uid, "admin"), "admin"})
	return nil
}

// adminOTPRequest sends a sign-in code to a hotel owner (same limits as the apps).
func (s *Server) adminOTPRequest(w http.ResponseWriter, r *http.Request) error {
	if !s.cfg.AdminOwners {
		return errNotFound
	}
	return s.requestOTP(w, r)
}

// adminOTPVerify signs in a hotel owner with the code. Only people who own a hotel get in.
func (s *Server) adminOTPVerify(w http.ResponseWriter, r *http.Request) error {
	if !s.cfg.AdminOwners {
		return errNotFound
	}
	var in struct {
		identity
		Code string `json:"code"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	id, _, err := in.identity.clean()
	if err != nil {
		return err
	}
	ctx := r.Context()
	var uid string
	err = s.tx(ctx, func(tx pgx.Tx) error {
		if err := s.checkOTP(ctx, tx, id, in.Code); err != nil {
			return err
		}
		col := "phone"
		if id.Type == "email" {
			col = "email"
		}
		err := tx.QueryRow(ctx, `SELECT u.id FROM users u WHERE u.`+col+` = $1 AND u.status = 'active'
			AND EXISTS (SELECT 1 FROM property_staff ps WHERE ps.user_id = u.id AND ps.role = 'owner')`, id.Value).Scan(&uid)
		if err == pgx.ErrNoRows {
			return fail(http.StatusForbidden, codeNotStaff, "Only hotel owners can use the admin page")
		}
		return err
	})
	var wc *wrongCode
	if asWrong(err, &wc) {
		_, _ = s.pool.Exec(ctx, `UPDATE otp_challenges SET attempts = attempts + 1 WHERE id = $1 AND attempts < max_attempts`, wc.id)
		return fail(http.StatusUnauthorized, codeInvalidOTP, "That code is wrong or has expired")
	}
	if err != nil {
		return err
	}
	respond(w, http.StatusOK, struct {
		httpx.Meta
		Token string `json:"token"`
		Role  string `json:"role"`
	}{httpx.NowMeta(), s.adminToken(uid, "owner"), "owner"})
	return nil
}

// canManage allows platform admins everywhere and owners only for their own hotels. Anyone else is told the hotel does not exist.
func (s *Server) canManage(r *http.Request, propertyID string) error {
	c := claimsFrom(r)
	if c.Role == "admin" {
		var exists bool
		if err := s.pool.QueryRow(r.Context(), `SELECT EXISTS (SELECT 1 FROM properties WHERE id = $1)`, propertyID).Scan(&exists); err != nil {
			if isBadUUID(err) {
				return errNotFound
			}
			return err
		}
		if !exists {
			return errNotFound
		}
		return nil
	}
	var ok bool
	if err := s.pool.QueryRow(r.Context(), `SELECT EXISTS (SELECT 1 FROM property_staff WHERE user_id = $1 AND property_id = $2 AND role = 'owner')`, c.Sub, propertyID).Scan(&ok); err != nil {
		if isBadUUID(err) {
			return errNotFound
		}
		return err
	}
	if !ok {
		return errNotFound
	}
	return nil
}

func requireAdmin(r *http.Request) error {
	if claimsFrom(r).Role != "admin" {
		return fail(http.StatusForbidden, codeForbidden, "Only a platform admin can do this")
	}
	return nil
}

// adminAudit records a change (who, what, which hotel) in the append-only audit log.
func (s *Server) adminAudit(r *http.Request, tx pgx.Tx, action, propertyID string, extra map[string]any) error {
	d := map[string]any{"property_id": propertyID}
	for k, v := range extra {
		d[k] = v
	}
	return s.audit(r, tx, action, d)
}

func (s *Server) adminRoutes(r chi.Router) {
	r.Get("/admin/config", s.h(s.adminConfig))
	r.Post("/admin/login", s.h(s.adminLogin))
	r.Post("/admin/otp/request", s.h(s.adminOTPRequest))
	r.Post("/admin/otp/verify", s.h(s.adminOTPVerify))
	r.Group(func(r chi.Router) {
		r.Use(s.authenticate("admin"))
		r.Get("/admin/me", s.h(s.adminMe))
		r.Get("/admin/hotels", s.h(s.adminListHotels))
		r.Post("/admin/hotels", s.h(s.adminCreateHotel))
		r.Get("/admin/hotels/{id}", s.h(s.adminGetHotel))
		r.Put("/admin/hotels/{id}", s.h(s.adminUpdateHotel))
		r.Post("/admin/hotels/{id}/rooms", s.h(s.adminCreateRoom))
		r.Put("/admin/hotels/{id}/rooms/{room}", s.h(s.adminUpdateRoom))
		r.Put("/admin/hotels/{id}/rooms/{room}/price", s.h(s.adminSetPrice))
		r.Post("/admin/hotels/{id}/photos", s.h(s.adminUploadPhoto))
		r.Post("/admin/hotels/{id}/photos/{photo}/cover", s.h(s.adminCoverPhoto))
		r.Delete("/admin/hotels/{id}/photos/{photo}", s.h(s.adminDeletePhoto))
		r.Post("/admin/hotels/{id}/staff", s.h(s.adminAddStaff))
		r.Delete("/admin/hotels/{id}/staff/{user}", s.h(s.adminRemoveStaff))
	})
	r.Get("/photos/{id}/{size}", s.h(s.servePhoto))
}

func (s *Server) adminMe(w http.ResponseWriter, r *http.Request) error {
	c := claimsFrom(r)
	var contact string
	if err := s.pool.QueryRow(r.Context(), `SELECT coalesce(email, phone) FROM users WHERE id = $1`, c.Sub).Scan(&contact); err != nil {
		return err
	}
	respond(w, http.StatusOK, struct {
		httpx.Meta
		Role    string `json:"role"`
		Contact string `json:"contact"`
	}{httpx.NowMeta(), c.Role, contact})
	return nil
}
