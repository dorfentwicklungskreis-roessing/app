// Package admin liefert das Web-Admin-Interface der Dorf-App aus:
// eine einzelne HTML-Seite mit OIDC-PKCE-Login (Rössing-ID) und Karte,
// die direkt gegen die REST-API arbeitet.
package admin

import (
	_ "embed"
	"net/http"
	"strings"
)

//go:embed index.html
var indexHTML string

// Register hängt das Web-Admin an den Mux. Issuer und Client-ID werden in
// die Seite injiziert, damit sie pro Umgebung konfigurierbar bleiben.
func Register(mux *http.ServeMux, issuer, clientID string) {
	page := strings.NewReplacer(
		"__OIDC_ISSUER__", issuer,
		"__OIDC_CLIENT_ID__", clientID,
	).Replace(indexHTML)

	mux.HandleFunc("GET /admin", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/admin/", http.StatusMovedPermanently)
	})
	mux.HandleFunc("GET /admin/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(page))
	})
}
