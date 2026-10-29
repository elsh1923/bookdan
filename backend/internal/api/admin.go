package api

import (
	"context"
	"net/http"
	"regexp"
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
	amenityList   = map[string]bool{"generator": true, "hot_water": true, "wifi": true, "parking": true, "pay_at_hotel": true,
		"ac": true, "pool": true, "restaurant": true, "gym": true, "spa": true, "airport_shuttle": true, "laundry": true, "front_desk_24h": true, "water_tank": true}
	phoneAnyRE = regexp.MustCompile(`^\+?[0-9 ()-]{7,20}$`)
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

// adminEnabled says whether the admin page is on: the first-time shared password is set, hotel owners may use it, or someone has an admin account.
func (s *Server) adminEnabled(ctx context.Context) bool {
	if s.cfg.AdminPassword != "" || s.ownerAdminEnabled() {
		return true
	}
	var any bool
	_ = s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM admin_accounts)`).Scan(&any)
	return any
}

// adminToken signs an admin-page session. adminRole (super, support or finance) is set for a platform admin and empty for a hotel owner.
func (s *Server) adminToken(userID, role, adminRole string) string {
	now := s.now()
	return s.keys.SignJWT(claims{Sub: userID, Role: role, AR: adminRole, App: "admin", Iat: now.Unix(), Exp: now.Add(adminTokenTTL).Unix()})
}

// adminConfig tells the page which sign-in forms to offer.
func (s *Server) adminConfig(w http.ResponseWriter, r *http.Request) error {
	var accounts bool
	if err := s.pool.QueryRow(r.Context(), `SELECT EXISTS (SELECT 1 FROM admin_accounts)`).Scan(&accounts); err != nil {
		return err
	}
	respond(w, http.StatusOK, struct {
		httpx.Meta
		Password bool `json:"password"`
		Owners   bool `json:"owners"`
	}{httpx.NowMeta(), s.cfg.AdminPassword != "" || accounts, s.ownerAdminEnabled()})
	return nil
}

// adminOTPRequest sends a sign-in code to a hotel owner (same limits as the apps).
func (s *Server) adminOTPRequest(w http.ResponseWriter, r *http.Request) error {
	if !s.ownerAdminEnabled() {
		return errNotFound
	}
	return s.requestOTP(w, r)
}

// adminDemoAccount refuses the demo accounts (no inbox, fixed public code) on the admin page: they could be used by anyone.
func (s *Server) adminDemoAccount(id identity, channel string) error {
	if s.fixedCode(id, channel) {
		return fail(http.StatusForbidden, codeForbidden, "This demo account cannot use the admin page")
	}
	return nil
}

// adminOTPVerify signs in a hotel owner with the code. Only people who own a hotel get in.
func (s *Server) adminOTPVerify(w http.ResponseWriter, r *http.Request) error {
	if !s.ownerAdminEnabled() {
		return errNotFound
	}
	var in struct {
		identity
		Code string `json:"code"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	id, channel, err := in.identity.clean()
	if err != nil {
		return err
	}
	if err := s.adminDemoAccount(id, channel); err != nil {
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
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE property_staff SET joined_at = now() WHERE user_id = $1 AND joined_at IS NULL`, uid)
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
	}{httpx.NowMeta(), s.adminToken(uid, "owner", ""), "owner"})
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
		r.Use(s.authenticate("admin"), s.adminGate)
		// Who may do what: super admins everything; support the support queue and reviews; finance the money pages. Hotel owners pass the role check and are limited
		// to their own hotels in each handler. Reading the hotel list, analytics, calls, coupons and rates is open to every admin role.
		super, support, finance := s.allow(), s.allow("support"), s.allow("finance")

		r.Get("/admin/me", s.h(s.adminMe))
		r.Post("/admin/me/password", s.h(s.adminChangePassword))
		r.Post("/admin/me/mfa/setup", s.h(s.adminMFASetup))
		r.Post("/admin/me/mfa/enable", s.h(s.adminMFAEnable))
		r.Post("/admin/me/mfa/disable", s.h(s.adminMFADisable))
		r.With(super).Get("/admin/admins", s.h(s.adminList))
		r.With(super).Post("/admin/admins", s.h(s.adminCreate))
		r.With(super).Put("/admin/admins/{id}", s.h(s.adminUpdate))
		r.With(super).Post("/admin/admins/{id}/reset-password", s.h(s.adminResetPassword))
		r.With(super).Post("/admin/admins/{id}/reset-mfa", s.h(s.adminResetMFA))

		r.Get("/admin/hotels", s.h(s.adminListHotels))
		r.Get("/admin/hotels/{id}", s.h(s.adminGetHotel))
		r.Get("/admin/coupons", s.h(s.adminCoupons))
		r.Get("/admin/analytics", s.h(s.adminAnalytics))
		r.Get("/admin/calls", s.h(s.adminCalls))
		r.Get("/admin/rates", s.h(s.adminRates))

		r.With(super).Post("/admin/hotels", s.h(s.adminCreateHotel))
		r.With(super).Put("/admin/hotels/{id}", s.h(s.adminUpdateHotel))
		r.With(super).Post("/admin/hotels/{id}/rooms", s.h(s.adminCreateRoom))
		r.With(super).Put("/admin/hotels/{id}/rooms/{room}", s.h(s.adminUpdateRoom))
		r.With(super).Put("/admin/hotels/{id}/rooms/{room}/price", s.h(s.adminSetPrice))
		r.With(super).Post("/admin/hotels/{id}/photos", s.h(s.adminUploadPhoto))
		r.With(super).Post("/admin/hotels/{id}/rooms/{room}/photos", s.h(s.adminUploadRoomPhoto))
		r.With(super).Post("/admin/hotels/{id}/photos/{photo}/cover", s.h(s.adminCoverPhoto))
		r.With(super).Delete("/admin/hotels/{id}/photos/{photo}", s.h(s.adminDeletePhoto))
		r.With(super).Put("/admin/hotels/{id}/business", s.h(s.adminSetBusiness))
		r.With(super).Post("/admin/coupons", s.h(s.adminCreateCoupon))
		r.With(super).Put("/admin/coupons/{id}", s.h(s.adminUpdateCoupon))
		r.With(super).Get("/admin/applications", s.h(s.adminApplications))
		r.With(super).Post("/admin/applications/{id}/approve", s.h(s.adminApproveApplication))
		r.With(super).Post("/admin/applications/{id}/reject", s.h(s.adminRejectApplication))
		r.With(super).Put("/admin/hotels/{id}/alerts", s.h(s.adminSetAlertRules))
		r.With(super).Put("/admin/hotels/{id}/staff/{user}/alerts", s.h(s.adminSetStaffAlerts))
		r.With(super).Post("/admin/hotels/{id}/staff", s.h(s.adminAddStaff))
		r.With(super).Delete("/admin/hotels/{id}/staff/{user}", s.h(s.adminRemoveStaff))

		r.With(support).Get("/admin/support/tickets", s.h(s.adminTickets))
		r.With(support).Get("/admin/support/tickets/{id}", s.h(s.adminGetTicket))
		r.With(support).Post("/admin/support/tickets/{id}/reply", s.h(s.adminReplyTicket))
		r.With(support).Post("/admin/support/tickets/{id}/close", s.h(s.adminCloseTicket))
		r.With(support).Post("/admin/support/tickets/{id}/resolve", s.h(s.adminResolveDispute))
		r.With(support).Post("/admin/support/tickets/{id}/refund", s.h(s.adminTicketRefund))
		r.With(support).Post("/admin/support/tickets/{id}/relocation", s.h(s.adminResolveRelocation))
		r.With(support).Get("/admin/reviews", s.h(s.adminReviews))
		r.With(support).Post("/admin/reviews/{id}/hide", s.h(s.adminHideReview))
		r.With(support).Post("/admin/reviews/{id}/restore", s.h(s.adminRestoreReview))

		r.With(finance).Post("/admin/hotels/{id}/business/payout/reveal", s.h(s.adminRevealPayout))
		r.With(finance).Get("/admin/payouts", s.h(s.adminPayouts))
		r.With(finance).Get("/admin/hotels/{id}/payouts", s.h(s.adminHotelPayouts))
		r.With(finance).Post("/admin/hotels/{id}/payouts", s.h(s.adminRecordPayout))
		r.With(finance).Post("/admin/hotels/{id}/receipts", s.h(s.adminRecordReceipt))
		r.With(finance).Get("/admin/hotels/{id}/statements", s.h(s.adminStatement))
		r.With(finance).Get("/admin/payment-issues", s.h(s.adminPaymentIssues))
		r.With(finance).Post("/admin/payment-issues/{id}/resolve", s.h(s.adminResolvePaymentIssue))
		r.With(finance).Put("/admin/rates/{currency}", s.h(s.adminSetRate))
		r.With(finance).Post("/admin/rates/refresh", s.h(s.adminRefreshRates))
	})
	r.Get("/photos/{id}/{size}", s.h(s.servePhoto))
}
