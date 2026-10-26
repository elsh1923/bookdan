package api_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/bookdan/backend/internal/api"
	"github.com/bookdan/backend/internal/config"
	"github.com/bookdan/backend/internal/server"
)

// fakeChapa plays Chapa's v2 API: it records what the server sends and answers verify with whatever the test sets.
type fakeChapa struct {
	mu      sync.Mutex
	hosted  map[string]any
	auth    string
	idem    string
	verify  map[string]any
	refunds []map[string]any
}

func (f *fakeChapa) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/payments/hosted", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.auth, f.idem = r.Header.Get("Authorization"), r.Header.Get("Idempotency-Key")
		_ = json.NewDecoder(r.Body).Decode(&f.hosted)
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"checkout_url": "https://checkout.chapa.co/checkout/payment/abc"}})
	})
	mux.HandleFunc("/v2/payments/", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(f.verify)
	})
	mux.HandleFunc("/v2/refunds", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.refunds = append(f.refunds, body)
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"status": "PENDING"}})
	})
	return mux
}

// answer sets what verify returns: the payment status, and the amount in cents.
func (f *fakeChapa) answer(status string, amountCents int64, currency string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.verify = map[string]any{"status": "success", "data": map[string]any{"status": status, "amount": amountCents, "currency": currency}}
}

func TestChapaPaymentsAreConfirmedWithChapa(t *testing.T) {
	fake := &fakeChapa{}
	chapaSrv := httptest.NewServer(fake.handler())
	defer chapaSrv.Close()

	const privateKey = "CHAPA_TEST_PRIV_abc"
	const webhookSecret = "whsec-test"
	cfg := config.Config{Env: "test", DemoMode: true, PublicURL: "http://example.test", PaymentProvider: "chapa", ChapaSecretKey: privateKey,
		DemoAccountDomains: []string{"bookdan.test", "example.com", "example.org"}}
	gw, err := api.NewChapaGateway(privateKey, webhookSecret, cfg.PublicURL, chapaSrv.URL)
	if err != nil {
		t.Fatal(err)
	}
	providers := api.DevProviders(cfg)
	providers.Gateway, providers.Push, providers.Mail = gw, push, mails
	providers.OTP, providers.EmailReal = api.RoutedSender(mailer, mailer), true
	app2, err := api.New(context.Background(), cfg, pool, keys, providers)
	if err != nil {
		t.Fatal(err)
	}
	srv2 := httptest.NewServer(server.New(cfg, pool, func(r chi.Router) { app2.Routes(r) }, app2.AdminPage))
	defer srv2.Close()
	main := srv
	srv = srv2
	defer func() { srv = main }()

	guest := newClient(t, "guest")
	guest.signIn("email", uniqueEmail("chapa"))
	withProfile(guest)

	book := func(offset int, key string) (id string) {
		q := quote(guest, meskel, standard, offset, offset+1, 2, 1)
		b := hold(guest, q.str("id"), key)
		if b.status != 200 {
			t.Fatalf("hold: %d %s", b.status, b.raw)
		}
		return b.str("id")
	}
	// start creates the hosted payment and checks what we sent Chapa. It returns the merchant reference.
	start := func(id string) (ref string) {
		r := guest.do("POST", "/bookings/"+id+"/pay", map[string]string{"method": "telebirr"})
		if r.status != 200 || !strings.HasPrefix(r.str("session", "payment_url"), "https://checkout.chapa.co/") {
			t.Fatalf("pay: %d %s", r.status, r.raw)
		}
		fake.mu.Lock()
		defer fake.mu.Unlock()
		if fake.auth != "Bearer "+privateKey {
			t.Fatalf("the private key must go in the Authorization header: %q", fake.auth)
		}
		if fake.idem == "" {
			t.Fatalf("Chapa requires an Idempotency-Key")
		}
		ref, _ = fake.hosted["merchant_reference"].(string)
		amount, isNumber := fake.hosted["amount"].(float64)
		if !strings.HasPrefix(ref, "bd") || len(ref) > 20 || fake.hosted["currency"] != "ETB" || !isNumber || amount <= 0 {
			t.Fatalf("hosted payment body: %v", fake.hosted)
		}
		if _, ok := fake.hosted["tx_ref"]; ok {
			t.Fatalf("v2 uses merchant_reference, not tx_ref")
		}
		if !strings.HasSuffix(fake.hosted["return_url"].(string), "/v1/payments/chapa/return?tx_ref="+ref) {
			t.Fatalf("return_url: %v", fake.hosted["return_url"])
		}
		if title, _ := fake.hosted["customization"].(map[string]any)["title"].(string); len(title) > 16 {
			t.Fatalf("Chapa allows 16 characters in the title: %q", title)
		}
		return ref
	}
	status := func(id string) string { return guest.do("GET", "/bookings/"+id, nil).str("status") }
	ret := func(ref string) {
		res, err := http.Get(srv2.URL + "/v1/payments/chapa/return?tx_ref=" + ref)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, res.Body)
		res.Body.Close()
		if res.StatusCode != 200 {
			t.Fatalf("return page: %d", res.StatusCode)
		}
	}
	// price returns what the booking costs in santim (cents).
	price := func(id string) int64 {
		pay := guest.do("GET", "/bookings/"+id, nil)
		if amt := pay.num("deposit_santim"); amt != 0 {
			return int64(amt)
		}
		return int64(pay.num("total_santim"))
	}

	t.Run("the guest coming back is not enough until Chapa says paid", func(t *testing.T) {
		id := book(40, "chapa-pending")
		ref := start(id)
		fake.answer("PENDING", price(id), "ETB")
		ret(ref)
		if s := status(id); s != "HELD" {
			t.Fatalf("a pending payment must leave the booking held, got %s", s)
		}
	})

	t.Run("a payment of the wrong amount never confirms a booking", func(t *testing.T) {
		id := book(42, "chapa-wrong")
		ref := start(id)
		fake.answer("SUCCESS", 100, "ETB")
		ret(ref)
		if s := status(id); s != "HELD" {
			t.Fatalf("a short payment must not confirm the booking, got %s", s)
		}
	})

	t.Run("a verified payment moves the booking on, once", func(t *testing.T) {
		id := book(44, "chapa-ok")
		ref := start(id)
		fake.answer("SUCCESS", price(id), "ETB")
		ret(ref)
		if s := status(id); s != "PAID_AWAITING_HOTEL" {
			t.Fatalf("after a verified payment: %s", s)
		}
		ret(ref) // a repeat changes nothing
		if s := status(id); s != "PAID_AWAITING_HOTEL" {
			t.Fatalf("after a repeat: %s", s)
		}
	})

	t.Run("the webhook needs Chapa's signature", func(t *testing.T) {
		id := book(46, "chapa-hook")
		ref := start(id)
		fake.answer("SUCCESS", price(id), "ETB")
		body := []byte(`{"webhook_type":"payment","merchant_reference":"` + ref + `","status":"success"}`)
		post := func(sig string) int {
			req, _ := http.NewRequest("POST", srv2.URL+"/v1/webhooks/payments/chapa", strings.NewReader(string(body)))
			req.Header.Set("Content-Type", "application/json")
			if sig != "" {
				req.Header.Set("x-chapa-signature", sig)
			}
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			res.Body.Close()
			return res.StatusCode
		}
		if code := post(""); code != 401 {
			t.Fatalf("an unsigned webhook: %d", code)
		}
		if code := post("00"); code != 401 {
			t.Fatalf("a wrongly signed webhook: %d", code)
		}
		if s := status(id); s != "HELD" {
			t.Fatalf("a refused webhook must change nothing, got %s", s)
		}
		mac := hmac.New(sha256.New, []byte(webhookSecret))
		mac.Write(body)
		if code := post(hex.EncodeToString(mac.Sum(nil))); code != 200 {
			t.Fatalf("a signed webhook: %d", code)
		}
		if s := status(id); s != "PAID_AWAITING_HOTEL" {
			t.Fatalf("after a signed webhook: %s", s)
		}
	})

	t.Run("a refund goes to Chapa with the payment reference and an amount in cents", func(t *testing.T) {
		if err := gw.Refund(context.Background(), "bd0123456789abcdef", 12345, "ETB"); err != nil {
			t.Fatal(err)
		}
		fake.mu.Lock()
		defer fake.mu.Unlock()
		last := fake.refunds[len(fake.refunds)-1]
		ref, _ := last["merchant_reference"].(string)
		if last["payment_reference"] != "bd0123456789abcdef" || last["amount"] != float64(12345) || len(ref) > 20 {
			t.Fatalf("refund body: %v", last)
		}
	})
}
