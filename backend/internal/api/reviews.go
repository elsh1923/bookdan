package api

import (
	"context"
	"math"
	"time"
)

// ratingSummary is what a hotel page shows above its reviews.
type ratingSummary struct {
	Average *float64 `json:"average"`
	Count   int      `json:"count"`
	// Distribution has the number of reviews per star: index 0 is one star, index 4 is five stars.
	Distribution [5]int `json:"distribution"`
	Categories   struct {
		Cleanliness *float64 `json:"cleanliness"`
		Location    *float64 `json:"location"`
		Staff       *float64 `json:"staff"`
		Value       *float64 `json:"value"`
	} `json:"categories"`
}

func round1(v *float64) *float64 {
	if v == nil {
		return nil
	}
	r := math.Round(*v*10) / 10
	return &r
}

func (s *Server) ratingSummary(ctx context.Context, propertyID string) (ratingSummary, error) {
	var out ratingSummary
	var avg, clean, loc, staff, value *float64
	err := s.pool.QueryRow(ctx, `SELECT count(*)::int, avg(rating)::float8,
			count(*) FILTER (WHERE rating = 1)::int, count(*) FILTER (WHERE rating = 2)::int, count(*) FILTER (WHERE rating = 3)::int,
			count(*) FILTER (WHERE rating = 4)::int, count(*) FILTER (WHERE rating = 5)::int,
			avg(rating_cleanliness)::float8, avg(rating_location)::float8, avg(rating_staff)::float8, avg(rating_value)::float8
		FROM reviews WHERE property_id = $1`, propertyID).Scan(&out.Count, &avg, &out.Distribution[0], &out.Distribution[1], &out.Distribution[2],
		&out.Distribution[3], &out.Distribution[4], &clean, &loc, &staff, &value)
	if err != nil {
		return out, err
	}
	out.Average = round1(avg)
	out.Categories.Cleanliness, out.Categories.Location, out.Categories.Staff, out.Categories.Value = round1(clean), round1(loc), round1(staff), round1(value)
	return out, nil
}

// timeline reads the booking's history: each status it reached and when.
func (s *Server) timeline(ctx context.Context, bookingID string) ([]timelineEvent, error) {
	rows, err := s.pool.Query(ctx, `SELECT to_status, created_at FROM booking_events WHERE booking_id = $1 ORDER BY id`, bookingID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []timelineEvent{}
	for rows.Next() {
		var e timelineEvent
		var at time.Time
		if err := rows.Scan(&e.Status, &at); err != nil {
			return nil, err
		}
		e.At = at.UTC().Format(time.RFC3339)
		out = append(out, e)
	}
	return out, rows.Err()
}
