package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bookdan/backend/internal/config"
)

func TestLegalPages(t *testing.T) {
	h := New(config.Config{SupportEmail: "help@example.com"}, nil, nil)
	for path, want := range map[string]string{"/privacy": "Privacy Policy", "/terms": "Terms of Use", "/delete-account": "Delete your BookDan account"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), want) || !strings.Contains(rec.Body.String(), "help@example.com") {
			t.Errorf("%s: status %d, missing %q or contact", path, rec.Code, want)
		}
	}
}
