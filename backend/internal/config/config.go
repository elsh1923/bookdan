// Package config reads runtime settings from the environment and refuses unsafe combinations.
package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	Env         string
	HTTPAddr    string
	DatabaseURL string
	// DemoMode shows the OTP instead of sending an SMS. Must be off in production.
	DemoMode bool

	// Secrets. In development a built-in value is used when these are empty (see api.NewKeys); production requires them.
	JWTSecret      string // signs access tokens
	IDKey          string // base64, 32 bytes: encrypts national ID numbers and derives voucher PINs
	VoucherSeedB64 string // base64, 32 bytes: Ed25519 seed that signs vouchers

	// PublicURL is how phones reach this server (used in payment page links).
	PublicURL string

	// Providers. Each names an integration. "dev"/"log" are development stand-ins; production refuses them until the real
	// integration has been written (see internal/api/providers.go).
	OTPProvider     string // log | (sms and email providers to be built)
	PaymentProvider string // dev | (telebirr, chapa to be built)
	PushProvider    string // log | (fcm to be built)
	// PaymentWebhookSecret signs payment-gateway callbacks (HMAC-SHA256 of the body).
	PaymentWebhookSecret string
}

// Load reads backend/.env when it exists (development convenience; real environment variables win),
// then the process environment.
func Load() Config {
	_ = godotenv.Load() // a missing .env is normal in production

	return Config{
		Env:                  get("APP_ENV", "development"),
		HTTPAddr:             httpAddr(),
		DatabaseURL:          get("DATABASE_URL", ""),
		DemoMode:             get("DEMO_MODE", "false") == "true",
		JWTSecret:            get("JWT_SECRET", ""),
		IDKey:                get("ID_ENCRYPTION_KEY", ""),
		VoucherSeedB64:       get("VOUCHER_SIGNING_SEED", ""),
		PublicURL:            strings.TrimRight(get("PUBLIC_URL", "http://localhost:8080"), "/"),
		OTPProvider:          get("OTP_PROVIDER", "log"),
		PaymentProvider:      get("PAYMENT_PROVIDER", "dev"),
		PushProvider:         get("PUSH_PROVIDER", "log"),
		PaymentWebhookSecret: get("PAYMENT_WEBHOOK_SECRET", ""),
	}
}

func (c Config) IsProduction() bool { return c.Env == "production" }

// Validate rejects configurations that must never run, so a misconfigured deploy fails at startup.
func (c Config) Validate() error {
	if !c.IsProduction() {
		return nil
	}
	var problems []string
	if c.DemoMode {
		problems = append(problems, "DEMO_MODE must be off when APP_ENV=production")
	}
	if c.DatabaseURL == "" {
		problems = append(problems, "DATABASE_URL is required when APP_ENV=production")
	}
	for name, v := range map[string]string{
		"JWT_SECRET": c.JWTSecret, "ID_ENCRYPTION_KEY": c.IDKey, "VOUCHER_SIGNING_SEED": c.VoucherSeedB64,
		"PAYMENT_WEBHOOK_SECRET": c.PaymentWebhookSecret,
	} {
		if v == "" {
			problems = append(problems, name+" is required when APP_ENV=production")
		}
	}
	if len(c.JWTSecret) > 0 && len(c.JWTSecret) < 32 {
		problems = append(problems, "JWT_SECRET must be at least 32 characters")
	}
	if !strings.HasPrefix(c.PublicURL, "https://") {
		problems = append(problems, "PUBLIC_URL must be an https URL when APP_ENV=production")
	}
	// These integrations have not been built yet: a production deploy must not silently use the stand-ins.
	if c.OTPProvider == "log" {
		problems = append(problems, "OTP_PROVIDER=log only prints codes; build the SMS/email integration (internal/api/providers.go)")
	}
	if c.PaymentProvider == "dev" {
		problems = append(problems, "PAYMENT_PROVIDER=dev is a fake gateway; build the Telebirr/Chapa integration (internal/api/providers.go)")
	}
	if c.PushProvider == "log" {
		problems = append(problems, "PUSH_PROVIDER=log only logs alerts; build the FCM integration (internal/api/providers.go)")
	}
	if len(problems) == 0 {
		return nil
	}
	return errors.New(fmt.Sprint("invalid production configuration: ", strings.Join(problems, "; ")))
}

func get(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

// httpAddr is HTTP_ADDR, or ":" plus PORT (what hosting platforms such as Render and Railway provide), or :8080.
func httpAddr() string {
	if v := get("HTTP_ADDR", ""); v != "" {
		return v
	}
	if p := get("PORT", ""); p != "" {
		return ":" + p
	}
	return ":8080"
}
