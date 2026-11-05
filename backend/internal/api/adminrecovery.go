package api

import (
	"context"
	"crypto/rand"
	"math/big"
	"net/http"
	"strings"
	"unicode"

	"github.com/jackc/pgx/v5"

	"github.com/bookdan/backend/internal/httpx"
)

// Recovery codes for the authenticator app. When an admin sets the app up they are shown eight single-use codes (once). If the phone is lost, one of them
// signs the admin in in place of the 6-digit code, and they set the app up again. Only a keyed hash of each code is kept, a spent code never works again, and a
// new set replaces the old one. Using one is written to the audit log. Turning the app off or removing it removes the codes.

const (
	recoveryCodeCount = 8
	recoveryAlphabet  = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // no 0, O, 1 or I: they are copied by hand
)

// newRecoveryCodes makes n codes like "K7QM2-X9PWD".
func newRecoveryCodes(n int) ([]string, error) {
	out := make([]string, 0, n)
	for len(out) < n {
		var b strings.Builder
		for i := 0; i < 10; i++ {
			if i == 5 {
				b.WriteByte('-')
			}
			v, err := rand.Int(rand.Reader, big.NewInt(int64(len(recoveryAlphabet))))
			if err != nil {
				return nil, err
			}
			b.WriteByte(recoveryAlphabet[v.Int64()])
		}
		out = append(out, b.String())
	}
	return out, nil
}

// normalizeRecovery makes what a person typed comparable: capitals, no spaces or dashes.
func normalizeRecovery(code string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(code) {
		if r != '-' && !unicode.IsSpace(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// looksLikeRecovery tells a recovery code from a 6-digit authenticator code.
func looksLikeRecovery(code string) bool {
	n := normalizeRecovery(code)
	if len(n) != 10 {
		return false
	}
	for _, r := range n {
		if !strings.ContainsRune(recoveryAlphabet, r) {
			return false
		}
	}
	return true
}

func (s *Server) recoveryHash(userID, code string) []byte {
	return s.keys.HashSecret("adminrec:"+userID, normalizeRecovery(code))
}

// storeRecoveryCodes replaces the account's codes with a new set and returns the set to show once.
func (s *Server) storeRecoveryCodes(ctx context.Context, tx pgx.Tx, userID string) ([]string, error) {
	codes, err := newRecoveryCodes(recoveryCodeCount)
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM admin_recovery_codes WHERE user_id = $1`, userID); err != nil {
		return nil, err
	}
	for _, c := range codes {
		if _, err := tx.Exec(ctx, `INSERT INTO admin_recovery_codes (user_id, code_hash) VALUES ($1, $2)`, userID, s.recoveryHash(userID, c)); err != nil {
			return nil, err
		}
	}
	return codes, nil
}

// useRecoveryCode spends one code. It is true only for a code of this account that was never used.
func (s *Server) useRecoveryCode(ctx context.Context, userID, code string) bool {
	tag, err := s.pool.Exec(ctx, `UPDATE admin_recovery_codes SET used_at = now() WHERE user_id = $1 AND code_hash = $2 AND used_at IS NULL`, userID, s.recoveryHash(userID, code))
	return err == nil && tag.RowsAffected() == 1
}

// recoveryLeft is how many unused codes the account has.
func (s *Server) recoveryLeft(ctx context.Context, userID string) int {
	var n int
	_ = s.pool.QueryRow(ctx, `SELECT count(*) FROM admin_recovery_codes WHERE user_id = $1 AND used_at IS NULL`, userID).Scan(&n)
	return n
}

// adminNewRecoveryCodes gives the signed-in admin a fresh set of codes (the old ones stop working). It needs the password and a current authenticator code.
func (s *Server) adminNewRecoveryCodes(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	ctx, uid := r.Context(), claimsFrom(r).Sub
	var codes []string
	err := s.tx(ctx, func(tx pgx.Tx) error {
		hash, totp, _, err := s.ownAccount(ctx, tx, r)
		if err != nil {
			return err
		}
		if totp == nil {
			return fail(http.StatusConflict, codeInvalidState, "Set up the authenticator app first")
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
		step, ok := totpAccept(secret, in.Code, s.now(), last)
		if !ok {
			return fail(http.StatusBadRequest, "MFA_INVALID", "That code is wrong or has expired")
		}
		if _, err := tx.Exec(ctx, `UPDATE admin_accounts SET totp_last_step = $2 WHERE user_id = $1`, uid, step); err != nil {
			return err
		}
		if codes, err = s.storeRecoveryCodes(ctx, tx, uid); err != nil {
			return err
		}
		return s.audit(r, tx, "admin_recovery_codes_renewed", nil)
	})
	if err != nil {
		return err
	}
	respond(w, http.StatusOK, struct {
		httpx.Meta
		RecoveryCodes []string `json:"recovery_codes"`
	}{httpx.NowMeta(), codes})
	return nil
}
