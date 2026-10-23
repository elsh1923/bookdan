package api

import (
	"context"
	"errors"
	"log/slog"

	"github.com/bookdan/backend/internal/config"
)

// ============================================================================================================
// INTEGRATIONS STILL TO BUILD. Everything outside this file is real. These three interfaces are the only places
// the server talks to a third party; each has a development stand-in. config.Validate refuses to start in production
// while a stand-in is selected, so none of them can ship by accident.
//
//   1. OTPSender   sends the sign-in code by SMS (Ethiopian number) or email.      TODO: choose providers.
//   2. Gateway     creates a payment page and refunds money (Telebirr, Chapa).     TODO: merchant accounts + API keys.
//   3. Pusher      sends alerts to phones through Firebase (fcm.go). BUILT: set PUSH_PROVIDER=fcm and FCM_SERVICE_ACCOUNT_B64.
// ============================================================================================================

// OTPSender delivers a one-time code. channel is "sms" or "email".
type OTPSender interface {
	Send(ctx context.Context, channel, to, code string) error
}

// PaymentSession is what a gateway returns when a payment is created.
type PaymentSession struct {
	ProviderRef string // the gateway's own id for this payment
	PaymentURL  string // the page the guest pays on
}

// Gateway talks to the payment provider.
type Gateway interface {
	// Name is stored in payments.provider (mock | telebirr | chapa).
	Name() string
	// CreateSession starts a payment of amountSantim for a booking.
	CreateSession(ctx context.Context, paymentID, bookingCode, method string, amountSantim int64, currency string) (PaymentSession, error)
	// Refund returns money to the guest. The server records the refund in the ledger either way; this moves the money.
	Refund(ctx context.Context, providerRef string, amountSantim int64, currency string) error
}

// Pusher sends an alert to phones. It returns the tokens the push service no longer knows (an uninstalled app), so the
// server can forget them.
type Pusher interface {
	Notify(ctx context.Context, tokens []string, title, body string, data map[string]string) (invalid []string, err error)
}

// ---- development stand-ins

type logSender struct{}

// The code itself is never logged outside DEMO_MODE; in demo mode it is fixed (123456) and printed so a developer can sign in.
func (logSender) Send(_ context.Context, channel, to, code string) error {
	slog.Info("OTP (development stand-in, nothing was sent)", "channel", channel, "to", to, "code", code)
	return nil
}

// devGateway is served by this same process (see devgateway.go): a page with Approve and Decline buttons.
type devGateway struct{ publicURL string }

func (devGateway) Name() string { return "mock" }

func (g devGateway) CreateSession(_ context.Context, paymentID, _ string, _ string, _ int64, _ string) (PaymentSession, error) {
	return PaymentSession{ProviderRef: "dev_" + paymentID, PaymentURL: g.publicURL + "/v1/dev/gateway/" + paymentID}, nil
}

func (devGateway) Refund(_ context.Context, ref string, amount int64, currency string) error {
	slog.Info("refund (development stand-in, no money moved)", "payment", ref, "santim", amount, "currency", currency)
	return nil
}

type logPusher struct{}

func (logPusher) Notify(_ context.Context, tokens []string, title, body string, data map[string]string) ([]string, error) {
	slog.Info("push alert (development stand-in, nothing was sent)", "devices", len(tokens), "title", title, "body", body, "data", data)
	return nil, nil
}

// routedSender sends each sign-in code over its own channel: SMS by the SMS provider, email by the email provider.
type routedSender struct{ sms, email OTPSender }

func (r routedSender) Send(ctx context.Context, channel, to, code string) error {
	if channel == "email" {
		return r.email.Send(ctx, channel, to, code)
	}
	return r.sms.Send(ctx, channel, to, code)
}

// Providers bundles the integrations.
type Providers struct {
	OTP     OTPSender
	Gateway Gateway
	Push    Pusher
	// EmailReal is true when sign-in codes really go out by email. Then real addresses get random codes even in demo mode.
	EmailReal bool
}

// DevProviders returns the stand-ins.
func DevProviders(cfg config.Config) Providers {
	return Providers{OTP: logSender{}, Gateway: devGateway{publicURL: cfg.PublicURL}, Push: logPusher{}}
}

// ProvidersFor selects integrations by name. Real ones are added here as they are built.
func ProvidersFor(cfg config.Config) (Providers, error) {
	p := DevProviders(cfg)
	switch cfg.OTPProvider {
	case "log", "":
	default:
		return p, errors.New("OTP_PROVIDER " + cfg.OTPProvider + " is not built yet")
	}
	switch cfg.EmailProvider {
	case "log", "":
	case "smtp":
		smtpSender, err := newSMTPSender(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUser, cfg.SMTPPassword, cfg.EmailFrom)
		if err != nil {
			return p, err
		}
		p.OTP = routedSender{sms: logSender{}, email: smtpSender}
		p.EmailReal = true
	default:
		return p, errors.New("EMAIL_PROVIDER " + cfg.EmailProvider + " is not built yet (use log or smtp)")
	}
	switch cfg.PaymentProvider {
	case "dev", "":
	default:
		return p, errors.New("PAYMENT_PROVIDER " + cfg.PaymentProvider + " is not built yet")
	}
	switch cfg.PushProvider {
	case "log", "":
	case "fcm":
		fcm, err := newFCMPusher(cfg.FCMServiceAccountB64)
		if err != nil {
			return p, err
		}
		p.Push = fcm
	default:
		return p, errors.New("PUSH_PROVIDER " + cfg.PushProvider + " is not built yet")
	}
	return p, nil
}

// RoutedSender builds the sender that sends SMS codes with sms and email codes with email (used by tests and by ProvidersFor).
func RoutedSender(sms, email OTPSender) OTPSender { return routedSender{sms: sms, email: email} }
