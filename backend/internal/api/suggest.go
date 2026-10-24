package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/bookdan/backend/internal/httpx"
)

// Place suggestions as the guest types: landmarks, areas and cities where hotels are, and hotels by name. Public, like search.

type suggestion struct {
	// Kind is landmark, area, city or hotel. Text is what to search for (or the hotel's name); PropertyID is set for a hotel.
	Kind       string `json:"kind"`
	Text       string `json:"text"`
	TextAm     string `json:"text_am,omitempty"`
	Subtitle   string `json:"subtitle"`
	PropertyID string `json:"property_id,omitempty"`
}

func likeEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

func (s *Server) suggest(w http.ResponseWriter, r *http.Request) error {
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	if len(q) > 60 {
		q = q[:60]
	}
	out, err := s.suggestions(r.Context(), q)
	if err != nil {
		return err
	}
	if out == nil {
		out = []suggestion{}
	}
	respond(w, http.StatusOK, struct {
		httpx.Meta
		Items []suggestion `json:"items"`
	}{httpx.NowMeta(), out})
	return nil
}

func (s *Server) suggestions(ctx context.Context, q string) ([]suggestion, error) {
	like := "%" + likeEscape(q) + "%"
	// read runs a query and turns each row into a suggestion.
	read := func(sql string, args []any, build func(pgx.Rows) (suggestion, error)) ([]suggestion, error) {
		rows, err := s.pool.Query(ctx, sql, args...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var list []suggestion
		for rows.Next() {
			sg, err := build(rows)
			if err != nil {
				return nil, err
			}
			list = append(list, sg)
		}
		return list, rows.Err()
	}

	// Landmarks first. With nothing typed these are the "popular places" the guest sees.
	out, err := read(`SELECT name, coalesce(name_am, ''), city FROM landmarks
		WHERE $1 = '' OR lower(name) LIKE $2 OR lower(coalesce(name_am, '')) LIKE $2 ORDER BY (lower(name) LIKE $3) DESC, name LIMIT 6`,
		[]any{q, like, likeEscape(q) + "%"}, func(r pgx.Rows) (suggestion, error) {
			sg := suggestion{Kind: "landmark"}
			err := r.Scan(&sg.Text, &sg.TextAm, &sg.Subtitle)
			return sg, err
		})
	if err != nil {
		return nil, err
	}
	if q == "" {
		return out, nil
	}

	// Areas and cities that have a published hotel (skipping a name already listed as a landmark).
	areas, err := read(`SELECT 'area', area, city FROM properties WHERE status = 'active' AND lower(area) LIKE $1 GROUP BY area, city
		UNION ALL SELECT 'city', city, 'Ethiopia' FROM properties WHERE status = 'active' AND lower(city) LIKE $1 GROUP BY city LIMIT 6`,
		[]any{like}, func(r pgx.Rows) (suggestion, error) {
			var sg suggestion
			err := r.Scan(&sg.Kind, &sg.Text, &sg.Subtitle)
			return sg, err
		})
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, sg := range out {
		seen[strings.ToLower(sg.Text)] = true
	}
	for _, sg := range areas {
		if !seen[strings.ToLower(sg.Text)] {
			seen[strings.ToLower(sg.Text)] = true
			out = append(out, sg)
		}
	}

	// Hotels by name.
	hotels, err := read(`SELECT id::text, name, area || ', ' || city FROM properties WHERE status = 'active' AND lower(name) LIKE $1 ORDER BY name LIMIT 4`,
		[]any{like}, func(r pgx.Rows) (suggestion, error) {
			sg := suggestion{Kind: "hotel"}
			err := r.Scan(&sg.PropertyID, &sg.Text, &sg.Subtitle)
			return sg, err
		})
	if err != nil {
		return nil, err
	}
	out = append(out, hotels...)
	if len(out) > 10 {
		out = out[:10]
	}
	return out, nil
}

// replyMinutes is how long each hotel usually takes to answer a request (a person answering, not the clock running out), in whole
// minutes, over the last 60 days. A hotel with fewer than three answers has no figure yet.
func (s *Server) replyMinutes(ctx context.Context, propertyIDs []string) (map[string]*int, error) {
	out := map[string]*int{}
	if len(propertyIDs) == 0 {
		return out, nil
	}
	rows, err := s.pool.Query(ctx, `SELECT b.property_id::text, round(avg(extract(epoch FROM (e.created_at - b.paid_at)) / 60))::int
		FROM bookings b JOIN booking_events e ON e.booking_id = b.id
		WHERE b.property_id = ANY($1::uuid[]) AND b.paid_at > now() - interval '60 days'
		  AND e.to_status IN ('CONFIRMED', 'DECLINED_REFUNDED') AND e.actor LIKE 'staff:%' AND e.created_at >= b.paid_at
		GROUP BY b.property_id HAVING count(*) >= 3`, propertyIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var m int
		if err := rows.Scan(&id, &m); err != nil {
			return nil, err
		}
		if m < 1 {
			m = 1
		}
		out[id] = &m
	}
	return out, rows.Err()
}
