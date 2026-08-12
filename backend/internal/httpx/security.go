package httpx

import (
	"log/slog"
	"net/http"
	"net/url"
	"runtime/debug"
	"strings"
)

// KachelHost ist der Kartenserver, von dem die Verwaltung Stil, Kacheln,
// Schriften und Sprites lädt (siehe internal/admin/static/karte.js).
// Wandert die Karte zu einem anderen Anbieter, muss dieser Wert mit —
// TestCSPPasstZumKartenSkript wacht darüber.
const KachelHost = "https://tiles.openfreemap.org"

// SecurityConfig steuert die Kopfzeilen.
type SecurityConfig struct {
	// ZusaetzlicheVerbindungen erlaubt weitere Quellen in connect-src/img-src
	// (z.B. ein anderer Kartenserver). Normalerweise leer.
	ZusaetzlicheVerbindungen []string
	// Anmeldedienst ist der Issuer der Rössing-ID. Sein Ursprung landet in
	// form-action, weil das Abmelde-Formular dorthin weitergeleitet wird —
	// Browser prüfen form-action auch gegen das Ziel der Weiterleitung.
	Anmeldedienst string
}

// ContentSecurityPolicy baut die CSP der ausgelieferten HTML-Seiten.
//
// Grundhaltung: alles aus dem eigenen Ursprung, keine Inline-Skripte, keine
// Einbettung in fremde Seiten. Ausnahmen gibt es nur für die Karte:
//   - MapLibre startet seinen Worker aus einem Blob (worker-src/child-src),
//   - Stil, Vektorkacheln und Schriften kommen per fetch vom Kachel-Server
//     (connect-src), Sprites und Rasterkacheln als Bild (img-src),
//   - gezeichnet wird auf ein Canvas, das keine CSP-Rechte braucht.
//
// Auch Inline-Styles bleiben verboten: Die Templates tragen keine
// style-Attribute, und MapLibre setzt seine Maße über das CSSOM
// (element.style.width = …), was die CSP nicht betrifft.
func ContentSecurityPolicy(cfg SecurityConfig) string {
	fremd := strings.Join(append([]string{KachelHost}, cfg.ZusaetzlicheVerbindungen...), " ")
	direktiven := []string{
		"default-src 'self'",
		"script-src 'self'",
		"style-src 'self'",
		"img-src 'self' data: blob: " + fremd,
		"connect-src 'self' " + fremd,
		"font-src 'self' data:",
		"worker-src blob:",
		"child-src blob:",
		"frame-src 'none'",
		"object-src 'none'",
		"base-uri 'none'",
		// normOrigin kürzt den Issuer auf Schema und Host — ein Pfad hat in
		// der CSP nichts verloren.
		"form-action " + strings.TrimSpace("'self' "+normOrigin(cfg.Anmeldedienst)),
		"frame-ancestors 'none'",
	}
	return strings.Join(direktiven, "; ")
}

// SecurityHeaders setzt die Sicherheits-Kopfzeilen auf jede Antwort.
func SecurityHeaders(cfg SecurityConfig) Middleware {
	csp := ContentSecurityPolicy(cfg)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("Content-Security-Policy", csp)
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
			h.Set("Cross-Origin-Opener-Policy", "same-origin")
			h.Set("Permissions-Policy", "geolocation=(), camera=(), microphone=(), payment=(), usb=()")
			if istHTTPS(r) {
				h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}
			next.ServeHTTP(w, r)
		})
	}
}

func istHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

// --- Herkunftsprüfung --------------------------------------------------------

// SameOriginConfig konfiguriert die Herkunftsprüfung.
type SameOriginConfig struct {
	// Origin ist der eigene Ursprung, z.B. https://app.xn--rssing-wxa.de.
	Origin string
	// Prefixes sind die Pfade, die geprüft werden. Sinnvoll ist genau das,
	// was per Cookie authentifiziert wird (/admin/).
	Prefixes []string
}

// SameOrigin weist schreibende Zugriffe mit fremdem Origin-Kopf ab. Das ist
// der zweite Riegel gegen Cross-Site-Request-Forgery neben SameSite=Lax:
// Browser schicken bei Formular-POSTs immer einen Origin-Kopf. Fehlt er
// (curl, App, Tests), wird nicht geblockt — dort gibt es auch keine Cookies,
// die ein fremder Ursprung mitschicken könnte.
//
// Token-authentifizierte Endpunkte (/api/v1, /mcp, /oauth/register) sind
// bewusst ausgenommen: sie kennen keine Cookies und claude.ai ruft sie mit
// eigenem Origin auf.
func SameOrigin(cfg SameOriginConfig) Middleware {
	erlaubt := map[string]bool{}
	if o := normOrigin(cfg.Origin); o != "" {
		erlaubt[o] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !unsicher(r.Method) || !passtZuPrefix(r.URL.Path, cfg.Prefixes) {
				next.ServeHTTP(w, r)
				return
			}
			origin := normOrigin(r.Header.Get("Origin"))
			if origin == "" || erlaubt[origin] || origin == eigenerUrsprung(r) {
				next.ServeHTTP(w, r)
				return
			}
			slog.Warn("Anfrage mit fremder Herkunft abgewiesen", "pfad", r.URL.Path, "origin", origin)
			http.Error(w, "Anfrage von fremder Seite abgewiesen", http.StatusForbidden)
		})
	}
}

func unsicher(methode string) bool {
	switch methode {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	return false
}

func passtZuPrefix(pfad string, prefixe []string) bool {
	for _, p := range prefixe {
		if strings.HasPrefix(pfad, p) {
			return true
		}
	}
	return false
}

// normOrigin bringt einen Ursprung auf die Vergleichsform (Schema + Host).
func normOrigin(roh string) string {
	roh = strings.TrimSpace(roh)
	if roh == "" || roh == "null" {
		return ""
	}
	u, err := url.Parse(roh)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return strings.ToLower(u.Scheme + "://" + u.Host)
}

// eigenerUrsprung ist der Ursprung, unter dem die Anfrage hereinkam.
func eigenerUrsprung(r *http.Request) string {
	schema := "http"
	if istHTTPS(r) {
		schema = "https"
	}
	return strings.ToLower(schema + "://" + r.Host)
}

// --- Panic-Recovery ----------------------------------------------------------

// Recover fängt Panics einzelner Handler ab: Der Server läuft weiter, der
// Aufrufer bekommt eine nichtssagende 500. Details landen nur im Log.
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gefangen := &statusSchreiber{ResponseWriter: w}
		defer func() {
			p := recover()
			if p == nil {
				return
			}
			if p == http.ErrAbortHandler {
				panic(p)
			}
			slog.Error("Panic im Handler abgefangen",
				"methode", r.Method, "pfad", r.URL.Path, "panik", p, "stack", string(debug.Stack()))
			if !gefangen.geschrieben {
				http.Error(gefangen, "interner Fehler", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(gefangen, r)
	})
}

// statusSchreiber merkt sich, ob schon eine Antwort begonnen wurde.
type statusSchreiber struct {
	http.ResponseWriter
	geschrieben bool
}

func (s *statusSchreiber) WriteHeader(code int) {
	s.geschrieben = true
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusSchreiber) Write(b []byte) (int, error) {
	s.geschrieben = true
	return s.ResponseWriter.Write(b)
}

// --- Größenbegrenzung --------------------------------------------------------

// LimitBody begrenzt die Größe jedes Anfrage-Körpers. Ohne Begrenzung könnte
// ein einziger Aufruf den Speicher des kleinen Pods aufbrauchen.
func LimitBody(max int64) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, max)
			}
			next.ServeHTTP(w, r)
		})
	}
}
