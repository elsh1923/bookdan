// Package config reads runtime settings from the environment and refuses unsafe combinations.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
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
	OTPProvider string // SMS sign-in codes: log | (an SMS provider, to be built)
	// EmailProvider sends sign-in codes by email: log (prints them) or smtp (Gmail with an app password, Brevo, Mailgun, SES...).
	EmailProvider string // log | smtp | brevo (a web API: works where the mail ports are blocked)
	BrevoAPIKey   string
	SMTPHost      string
	SMTPPort      string
	SMTPUser      string
	SMTPPassword  string
	EmailFrom     string // for example: BookDan <you@example.com> (Gmail only sends as the signed-in address)
	// DemoAccountDomains are email domains with no real inbox (the demo hotel owner, test addresses). When DEMO_MODE is on they keep
	// the fixed code 123456 and nothing is emailed to them. Every other address gets a real random code.
	DemoAccountDomains []string
	PaymentProvider    string // dev | chapa
	PushProvider       string // log | fcm
	// FCMServiceAccountB64 is the Firebase service-account key (JSON file) in base64. Required when PUSH_PROVIDER=fcm.
	FCMServiceAccountB64 string
	// The admin web page (/admin). It is off unless ADMIN_PASSWORD is set. Platform admins are the emails in ADMIN_EMAILS and sign in
	// with that password. Hotel owners may also use it for their own hotels, signing in with a code, but only when ADMIN_OWNERS=true:
	// turn that on only when sign-in codes are real (a fixed demo code would let anyone in).
	AdminEmails   []string
	AdminPassword string
	AdminOwners   bool
	// PartnerAppURL is where hotel staff download the BookDan Partner app (a link to the APK today, the Play Store later). It goes in invitation emails.
	PartnerAppURL string
	// IDRetentionDays is how many days after check-out a hotel keeps the full ID number of a guest before it is erased (the type and the last
	// four characters stay). 0 = never erased. How long the law requires is the owner's decision (docs/design.md, DECISION 5).
	IDRetentionDays int
	// PaymentWebhookSecret signs payment-gateway callbacks (HMAC-SHA256 of the body).
	PaymentWebhookSecret string
	// ChapaSecretKey is the secret key from the Chapa dashboard (Settings, API). Required when PAYMENT_PROVIDER=chapa. ChapaWebhookSecret is the optional
	// secret hash set for webhooks there; Chapa signs with it (or with the secret key).
	ChapaSecretKey     string
	ChapaWebhookSecret string
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
		EmailProvider:        get("EMAIL_PROVIDER", "log"),
		BrevoAPIKey:          get("BREVO_API_KEY", ""),
		SMTPHost:             get("SMTP_HOST", "smtp.gmail.com"),
		SMTPPort:             get("SMTP_PORT", "587"),
		SMTPUser:             get("SMTP_USER", ""),
		SMTPPassword:         get("SMTP_PASSWORD", ""),
		EmailFrom:            get("EMAIL_FROM", ""),
		DemoAccountDomains:   splitListDefault(get("DEMO_ACCOUNT_DOMAINS", ""), []string{"bookdan.test", "example.com", "example.org"}),
		PaymentProvider:      get("PAYMENT_PROVIDER", "dev"),
		PushProvider:         get("PUSH_PROVIDER", "log"),
		FCMServiceAccountB64: get("FCM_SERVICE_ACCOUNT_B64", ""),
		AdminEmails:          splitList(get("ADMIN_EMAILS", "")),
		AdminPassword:        get("ADMIN_PASSWORD", ""),
		AdminOwners:          get("ADMIN_OWNERS", "false") == "true",
		PartnerAppURL:        get("PARTNER_APP_URL", ""),
		IDRetentionDays:      atoi(get("ID_RETENTION_DAYS", "0")),
		PaymentWebhookSecret: get("PAYMENT_WEBHOOK_SECRET", ""),
		ChapaSecretKey:       get("CHAPA_SECRET_KEY", ""),
		ChapaWebhookSecret:   get("CHAPA_WEBHOOK_SECRET", ""),
	}
}

func atoi(v string) int {
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n < 0 {
		return 0
	}
	return n
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
	} {
		if v == "" {
			problems = append(problems, name+" is required when APP_ENV=production")
		}
	}
	// Chapa signs its own callbacks (and every result is confirmed with Chapa), so only the other gateways need the shared webhook secret.
	if c.PaymentProvider != "chapa" && c.PaymentWebhookSecret == "" {
		problems = append(problems, "PAYMENT_WEBHOOK_SECRET is required when APP_ENV=production")
	}
	if c.PaymentProvider == "chapa" && c.ChapaSecretKey == "" {
		problems = append(problems, "CHAPA_SECRET_KEY is required when PAYMENT_PROVIDER=chapa")
	}
	if c.PaymentProvider == "chapa" && strings.Contains(c.ChapaSecretKey, "_TEST-") {
		problems = append(problems, "CHAPA_SECRET_KEY is a test key: a production server needs the live key")
	}
	if len(c.JWTSecret) > 0 && len(c.JWTSecret) < 32 {
		problems = append(problems, "JWT_SECRET must be at least 32 characters")
	}
	if !strings.HasPrefix(c.PublicURL, "https://") {
		problems = append(problems, "PUBLIC_URL must be an https URL when APP_ENV=production")
	}
	// These integrations have not been built yet: a production deploy must not silently use the stand-ins.
	if c.OTPProvider == "log" {
		problems = append(problems, "OTP_PROVIDER=log only prints SMS codes; build the SMS integration (internal/api/providers.go)")
	}
	switch c.EmailProvider {
	case "smtp":
		if c.SMTPUser == "" || c.SMTPPassword == "" || c.EmailFrom == "" {
			problems = append(problems, "SMTP_USER, SMTP_PASSWORD and EMAIL_FROM are required when EMAIL_PROVIDER=smtp")
		}
	case "brevo":
		if c.BrevoAPIKey == "" || c.EmailFrom == "" {
			problems = append(problems, "BREVO_API_KEY and EMAIL_FROM are required when EMAIL_PROVIDER=brevo")
		}
	default:
		problems = append(problems, "EMAIL_PROVIDER must be smtp or brevo in production")
	}
	if c.PaymentProvider == "dev" {
		problems = append(problems, "PAYMENT_PROVIDER=dev is a fake gateway; build the Telebirr/Chapa integration (internal/api/providers.go)")
	}
	if c.PushProvider == "log" {
		problems = append(problems, "PUSH_PROVIDER=log only logs alerts; set PUSH_PROVIDER=fcm and FCM_SERVICE_ACCOUNT_B64")
	}
	if c.AdminPassword != "" && len(c.AdminPassword) < 16 {
		problems = append(problems, "ADMIN_PASSWORD must be at least 16 characters")
	}
	if c.AdminPassword != "" && len(c.AdminEmails) == 0 {
		problems = append(problems, "ADMIN_EMAILS is required when ADMIN_PASSWORD is set")
	}
	if c.AdminOwners && c.DemoMode {
		problems = append(problems, "ADMIN_OWNERS needs real sign-in codes (DEMO_MODE off)")
	}
	if c.PushProvider == "fcm" && c.FCMServiceAccountB64 == "" {
		problems = append(problems, "FCM_SERVICE_ACCOUNT_B64 is required when PUSH_PROVIDER=fcm")
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

func splitList(v string) []string {
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.ToLower(strings.TrimSpace(p)); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func splitListDefault(v string, fallback []string) []string {
	if l := splitList(v); len(l) > 0 {
		return l
	}
	return fallback
}
