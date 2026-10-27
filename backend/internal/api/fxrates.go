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

// Display currencies. Hotels price in ETB and BookDan charges and refunds in ETB only; the apps can show an approximate USD or EUR amount
// next to it. The rates are set by a platform admin (there is no outside feed to depend on or to be wrong), and are served to everyone at
// GET /v1/rates. A rate older than fxStaleAfter is marked stale, and the apps then show ETB only rather than a misleading number.
// A change of more than fxMaxJump from the current rate must be confirmed, so a typo does not silently misprice every screen.

const (
	fxStaleAfter = 30 * 24 * time.Hour
	fxMaxJump    = 0.30
)

var displayCurrencies = map[string]bool{"USD": true, "EUR": true}

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
		Base  string      `json:"base"`
		Rates []fxRateOut `json:"rates"`
	}{httpx.NowMeta(), "ETB", list})
	return nil
}

// adminSetRate sets the rate for USD or EUR: birr per one unit, for example 148.5.
func (s *Server) adminSetRate(w http.ResponseWriter, r *http.Request) error {
	if err := requireAdmin(r); err != nil {
		return err
	}
	cur := strings.ToUpper(chi.URLParam(r, "currency"))
	if !displayCurrencies[cur] {
		return fail(http.StatusNotFound, httpx.CodeNotFound, "Only USD and EUR have a display rate")
	}
	var in struct {
		BirrPer float64 `json:"birr_per_unit"`
		Confirm bool    `json:"confirm"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if math.IsNaN(in.BirrPer) || in.BirrPer < 1 || in.BirrPer > 10000 {
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "birr_per_unit must be between 1 and 10000 (for example 148.5)")
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
