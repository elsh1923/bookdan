package api

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/bookdan/backend/internal/config"
)

// Keys holds every secret the server uses. In development, missing secrets fall back to fixed, PUBLIC values so the
// stack runs with no setup; config.Validate refuses that in production.
type Keys struct {
	jwtKey      []byte
	aead        cipher.AEAD
	hashKey     []byte // keyed hash of ID numbers
	pinKey      []byte // derives voucher PINs and hashes them
	voucherPriv ed25519.PrivateKey
	VoucherPub  ed25519.PublicKey
	VoucherKID  string
}

const devSecret = "bookdan-development-only-not-a-secret"

func NewKeys(cfg config.Config) (*Keys, error) {
	jwt := cfg.JWTSecret
	if jwt == "" {
		jwt = devSecret + "-jwt"
	}
	master, err := decodeKey("ID_ENCRYPTION_KEY", cfg.IDKey, devSecret+"-id")
	if err != nil {
		return nil, err
	}
	seed, err := decodeKey("VOUCHER_SIGNING_SEED", cfg.VoucherSeedB64, devSecret+"-voucher")
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(master)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	priv := ed25519.NewKeyFromSeed(seed)
	pub := priv.Public().(ed25519.PublicKey)
	sum := sha256.Sum256(pub)
	return &Keys{
		jwtKey:      []byte(jwt),
		aead:        aead,
		hashKey:     derive(master, "id-hash"),
		pinKey:      derive(master, "voucher-pin"),
		voucherPriv: priv,
		VoucherPub:  pub,
		VoucherKID:  base64.RawURLEncoding.EncodeToString(sum[:6]),
	}, nil
}

// decodeKey reads a base64 32-byte key; an empty value in development becomes a fixed 32-byte key from the fallback text.
func decodeKey(name, value, fallback string) ([]byte, error) {
	if value == "" {
		sum := sha256.Sum256([]byte(fallback))
		return sum[:], nil
	}
	b, err := base64.StdEncoding.DecodeString(value)
	if err != nil || len(b) != 32 {
		return nil, fmt.Errorf("%s must be 32 bytes, base64 encoded (generate one with: openssl rand -base64 32)", name)
	}
	return b, nil
}

func derive(master []byte, label string) []byte {
	m := hmac.New(sha256.New, master)
	m.Write([]byte(label))
	return m.Sum(nil)
}

// ---- ID numbers: encrypted so a database leak does not expose them, hashed so the same ID can be matched without decrypting.

func (k *Keys) Seal(plain string) []byte {
	nonce := make([]byte, k.aead.NonceSize())
	_, _ = rand.Read(nonce)
	return k.aead.Seal(nonce, nonce, []byte(plain), nil)
}

func (k *Keys) Open(sealed []byte) (string, error) {
	n := k.aead.NonceSize()
	if len(sealed) < n {
		return "", errors.New("ciphertext too short")
	}
	plain, err := k.aead.Open(nil, sealed[:n], sealed[n:], nil)
	return string(plain), err
}

func (k *Keys) HashID(normalized string) []byte {
	m := hmac.New(sha256.New, k.hashKey)
	m.Write([]byte(normalized))
	return m.Sum(nil)
}

// ---- OTP and refresh tokens: only hashes are stored.

func (k *Keys) HashSecret(label, value string) []byte {
	m := hmac.New(sha256.New, k.jwtKey)
	m.Write([]byte(label + ":" + value))
	return m.Sum(nil)
}

// ---- Voucher PIN: derived from the booking id, so the guest sees the same PIN every time; only its hash is stored.

func (k *Keys) VoucherPIN(bookingID string) string {
	m := hmac.New(sha256.New, k.pinKey)
	m.Write([]byte(bookingID))
	return fmt.Sprintf("%04d", binary.BigEndian.Uint32(m.Sum(nil)[:4])%10000)
}

func (k *Keys) PINHash(bookingID, pin string) []byte {
	m := hmac.New(sha256.New, k.pinKey)
	m.Write([]byte("hash:" + bookingID + ":" + pin))
	return m.Sum(nil)
}

// voucherPayload field order matches the apps (v, kid, pid, code, in, out, exp).
type voucherPayload struct {
	V    int    `json:"v"`
	KID  string `json:"kid"`
	PID  string `json:"pid"`
	Code string `json:"code"`
	In   string `json:"in"`
	Out  string `json:"out"`
	Exp  int64  `json:"exp"`
}

// SignVoucher returns base64url(payload) + "." + base64url(Ed25519 signature), the format the apps read.
func (k *Keys) SignVoucher(propertyID, code, checkIn, checkOut string, exp time.Time) string {
	payload, _ := json.Marshal(voucherPayload{1, k.VoucherKID, propertyID, code, checkIn, checkOut, exp.Unix()})
	sig := ed25519.Sign(k.voucherPriv, payload)
	return base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(sig)
}

// VerifyVoucher checks the signature and the expiry of a voucher token (the text inside the guest's QR code) and returns what it says.
func (k *Keys) VerifyVoucher(token string, now time.Time) (voucherPayload, error) {
	var p voucherPayload
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return p, errors.New("malformed voucher")
	}
	payload, err1 := base64.RawURLEncoding.DecodeString(parts[0])
	sig, err2 := base64.RawURLEncoding.DecodeString(parts[1])
	if err1 != nil || err2 != nil || len(sig) != ed25519.SignatureSize || !ed25519.Verify(k.VoucherPub, payload, sig) {
		return p, errors.New("voucher signature is not valid")
	}
	if err := json.Unmarshal(payload, &p); err != nil || p.V != 1 || p.PID == "" || p.Code == "" {
		return voucherPayload{}, errors.New("malformed voucher")
	}
	if now.Unix() > p.Exp {
		return p, errVoucherExpired
	}
	return p, nil
}

var errVoucherExpired = errors.New("voucher expired")

// ---- Access tokens: HS256 JWTs, 15 minutes.

type claims struct {
	Sub  string `json:"sub"`           // user id
	Role string `json:"role"`          // guest | owner | receptionist | admin
	App  string `json:"app"`           // guest | partner
	Dev  string `json:"dev,omitempty"` // device row id
	Iat  int64  `json:"iat"`
	Exp  int64  `json:"exp"`
}

var jwtHeader = base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))

func (k *Keys) SignJWT(c claims) string {
	body, _ := json.Marshal(c)
	signing := jwtHeader + "." + base64.RawURLEncoding.EncodeToString(body)
	return signing + "." + base64.RawURLEncoding.EncodeToString(k.jwtMAC(signing))
}

func (k *Keys) jwtMAC(signing string) []byte {
	m := hmac.New(sha256.New, k.jwtKey)
	m.Write([]byte(signing))
	return m.Sum(nil)
}

var errBadToken = errors.New("invalid token")

func (k *Keys) VerifyJWT(token string, now time.Time) (claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] != jwtHeader {
		return claims{}, errBadToken
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !hmac.Equal(sig, k.jwtMAC(parts[0]+"."+parts[1])) {
		return claims{}, errBadToken
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return claims{}, errBadToken
	}
	var c claims
	if err := json.Unmarshal(raw, &c); err != nil || c.Sub == "" || now.Unix() >= c.Exp {
		return claims{}, errBadToken
	}
	return c, nil
}

// randomToken is a URL-safe random string (refresh tokens).
func randomToken(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// newUUID returns a random (version 4) UUID.
func newUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
