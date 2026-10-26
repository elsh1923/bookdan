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

// fakeChapa plays Chapa's side: it records what the server sends and answers verify with whatever the test sets.
type fakeChapa struct {
	mu        sync.Mutex
	initBody  map[string]any
	auth      string
	verify    map[string]any // the JSON verify answers with
	refunds   []string
	refundBod map[string]any
}

func (f *fakeChapa) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/transaction/initialize", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.auth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&f.initBody)
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"checkout_url": "https://checkout.chapa.co/checkout/payment/abc"}})
	})
	mux.HandleFunc("/transaction/verify/", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(f.verify)
	})
	mux.HandleFunc("/refund/", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.refunds = append(f.refunds, strings.TrimPrefix(r.URL.Path, "/refund/"))
		_ = json.NewDecoder(r.Body).Decode(&f.refundBod)
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"ref_id": "rf1"}})
	})
	return mux
}

func (f *fakeChapa) answer(status, dataStatus string, amount any, currency string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.verify = map[string]any{"status": status, "data": map[string]any{"status": dataStatus, "amount": amount, "currency": currency}}
}

func TestChapaPaymentsAreConfirmedWithChapa(t *testing.T) {
	fake := &fakeChapa{}
	chapaSrv := httptest.NewServer(fake.handler())
	defer chapaSrv.Close()

	cfg := config.Config{Env: "test", DemoMode: true, PublicURL: "http://example.test", PaymentProvider: "chapa", ChapaSecretKey: "CHASECK_TEST-abc",
		DemoAccountDomains: []string{"bookdan.test", "example.com", "example.org"}}
	gw, err := api.NewChapaGateway(cfg.ChapaSecretKey, "", cfg.PublicURL, chapaSrv.URL)
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
	start := func(id string) (ref string, total float64) {
		r := guest.do("POST", "/bookings/"+id+"/pay", map[string]string{"method": "telebirr"})
		if r.status != 200 || !strings.HasPrefix(r.str("session", "payment_url"), "https://checkout.chapa.co/") {
			t.Fatalf("pay: %d %s", r.status, r.raw)
		}
		fake.mu.Lock()
		defer fake.mu.Unlock()
		if fake.auth != "Bearer CHASECK_TEST-abc" {
			t.Fatalf("the secret key must go in the Authorization header: %q", fake.auth)
		}
		ref, _ = fake.initBody["tx_ref"].(string)
		amount, _ := fake.initBody["amount"].(string)
		if !strings.HasPrefix(ref, "bd-") || fake.initBody["currency"] != "ETB" || !strings.Contains(amount, ".") {
			t.Fatalf("initialize body: %v", fake.initBody)
		}
		if !strings.HasSuffix(fake.initBody["return_url"].(string), "/v1/payments/chapa/return?tx_ref="+ref) {
			t.Fatalf("return_url: %v", fake.initBody["return_url"])
		}
		if title, _ := fake.initBody["customization"].(map[string]any)["title"].(string); len(title) > 16 {
			t.Fatalf("Chapa allows 16 characters in the title: %q", title)
		}
		return ref, 0
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

	t.Run("the guest coming back is not enough until Chapa says paid", func(t *testing.T) {
		id := book(40, "chapa-pending")
		ref, _ := start(id)
		fake.answer("success", "pending", "1000", "ETB")
		ret(ref)
		if s := status(id); s != "HELD" {
			t.Fatalf("a pending payment must leave the booking held, got %s", s)
		}
	})

	t.Run("a payment of the wrong amount never confirms a booking", func(t *testing.T) {
		id := book(42, "chapa-wrong")
		ref, _ := start(id)
		fake.answer("success", "success", "1.00", "ETB")
		ret(ref)
		if s := status(id); s != "HELD" {
			t.Fatalf("a short payment must not confirm the booking, got %s", s)
		}
	})

	t.Run("a verified payment moves the booking on, once", func(t *testing.T) {
		id := book(44, "chapa-ok")
		ref, _ := start(id)
		var amount float64
		pay := guest.do("GET", "/bookings/"+id, nil)
		amount = pay.num("deposit_santim")
		if amount == 0 {
			amount = pay.num("total_santim")
		}
		fake.answer("success", "success", float64(int64(amount))/100, "ETB")
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
		ref, _ := start(id)
		pay := guest.do("GET", "/bookings/"+id, nil)
		amount := pay.num("deposit_santim")
		if amount == 0 {
			amount = pay.num("total_santim")
		}
		fake.answer("success", "success", float64(int64(amount))/100, "ETB")
		body := []byte(`{"tx_ref":"` + ref + `","status":"success"}`)
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
		mac := hmac.New(sha256.New, []byte("CHASECK_TEST-abc"))
		mac.Write(body)
		if code := post(hex.EncodeToString(mac.Sum(nil))); code != 200 {
			t.Fatalf("a signed webhook: %d", code)
		}
		if s := status(id); s != "PAID_AWAITING_HOTEL" {
			t.Fatalf("after a signed webhook: %s", s)
		}
	})

	t.Run("a refund goes to Chapa with a repeat-safe reference", func(t *testing.T) {
		if err := gw.Refund(context.Background(), "bd-123", 12345, "ETB"); err != nil {
			t.Fatal(err)
		}
		fake.mu.Lock()
		defer fake.mu.Unlock()
		if len(fake.refunds) != 1 || fake.refunds[0] != "bd-123" || fake.refundBod["amount"] != "123.45" || fake.refundBod["reference"] != "rf-bd-123" {
			t.Fatalf("refund call: %v %v", fake.refunds, fake.refundBod)
		}
	})
}
