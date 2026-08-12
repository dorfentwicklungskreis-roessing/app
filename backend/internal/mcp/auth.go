package mcp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/auth"
)

// OAuth nach MCP-Spezifikation: Der Server ist eine OAuth-Protected Resource
// (RFC 9728), Zitadel (die Rössing-ID) ist der Authorization Server. Clients
// wie claude.ai entdecken den Login-Weg über die Metadata-URL im
// WWW-Authenticate-Header, machen Authorization Code + PKCE gegen Zitadel
// und schicken das JWT als Bearer-Token. MCP verlangt die Projektrolle
// `admin` — es gibt bewusst keinen Token-Fallback.

// registerWellKnown veröffentlicht die Protected-Resource-Metadata.
func (s *Server) registerWellKnown(mux *http.ServeMux) {
	meta := map[string]any{
		"resource":                 s.Resource,
		"authorization_servers":    []string{s.Issuer},
		"bearer_methods_supported": []string{"header"},
		"scopes_supported":         []string{"openid", "profile", "email"},
		"resource_name":            "Dorfpflege Rössing (MCP)",
	}
	handler := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		_ = json.NewEncoder(w).Encode(meta)
	}
	mux.HandleFunc("GET /.well-known/oauth-protected-resource", handler)
	// Pfadspezifische Variante (RFC 9728 §3.1) für Clients, die die
	// Ressource /mcp direkt auflösen.
	mux.HandleFunc("GET /.well-known/oauth-protected-resource/mcp", handler)
}

func (s *Server) unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate",
		fmt.Sprintf(`Bearer resource_metadata=%q`, s.Resource+"/.well-known/oauth-protected-resource"))
	http.Error(w, "unauthorized", http.StatusUnauthorized)
}

// authenticate prüft das OAuth-JWT und verlangt die admin-Rolle.
func (s *Server) authenticate(r *http.Request) (auth.User, int) {
	ah := r.Header.Get("Authorization")
	if !strings.HasPrefix(ah, "Bearer ") {
		return auth.User{}, http.StatusUnauthorized
	}
	u, err := s.Verifier.Verify(r.Context(), strings.TrimPrefix(ah, "Bearer "))
	if err != nil {
		return auth.User{}, http.StatusUnauthorized
	}
	if !u.IsAdmin() {
		// Gültig eingeloggt, aber keine Verwaltungsrechte.
		return auth.User{}, http.StatusForbidden
	}
	return u, 0
}

func (s *Server) withAuth(h func(http.ResponseWriter, *http.Request, auth.User)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, status := s.authenticate(r)
		switch status {
		case 0:
			h(w, r, u)
		case http.StatusForbidden:
			http.Error(w, "admin-Rolle erforderlich", http.StatusForbidden)
		default:
			s.unauthorized(w)
		}
	}
}
