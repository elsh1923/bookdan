package api

import (
	"embed"
	"net/http"

	"github.com/go-chi/chi/v5"
)

//go:embed adminui/*
var adminFiles embed.FS

// AdminPage serves the admin web page at /admin. It is plain HTML, CSS and JavaScript (no build step) and talks to /v1/admin/*.
// The strict Content-Security-Policy means the page can load nothing but its own files and the hotel photos from this server.
func (s *Server) AdminPage(r chi.Router) {
	serve := func(name, contentType string) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) {
			if !s.adminEnabled() {
				http.NotFound(w, nil)
				return
			}
			body, err := adminFiles.ReadFile("adminui/" + name)
			if err != nil {
				http.NotFound(w, nil)
				return
			}
			h := w.Header()
			h.Set("Content-Type", contentType)
			h.Set("Cache-Control", "no-store")
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "no-referrer")
			h.Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data: blob:; connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
			_, _ = w.Write(body)
		}
	}
	r.Get("/admin", serve("index.html", "text/html; charset=utf-8"))
	r.Get("/admin/", serve("index.html", "text/html; charset=utf-8"))
	r.Get("/admin/app.js", serve("app.js", "text/javascript; charset=utf-8"))
	r.Get("/admin/style.css", serve("style.css", "text/css; charset=utf-8"))
}
