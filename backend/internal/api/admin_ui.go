package api

import (
	"embed"
	"net/http"

	"github.com/go-chi/chi/v5"
)

//go:embed adminui/*
var adminFiles embed.FS

//go:embed applyui/*
var applyFiles embed.FS

//go:embed hotelui/*
var hotelFiles embed.FS

// AdminPage serves the admin web page at /admin. It is plain HTML, CSS and JavaScript (no build step) and talks to /v1/admin/*.
// The strict Content-Security-Policy means the page can load nothing but its own files and the hotel photos from this server.
func (s *Server) AdminPage(r chi.Router) {
	serve := func(name, contentType string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if !s.adminEnabled(r.Context()) {
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
			h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
			h.Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data: blob: https://tile.openstreetmap.org; connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
			_, _ = w.Write(body)
		}
	}
	// The public page where a hotel applies to be listed. It loads only its own files and talks only to this server.
	apply := func(name, contentType string) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) {
			body, err := applyFiles.ReadFile("applyui/" + name)
			if err != nil {
				http.NotFound(w, nil)
				return
			}
			h := w.Header()
			h.Set("Content-Type", contentType)
			h.Set("Cache-Control", "no-cache")
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "no-referrer")
			h.Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
			_, _ = w.Write(body)
		}
	}
	// The hotel portal: where a hotel's owner and staff run their hotel (bookings, rooms and prices, availability, photos, team, earnings, reviews, help). It signs
	// in with the same code as the partner app and talks to /v1/partner/*. Anyone can load the page; only registered hotel staff get in.
	portal := func(name, contentType string) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) {
			body, err := hotelFiles.ReadFile("hotelui/" + name)
			if err != nil {
				http.NotFound(w, nil)
				return
			}
			h := w.Header()
			h.Set("Content-Type", contentType)
			h.Set("Cache-Control", "no-cache")
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
			h.Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data: blob: https://tile.openstreetmap.org; connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
			_, _ = w.Write(body)
		}
	}
	r.Get("/hotel", portal("index.html", "text/html; charset=utf-8"))
	r.Get("/hotel/", portal("index.html", "text/html; charset=utf-8"))
	r.Get("/hotel/app.js", portal("app.js", "text/javascript; charset=utf-8"))
	r.Get("/hotel/style.css", portal("style.css", "text/css; charset=utf-8"))
	r.Get("/hotel/leaflet/leaflet.js", portal("leaflet/leaflet.js", "text/javascript; charset=utf-8"))
	r.Get("/hotel/leaflet/leaflet.css", portal("leaflet/leaflet.css", "text/css; charset=utf-8"))
	for _, img := range []string{"marker-icon.png", "marker-icon-2x.png", "marker-shadow.png", "layers.png", "layers-2x.png"} {
		r.Get("/hotel/leaflet/images/"+img, portal("leaflet/images/"+img, "image/png"))
	}
	r.Get("/apply", apply("index.html", "text/html; charset=utf-8"))
	r.Get("/apply/", apply("index.html", "text/html; charset=utf-8"))
	r.Get("/apply/apply.js", apply("apply.js", "text/javascript; charset=utf-8"))
	r.Get("/apply/style.css", apply("style.css", "text/css; charset=utf-8"))
	r.Get("/admin", serve("index.html", "text/html; charset=utf-8"))
	r.Get("/admin/", serve("index.html", "text/html; charset=utf-8"))
	r.Get("/admin/app.js", serve("app.js", "text/javascript; charset=utf-8"))
	r.Get("/admin/leaflet/leaflet.js", serve("leaflet/leaflet.js", "text/javascript; charset=utf-8"))
	r.Get("/admin/leaflet/leaflet.css", serve("leaflet/leaflet.css", "text/css; charset=utf-8"))
	for _, img := range []string{"marker-icon.png", "marker-icon-2x.png", "marker-shadow.png", "layers.png", "layers-2x.png"} {
		r.Get("/admin/leaflet/images/"+img, serve("leaflet/images/"+img, "image/png"))
	}
	r.Get("/admin/style.css", serve("style.css", "text/css; charset=utf-8"))
}
