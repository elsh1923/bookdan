package config

import "testing"

// A production configuration with every integration real, for the "safe" case.
func safeProduction() Config {
	return Config{
		Env: "production", DatabaseURL: "postgres://x", PublicURL: "https://api.example.com",
		JWTSecret: "0123456789abcdef0123456789abcdef", IDKey: "k", VoucherSeedB64: "s", PaymentWebhookSecret: "w",
		OTPProvider: "sms", PaymentProvider: "telebirr", PushProvider: "fcm",
	}
}

func TestValidate(t *testing.T) {
	demo := safeProduction()
	demo.DemoMode = true
	noDB := safeProduction()
	noDB.DatabaseURL = ""
	shortSecret := safeProduction()
	shortSecret.JWTSecret = "short"
	httpURL := safeProduction()
	httpURL.PublicURL = "http://api.example.com"
	fakeGateway := safeProduction()
	fakeGateway.PaymentProvider = "dev"
	logOTP := safeProduction()
	logOTP.OTPProvider = "log"
	logPush := safeProduction()
	logPush.PushProvider = "log"
	noKey := safeProduction()
	noKey.IDKey = ""

	cases := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{"development without anything is allowed", Config{Env: "development"}, false},
		{"development with demo mode is allowed", Config{Env: "development", DemoMode: true}, false},
		{"production, everything real", safeProduction(), false},
		{"production without a database is refused", noDB, true},
		{"production with demo mode is refused", demo, true},
		{"production with a short signing secret is refused", shortSecret, true},
		{"production over plain http is refused", httpURL, true},
		{"production with the fake payment gateway is refused", fakeGateway, true},
		{"production that only logs OTPs is refused", logOTP, true},
		{"production that only logs push alerts is refused", logPush, true},
		{"production without the ID encryption key is refused", noKey, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := c.cfg.Validate(); (err != nil) != c.wantErr {
				t.Fatalf("err=%v, wantErr=%v", err, c.wantErr)
			}
		})
	}
}
