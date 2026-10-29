package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/bookdan/backend/internal/httpx"
)

// Admin accounts: BookDan staff each have their own sign-in, a role, and optionally an authenticator-app code.
//
//   - super:   everything, including adding and disabling other admins;
//   - support: the support queue (answers, disputes, goodwill refunds, relocations) and review moderation;
//   - finance: payouts, receipts, statements, payment issues and exchange rates, and the hotels' payout details.
//
// Every role can read the hotel list, analytics, calls, coupons and rates. Hotel owners who use the admin page keep their own limited access.
//
// The shared ADMIN_PASSWORD still works for the emails in ADMIN_EMAILS, as the first way in (it acts as a super admin). As soon as an email has its own
// account here, only that account's password works for it, so the shared password can be taken out of the server settings.
//
// Safeguards: five wrong passwords or codes lock the account for 15 minutes (kept in the database, so it holds across servers); an account can be
// disabled at once (every request re-checks it); a password set by someone else must be replaced at the first sign-in; a new password ends the old sessions;
// the last active super admin cannot be disabled or demoted; every change is in the audit log.

const (
	adminLockAfter = 5
	adminLockFor   = 15 * time.Minute
)

var adminRoles = map[string]bool{"super": true, "support": true, "finance": true}

// adminCan is what each role may do, for the page to show the right tabs (the server enforces it on every route).
func adminCan(role string) map[string]bool {
	super := role == "super"
	return map[string]bool{
		"hotels_write": super, "coupons_write": super, "applications": super, "admins": super,
		"support": super || role == "support", "reviews": super || role == "support",
		"finance": super || role == "finance", "rates_write": super || role == "finance",
	}
}

type adminAcct struct {
	UserID      string
	Email       string
	Role        string
	Hash        string
	MustChange  bool
	Disabled    bool
	TOTPEnc     []byte
	LastStep    int64
	LockedUntil *time.Time
	ChangedAt   *time.Time
}

// adminAccount finds the account for an email (nil when the person has none).
func (s *Server) adminAccount(ctx context.Context, email string) (*adminAcct, error) {
	var a adminAcct
	err := s.pool.QueryRow(ctx, `SELECT a.user_id, u.email, a.role, a.password_hash, a.must_change_password, a.disabled_at IS NOT NULL, a.totp_secret_enc, a.totp_last_step, a.locked_until, a.password_changed_at
		FROM admin_accounts a JOIN users u ON u.id = a.user_id WHERE u.email = $1`, email).
		Scan(&a.UserID, &a.Email, &a.Role, &a.Hash, &a.MustChange, &a.Disabled, &a.TOTPEnc, &a.LastStep, &a.LockedUntil, &a.ChangedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return &a, err
}

// adminFailed counts a wrong password or code and locks the account after too many.
func (s *Server) adminFailed(ctx context.Context, userID string) {
	_, err := s.pool.Exec(ctx, `UPDATE admin_accounts SET
			locked_until = CASE WHEN failed_attempts + 1 >= $2 THEN now() + $3::interval ELSE locked_until END,
			failed_attempts = CASE WHEN failed_attempts + 1 >= $2 THEN 0 ELSE failed_attempts + 1 END
		WHERE user_id = $1`, userID, adminLockAfter, fmt.Sprintf("%d seconds", int(adminLockFor.Seconds())))
	if err != nil {
		slog.Error("admin failed-attempt count", "err", err)
	}
}

// auditAnonymous writes an audit entry for someone who is not signed in (a failed sign-in).
func (s *Server) auditAnonymous(r *http.Request, userID, action string, details map[string]any) {
	var uid any
	if userID != "" {
		uid = userID
	}
	if _, err := s.pool.Exec(r.Context(), `INSERT INTO audit_log (user_id, action, ip, details) VALUES ($1, $2, NULLIF($3, '')::inet, $4)`, uid, action, clientIP(r), details); err != nil {
		slog.Error("admin audit", "err", err)
	}
}

type adminLoginOut struct {
	httpx.Meta
	Token      string `json:"token"`
	Role       string `json:"role"`
	AdminRole  string `json:"admin_role"`
	MustChange bool   `json:"must_change_password"`
}

// adminLogin signs in a platform admin: their own account if they have one, otherwise the shared password of the first-time setup.
func (s *Server) adminLogin(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	ctx := r.Context()
	var accounts int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM admin_accounts`).Scan(&accounts); err != nil {
		return err
	}
	if s.cfg.AdminPassword == "" && accounts == 0 {
		return errNotFound
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if !s.adminLimit.allow(clientIP(r)+"|"+email, s.now()) {
		return fail(http.StatusTooManyRequests, codeTooMany, "Too many attempts. Try again in a few minutes.")
	}
	wrong := fail(http.StatusUnauthorized, httpx.CodeUnauthenticated, "Wrong email or password")
	acct, err := s.adminAccount(ctx, email)
	if err != nil {
		return err
	}

	var uid, role string
	mustChange := false
	if acct != nil {
		if acct.Disabled {
			spendPasswordTime(in.Password)
			s.auditAnonymous(r, acct.UserID, "admin_login_failed", map[string]any{"email": email, "reason": "disabled"})
			return wrong
		}
		if acct.LockedUntil != nil && acct.LockedUntil.After(s.now()) {
			return fail(http.StatusTooManyRequests, codeTooMany, "This account is locked for a few minutes after too many wrong attempts.")
		}
		if !checkPassword(in.Password, acct.Hash) {
			s.adminFailed(ctx, acct.UserID)
			s.auditAnonymous(r, acct.UserID, "admin_login_failed", map[string]any{"email": email, "reason": "password"})
			return wrong
		}
		if acct.TOTPEnc != nil {
			if strings.TrimSpace(in.Code) == "" {
				return fail(http.StatusUnauthorized, "MFA_REQUIRED", "Enter the code from your authenticator app")
			}
			if !s.acceptTOTP(ctx, acct.UserID, acct.TOTPEnc, in.Code, acct.LastStep) {
				s.adminFailed(ctx, acct.UserID)
				s.auditAnonymous(r, acct.UserID, "admin_login_failed", map[string]any{"email": email, "reason": "code"})
				return fail(http.StatusUnauthorized, "MFA_INVALID", "That code is wrong or has expired")
			}
		}
		if _, err := s.pool.Exec(ctx, `UPDATE admin_accounts SET failed_attempts = 0, locked_until = NULL, last_login_at = now() WHERE user_id = $1`, acct.UserID); err != nil {
			return err
		}
		uid, role, mustChange = acct.UserID, acct.Role, acct.MustChange
	} else {
		// The shared password of the first-time setup. Both checks always run, so the response time does not reveal whether the email was on the list.
		passOK := s.cfg.AdminPassword != "" && secretsEqual(s.cfg.AdminPassword, in.Password)
		listed := false
		for _, e := range s.cfg.AdminEmails {
			if e == email {
				listed = true
			}
		}
		if !passOK || !listed {
			spendPasswordTime(in.Password)
			s.auditAnonymous(r, "", "admin_login_failed", map[string]any{"email": email, "reason": "shared password"})
			return wrong
		}
		if err := s.pool.QueryRow(ctx, `INSERT INTO users (email, role) VALUES ($1, 'admin')
			ON CONFLICT (email) DO UPDATE SET role = 'admin' RETURNING id`, email).Scan(&uid); err != nil {
			return err
		}
		role = "super"
	}
	if _, err := s.pool.Exec(ctx, `INSERT INTO audit_log (user_id, action, ip, details) VALUES ($1, 'admin_login', NULLIF($2, '')::inet, $3)`, uid, clientIP(r), map[string]any{"role": role}); err != nil {
		return err
	}
	respond(w, http.StatusOK, adminLoginOut{httpx.NowMeta(), s.adminToken(uid, "admin", role), "admin", role, mustChange})
	return nil
}

// acceptTOTP checks an authenticator code and records the step it matched, so the same code is not accepted twice (also against two requests at once).
func (s *Server) acceptTOTP(ctx context.Context, userID string, sealed []byte, code string, lastStep int64) bool {
	plain, err := s.keys.Open(sealed)
	if err != nil {
		return false
	}
	secret, err := b32.DecodeString(plain)
	if err != nil {
		return false
	}
	step, ok := totpAccept(secret, code, s.now(), lastStep)
	if !ok {
		return false
	}
	tag, err := s.pool.Exec(ctx, `UPDATE admin_accounts SET totp_last_step = $2 WHERE user_id = $1 AND totp_last_step < $2`, userID, step)
	return err == nil && tag.RowsAffected() == 1
}

// ---- who is allowed in, on every request

// adminGate runs after the token is verified. For a platform admin it re-checks the account: still enabled, still the same role, the password not
// changed since the token was issued, and (for the shared first-time password) the email still listed. A password someone else chose must be replaced
// before anything else is allowed. Hotel owners pass: their routes check their own hotels.
func (s *Server) adminGate(next http.Handler) http.Handler {
	return s.h(func(w http.ResponseWriter, r *http.Request) error {
		c := claimsFrom(r)
		if c.Role != "admin" {
			next.ServeHTTP(w, r)
			return nil
		}
		var role string
		var disabled, must bool
		var changed *time.Time
		err := s.pool.QueryRow(r.Context(), `SELECT role, disabled_at IS NOT NULL, must_change_password, password_changed_at FROM admin_accounts WHERE user_id = $1`, c.Sub).Scan(&role, &disabled, &must, &changed)
		switch {
		case err == pgx.ErrNoRows:
			// No account of their own: only the first-time shared password, and only while it is still set and the email is on the list.
			var email string
			if err := s.pool.QueryRow(r.Context(), `SELECT coalesce(email, '') FROM users WHERE id = $1`, c.Sub).Scan(&email); err != nil {
				return err
			}
			listed := false
			for _, e := range s.cfg.AdminEmails {
				if e == email {
					listed = true
				}
			}
			if !listed || s.cfg.AdminPassword == "" {
				return errAuth
			}
			role = "super"
		case err != nil:
			return err
		}
		if disabled || role != c.AR || (changed != nil && c.Iat < changed.Unix()) {
			return errAuth
		}
		if must && r.URL.Path != "/v1/admin/me" && r.URL.Path != "/v1/admin/me/password" {
			return fail(http.StatusForbidden, "PASSWORD_CHANGE_REQUIRED", "Choose your own password before you continue")
		}
		next.ServeHTTP(w, r)
		return nil
	})
}

// allow lets a route through for the listed admin roles (super always). Hotel owners pass: those routes check their own hotels in the handler.
func (s *Server) allow(roles ...string) func(http.Handler) http.Handler {
	set := map[string]bool{"super": true}
	for _, r := range roles {
		set[r] = true
	}
	return func(next http.Handler) http.Handler {
		return s.h(func(w http.ResponseWriter, r *http.Request) error {
			if c := claimsFrom(r); c.Role == "admin" && !set[c.AR] {
				return fail(http.StatusForbidden, codeForbidden, "Your admin role cannot do this")
			}
			next.ServeHTTP(w, r)
			return nil
		})
	}
}

// requireSuper is for managing the admins themselves.
func requireSuper(r *http.Request) error {
	if c := claimsFrom(r); c.Role != "admin" || c.AR != "super" {
		return fail(http.StatusForbidden, codeForbidden, "Only a super admin can do this")
	}
	return nil
}

// ---- my account

type adminMeOut struct {
	httpx.Meta
	Role       string          `json:"role"`
	Contact    string          `json:"contact"`
	AdminRole  string          `json:"admin_role,omitempty"`
	Can        map[string]bool `json:"can,omitempty"`
	MFA        bool            `json:"mfa"`
	HasAccount bool            `json:"has_account"`
	MustChange bool            `json:"must_change_password"`
}

func (s *Server) adminMe(w http.ResponseWriter, r *http.Request) error {
	c := claimsFrom(r)
	var contact string
	if err := s.pool.QueryRow(r.Context(), `SELECT coalesce(email, phone) FROM users WHERE id = $1`, c.Sub).Scan(&contact); err != nil {
		return err
	}
	out := adminMeOut{Meta: httpx.NowMeta(), Role: c.Role, Contact: contact}
	if c.Role == "admin" {
		out.AdminRole, out.Can = c.AR, adminCan(c.AR)
		var mfa *time.Time
		err := s.pool.QueryRow(r.Context(), `SELECT totp_enabled_at, must_change_password FROM admin_accounts WHERE user_id = $1`, c.Sub).Scan(&mfa, &out.MustChange)
		if err == nil {
			out.HasAccount, out.MFA = true, mfa != nil
		} else if err != pgx.ErrNoRows {
			return err
		}
	}
	respond(w, http.StatusOK, out)
	return nil
}

// ownAccount loads the signed-in admin's account, locked for the change. A person on the shared first-time password has none yet.
func (s *Server) ownAccount(ctx context.Context, tx pgx.Tx, r *http.Request) (hash string, totp []byte, email string, err error) {
	if err := requireAdmin(r); err != nil {
		return "", nil, "", err
	}
	err = tx.QueryRow(ctx, `SELECT a.password_hash, a.totp_secret_enc, u.email FROM admin_accounts a JOIN users u ON u.id = a.user_id WHERE a.user_id = $1 FOR UPDATE OF a`, claimsFrom(r).Sub).Scan(&hash, &totp, &email)
	if err == pgx.ErrNoRows {
		return "", nil, "", fail(http.StatusConflict, "NO_ACCOUNT", "You are using the shared setup password. Ask a super admin to create your own account, or create it yourself in the Admins tab.")
	}
	return hash, totp, email, err
}

// adminChangePassword sets a new password (the current one is needed) and returns a fresh token: the old sessions end.
func (s *Server) adminChangePassword(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Current string `json:"current_password"`
		New     string `json:"new_password"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	ctx, uid := r.Context(), claimsFrom(r).Sub
	var role string
	wrongCurrent := false
	err := s.tx(ctx, func(tx pgx.Tx) error {
		hash, _, email, err := s.ownAccount(ctx, tx, r)
		if err != nil {
			return err
		}
		if !checkPassword(in.Current, hash) {
			wrongCurrent = true
			return fail(http.StatusUnauthorized, httpx.CodeUnauthenticated, "The current password is wrong")
		}
		if p := passwordProblem(in.New, email); p != "" {
			return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, p)
		}
		if checkPassword(in.New, hash) {
			return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "Choose a password you have not used before")
		}
		nh, err := hashPassword(in.New)
		if err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `UPDATE admin_accounts SET password_hash = $2, must_change_password = false, password_changed_at = now() WHERE user_id = $1 RETURNING role`, uid, nh).Scan(&role); err != nil {
			return err
		}
		return s.audit(r, tx, "admin_password_changed", nil)
	})
	if err != nil {
		if wrongCurrent {
			s.adminFailed(ctx, uid) // guessing the current password counts like guessing at sign-in
		}
		return err
	}
	respond(w, http.StatusOK, adminLoginOut{httpx.NowMeta(), s.adminToken(uid, "admin", role), "admin", role, false})
	return nil
}

// adminMFASetup starts adding an authenticator app: a new secret to put into the app, not active until a code from the app confirms it.
func (s *Server) adminMFASetup(w http.ResponseWriter, r *http.Request) error {
	ctx, uid := r.Context(), claimsFrom(r).Sub
	var secret []byte
	var email string
	err := s.tx(ctx, func(tx pgx.Tx) error {
		_, totp, em, err := s.ownAccount(ctx, tx, r)
		if err != nil {
			return err
		}
		if totp != nil {
			return fail(http.StatusConflict, codeInvalidState, "The authenticator app is already set up. Turn it off first to set it up again.")
		}
		email = em
		if secret, err = newTOTPSecret(); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE admin_accounts SET totp_pending_enc = $2 WHERE user_id = $1`, uid, s.keys.Seal(b32.EncodeToString(secret)))
		return err
	})
	if err != nil {
		return err
	}
	respond(w, http.StatusOK, struct {
		httpx.Meta
		Secret string `json:"secret"`
		URI    string `json:"uri"`
	}{httpx.NowMeta(), b32.EncodeToString(secret), totpURI(email, secret)})
	return nil
}

// adminMFAEnable confirms the authenticator app with a code from it.
func (s *Server) adminMFAEnable(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Code string `json:"code"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	ctx, uid := r.Context(), claimsFrom(r).Sub
	err := s.tx(ctx, func(tx pgx.Tx) error {
		if _, _, _, err := s.ownAccount(ctx, tx, r); err != nil {
			return err
		}
		var pending []byte
		if err := tx.QueryRow(ctx, `SELECT totp_pending_enc FROM admin_accounts WHERE user_id = $1`, uid).Scan(&pending); err != nil {
			return err
		}
		if pending == nil {
			return fail(http.StatusConflict, codeInvalidState, "Start the setup first")
		}
		plain, err := s.keys.Open(pending)
		if err != nil {
			return err
		}
		secret, err := b32.DecodeString(plain)
		if err != nil {
			return err
		}
		step, ok := totpAccept(secret, in.Code, s.now(), 0)
		if !ok {
			return fail(http.StatusBadRequest, "MFA_INVALID", "That code is wrong or has expired")
		}
		if _, err := tx.Exec(ctx, `UPDATE admin_accounts SET totp_secret_enc = totp_pending_enc, totp_pending_enc = NULL, totp_enabled_at = now(), totp_last_step = $2 WHERE user_id = $1`, uid, step); err != nil {
			return err
		}
		return s.audit(r, tx, "admin_mfa_enabled", nil)
	})
	if err != nil {
		return err
	}
	respond(w, http.StatusOK, httpx.NowMeta())
	return nil
}

// adminMFADisable turns the authenticator app off; it needs the password and a current code.
func (s *Server) adminMFADisable(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	ctx, uid := r.Context(), claimsFrom(r).Sub
	err := s.tx(ctx, func(tx pgx.Tx) error {
		hash, totp, _, err := s.ownAccount(ctx, tx, r)
		if err != nil {
			return err
		}
		if totp == nil {
			return fail(http.StatusConflict, codeInvalidState, "The authenticator app is not set up")
		}
		if !checkPassword(in.Password, hash) {
			return fail(http.StatusUnauthorized, httpx.CodeUnauthenticated, "The password is wrong")
		}
		var last int64
		if err := tx.QueryRow(ctx, `SELECT totp_last_step FROM admin_accounts WHERE user_id = $1`, uid).Scan(&last); err != nil {
			return err
		}
		plain, err := s.keys.Open(totp)
		if err != nil {
			return err
		}
		secret, err := b32.DecodeString(plain)
		if err != nil {
			return err
		}
		if _, ok := totpAccept(secret, in.Code, s.now(), last); !ok {
			return fail(http.StatusBadRequest, "MFA_INVALID", "That code is wrong or has expired")
		}
		if _, err := tx.Exec(ctx, `UPDATE admin_accounts SET totp_secret_enc = NULL, totp_enabled_at = NULL, totp_pending_enc = NULL, totp_last_step = 0 WHERE user_id = $1`, uid); err != nil {
			return err
		}
		return s.audit(r, tx, "admin_mfa_disabled", nil)
	})
	if err != nil {
		return err
	}
	respond(w, http.StatusOK, httpx.NowMeta())
	return nil
}

// ---- managing the admins (super only)

type adminRow struct {
	ID          string  `json:"id"`
	Email       string  `json:"email"`
	Role        string  `json:"role"`
	Disabled    bool    `json:"disabled"`
	MFA         bool    `json:"mfa"`
	MustChange  bool    `json:"must_change_password"`
	Locked      bool    `json:"locked"`
	CreatedAt   string  `json:"created_at"`
	LastLoginAt *string `json:"last_login_at"`
}

func (s *Server) adminList(w http.ResponseWriter, r *http.Request) error {
	if err := requireSuper(r); err != nil {
		return err
	}
	rows, err := s.pool.Query(r.Context(), `SELECT a.user_id, u.email, a.role, a.disabled_at IS NOT NULL, a.totp_enabled_at IS NOT NULL, a.must_change_password,
			coalesce(a.locked_until > now(), false), a.created_at, a.last_login_at
		FROM admin_accounts a JOIN users u ON u.id = a.user_id ORDER BY a.created_at`)
	if err != nil {
		return err
	}
	defer rows.Close()
	out := []adminRow{}
	for rows.Next() {
		var a adminRow
		var created time.Time
		var last *time.Time
		if err := rows.Scan(&a.ID, &a.Email, &a.Role, &a.Disabled, &a.MFA, &a.MustChange, &a.Locked, &created, &last); err != nil {
			return err
		}
		a.CreatedAt = created.UTC().Format(time.RFC3339)
		if last != nil {
			t := last.UTC().Format(time.RFC3339)
			a.LastLoginAt = &t
		}
		out = append(out, a)
	}
	respond(w, http.StatusOK, struct {
		httpx.Meta
		Admins []adminRow `json:"admins"`
	}{httpx.NowMeta(), out})
	return rows.Err()
}

// adminCreate adds an admin. The password is generated and shown once (or the super admin may choose one); the new admin must replace it at the first sign-in.
func (s *Server) adminCreate(w http.ResponseWriter, r *http.Request) error {
	if err := requireSuper(r); err != nil {
		return err
	}
	var in struct {
		Email    string `json:"email"`
		Role     string `json:"role"`
		Password string `json:"password"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	id, channel, err := identity{Type: "email", Value: in.Email}.clean()
	if err != nil || channel != "email" {
		return fail(http.StatusBadRequest, codeInvalidEmail, "Enter a valid email address")
	}
	if !adminRoles[in.Role] {
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "The role must be super, support or finance")
	}
	generated := in.Password == ""
	password := in.Password
	if generated {
		password = tempPassword()
	} else if p := passwordProblem(password, id.Value); p != "" {
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, p)
	}
	hash, err := hashPassword(password)
	if err != nil {
		return err
	}
	ctx, actorID := r.Context(), claimsFrom(r).Sub
	var uid string
	err = s.tx(ctx, func(tx pgx.Tx) error {
		// An address that belongs to a guest or a hotel account cannot become an admin: those are different people's accounts.
		err := tx.QueryRow(ctx, `INSERT INTO users (email, role) VALUES ($1, 'admin') ON CONFLICT (email) DO UPDATE SET role = 'admin' WHERE users.role = 'admin' RETURNING id`, id.Value).Scan(&uid)
		if err == pgx.ErrNoRows {
			return fail(http.StatusConflict, codeInvalidState, "That email already belongs to a guest or hotel account. Use another address.")
		}
		if err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `INSERT INTO admin_accounts (user_id, role, password_hash, created_by) VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING`, uid, in.Role, hash, actorID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return fail(http.StatusConflict, codeInvalidState, "That person is already an admin")
		}
		return s.audit(r, tx, "admin_created", map[string]any{"admin_id": uid, "email": id.Value, "role": in.Role})
	})
	if err != nil {
		return err
	}
	out := struct {
		httpx.Meta
		ID       string `json:"id"`
		Email    string `json:"email"`
		Role     string `json:"role"`
		Password string `json:"temporary_password,omitempty"`
	}{Meta: httpx.NowMeta(), ID: uid, Email: id.Value, Role: in.Role}
	if generated {
		out.Password = password
	}
	respond(w, http.StatusOK, out)
	return nil
}

// adminUpdate changes an admin's role, disables or enables the account, or unlocks it.
func (s *Server) adminUpdate(w http.ResponseWriter, r *http.Request) error {
	if err := requireSuper(r); err != nil {
		return err
	}
	var in struct {
		Role     *string `json:"role"`
		Disabled *bool   `json:"disabled"`
		Unlock   bool    `json:"unlock"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if in.Role != nil && !adminRoles[*in.Role] {
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "The role must be super, support or finance")
	}
	ctx, id, me := r.Context(), chi.URLParam(r, "id"), claimsFrom(r).Sub
	if id == me && (in.Role != nil || in.Disabled != nil) {
		return fail(http.StatusConflict, codeInvalidState, "You cannot change your own role or disable yourself")
	}
	err := s.tx(ctx, func(tx pgx.Tx) error {
		// Lock the super admins first, so two people cannot each remove the other as the last one.
		if _, err := tx.Exec(ctx, `SELECT 1 FROM admin_accounts WHERE role = 'super' AND disabled_at IS NULL FOR UPDATE`); err != nil {
			return err
		}
		var role string
		var disabled bool
		err := tx.QueryRow(ctx, `SELECT role, disabled_at IS NOT NULL FROM admin_accounts WHERE user_id = $1 FOR UPDATE`, id).Scan(&role, &disabled)
		if err == pgx.ErrNoRows || isBadUUID(err) {
			return errNotFound
		}
		if err != nil {
			return err
		}
		newRole, newDisabled := role, disabled
		if in.Role != nil {
			newRole = *in.Role
		}
		if in.Disabled != nil {
			newDisabled = *in.Disabled
		}
		if role == "super" && !disabled && (newRole != "super" || newDisabled) {
			var others int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM admin_accounts WHERE role = 'super' AND disabled_at IS NULL AND user_id <> $1`, id).Scan(&others); err != nil {
				return err
			}
			if others == 0 {
				return fail(http.StatusConflict, codeInvalidState, "There must be at least one active super admin")
			}
		}
		if newRole != role {
			if _, err := tx.Exec(ctx, `UPDATE admin_accounts SET role = $2 WHERE user_id = $1`, id, newRole); err != nil {
				return err
			}
			if err := s.audit(r, tx, "admin_role_changed", map[string]any{"admin_id": id, "from": role, "to": newRole}); err != nil {
				return err
			}
		}
		if newDisabled != disabled {
			if newDisabled {
				_, err = tx.Exec(ctx, `UPDATE admin_accounts SET disabled_at = now(), disabled_by = $2 WHERE user_id = $1`, id, me)
			} else {
				_, err = tx.Exec(ctx, `UPDATE admin_accounts SET disabled_at = NULL, disabled_by = NULL, failed_attempts = 0, locked_until = NULL WHERE user_id = $1`, id)
			}
			if err != nil {
				return err
			}
			action := "admin_enabled"
			if newDisabled {
				action = "admin_disabled"
			}
			if err := s.audit(r, tx, action, map[string]any{"admin_id": id}); err != nil {
				return err
			}
		}
		if in.Unlock {
			if _, err := tx.Exec(ctx, `UPDATE admin_accounts SET failed_attempts = 0, locked_until = NULL WHERE user_id = $1`, id); err != nil {
				return err
			}
			return s.audit(r, tx, "admin_unlocked", map[string]any{"admin_id": id})
		}
		return nil
	})
	if err != nil {
		return err
	}
	respond(w, http.StatusOK, httpx.NowMeta())
	return nil
}

// adminResetPassword gives an admin a new generated password (shown once) that they must replace, and ends their sessions.
func (s *Server) adminResetPassword(w http.ResponseWriter, r *http.Request) error {
	if err := requireSuper(r); err != nil {
		return err
	}
	ctx, id := r.Context(), chi.URLParam(r, "id")
	password := tempPassword()
	hash, err := hashPassword(password)
	if err != nil {
		return err
	}
	err = s.tx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE admin_accounts SET password_hash = $2, must_change_password = true, password_changed_at = now(), failed_attempts = 0, locked_until = NULL WHERE user_id = $1`, id, hash)
		if err != nil {
			if isBadUUID(err) {
				return errNotFound
			}
			return err
		}
		if tag.RowsAffected() == 0 {
			return errNotFound
		}
		return s.audit(r, tx, "admin_password_reset", map[string]any{"admin_id": id})
	})
	if err != nil {
		return err
	}
	respond(w, http.StatusOK, struct {
		httpx.Meta
		Password string `json:"temporary_password"`
	}{httpx.NowMeta(), password})
	return nil
}

// adminResetMFA removes the authenticator app of an admin who lost their phone.
func (s *Server) adminResetMFA(w http.ResponseWriter, r *http.Request) error {
	if err := requireSuper(r); err != nil {
		return err
	}
	ctx, id := r.Context(), chi.URLParam(r, "id")
	err := s.tx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE admin_accounts SET totp_secret_enc = NULL, totp_pending_enc = NULL, totp_enabled_at = NULL, totp_last_step = 0 WHERE user_id = $1`, id)
		if err != nil {
			if isBadUUID(err) {
				return errNotFound
			}
			return err
		}
		if tag.RowsAffected() == 0 {
			return errNotFound
		}
		return s.audit(r, tx, "admin_mfa_reset", map[string]any{"admin_id": id})
	})
	if err != nil {
		return err
	}
	respond(w, http.StatusOK, httpx.NowMeta())
	return nil
}
