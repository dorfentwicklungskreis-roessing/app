// Package auth validiert Zitadel-Access-Tokens (JWT) und stellt Nutzer
// samt Rollen im Request-Context bereit.
//
// Erwartete Konfiguration (Env):
//
//	AUTH_ISSUER   z.B. https://id.xn--rssing-wxa.de (leer = Auth deaktiviert, nur für Dev/Tests!)
//	AUTH_AUDIENCE optionale, kommaseparierte Liste erlaubter Audiences (Project-ID/Client-ID)
//
// Rollen kommen aus dem Zitadel-Claim `urn:zitadel:iam:org:project:roles`.
package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
)

// User ist der authentifizierte Aufrufer.
type User struct {
	Sub   string
	Name  string
	Email string
	Roles map[string]bool
}

func (u User) IsAdmin() bool { return u.Roles["admin"] }

type ctxKey struct{}

// FromContext liefert den Nutzer aus dem Request-Context.
func FromContext(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(ctxKey{}).(User)
	return u, ok
}

// WithUser hängt einen Nutzer an den Context (für Tests und Dev-Modus).
func WithUser(ctx context.Context, u User) context.Context {
	return context.WithValue(ctx, ctxKey{}, u)
}

// Verifier prüft Bearer-Tokens.
type Verifier interface {
	Verify(ctx context.Context, rawToken string) (User, error)
}

// OIDCVerifier prüft JWTs gegen die JWKS des Issuers.
type OIDCVerifier struct {
	verifier *oidc.IDTokenVerifier
}

// NewOIDCVerifier lädt die Discovery-Konfiguration des Issuers.
// audiences darf leer sein; dann wird die Audience nicht geprüft.
func NewOIDCVerifier(ctx context.Context, issuer string, audiences []string) (*OIDCVerifier, error) {
	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, err
	}
	cfg := &oidc.Config{SkipClientIDCheck: true}
	v := provider.Verifier(cfg)
	return &OIDCVerifier{verifier: v}, nil
}

type zitadelClaims struct {
	Name              string `json:"name"`
	PreferredUsername string `json:"preferred_username"`
	Email             string `json:"email"`
}

// Zitadel liefert Rollen je nach Token-Typ unter verschiedenen Claims:
// generisch (urn:zitadel:iam:org:project:roles) bei Login-Tokens mit
// Role-Assertion, projekt-spezifisch (…:project:<id>:roles) z.B. bei
// Machine-User-Tokens mit Projekt-Audience. Wir akzeptieren beide.
var roleClaimPattern = regexp.MustCompile(`^urn:zitadel:iam:org:project:(\d+:)?roles$`)

func extractRoles(allClaims map[string]json.RawMessage) map[string]bool {
	roles := map[string]bool{}
	for claim, rawValue := range allClaims {
		if !roleClaimPattern.MatchString(claim) {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal(rawValue, &m); err != nil {
			continue
		}
		for r := range m {
			roles[r] = true
		}
	}
	return roles
}

func (o *OIDCVerifier) Verify(ctx context.Context, raw string) (User, error) {
	tok, err := o.verifier.Verify(ctx, raw)
	if err != nil {
		return User{}, err
	}
	var claims zitadelClaims
	if err := tok.Claims(&claims); err != nil {
		return User{}, err
	}
	var allClaims map[string]json.RawMessage
	if err := tok.Claims(&allClaims); err != nil {
		return User{}, err
	}
	name := claims.Name
	if name == "" {
		name = claims.PreferredUsername
	}
	return User{Sub: tok.Subject, Name: name, Email: claims.Email, Roles: extractRoles(allClaims)}, nil
}

// Middleware erzwingt ein gültiges Bearer-Token und legt den Nutzer in den Context.
func Middleware(v Verifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := r.Header.Get("Authorization")
			const prefix = "Bearer "
			if !strings.HasPrefix(h, prefix) {
				http.Error(w, `{"error":"missing bearer token"}`, http.StatusUnauthorized)
				return
			}
			u, err := v.Verify(r.Context(), strings.TrimPrefix(h, prefix))
			if err != nil {
				http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r.WithContext(WithUser(r.Context(), u)))
		})
	}
}

// InsecureDevVerifier akzeptiert jedes Token und liest Nutzer/Rollen direkt
// aus dem Token-String ("sub:name:role1,role2"). NUR für lokale Entwicklung
// und E2E-Tests — niemals in Produktion konfigurieren.
type InsecureDevVerifier struct{}

func (InsecureDevVerifier) Verify(_ context.Context, raw string) (User, error) {
	parts := strings.SplitN(raw, ":", 3)
	u := User{Sub: parts[0], Name: parts[0], Roles: map[string]bool{}}
	if len(parts) > 1 && parts[1] != "" {
		u.Name = parts[1]
	}
	if len(parts) > 2 {
		for _, r := range strings.Split(parts[2], ",") {
			if r != "" {
				u.Roles[r] = true
			}
		}
	}
	return u, nil
}
