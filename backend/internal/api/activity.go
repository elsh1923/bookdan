package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/bookdan/backend/internal/httpx"
)

type activityOut struct {
	At       string         `json:"at"`
	Action   string         `json:"action"`
	Hotel    string         `json:"hotel"`
	HotelID  string         `json:"property_id"`
	Who      string         `json:"who"`
	ByHotel  bool           `json:"by_hotel"`
	Details  map[string]any `json:"details"`
	Describe string         `json:"description"`
}

var activityText = map[string]string{
	"admin_hotel_updated":  "edited the hotel details",
	"admin_room_created":   "added a room type",
	"admin_room_updated":   "edited a room type",
	"admin_price_set":      "changed a room price",
	"admin_photo_added":    "added a photo",
	"admin_photo_cover":    "changed the cover photo",
	"admin_photo_deleted":  "deleted a photo",
	"admin_staff_added":    "added a team member",
	"admin_staff_removed":  "removed a team member",
	"admin_staff_alerts":   "changed a person's alert settings",
	"admin_alert_rules":    "changed the booking alert rules",
	"admin_hotel_created":  "created the hotel",
	"admin_business_saved": "saved the business details",
}

// adminActivity: the log of changes made to hotels, newest first. Hotels' owners change their own rooms, prices and photos straight away through the hotel portal,
// so this is where BookDan staff see what changed, when and by whom. ?by=hotel shows only the hotels' own changes; ?property_id= one hotel.
func (s *Server) adminActivity(w http.ResponseWriter, r *http.Request) error {
	q := r.URL.Query()
	limit := 100
	if n, err := strconv.Atoi(q.Get("limit")); err == nil && n > 0 && n <= 500 {
		limit = n
	}
	rows, err := s.pool.Query(r.Context(), `
		SELECT a.at, a.action, coalesce(p.name, ''), coalesce(a.details->>'property_id', ''), coalesce(u.email, u.phone, 'unknown'), coalesce(a.details->>'by', 'admin') <> 'admin', a.details
		FROM audit_log a
		LEFT JOIN users u ON u.id = a.user_id
		LEFT JOIN properties p ON p.id::text = a.details->>'property_id'
		WHERE a.action = ANY($1)
		  AND ($2 = '' OR a.details->>'property_id' = $2)
		  AND ($3 <> 'hotel' OR coalesce(a.details->>'by', 'admin') <> 'admin')
		ORDER BY a.at DESC, a.id DESC LIMIT $4`, activityActions(), q.Get("property_id"), q.Get("by"), limit)
	if err != nil {
		return err
	}
	defer rows.Close()
	items := []activityOut{}
	for rows.Next() {
		var a activityOut
		var at time.Time
		if err := rows.Scan(&at, &a.Action, &a.Hotel, &a.HotelID, &a.Who, &a.ByHotel, &a.Details); err != nil {
			return err
		}
		a.At = at.UTC().Format(time.RFC3339)
		a.Describe = activityText[a.Action]
		items = append(items, a)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	respond(w, http.StatusOK, struct {
		httpx.Meta
		Items []activityOut `json:"items"`
	}{httpx.NowMeta(), items})
	return nil
}

func activityActions() []string {
	out := make([]string, 0, len(activityText))
	for k := range activityText {
		out = append(out, k)
	}
	return out
}
