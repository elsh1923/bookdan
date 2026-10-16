package api

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"math/big"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/bookdan/backend/internal/httpx"
)

var (
	phoneRE = regexp.MustCompile(`^\+251[79][0-9]{8}$`)
	emailRE = regexp.MustCompile(`^[^@[:space:]]+@[^@[:space:]]+\.[^@[:space:]]+$`)
)

const (
	otpTTL         = 5 * time.Minute
	otpMaxPerHour  = 5
	demoOTP        = "123456"
	otpWindow      = 10 * time.Minute
	otpMaxInWindow = 5
)

type identity struct {
	Type  string `json:"type"` // phone | email
	Value string `json:"value"`
}

// clean validates and normalises the identity (phone to +251..., email to lower case) and returns the channel.
func (i identity) clean() (identity, string, error) {
	v := strings.TrimSpace(i.Value)
	switch i.Type {
	case "phone":
		if !phoneRE.MatchString(v) {
			return i, "", fail(http.StatusBadRequest, codeInvalidPhone, "Enter an Ethiopian phone number")
		}
		return identity{"phone", v}, "sms", nil
	case "email":
		v = strings.ToLower(v)
		if len(v) > 254 || !emailRE.MatchString(v) {
			return i, "", fail(http.StatusBadRequest, codeInvalidEmail, "Enter a valid email address")
		}
		return identity{"email", v}, "email", nil
	}
	return i, "", fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "type must be phone or email")
}

func (s *Server) requestOTP(w http.ResponseWriter, r *http.Request) error {
	var in identity
	if err := decode(r, &in); err != nil {
		return err
	}
	id, channel, err := in.clean()
	if err != nil {
		return err
	}
	ctx := r.Context()

	var recent int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM otp_challenges WHERE identifier = $1 AND created_at > now() - $2::interval`,
		id.Value, fmt.Sprintf("%d seconds", int(otpWindow.Seconds()))).Scan(&recent); err != nil {
		return err
	}
	if recent >= otpMaxInWindow {
		return fail(http.StatusTooManyRequests, codeTooMany, "Too many codes requested. Try again in a few minutes.")
	}

	code := demoOTP
	if !s.cfg.DemoMode {
		n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
		if err != nil {
			return err
		}
		code = fmt.Sprintf("%06d", n.Int64())
	}
	if _, err := s.pool.Exec(ctx, `INSERT INTO otp_challenges (channel, identifier, code_hash, expires_at, ip)
		VALUES ($1, $2, $3, now() + $4::interval, NULLIF($5, '')::inet)`,
		channel, id.Value, s.keys.HashSecret("otp:"+id.Value, code), fmt.Sprintf("%d seconds", int(otpTTL.Seconds())), clientIP(r)); err != nil {
		return err
	}
	if err := s.p.OTP.Send(ctx, channel, id.Value, code); err != nil {
		return fmt.Errorf("send otp: %w", err)
	}
	respond(w, http.StatusOK, httpx.NowMeta())
	return nil
}

func clientIP(r *http.Request) string {
	host := r.RemoteAddr
	if i := strings.LastIndex(host, ":"); i > 0 {
		host = host[:i]
	}
	host = strings.Trim(host, "[]")
	if host == "" {
		return ""
	}
	return host
}

type tokensOut struct {
	httpx.Meta
	Token        string    `json:"token"`
	RefreshToken string    `json:"refresh_token"`
	Identity     *identity `json:"identity,omitempty"`
}

func (s *Server) verifyOTP(w http.ResponseWriter, r *http.Request) error {
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
	app := r.Header.Get("X-App")
	if app != "guest" && app != "partner" {
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "X-App header must be guest or partner")
	}
	deviceID := r.Header.Get("X-Device-Id")
	if len(deviceID) < 8 || len(deviceID) > 100 {
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "X-Device-Id header is required")
	}
	platform := r.Header.Get("X-Platform")
	if platform != "android" && platform != "ios" {
		platform = "web"
	}
	ctx := r.Context()

	var out tokensOut
	err = s.tx(ctx, func(tx pgx.Tx) error {
		// The newest live challenge for this identity. Every attempt counts, so a code cannot be guessed.
		var cid string
		var hash []byte
		var attempts, maxAttempts int
		err := tx.QueryRow(ctx, `SELECT id, code_hash, attempts, max_attempts FROM otp_challenges
			WHERE identifier = $1 AND consumed_at IS NULL AND expires_at > now()
			ORDER BY created_at DESC LIMIT 1 FOR UPDATE`, id.Value).Scan(&cid, &hash, &attempts, &maxAttempts)
		if err == pgx.ErrNoRows || (err == nil && attempts >= maxAttempts) {
			return fail(http.StatusUnauthorized, codeInvalidOTP, "That code is wrong or has expired")
		}
		if err != nil {
			return err
		}
		if !hmac.Equal(hash, s.keys.HashSecret("otp:"+id.Value, strings.TrimSpace(in.Code))) {
			// Count the failed attempt even though this request fails: it is saved by a separate statement below.
			return &wrongCode{cid}
		}
		if _, err := tx.Exec(ctx, `UPDATE otp_challenges SET consumed_at = now() WHERE id = $1`, cid); err != nil {
			return err
		}

		userID, role, err := s.userFor(ctx, tx, id, app)
		if err != nil {
			return err
		}
		var devPK string
		if err := tx.QueryRow(ctx, `INSERT INTO devices (user_id, device_id, app, platform, last_seen_at) VALUES ($1, $2, $3, $4, now())
			ON CONFLICT (user_id, device_id, app) DO UPDATE SET last_seen_at = now(), revoked_at = NULL, platform = EXCLUDED.platform
			RETURNING id`, userID, deviceID, app, platform).Scan(&devPK); err != nil {
			return err
		}
		family := newUUID()
		access, refresh, err := s.issue(ctx, tx, userID, role, app, devPK, family)
		if err != nil {
			return err
		}
		out = tokensOut{Meta: httpx.NowMeta(), Token: access, RefreshToken: refresh, Identity: &id}
		return nil
	})
	var wc *wrongCode
	if asWrong(err, &wc) {
		// The failed attempt must persist, so it is written outside the rolled-back transaction.
		_, _ = s.pool.Exec(ctx, `UPDATE otp_challenges SET attempts = attempts + 1 WHERE id = $1 AND attempts < max_attempts`, wc.id)
		return fail(http.StatusUnauthorized, codeInvalidOTP, "That code is wrong or has expired")
	}
	if err != nil {
		return err
	}
	respond(w, http.StatusOK, out)
	return nil
}

type wrongCode struct{ id string }

func (w *wrongCode) Error() string { return "wrong code" }

func asWrong(err error, target **wrongCode) bool {
	if w, ok := err.(*wrongCode); ok {
		*target = w
		return true
	}
	return false
}

// userFor finds the account for an identity. Guests are created on first sign-in; the partner app only admits staff.
func (s *Server) userFor(ctx context.Context, tx pgx.Tx, id identity, app string) (userID, role string, err error) {
	col := "phone"
	if id.Type == "email" {
		col = "email"
	}
	var status string
	err = tx.QueryRow(ctx, `SELECT id, role, status FROM users WHERE `+col+` = $1`, id.Value).Scan(&userID, &role, &status)
	if err == pgx.ErrNoRows {
		if app == "partner" {
			return "", "", fail(http.StatusForbidden, codeNotStaff, "This number or email is not registered with a hotel")
		}
		err = tx.QueryRow(ctx, `INSERT INTO users (`+col+`) VALUES ($1) RETURNING id, role`, id.Value).Scan(&userID, &role)
		return userID, role, err
	}
	if err != nil {
		return "", "", err
	}
	if status != "active" {
		return "", "", fail(http.StatusForbidden, codeForbidden, "This account is suspended")
	}
	if app == "partner" {
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM property_staff WHERE user_id = $1`, userID).Scan(&n); err != nil {
			return "", "", err
		}
		if n == 0 {
			return "", "", fail(http.StatusForbidden, codeNotStaff, "This number or email is not registered with a hotel")
		}
	}
	return userID, role, nil
}

// issue mints an access token and a new refresh token in the given family.
func (s *Server) issue(ctx context.Context, tx pgx.Tx, userID, role, app, devPK, family string) (access, refresh string, err error) {
	now := s.now()
	access = s.keys.SignJWT(claims{Sub: userID, Role: role, App: app, Dev: devPK, Iat: now.Unix(), Exp: now.Add(accessTTL).Unix()})
	refresh = randomToken(32)
	_, err = tx.Exec(ctx, `INSERT INTO refresh_tokens (user_id, device_pk, family_id, token_hash, expires_at)
		VALUES ($1, $2, $3, $4, $5)`, userID, devPK, family, s.keys.HashSecret("refresh", refresh), now.Add(refreshTTL))
	return access, refresh, err
}

func (s *Server) refresh(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if in.RefreshToken == "" {
		return errAuth
	}
	ctx := r.Context()
	var out tokensOut
	reuse := false
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var id, userID, devPK, family, role, app, status string
		var expires time.Time
		var rotated, revoked *time.Time
		err := tx.QueryRow(ctx, `SELECT t.id, t.user_id, t.device_pk, t.family_id, t.expires_at, t.rotated_at, t.revoked_at, u.role, u.status, d.app
			FROM refresh_tokens t JOIN users u ON u.id = t.user_id JOIN devices d ON d.id = t.device_pk
			WHERE t.token_hash = $1 FOR UPDATE OF t`, s.keys.HashSecret("refresh", in.RefreshToken)).
			Scan(&id, &userID, &devPK, &family, &expires, &rotated, &revoked, &role, &status, &app)
		if err == pgx.ErrNoRows {
			return errAuth
		}
		if err != nil {
			return err
		}
		if rotated != nil {
			// A refresh token is used once. Seeing one again means a copy leaked: end the whole session family.
			reuse = true
			return errAuth
		}
		if revoked != nil || !expires.After(s.now()) || status != "active" {
			return errAuth
		}
		if _, err := tx.Exec(ctx, `UPDATE refresh_tokens SET rotated_at = now() WHERE id = $1`, id); err != nil {
			return err
		}
		access, refresh, err := s.issue(ctx, tx, userID, role, app, devPK, family)
		if err != nil {
			return err
		}
		out = tokensOut{Meta: httpx.NowMeta(), Token: access, RefreshToken: refresh}
		return nil
	})
	if reuse {
		_, _ = s.pool.Exec(ctx, `UPDATE refresh_tokens SET revoked_at = now() WHERE revoked_at IS NULL AND family_id =
			(SELECT family_id FROM refresh_tokens WHERE token_hash = $1)`, s.keys.HashSecret("refresh", in.RefreshToken))
	}
	if err != nil {
		return err
	}
	respond(w, http.StatusOK, out)
	return nil
}

func (s *Server) voucherKey(w http.ResponseWriter, _ *http.Request) error {
	respond(w, http.StatusOK, struct {
		httpx.Meta
		KID       string `json:"kid"`
		Algorithm string `json:"alg"`
		PublicKey string `json:"public_key"`
	}{httpx.NowMeta(), s.keys.VoucherKID, "Ed25519", base64.RawURLEncoding.EncodeToString(s.keys.VoucherPub)})
	return nil
}
