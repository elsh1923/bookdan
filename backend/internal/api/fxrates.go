package api

import (
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/bookdan/backend/internal/httpx"
)

// Display currencies. Hotels price in ETB and BookDan charges and refunds in ETB only; the apps can show an approximate amount in one of the
// currencies below (a guest picks one in the app, like in a travel booking app). The rates are served to everyone at GET /v1/rates. A rate older than
// fxStaleAfter is marked stale, and the apps then show ETB only rather than a misleading number.
// Where the rates come from: a platform admin can type one (it is then "manual" and nothing overwrites it), and the server fills the rest from a public
// exchange-rate feed (FX_FEED_URL, default open.er-api.com, no key): once a day, and when an admin presses "Fetch current rates". The feed only ever
// writes the currencies that were never set, whose rate went stale, or that the feed set earlier; a rate an admin typed is left alone.
// A change of more than fxMaxJump from the current rate must be confirmed (typed rates only), so a typo does not silently misprice every screen.

const (
	fxStaleAfter = 30 * 24 * time.Hour
	fxMaxJump    = 0.30
	fxMinBirr    = 0.01    // birr for one unit: the yen is worth less than a birr
	fxMaxBirr    = 10000.0 // birr for one unit
	fxFeedBy     = "feed: open.er-api.com"
)

// displayCurrencyList is every currency a guest can choose to see prices in, most popular first (the Gulf and the usual travel currencies), then the rest by
// code. The apps carry the same list with names (packages/domain/src/currencies.ts); keep them in step.
var displayCurrencyList = []string{
	"AED", "SAR", "QAR", "KWD", "OMR", "BHD", "USD", "GBP", "EUR",
	"AUD", "BRL", "CAD", "CHF", "CNY", "DJF", "DKK", "EGP", "ILS", "INR", "JPY", "KES", "MYR", "NOK", "NZD", "SEK", "SGD", "TRY", "TZS", "UGX", "ZAR",
}

var displayCurrencies = func() map[string]bool {
	m := map[string]bool{}
	for _, c := range displayCurrencyList {
		m[c] = true
	}
	return m
}()

type fxRateOut struct {
	Currency  string  `json:"currency"`
	BirrPer   float64 `json:"birr_per_unit"`
	UpdatedAt string  `json:"updated_at"`
	UpdatedBy string  `json:"updated_by,omitempty"` // admin view only
	Stale     bool    `json:"stale"`
}

func (s *Server) readRates(r *http.Request, admin bool) ([]fxRateOut, error) {
	rows, err := s.pool.Query(r.Context(), `SELECT currency, birr_per_unit_e4, updated_at, updated_by FROM fx_rates ORDER BY currency`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []fxRateOut{}
	for rows.Next() {
		var c, by string
		var e4 int64
		var at time.Time
		if err := rows.Scan(&c, &e4, &at, &by); err != nil {
			return nil, err
		}
		o := fxRateOut{Currency: c, BirrPer: float64(e4) / 10000, UpdatedAt: at.UTC().Format(time.RFC3339), Stale: s.now().Sub(at) > fxStaleAfter}
		if admin {
			o.UpdatedBy = by
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// rates is public: the apps read it at start-up.
func (s *Server) rates(w http.ResponseWriter, r *http.Request) error {
	list, err := s.readRates(r, false)
	if err != nil {
		return err
	}
	w.Header().Set("Cache-Control", "public, max-age=300")
	respond(w, http.StatusOK, struct {
		httpx.Meta
		Base  string      `json:"base"`
		Rates []fxRateOut `json:"rates"`
	}{httpx.NowMeta(), "ETB", list})
	return nil
}

func (s *Server) adminRates(w http.ResponseWriter, r *http.Request) error {
	if err := requireAdmin(r); err != nil {
		return err
	}
	list, err := s.readRates(r, true)
	if err != nil {
		return err
	}
	respond(w, http.StatusOK, struct {
		httpx.Meta
		Base      string      `json:"base"`
		Rates     []fxRateOut `json:"rates"`
		Supported []string    `json:"supported"`
		Feed      bool        `json:"feed"`
	}{httpx.NowMeta(), "ETB", list, displayCurrencyList, s.cfg.FXFeedURL != ""})
	return nil
}

// adminSetRate sets the rate of a display currency by hand: birr per one unit, for example 148.5. A typed rate is never overwritten by the feed.
func (s *Server) adminSetRate(w http.ResponseWriter, r *http.Request) error {
	if err := requireAdmin(r); err != nil {
		return err
	}
	cur := strings.ToUpper(chi.URLParam(r, "currency"))
	if !displayCurrencies[cur] {
		return fail(http.StatusNotFound, httpx.CodeNotFound, "That currency has no display rate")
	}
	var in struct {
		BirrPer float64 `json:"birr_per_unit"`
		Confirm bool    `json:"confirm"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if math.IsNaN(in.BirrPer) || in.BirrPer < fxMinBirr || in.BirrPer > fxMaxBirr {
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "birr_per_unit must be between 0.01 and 10000 (for example 148.5)")
	}
	e4 := int64(math.Round(in.BirrPer * 10000))
	ctx := r.Context()
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var who string
		if err := tx.QueryRow(ctx, `SELECT coalesce(email, phone, id::text) FROM users WHERE id = $1`, claimsFrom(r).Sub).Scan(&who); err != nil {
			return err
		}
		var old int64
		err := tx.QueryRow(ctx, `SELECT birr_per_unit_e4 FROM fx_rates WHERE currency = $1 FOR UPDATE`, cur).Scan(&old)
		if err != nil && err != pgx.ErrNoRows {
			return err
		}
		if err == nil && !in.Confirm && math.Abs(float64(e4-old))/float64(old) > fxMaxJump {
			return fail(http.StatusConflict, "RATE_JUMP", fmt.Sprintf("That is more than %d%% away from the current rate of %.4f. Check it, then confirm.", int(fxMaxJump*100), float64(old)/10000))
		}
		if _, err := tx.Exec(ctx, `INSERT INTO fx_rates (currency, birr_per_unit_e4, updated_at, updated_by) VALUES ($1, $2, now(), $3)
			ON CONFLICT (currency) DO UPDATE SET birr_per_unit_e4 = EXCLUDED.birr_per_unit_e4, updated_at = now(), updated_by = EXCLUDED.updated_by`, cur, e4, who); err != nil {
			return err
		}
		return s.audit(r, tx, "fx_rate_set", map[string]any{"currency": cur, "birr_per_unit_e4": e4, "was_e4": old})
	})
	if err != nil {
		return err
	}
	respond(w, http.StatusOK, httpx.NowMeta())
	return nil
}
