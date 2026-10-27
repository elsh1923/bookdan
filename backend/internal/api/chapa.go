package api

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/bookdan/backend/internal/httpx"
)

// Chapa's v2 API (https://docs.chapa.global) takes Telebirr, CBE Birr, M-Pesa, bank and card payments on one hosted page.
// The flow: we create a hosted payment and send the guest to Chapa's page. Chapa reports the result by a signed webhook
// (x-chapa-signature) and sends the guest's browser back to return_url. Neither is trusted alone: every result is confirmed by
// asking Chapa to verify it with our private key, and the amount and currency must match what we asked for.
// Amounts are in cents, which is the same unit as santim, so no conversion is needed.

const chapaBase = "https://api.chapa.global"

// The merchant reference we give Chapa: "bd" and 16 hex digits of the payment id (Chapa allows 20 characters).
const chapaRefPrefix = "bd"

type chapaGateway struct {
	secret        string // the private key: Authorization: Bearer
	webhookSecret string // the webhook secret from the dashboard: verifies x-chapa-signature
	publicURL     string
	base          string
	hc            *http.Client
}

// NewChapaGateway returns the Chapa gateway. base is the API address ("" for Chapa's own; tests pass a fake).
func NewChapaGateway(secret, webhookSecret, publicURL, base string) (Gateway, error) {
	if strings.TrimSpace(secret) == "" {
		return nil, errors.New("CHAPA_SECRET_KEY is required when PAYMENT_PROVIDER=chapa")
	}
	if base == "" {
		base = chapaBase
	}
	return &chapaGateway{secret: strings.TrimSpace(secret), webhookSecret: strings.TrimSpace(webhookSecret), publicURL: strings.TrimRight(publicURL, "/"),
		base: strings.TrimRight(base, "/"), hc: &http.Client{Timeout: 20 * time.Second}}, nil
}

func (g *chapaGateway) Name() string { return "chapa" }

// chapaMerchantRef is the reference Chapa knows the payment by. It is also what we store as provider_ref.
func chapaMerchantRef(paymentID string) string {
	id := strings.ReplaceAll(paymentID, "-", "")
	if len(id) > 16 {
		id = id[:16]
	}
	return chapaRefPrefix + id
}

// call sends one request to Chapa. out, when given, receives the JSON answer. idem is the Idempotency-Key (Chapa requires it on
// payment and refund creation, so a retry never charges or refunds twice).
func (g *chapaGateway) call(ctx context.Context, method, path string, body any, out any, idem string) error {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, g.base+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+g.secret)
	req.Header.Set("Content-Type", "application/json")
	if idem != "" {
		req.Header.Set("Idempotency-Key", idem)
	}
	res, err := g.hc.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode >= 300 {
		// Chapa's message says what is wrong (a bad key, a missing permission). It holds no secrets.
		var e struct {
			Message any `json:"message"`
		}
		_ = json.Unmarshal(raw, &e)
		slog.Error("chapa request failed", "path", path, "status", res.StatusCode, "message", fmt.Sprint(e.Message))
		return fmt.Errorf("chapa %s answered %d", path, res.StatusCode)
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("chapa %s: unreadable answer", path)
		}
	}
	return nil
}

// CreateSession starts a hosted payment and returns the page the guest pays on.
func (g *chapaGateway) CreateSession(ctx context.Context, paymentID, bookingCode, _ string, amountSantim int64, currency string) (PaymentSession, error) {
	if currency != "ETB" && currency != "USD" {
		return PaymentSession{}, errors.New("chapa takes ETB and USD only")
	}
	ref := chapaMerchantRef(paymentID)
	body := map[string]any{
		"amount":             amountSantim, // cents
		"currency":           currency,
		"merchant_reference": ref,
		"callback_url":       g.publicURL + "/v1/webhooks/payments/chapa",
		// The guest lands on our page, which confirms the payment with Chapa and then sends them back into the app.
		"return_url":    g.publicURL + "/v1/payments/chapa/return?tx_ref=" + url.QueryEscape(ref),
		"customization": map[string]string{"title": "BookDan", "description": "Booking " + bookingCode},
	}
	var out struct {
		Status string `json:"status"`
		Data   struct {
			CheckoutURL string `json:"checkout_url"`
		} `json:"data"`
	}
	if err := g.call(ctx, http.MethodPost, "/v2/payments/hosted", body, &out, "pay-"+paymentID); err != nil {
		return PaymentSession{}, err
	}
	if out.Status != "success" || out.Data.CheckoutURL == "" {
		return PaymentSession{}, errors.New("chapa did not return a payment page")
	}
	return PaymentSession{ProviderRef: ref, PaymentURL: out.Data.CheckoutURL}, nil
}

// Refund asks Chapa to return the payment. The Idempotency-Key makes a repeat safe: Chapa does not refund the same payment twice.
func (g *chapaGateway) Refund(ctx context.Context, providerRef, refundKey string, amountSantim int64, _ string) error {
	merchantRef, idem := "rf"+strings.TrimPrefix(providerRef, chapaRefPrefix), "refund-"+providerRef // at most 20 characters
	if refundKey != "" {
		// A partial refund has its own references: a second refund on the same payment must not be taken for a repeat of the first.
		short := strings.ReplaceAll(refundKey, "-", "")
		if len(short) > 18 {
			short = short[:18]
		}
		merchantRef, idem = "rf"+short, "refund-"+providerRef+"-"+refundKey
	}
	body := map[string]any{
		"payment_reference":  providerRef,
		"merchant_reference": merchantRef,
		"amount":             amountSantim,
		"reason":             "BookDan booking refund",
	}
	var out struct {
		Status string `json:"status"`
	}
	if err := g.call(ctx, http.MethodPost, "/v2/refunds", body, &out, idem); err != nil {
		return err
	}
	if out.Status != "success" {
		return errors.New("chapa did not accept the refund")
	}
	return nil
}

// chapaTx is what Chapa says about a payment.
type chapaTx struct {
	Paid     bool
	Failed   bool
	Amount   int64 // santim (cents)
	Currency string
}

// verify asks Chapa for the truth about one payment.
func (g *chapaGateway) verify(ctx context.Context, ref string) (chapaTx, error) {
	var out struct {
		Status string `json:"status"`
		Data   struct {
			Status   string      `json:"status"`
			Amount   json.Number `json:"amount"`
			Currency string      `json:"currency"`
		} `json:"data"`
	}
	if err := g.call(ctx, http.MethodGet, "/v2/payments/"+url.PathEscape(ref)+"/verify", nil, &out, ""); err != nil {
		return chapaTx{}, err
	}
	tx := chapaTx{Currency: strings.ToUpper(out.Data.Currency)}
	if f, err := out.Data.Amount.Float64(); err == nil {
		tx.Amount = int64(math.Round(f))
	}
	switch out.Data.Status {
	case "SUCCESS":
		tx.Paid = out.Status == "success"
	case "FAILED", "CANCELLED", "BLOCKED":
		tx.Failed = true
	}
	return tx, nil // PENDING and INCOMPLETE: Chapa will be asked again
}

// ---- results

// chapaSettle confirms one payment with Chapa and applies the result. It is safe to call many times (the webhook, the guest's return
// and a retry may all arrive): processPaymentEvent applies a given event once.
func (s *Server) chapaSettle(ctx context.Context, ref string) error {
	g, ok := s.p.Gateway.(*chapaGateway)
	if !ok {
		return errNotFound
	}
	var payID, bookingID string
	var amount int64
	var cur, status string
	err := s.pool.QueryRow(ctx, `SELECT id, booking_id, amount_santim, currency, status FROM payments WHERE provider = 'chapa' AND provider_ref = $1`, ref).Scan(&payID, &bookingID, &amount, &cur, &status)
	if err == pgx.ErrNoRows {
		return errNotFound
	}
	if err != nil {
		return err
	}
	if status != "pending" && status != "failed" {
		return nil // already decided (a payment recorded as failed is checked again: the guest may have paid after all)
	}
	tx, err := g.verify(ctx, ref)
	if err != nil {
		return err
	}
	switch {
	case tx.Paid:
		if tx.Amount != amount || tx.Currency != cur {
			// Paid, but not the amount we asked for: never confirm a booking on it. A person looks at this one.
			slog.Error("chapa payment does not match the booking", "ref", ref, "asked_santim", amount, "paid_santim", tx.Amount, "asked", cur, "paid", tx.Currency)
			s.raisePaymentIssue(ctx, payID, bookingID, "amount_mismatch", map[string]any{
				"asked_santim": amount, "asked_currency": cur, "paid_santim": tx.Amount, "paid_currency": tx.Currency, "gateway_ref": ref,
			})
			return nil
		}
		return s.processPaymentEvent(ctx, "chapa", "chapa_"+ref+"_succeeded", ref, "succeeded", []byte(`{"source":"chapa verify"}`))
	case tx.Failed:
		return s.processPaymentEvent(ctx, "chapa", "chapa_"+ref+"_failed", ref, "failed", []byte(`{"source":"chapa verify"}`))
	}
	return nil // still pending: Chapa will call again
}

// chapaWebhook receives Chapa's server-to-server notice. The signature proves it came from Chapa; the result itself always comes
// from verify.
func (s *Server) chapaWebhook(w http.ResponseWriter, r *http.Request) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, 64<<10))
	if err != nil {
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "Could not read the request")
	}
	g, ok := s.p.Gateway.(*chapaGateway)
	if !ok {
		return errNotFound
	}
	if !g.signatureOK(body, r.Header.Get("x-chapa-signature")) {
		return errAuth
	}
	var ev struct {
		WebhookType       string `json:"webhook_type"`
		MerchantReference string `json:"merchant_reference"`
	}
	if err := json.Unmarshal(body, &ev); err != nil || !strings.HasPrefix(ev.MerchantReference, chapaRefPrefix) {
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "merchant_reference is required")
	}
	// Refund and payout notices are not payments: acknowledge them and leave them alone.
	if ev.WebhookType == "payment" || ev.WebhookType == "" {
		if err := s.chapaSettle(r.Context(), ev.MerchantReference); err != nil && err != errNotFound {
			return err
		}
	}
	respond(w, http.StatusOK, httpx.NowMeta())
	return nil
}

// signatureOK checks x-chapa-signature: the lowercase hex HMAC-SHA256 of the raw body, keyed with the webhook secret.
// Without a webhook secret nothing is accepted.
func (g *chapaGateway) signatureOK(body []byte, signature string) bool {
	if g.webhookSecret == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(g.webhookSecret))
	mac.Write(body)
	got, err := hex.DecodeString(strings.TrimSpace(signature))
	return err == nil && hmac.Equal(got, mac.Sum(nil))
}

var chapaReturnPage = template.Must(template.New("r").Parse(`<!doctype html><html><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1"><title>BookDan</title>
<style>body{font-family:system-ui,sans-serif;margin:0;padding:24px;background:#f3f6fb;color:#0f1b33}
.card{max-width:420px;margin:60px auto;background:#fff;border-radius:16px;padding:24px;text-align:center;box-shadow:0 6px 24px rgba(15,27,51,.1)}
a.btn{display:block;margin-top:16px;padding:16px;border-radius:12px;background:#2563eb;color:#fff;font-weight:600;text-decoration:none}</style></head>
<body><div class="card"><h1>{{.Title}}</h1><p>{{.Body}}</p><a class="btn" href="hotelapp://payment-return">Return to the app</a></div>
<script>setTimeout(function(){location.href="hotelapp://payment-return"},600)</script></body></html>`))

// chapaReturn is where the guest's browser lands after Chapa's page. It confirms the payment with Chapa, so the booking moves on at once
// even if the webhook is late, then hands the guest back to the app (which reads the booking itself).
func (s *Server) chapaReturn(w http.ResponseWriter, r *http.Request) {
	ref := r.URL.Query().Get("tx_ref")
	if !strings.HasPrefix(ref, chapaRefPrefix) {
		http.NotFound(w, r)
		return
	}
	title, body := "Thank you", "We are confirming your payment. Go back to the app to see your booking."
	if err := s.chapaSettle(r.Context(), ref); err != nil && err != errNotFound {
		slog.Error("chapa return: could not confirm", "ref", ref, "err", err)
		title, body = "Still confirming", "We could not confirm your payment yet. It will update in the app within a few minutes."
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = chapaReturnPage.Execute(w, map[string]string{"Title": title, "Body": body})
}
