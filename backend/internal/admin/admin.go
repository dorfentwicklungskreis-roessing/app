// Package admin liefert das Web-Admin-Interface der Dorf-App aus:
// eine einzelne HTML-Seite mit OIDC-PKCE-Login (Rössing-ID) und Karte,
// die direkt gegen die REST-API arbeitet.
package admin

import (
	"embed"
	_ "embed"
	"net/http"
	"strings"
)

//go:embed index.html
var indexHTML string

//go:embed index_root.html
var rootHTML string

// MapLibre GL liegt lokal im Repo statt auf einem CDN: keine Drittanbieter-
// Requests aus dem Browser der Nutzer, keine Abhängigkeit von unpkg-Verfügbarkeit
// und ein reproduzierbarer Browser-E2E in der CI.
//
//go:embed vendor/maplibre-gl.js vendor/maplibre-gl.css
var vendorFS embed.FS

// Register hängt das Web-Admin an den Mux. Issuer und Client-ID werden in
// die Seite injiziert, damit sie pro Umgebung konfigurierbar bleiben.
func Register(mux *http.ServeMux, issuer, clientID string) {
	page := strings.NewReplacer(
		"__OIDC_ISSUER__", issuer,
		"__OIDC_CLIENT_ID__", clientID,
	).Replace(indexHTML)

	// Startseite: kurze Erklärung + Links zu App-Download und Verwaltung.
	// Muss hier liegen, weil "/" als Catch-all sonst jeden unbekannten Pfad fängt.
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(rootHTML))
	})
	mux.HandleFunc("GET /admin", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/admin/", http.StatusMovedPermanently)
	})
	// Konkreter als "GET /admin/" — Go's Mux wählt das speziellere Muster.
	mux.Handle("GET /admin/vendor/", cacheForever(http.StripPrefix("/admin/", http.FileServerFS(vendorFS))))
	mux.HandleFunc("GET /admin/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(page))
	})
}

// cacheForever markiert die versionierten Vendor-Assets als lange cachebar.
func cacheForever(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=86400")
		next.ServeHTTP(w, r)
	})
}
