package config

import "testing"

// A production configuration with every integration real, for the "safe" case.
func safeProduction() Config {
	return Config{
		Env: "production", DatabaseURL: "postgres://x", PublicURL: "https://api.example.com",
		JWTSecret: "0123456789abcdef0123456789abcdef", IDKey: "k", VoucherSeedB64: "s", PaymentWebhookSecret: "w",
		OTPProvider: "africastalking", ATUsername: "bookdan", ATAPIKey: "atsk_live", PaymentProvider: "telebirr", PushProvider: "fcm", FCMServiceAccountB64: "key",
		EmailProvider: "smtp", SMTPUser: "u@x.com", SMTPPassword: "p", EmailFrom: "BookDan <u@x.com>",
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
	noFCMKey := safeProduction()
	noFCMKey.FCMServiceAccountB64 = ""
	logEmail := safeProduction()
	logEmail.EmailProvider = "log"
	noSMTPPass := safeProduction()
	noSMTPPass.SMTPPassword = ""
	brevoOK := safeProduction()
	brevoOK.EmailProvider, brevoOK.BrevoAPIKey = "brevo", "xkeysib-1"
	brevoNoKey := safeProduction()
	brevoNoKey.EmailProvider = "brevo"

	smsOff := safeProduction()
	smsOff.OTPProvider = "off"
	smsUnknown := safeProduction()
	smsUnknown.OTPProvider = "sms"
	atNoKey := safeProduction()
	atNoKey.ATAPIKey = ""
	atSandbox := safeProduction()
	atSandbox.ATSandbox = true
	atSandboxUser := safeProduction()
	atSandboxUser.ATUsername = "sandbox"
	twilioOK := safeProduction()
	twilioOK.OTPProvider, twilioOK.TwilioSID, twilioOK.TwilioToken, twilioOK.SMSFrom = "twilio", "AC1", "tok", "+15005550006"
	twilioNoFrom := twilioOK
	twilioNoFrom.SMSFrom = ""

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
		{"production with fcm but no Firebase key is refused", noFCMKey, true},
		{"production that only logs emailed codes is refused", logEmail, true},
		{"production with smtp but no password is refused", noSMTPPass, true},
		{"production with brevo and its key is allowed", brevoOK, false},
		{"production with brevo but no key is refused", brevoNoKey, true},
		{"production with phone sign-in switched off is allowed", smsOff, false},
		{"production with an unknown phone provider is refused", smsUnknown, true},
		{"production with Africas Talking but no key is refused", atNoKey, true},
		{"production on the Africas Talking sandbox is refused", atSandbox, true},
		{"production with the sandbox username is refused", atSandboxUser, true},
		{"production with Twilio and its credentials is allowed", twilioOK, false},
		{"production with Twilio but no sender is refused", twilioNoFrom, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := c.cfg.Validate(); (err != nil) != c.wantErr {
				t.Fatalf("err=%v, wantErr=%v", err, c.wantErr)
			}
		})
	}
}
