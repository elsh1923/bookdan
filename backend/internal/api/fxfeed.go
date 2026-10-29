package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/bookdan/backend/internal/httpx"
)

// The exchange-rate feed (see fxrates.go): fetch the rates of every currency per one birr, turn them into birr per unit, and write the ones the feed is allowed
// to write.

var errFeed = errors.New("the exchange-rate feed could not be read")

// fetchFeed reads the feed: for each currency, how much of it one birr buys.
func (s *Server) fetchFeed(ctx context.Context) (map[string]float64, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.cfg.FXFeedURL, nil)
	if err != nil {
		return nil, err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, errFeed
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, errFeed
	}
	var feed struct {
		Result string             `json:"result"`
		Rates  map[string]float64 `json:"rates"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 2<<20)).Decode(&feed); err != nil || (feed.Result != "" && feed.Result != "success") || len(feed.Rates) == 0 {
		return nil, errFeed
	}
	return feed.Rates, nil
}

// refreshRates writes the feed's rates for the display currencies it is allowed to write (never set, stale, or set by the feed before; or all of them with
// force). It returns the currencies written and the ones left alone because an admin typed them.
func (s *Server) refreshRates(ctx context.Context, force bool) (written, manual []string, err error) {
	if s.cfg.FXFeedURL == "" {
		return nil, nil, errFeed
	}
	feed, err := s.fetchFeed(ctx)
	if err != nil {
		return nil, nil, err
	}
	err = s.tx(ctx, func(tx pgx.Tx) error {
		written, manual = nil, nil
		for _, cur := range displayCurrencyList {
			perBirr := feed[cur]
			if perBirr <= 0 || math.IsNaN(perBirr) || math.IsInf(perBirr, 0) {
				continue
			}
			birr := 1 / perBirr
			if birr < fxMinBirr || birr > fxMaxBirr {
				continue // a value that cannot be right is not written
			}
			e4 := int64(math.Round(birr * 10000))
			var by string
			var at time.Time
			err := tx.QueryRow(ctx, "SELECT updated_by, updated_at FROM fx_rates WHERE currency = $1 FOR UPDATE", cur).Scan(&by, &at)
			if err != nil && err != pgx.ErrNoRows {
				return err
			}
			if err == nil && !force && by != fxFeedBy && s.now().Sub(at) <= fxStaleAfter {
				manual = append(manual, cur)
				continue
			}
			if _, err := tx.Exec(ctx, `INSERT INTO fx_rates (currency, birr_per_unit_e4, updated_at, updated_by) VALUES ($1, $2, now(), $3)
				ON CONFLICT (currency) DO UPDATE SET birr_per_unit_e4 = EXCLUDED.birr_per_unit_e4, updated_at = now(), updated_by = EXCLUDED.updated_by`, cur, e4, fxFeedBy); err != nil {
				return err
			}
			written = append(written, cur)
		}
		return nil
	})
	return written, manual, err
}

// refreshDisplayRates is the daily job: the feed fills the rates nobody typed. A failure is logged only (old rates keep working until they go stale).
func (s *Server) refreshDisplayRates(ctx context.Context) {
	if s.cfg.FXFeedURL == "" {
		return
	}
	written, manual, err := s.refreshRates(ctx, false)
	if err != nil {
		slog.Warn("display exchange rates could not be refreshed", "err", err)
		return
	}
	slog.Info("display exchange rates refreshed", "written", len(written), "typed_by_an_admin", len(manual))
}

// adminRefreshRates is the "Fetch current rates" button. force true replaces typed rates too.
func (s *Server) adminRefreshRates(w http.ResponseWriter, r *http.Request) error {
	if err := requireAdmin(r); err != nil {
		return err
	}
	var in struct {
		Force bool `json:"force"`
	}
	if r.ContentLength > 0 {
		if err := decode(r, &in); err != nil {
			return err
		}
	}
	if s.cfg.FXFeedURL == "" {
		return fail(http.StatusConflict, "FEED_OFF", "The exchange-rate feed is switched off on this server (FX_FEED_URL is empty). Type the rates by hand.")
	}
	written, manual, err := s.refreshRates(r.Context(), in.Force)
	if err == errFeed {
		return fail(http.StatusBadGateway, "FEED_UNAVAILABLE", "The exchange-rate feed could not be read just now. Try again in a few minutes, or type the rates by hand.")
	}
	if err != nil {
		return err
	}
	if _, err := s.pool.Exec(r.Context(), `INSERT INTO audit_log (user_id, action, ip, details) VALUES ($1, 'fx_rates_refreshed', NULLIF($2, '')::inet, $3)`,
		claimsFrom(r).Sub, clientIP(r), map[string]any{"written": len(written), "left_alone": len(manual), "force": in.Force}); err != nil {
		return err
	}
	respond(w, http.StatusOK, struct {
		httpx.Meta
		Written []string `json:"written"`
		Manual  []string `json:"left_alone"`
	}{httpx.NowMeta(), nilToEmpty(written), nilToEmpty(manual)})
	return nil
}

func nilToEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
