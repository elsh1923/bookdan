package api

import (
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha1" //nolint:gosec // RFC 6238 authenticator codes are defined with HMAC-SHA1
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"math/big"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Passwords and authenticator codes for the admin accounts (adminaccounts.go).
//
// A password is stored as PBKDF2-HMAC-SHA256 with a random salt and 600,000 rounds (the current OWASP figure), in the text form
// "pbkdf2-sha256$rounds$salt$key". It is checked in constant time. The standard library has it, so there is nothing to install.

const (
	pwRounds   = 600_000
	pwMinLen   = 12
	pwMaxLen   = 128
	pwKeyBytes = 32
)

func hashPassword(pw string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := pbkdf2.Key(sha256.New, pw, salt, pwRounds, pwKeyBytes)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("pbkdf2-sha256$%d$%s$%s", pwRounds, base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

// checkPassword reports whether pw matches a stored hash. A malformed hash never matches. The round count is bounded so a damaged row cannot make a
// sign-in run for minutes.
func checkPassword(pw, stored string) bool {
	parts := strings.Split(stored, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
		return false
	}
	rounds, err := strconv.Atoi(parts[1])
	if err != nil || rounds < 100_000 || rounds > 2_000_000 {
		return false
	}
	salt, err1 := base64.RawStdEncoding.DecodeString(parts[2])
	want, err2 := base64.RawStdEncoding.DecodeString(parts[3])
	if err1 != nil || err2 != nil || len(want) != pwKeyBytes {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, pw, salt, rounds, pwKeyBytes)
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(got, want) == 1
}

var (
	dummyOnce sync.Once
	dummyHash string
)

// spendPasswordTime does the same work as a real check against nothing, so a sign-in for an address that has no account takes as long as one that does.
func spendPasswordTime(pw string) {
	dummyOnce.Do(func() { dummyHash, _ = hashPassword("a password nobody has") })
	checkPassword(pw, dummyHash)
}

// passwordProblem says what is wrong with a new password ("" when it is fine): long enough, not absurdly long, and not the person's own address.
func passwordProblem(pw, email string) string {
	switch {
	case len([]rune(pw)) < pwMinLen:
		return fmt.Sprintf("The password must have at least %d characters", pwMinLen)
	case len(pw) > pwMaxLen:
		return fmt.Sprintf("The password can have at most %d characters", pwMaxLen)
	}
	low := strings.ToLower(pw)
	if local, _, ok := strings.Cut(strings.ToLower(email), "@"); ok && len(local) >= 4 && strings.Contains(low, local) {
		return "The password must not contain your email name"
	}
	same := true
	for _, c := range pw {
		if c != []rune(pw)[0] {
			same = false
			break
		}
	}
	if same {
		return "Choose a password with different characters"
	}
	return ""
}

// secretsEqual compares two secrets in constant time whatever their length.
func secretsEqual(a, b string) bool {
	x, y := sha256.Sum256([]byte(a)), sha256.Sum256([]byte(b))
	return hmac.Equal(x[:], y[:])
}

const tempPasswordAlphabet = "abcdefghjkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789" // no look-alike letters: it is read out or typed once

// tempPassword is a random password (16 characters) shown once to the admin who created or reset an account.
func tempPassword() string {
	out := make([]byte, 16)
	for i := range out {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(tempPasswordAlphabet))))
		if err != nil {
			panic(err) // the system's random source failing is not recoverable
		}
		out[i] = tempPasswordAlphabet[n.Int64()]
	}
	return string(out)
}

// ---- authenticator app codes (RFC 6238: 6 digits, 30-second steps, HMAC-SHA1)

const totpStep = 30

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

func newTOTPSecret() ([]byte, error) {
	secret := make([]byte, 20)
	_, err := rand.Read(secret)
	return secret, err
}

func totpAt(secret []byte, step int64) string {
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(step))
	m := hmac.New(sha1.New, secret)
	m.Write(msg[:])
	sum := m.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	code := (uint32(sum[off]&0x7f) << 24) | uint32(sum[off+1])<<16 | uint32(sum[off+2])<<8 | uint32(sum[off+3])
	return fmt.Sprintf("%06d", code%1_000_000)
}

// totpAccept checks a typed code against the current step and the one either side (clock drift) and returns the step it matched. A step at or before
// lastStep is refused, so a code that was already used cannot be used again.
func totpAccept(secret []byte, code string, now time.Time, lastStep int64) (int64, bool) {
	code = strings.ReplaceAll(strings.TrimSpace(code), " ", "")
	if len(code) != 6 {
		return 0, false
	}
	cur := now.Unix() / totpStep
	for _, d := range []int64{0, -1, 1} {
		st := cur + d
		if st <= lastStep {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(totpAt(secret, st)), []byte(code)) == 1 {
			return st, true
		}
	}
	return 0, false
}

// totpURI is what an authenticator app reads (it can also be typed in by hand as the secret).
func totpURI(email string, secret []byte) string {
	label := url.PathEscape("BookDan:" + email)
	return "otpauth://totp/" + label + "?secret=" + b32.EncodeToString(secret) + "&issuer=BookDan&algorithm=SHA1&digits=6&period=30"
}
