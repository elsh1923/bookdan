package api

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/bookdan/backend/internal/httpx"
)

// Short Google Maps links (maps.app.goo.gl/...) that phones share carry no position: they redirect to a long address that does.
// The admin and hotel portals cannot follow that redirect themselves (their content policy allows no outside requests), so the
// server follows it for them. It only ever contacts Google Maps hosts over https, never a host the caller chose.

var mapLinkHosts = map[string]bool{
	"maps.app.goo.gl": true, "goo.gl": true, "www.google.com": true, "google.com": true, "maps.google.com": true,
}

var mapLinkCoords = []*regexp.Regexp{
	regexp.MustCompile(`@(-?\d+\.\d+),\s*(-?\d+\.\d+)`),
	regexp.MustCompile(`!3d(-?\d+\.\d+)!4d(-?\d+\.\d+)`),
	regexp.MustCompile(`[?&](?:q|ll|query|destination)=(-?\d+\.\d+)(?:,|%2C)\s*(-?\d+\.\d+)`),
	regexp.MustCompile(`/place/(-?\d+\.\d+),\s*(-?\d+\.\d+)`),
}

// coordsIn finds a latitude and longitude in an address, or reports false.
func coordsIn(s string) (lat, lng float64, ok bool) {
	if u, err := url.QueryUnescape(s); err == nil {
		s = u
	}
	for _, re := range mapLinkCoords {
		if m := re.FindStringSubmatch(s); m != nil {
			la, _ := strconv.ParseFloat(m[1], 64)
			ln, _ := strconv.ParseFloat(m[2], 64)
			if la >= -90 && la <= 90 && ln >= -180 && ln <= 180 {
				return la, ln, true
			}
		}
	}
	return 0, 0, false
}

// mapLinkNext asks one address where it redirects to, without following it. Replaced in tests.
var mapLinkNext = func(ctx context.Context, u string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124 Safari/537.36")
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	return res.Header.Get("Location"), nil
}

// adminResolveMapLink turns a pasted Google Maps link (short or long) into a position.
func (s *Server) adminResolveMapLink(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		URL string `json:"url"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	cur := strings.TrimSpace(in.URL)
	notFound := fail(http.StatusUnprocessableEntity, httpx.CodeInvalidRequest, "Could not find a position in that link. Tap the map instead.")
	if len(cur) > 2000 {
		return notFound
	}
	for hop := 0; hop < 6; hop++ {
		if lat, lng, ok := coordsIn(cur); ok {
			respond(w, http.StatusOK, map[string]float64{"lat": lat, "lng": lng})
			return nil
		}
		u, err := url.Parse(cur)
		if err != nil || u.Scheme != "https" || !mapLinkHosts[strings.ToLower(u.Hostname())] {
			return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "Paste a Google Maps link (it starts with https://maps.app.goo.gl or https://www.google.com/maps).")
		}
		next, err := mapLinkNext(r.Context(), cur)
		if err != nil || next == "" {
			return notFound
		}
		if ref, err := u.Parse(next); err == nil {
			next = ref.String()
		}
		cur = next
	}
	return notFound
}
