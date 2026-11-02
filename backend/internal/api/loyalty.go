package api

import (
	"context"
	"net/http"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/bookdan/backend/internal/httpx"
)

// Member tiers reward guests who keep coming back to BookDan. A guest's tier depends on how many DIFFERENT hotels they have finished
// a stay at (cancelled, declined and no-show bookings do not count). The tier's discount comes off the room price at checkout, and only
// when the guest pays online and uses no coupon, so BookDan's cost stays on money it actually collects.

type loyaltyTier struct {
	Name       string `json:"name"`
	MinHotels  int    `json:"min_hotels"`
	DiscountBP int    `json:"discount_bps"`
	SortOrder  int    `json:"sort_order"`
}

// loyaltyStatus is what the guest sees: the tier they have now (null before the first finished stay), how many hotels they have
// stayed at, and how far they are from the next tier.
type loyaltyStatus struct {
	httpx.Meta
	Tier         *string `json:"tier"`
	Hotels       int     `json:"hotels"`
	DiscountBP   int     `json:"discount_bps"`
	NextTier     *string `json:"next_tier"`
	HotelsToNext int     `json:"hotels_to_next"`
}

// loyaltyTiers returns the tiers, lowest first.
func (s *Server) loyaltyTiers(ctx context.Context) ([]loyaltyTier, error) {
	rows, err := s.pool.Query(ctx, `SELECT name, min_hotels, discount_bps, sort_order FROM loyalty_tiers ORDER BY min_hotels`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []loyaltyTier{}
	for rows.Next() {
		var t loyaltyTier
		if err := rows.Scan(&t.Name, &t.MinHotels, &t.DiscountBP, &t.SortOrder); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// loyaltyFor works out a guest's tier from their finished stays.
func (s *Server) loyaltyFor(ctx context.Context, userID string) (loyaltyStatus, error) {
	var st loyaltyStatus
	st.Meta = httpx.NowMeta()
	if err := s.pool.QueryRow(ctx, `SELECT count(DISTINCT property_id) FROM bookings WHERE user_id = $1 AND status = 'COMPLETED'`, userID).Scan(&st.Hotels); err != nil {
		return st, err
	}
	tiers, err := s.loyaltyTiers(ctx)
	if err != nil {
		return st, err
	}
	for _, t := range tiers {
		if st.Hotels >= t.MinHotels {
			name := t.Name
			st.Tier, st.DiscountBP = &name, t.DiscountBP
		} else {
			name := t.Name
			st.NextTier, st.HotelsToNext = &name, t.MinHotels-st.Hotels
			break
		}
	}
	return st, nil
}

func (s *Server) getLoyalty(w http.ResponseWriter, r *http.Request) error {
	st, err := s.loyaltyFor(r.Context(), claimsFrom(r).Sub)
	if err != nil {
		return err
	}
	respond(w, http.StatusOK, st)
	return nil
}

// adminLoyalty lists the tiers for the admin page (any admin role can read them).
func (s *Server) adminLoyalty(w http.ResponseWriter, r *http.Request) error {
	tiers, err := s.loyaltyTiers(r.Context())
	if err != nil {
		return err
	}
	respond(w, http.StatusOK, struct {
		httpx.Meta
		Tiers []loyaltyTier `json:"tiers"`
	}{httpx.NowMeta(), tiers})
	return nil
}

var tierNameRE = regexp.MustCompile(`^[a-z]{3,12}$`)

// adminSetLoyalty replaces the tiers. Platform admins only; the change is written to the audit log.
func (s *Server) adminSetLoyalty(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Tiers []loyaltyTier `json:"tiers"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if len(in.Tiers) < 1 || len(in.Tiers) > 5 {
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "Give between 1 and 5 tiers")
	}
	seenName, seenMin := map[string]bool{}, map[int]bool{}
	for i := range in.Tiers {
		t := &in.Tiers[i]
		t.Name = strings.ToLower(strings.TrimSpace(t.Name))
		if !tierNameRE.MatchString(t.Name) || seenName[t.Name] {
			return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "Each tier needs its own name of 3 to 12 letters")
		}
		if t.MinHotels < 1 || t.MinHotels > 100 || seenMin[t.MinHotels] {
			return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "Each tier needs its own number of hotels, from 1 to 100")
		}
		if t.DiscountBP < 0 || t.DiscountBP > 3000 {
			return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "A discount is from 0% to 30%")
		}
		seenName[t.Name], seenMin[t.MinHotels] = true, true
		t.SortOrder = i + 1
	}
	// Sorted by hotels, so the sort order matches the climb.
	for i := 1; i < len(in.Tiers); i++ {
		for j := i; j > 0 && in.Tiers[j].MinHotels < in.Tiers[j-1].MinHotels; j-- {
			in.Tiers[j], in.Tiers[j-1] = in.Tiers[j-1], in.Tiers[j]
		}
	}
	for i := range in.Tiers {
		in.Tiers[i].SortOrder = i + 1
	}
	err := s.tx(r.Context(), func(tx pgx.Tx) error {
		if _, err := tx.Exec(r.Context(), `DELETE FROM loyalty_tiers`); err != nil {
			return err
		}
		for _, t := range in.Tiers {
			if _, err := tx.Exec(r.Context(), `INSERT INTO loyalty_tiers (name, min_hotels, discount_bps, sort_order) VALUES ($1, $2, $3, $4)`, t.Name, t.MinHotels, t.DiscountBP, t.SortOrder); err != nil {
				return err
			}
		}
		return s.audit(r, tx, "admin_loyalty_changed", map[string]any{"tiers": in.Tiers})
	})
	if err != nil {
		return err
	}
	return s.adminLoyalty(w, r)
}

// memberDiscount returns the discount rate (basis points) and tier name for a guest's new quote; zero when there is none.
func (s *Server) memberDiscount(ctx context.Context, userID string) (int, *string, error) {
	st, err := s.loyaltyFor(ctx, userID)
	if err != nil {
		return 0, nil, err
	}
	return st.DiscountBP, st.Tier, nil
}
