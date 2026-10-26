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
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/bookdan/backend/internal/httpx"
)

// Chapa (https://developer.chapa.co) takes Telebirr, CBE Birr, M-Pesa, bank and card payments on one hosted page.
// The flow: we create a payment (initialize) and send the guest to Chapa's page; Chapa tells us the result by a signed webhook
// and by sending the guest's browser back to return_url. Neither is trusted by itself: every result is confirmed by asking
// Chapa (verify) with our secret key, and the amount and currency must match what we asked for.

const chapaBase = "https://api.chapa.co/v1"

type chapaGateway struct {
	secret        string
	webhookSecret string // optional second secret set in the Chapa dashboard; the API secret key is also accepted
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

// chapaTxRef is the reference Chapa knows the payment by. It is also what we store as provider_ref.
func chapaTxRef(paymentID string) string { return "bd-" + paymentID }

// chapaAmount writes santim as the decimal string Chapa expects (12345 -> "123.45").
func chapaAmount(santim int64) string {
	return strconv.FormatInt(santim/100, 10) + "." + fmt.Sprintf("%02d", santim%100)
}

func (g *chapaGateway) call(ctx context.Context, method, path string, body any, out any) (int, error) {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, g.base+path, rd)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+g.secret)
	req.Header.Set("Content-Type", "application/json")
	res, err := g.hc.Do(req)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if out != nil {
		_ = json.Unmarshal(raw, out)
	}
	if res.StatusCode >= 300 {
		// Chapa's message says what is wrong (a bad key, a missing permission). It holds no secrets.
		var e struct {
			Message any `json:"message"`
		}
		_ = json.Unmarshal(raw, &e)
		slog.Error("chapa request failed", "path", path, "status", res.StatusCode, "message", fmt.Sprint(e.Message))
		return res.StatusCode, fmt.Errorf("chapa %s answered %d", path, res.StatusCode)
	}
	return res.StatusCode, nil
}

func (g *chapaGateway) CreateSession(ctx context.Context, paymentID, bookingCode, _ string, amountSantim int64, currency string) (PaymentSession, error) {
	if currency != "ETB" && currency != "USD" {
		return PaymentSession{}, errors.New("chapa takes ETB and USD only")
	}
	ref := chapaTxRef(paymentID)
	body := map[string]any{
		"amount":       chapaAmount(amountSantim),
		"currency":     currency,
		"tx_ref":       ref,
		"callback_url": g.publicURL + "/v1/webhooks/payments/chapa",
		// The guest lands on our page, which confirms the payment with Chapa and then sends them back into the app.
		"return_url": g.publicURL + "/v1/payments/chapa/return?tx_ref=" + url.QueryEscape(ref),
		"customization": map[string]string{
			"title":       "BookDan", // Chapa allows 16 characters here
			"description": "Booking " + bookingCode,
		},
	}
	var out struct {
		Status string `json:"status"`
		Data   struct {
			CheckoutURL string `json:"checkout_url"`
		} `json:"data"`
	}
	if _, err := g.call(ctx, http.MethodPost, "/transaction/initialize", body, &out); err != nil {
		return PaymentSession{}, err
	}
	if out.Status != "success" || out.Data.CheckoutURL == "" {
		return PaymentSession{}, errors.New("chapa did not return a payment page")
	}
	return PaymentSession{ProviderRef: ref, PaymentURL: out.Data.CheckoutURL}, nil
}

// Refund asks Chapa to return the payment. The reference makes the request safe to repeat: Chapa refuses a second refund
// with the same reference.
func (g *chapaGateway) Refund(ctx context.Context, providerRef string, amountSantim int64, _ string) error {
	body := map[string]any{"amount": chapaAmount(amountSantim), "reason": "BookDan booking refund", "reference": "rf-" + providerRef}
	var out struct {
		Status string `json:"status"`
	}
	if _, err := g.call(ctx, http.MethodPost, "/refund/"+url.PathEscape(providerRef), body, &out); err != nil {
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
	Amount   int64 // santim
	Currency string
}

// verify asks Chapa for the truth about one payment.
func (g *chapaGateway) verify(ctx context.Context, txRef string) (chapaTx, error) {
	var out struct {
		Status string `json:"status"`
		Data   struct {
			Status   string          `json:"status"`
			Amount   json.RawMessage `json:"amount"`
			Currency string          `json:"currency"`
		} `json:"data"`
	}
	if _, err := g.call(ctx, http.MethodGet, "/transaction/verify/"+url.PathEscape(txRef), nil, &out); err != nil {
		return chapaTx{}, err
	}
	tx := chapaTx{Currency: strings.ToUpper(out.Data.Currency)}
	amt := strings.Trim(string(out.Data.Amount), `"`)
	if f, err := strconv.ParseFloat(amt, 64); err == nil {
		tx.Amount = int64(f*100 + 0.5)
	}
	switch strings.ToLower(out.Data.Status) {
	case "success":
		tx.Paid = out.Status == "success"
	case "failed", "cancelled", "canceled", "reversed":
		tx.Failed = true
	}
	return tx, nil
}

// ---- results

// chapaSettle confirms one payment with Chapa and applies the result. It is safe to call many times (the webhook, the guest's return
// and a retry may all arrive): processPaymentEvent applies a given event once.
func (s *Server) chapaSettle(ctx context.Context, txRef string) error {
	g, ok := s.p.Gateway.(*chapaGateway)
	if !ok {
		return errNotFound
	}
	var amount int64
	var cur, status string
	err := s.pool.QueryRow(ctx, `SELECT amount_santim, currency, status FROM payments WHERE provider = 'chapa' AND provider_ref = $1`, txRef).Scan(&amount, &cur, &status)
	if err == pgx.ErrNoRows {
		return errNotFound
	}
	if err != nil {
		return err
	}
	if status != "pending" {
		return nil // already decided
	}
	tx, err := g.verify(ctx, txRef)
	if err != nil {
		return err
	}
	switch {
	case tx.Paid:
		if tx.Amount != amount || tx.Currency != cur {
			// Paid, but not the amount we asked for: never confirm a booking on it. A person looks at this one.
			slog.Error("chapa payment does not match the booking", "tx_ref", txRef, "asked_santim", amount, "paid_santim", tx.Amount, "asked", cur, "paid", tx.Currency)
			return nil
		}
		return s.processPaymentEvent(ctx, "chapa", "chapa_"+txRef+"_succeeded", txRef, "succeeded", []byte(`{"source":"chapa verify"}`))
	case tx.Failed:
		return s.processPaymentEvent(ctx, "chapa", "chapa_"+txRef+"_failed", txRef, "failed", []byte(`{"source":"chapa verify"}`))
	}
	return nil // still pending: Chapa will call again
}

// chapaWebhook is Chapa's server-to-server notice. The signature keeps strangers from making us call Chapa; the result itself
// always comes from verify.
func (s *Server) chapaWebhook(w http.ResponseWriter, r *http.Request) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, 64<<10))
	if err != nil {
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "Could not read the request")
	}
	g, ok := s.p.Gateway.(*chapaGateway)
	if !ok {
		return errNotFound
	}
	if !g.signatureOK(body, r.Header.Get("x-chapa-signature"), r.Header.Get("Chapa-Signature")) {
		return errAuth
	}
	var ev struct {
		TxRef     string `json:"tx_ref"`
		TrxRef    string `json:"trx_ref"`
		Reference string `json:"reference"`
	}
	if err := json.Unmarshal(body, &ev); err != nil {
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "tx_ref is required")
	}
	ref := firstNonEmpty(ev.TxRef, ev.TrxRef)
	if !strings.HasPrefix(ref, "bd-") {
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "tx_ref is required")
	}
	if err := s.chapaSettle(r.Context(), ref); err != nil && err != errNotFound {
		return err
	}
	respond(w, http.StatusOK, httpx.NowMeta())
	return nil
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

// signatureOK accepts a webhook signed (HMAC-SHA256 of the body, hex) with the webhook secret or the API secret key, in either of
// the two headers Chapa documents.
func (g *chapaGateway) signatureOK(body []byte, headers ...string) bool {
	keys := []string{g.secret}
	if g.webhookSecret != "" {
		keys = append(keys, g.webhookSecret)
	}
	for _, key := range keys {
		mac := hmac.New(sha256.New, []byte(key))
		mac.Write(body)
		want := mac.Sum(nil)
		for _, h := range headers {
			if got, err := hex.DecodeString(strings.TrimSpace(h)); err == nil && hmac.Equal(got, want) {
				return true
			}
		}
	}
	return false
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
	title, body := "Thank you", "We are confirming your payment. Go back to the app to see your booking."
	if !strings.HasPrefix(ref, "bd-") {
		http.NotFound(w, r)
		return
	}
	if err := s.chapaSettle(r.Context(), ref); err != nil && err != errNotFound {
		slog.Error("chapa return: could not confirm", "tx_ref", ref, "err", err)
		title, body = "Still confirming", "We could not confirm your payment yet. It will update in the app within a few minutes."
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = chapaReturnPage.Execute(w, map[string]string{"Title": title, "Body": body})
}

var _ = chi.URLParam
