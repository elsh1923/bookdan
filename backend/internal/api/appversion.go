package api

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/bookdan/backend/internal/httpx"
)

// Minimum app version. When the server changes in a way an old build cannot cope with, the owner sets MIN_APP_VERSION_GUEST (or _PARTNER) to the first build
// that can. From then on:
//   - GET /v1/app-config tells an app the minimum and where to download the new build, so it can show an update screen before the person gets an error;
//   - any request from a build that sends its version (X-App-Version) and is older than the minimum is refused with 426 UPGRADE_REQUIRED.
//
// A build that sends no version (the ones installed before this existed) is never refused: the server cannot know how old it is, and locking everyone out by
// surprise is worse than a missing screen. Most changes need no minimum at all, because the apps' screens are updated over the air.

// version is major.minor.patch.
type version [3]int

// parseVersion reads "1.4.2", "1.4", "v1.4.2" or "1.4.2-beta.1". Anything else is not a version.
func parseVersion(s string) (version, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	if i := strings.IndexAny(s, "-+ "); i >= 0 {
		s = s[:i]
	}
	if s == "" {
		return version{}, false
	}
	parts := strings.Split(s, ".")
	if len(parts) > 3 {
		return version{}, false
	}
	var v version
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || n > 1_000_000 {
			return version{}, false
		}
		v[i] = n
	}
	return v, true
}

func (a version) less(b version) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

// minVersionFor is the minimum version and the download link for an app ("guest" or "partner"). An invalid setting means no minimum.
func (s *Server) minVersionFor(app string) (min string, link string) {
	switch app {
	case "guest":
		min, link = s.cfg.MinAppVersionGuest, s.cfg.GuestAppURL
	case "partner":
		min, link = s.cfg.MinAppVersionPartner, s.cfg.PartnerAppURL
	}
	if min != "" {
		if _, ok := parseVersion(min); !ok {
			slog.Warn("the minimum app version is not a version number like 1.2.0, so no minimum is enforced", "app", app, "value", min)
			min = ""
		}
	}
	return min, link
}

// tooOld reports whether a build with this version of this app must update.
func (s *Server) tooOld(app, have string) bool {
	min, _ := s.minVersionFor(app)
	if min == "" || have == "" {
		return false
	}
	m, _ := parseVersion(min)
	h, ok := parseVersion(have)
	return ok && h.less(m)
}

// versionGate refuses requests from builds older than the minimum.
func (s *Server) versionGate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.tooOld(r.Header.Get("X-App"), r.Header.Get("X-App-Version")) {
			httpx.Error(w, r, http.StatusUpgradeRequired, codeUpgradeRequired, "This version of BookDan is too old. Please install the latest version to continue.", nil)
			return
		}
		next.ServeHTTP(w, r)
	})
}

const codeUpgradeRequired = "UPGRADE_REQUIRED"

// appConfig is read by the apps when they open. ?app=guest|partner (or the X-App header) picks the app; the answer is the same for everyone.
func (s *Server) appConfig(w http.ResponseWriter, r *http.Request) error {
	app := r.URL.Query().Get("app")
	if app == "" {
		app = r.Header.Get("X-App")
	}
	if app != "guest" && app != "partner" {
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "app must be guest or partner")
	}
	min, link := s.minVersionFor(app)
	w.Header().Set("Cache-Control", "public, max-age=60")
	respond(w, http.StatusOK, struct {
		httpx.Meta
		App         string `json:"app"`
		MinVersion  string `json:"min_version"`
		DownloadURL string `json:"download_url"`
	}{httpx.NowMeta(), app, min, link})
	return nil
}
