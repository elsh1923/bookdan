package api

import (
	"html/template"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/bookdan/backend/internal/httpx"
)

// The development payment page. It stands in for Telebirr/Chapa so the whole flow works end to end before a merchant account
// exists. It is registered only when PAYMENT_PROVIDER=dev outside production (see Routes).

var gatewayPage = template.Must(template.New("g").Parse(`<!doctype html><html><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1"><title>Test payment</title>
<style>body{font-family:system-ui,sans-serif;margin:0;padding:24px;background:#f3f6fb;color:#0f1b33}
.card{max-width:420px;margin:40px auto;background:#fff;border-radius:16px;padding:24px;box-shadow:0 6px 24px rgba(15,27,51,.1)}
.tag{display:inline-block;background:#fff1d6;color:#b26a00;border-radius:6px;padding:2px 8px;font-size:12px;font-weight:600}
button{width:100%;min-height:52px;border:0;border-radius:12px;font-size:16px;font-weight:600;margin-top:12px}
.ok{background:#2563eb;color:#fff}.no{background:#fff;color:#c0392b;border:1.5px solid #c0392b}
h1{font-size:22px}.amt{font-size:32px;font-weight:700;margin:12px 0}</style></head><body><div class="card">
<span class="tag">TEST PAYMENT · no real money</span>
{{if .Done}}<h1>{{.Message}}</h1><p>You can go back to the app.</p>
<a href="{{.ReturnURL}}"><button class="ok">Return to the app</button></a>
<script>setTimeout(function(){location.href={{.ReturnURL}}},400)</script>
{{else}}<h1>Pay for booking {{.Code}}</h1><div class="amt">{{.Amount}} {{.Currency}}</div>
<form method="post"><input type="hidden" name="outcome" value="succeeded"><button class="ok">Approve payment</button></form>
<form method="post"><input type="hidden" name="outcome" value="failed"><button class="no">Decline payment</button></form>{{end}}
</div></body></html>`))

type gatewayView struct {
	Done      bool
	Message   string
	ReturnURL string
	Code      string
	Amount    string
	Currency  string
}

func (s *Server) devGatewayPage(w http.ResponseWriter, r *http.Request) {
	v, err := s.gatewayLookup(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = gatewayPage.Execute(w, v)
}

func (s *Server) gatewayLookup(r *http.Request) (gatewayView, error) {
	var v gatewayView
	var santim int64
	err := s.pool.QueryRow(r.Context(), `SELECT b.code, p.amount_santim, p.currency FROM payments p JOIN bookings b ON b.id = p.booking_id
		WHERE p.id = $1 AND p.provider = 'mock'`, chi.URLParam(r, "id")).Scan(&v.Code, &santim, &v.Currency)
	if err == pgx.ErrNoRows || isBadUUID(err) {
		return v, errNotFound
	}
	v.Amount = formatSantim(santim)
	return v, err
}

func formatSantim(s int64) string {
	whole, frac := s/100, s%100
	out := ""
	for i, c := range itoa(whole) {
		if i > 0 && (len(itoa(whole))-i)%3 == 0 {
			out += ","
		}
		out += string(c)
	}
	if frac != 0 {
		out += "." + twoDigits(frac)
	}
	return out
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}

func twoDigits(n int64) string { return string([]byte{byte('0' + n/10), byte('0' + n%10)}) }

func (s *Server) devGatewayComplete(w http.ResponseWriter, r *http.Request) {
	v, err := s.gatewayLookup(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	outcome := r.PostForm.Get("outcome")
	if outcome != "succeeded" && outcome != "failed" {
		http.Error(w, "bad outcome", http.StatusBadRequest)
		return
	}
	id := chi.URLParam(r, "id")
	// The page reports the result the same way a real gateway would: as an event, handled once.
	if err := s.processPaymentEvent(r.Context(), "mock", "dev_"+id+"_"+outcome, "dev_"+id, outcome, []byte(`{"source":"dev gateway page"}`)); err != nil {
		http.Error(w, "could not record the result", http.StatusInternalServerError)
		return
	}
	v.Done, v.ReturnURL = true, "hotelapp://payment-return"
	v.Message = map[string]string{"succeeded": "Payment approved", "failed": "Payment declined"}[outcome]
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = gatewayPage.Execute(w, v)
}

var _ = httpx.CodeNotFound // keep the shared error codes imported for future gateway handlers
