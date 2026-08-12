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

// registerWellKnown veröffentlicht die OAuth-Metadata:
//
//   - Protected-Resource-Metadata (RFC 9728) — zeigt als Authorization
//     Server auf UNS (s.Resource), nicht direkt auf Zitadel, denn:
//   - AS-Metadata (RFC 8414) wird von uns gespiegelt: Authorize/Token-
//     Endpoints kommen von Zitadel, aber wir ergänzen einen
//     registration_endpoint, weil claude.ai sich per Dynamic Client
//     Registration (RFC 7591) anmeldet und Zitadel das nicht kann.
//   - POST /oauth/register beantwortet die DCR-Anfrage mit der fest in
//     Zitadel angelegten PKCE-Client-ID (Redirect-URIs werden validiert).
func (s *Server) registerWellKnown(mux *http.ServeMux) {
	writeJSON := func(w http.ResponseWriter, status int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(v)
	}

	resourceMeta := map[string]any{
		"resource":                 s.Resource,
		"authorization_servers":    []string{s.Resource},
		"bearer_methods_supported": []string{"header"},
		"scopes_supported":         []string{"openid", "profile", "email"},
		"resource_name":            "Dorfpflege Rössing (MCP)",
	}
	resourceHandler := func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, resourceMeta)
	}
	mux.HandleFunc("GET /.well-known/oauth-protected-resource", resourceHandler)
	// Pfadspezifische Variante (RFC 9728 §3.1) für Clients, die die
	// Ressource /mcp direkt auflösen.
	mux.HandleFunc("GET /.well-known/oauth-protected-resource/mcp", resourceHandler)

	asHandler := func(w http.ResponseWriter, _ *http.Request) {
		upstream, err := s.upstreamConfig()
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "IdP-Discovery fehlgeschlagen"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"issuer":                                s.Resource,
			"authorization_endpoint":                upstream.AuthorizationEndpoint,
			"token_endpoint":                        upstream.TokenEndpoint,
			"jwks_uri":                              upstream.JwksURI,
			"registration_endpoint":                 s.Resource + "/oauth/register",
			"response_types_supported":              []string{"code"},
			"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
			"code_challenge_methods_supported":      []string{"S256"},
			"token_endpoint_auth_methods_supported": []string{"none"},
			"scopes_supported":                      []string{"openid", "profile", "email", "offline_access"},
		})
	}
	mux.HandleFunc("GET /.well-known/oauth-authorization-server", asHandler)
	mux.HandleFunc("GET /.well-known/oauth-authorization-server/{rest...}", asHandler)

	mux.HandleFunc("POST /oauth/register", s.handleRegister)
}

// upstreamDiscovery sind die Endpunkte des echten IdP (Zitadel).
type upstreamDiscovery struct {
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	JwksURI               string `json:"jwks_uri"`
}

func (s *Server) upstreamConfig() (*upstreamDiscovery, error) {
	s.discoveryOnce.Do(func() {
		resp, err := http.Get(s.Issuer + "/.well-known/openid-configuration")
		if err != nil {
			s.discoveryErr = err
			return
		}
		defer resp.Body.Close()
		var d upstreamDiscovery
		if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
			s.discoveryErr = err
			return
		}
		s.discovery = &d
	})
	return s.discovery, s.discoveryErr
}

// allowedRedirects: nur die offiziellen Claude-Callback-URLs.
var allowedRedirects = map[string]bool{
	"https://claude.ai/api/mcp/auth_callback":  true,
	"https://claude.com/api/mcp/auth_callback": true,
}

// handleRegister beantwortet Dynamic Client Registration (RFC 7591).
// Es wird kein neuer Client angelegt — jede gültige Registrierung erhält
// die feste, in Zitadel hinterlegte Public-PKCE-Client-ID.
func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RedirectURIs []string `json:"redirect_uris"`
		ClientName   string   `json:"client_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.RedirectURIs) == 0 {
		writeRegisterError(w, "invalid_client_metadata", "redirect_uris fehlen")
		return
	}
	for _, u := range req.RedirectURIs {
		if !allowedRedirects[u] {
			writeRegisterError(w, "invalid_redirect_uri", "Redirect-URI nicht erlaubt: "+u)
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"client_id":                  s.ClientID,
		"redirect_uris":              req.RedirectURIs,
		"token_endpoint_auth_method": "none",
		"grant_types":                []string{"authorization_code", "refresh_token"},
		"response_types":             []string{"code"},
		"client_name":                req.ClientName,
	})
}

func writeRegisterError(w http.ResponseWriter, code, desc string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code, "error_description": desc})
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
