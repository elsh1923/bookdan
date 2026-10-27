package api

import (
	"encoding/base64"
	"net/http"
	"strings"
	"time"

	"github.com/bookdan/backend/internal/httpx"
)

// Paged lists. The guest's bookings and the hotel's bookings are listed newest first, a page at a time: ?limit=N (at most the old fixed cap) and
// ?cursor=... (the next_cursor of the previous page). A response that has more after it carries next_cursor. Without parameters a list behaves as it did
// before (the newest 100 for guests, the newest 200 for hotels), so apps that do not page yet keep working.

// pageCursor is "where the last page ended": the created_at and id of its last booking, in a form an app treats as opaque.
func encodeCursor(at time.Time, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(at.UTC().Format(time.RFC3339Nano) + "|" + id))
}

// decodeCursor reads a cursor back. An empty cursor means the first page.
func decodeCursor(c string) (at *time.Time, id string, err error) {
	if c == "" {
		return nil, "", nil
	}
	raw, derr := base64.RawURLEncoding.DecodeString(c)
	if derr != nil {
		return nil, "", fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "cursor is not valid")
	}
	parts := strings.SplitN(string(raw), "|", 2)
	if len(parts) != 2 {
		return nil, "", fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "cursor is not valid")
	}
	t, terr := time.Parse(time.RFC3339Nano, parts[0])
	if terr != nil || len(parts[1]) != 36 {
		return nil, "", fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "cursor is not valid")
	}
	return &t, parts[1], nil
}

// pageLimit reads ?limit= (1 to max; absent means max).
func pageLimit(r *http.Request, max int) (int, error) {
	v := r.URL.Query().Get("limit")
	if v == "" {
		return max, nil
	}
	n := atoiDefault(v, -1)
	if n < 1 || n > max {
		return 0, fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "limit must be a number from 1 to "+itoa(int64(max)))
	}
	return n, nil
}
