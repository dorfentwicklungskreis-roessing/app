// Package httpx bündelt die HTTP-Härtung des Dorf-App-Backends:
// Rate-Limiting, Sicherheits-Kopfzeilen, Herkunftsprüfung, Panic-Recovery und
// die Begrenzung der Anfragegröße. Alles hier ist bewusst frei von
// Fachlogik — die Middlewares werden in cmd/server um den Router gelegt.
package httpx

import (
	"crypto/sha256"
	"encoding/base64"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
)

// Middleware ist die übliche Handler-Verpackung.
type Middleware func(http.Handler) http.Handler

// Chain legt die Middlewares von außen nach innen um h: Chain(h, a, b) ruft
// erst a, dann b, dann h.
func Chain(h http.Handler, mw ...Middleware) http.Handler {
	for i := len(mw) - 1; i >= 0; i-- {
		if mw[i] == nil {
			continue
		}
		h = mw[i](h)
	}
	return h
}

// ClientIP liefert die IP des Aufrufers. Hinter dem Ingress steht die echte
// Adresse in X-Forwarded-For; genutzt wird der erste Eintrag.
func ClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if erste, _, _ := strings.Cut(xff, ","); strings.TrimSpace(erste) != "" {
			return strings.TrimSpace(erste)
		}
	}
	if ip := strings.TrimSpace(r.Header.Get("X-Real-Ip")); ip != "" {
		return ip
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// kurzHash verkürzt ein Geheimnis (Token, Cookie) zu einem stabilen, aber
// nicht rückrechenbaren Schlüssel. So landen keine Tokens in Speicherstrukturen
// oder Logs.
func kurzHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return base64.RawURLEncoding.EncodeToString(sum[:9])
}

// envOr liest eine Umgebungsvariable mit Vorgabewert.
func envOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

// envInt liest eine Zahl aus der Umgebung; unbrauchbare Werte fallen auf die
// Vorgabe zurück.
func envInt(key string, def int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

// aus erkennt die üblichen Schreibweisen für „abgeschaltet“.
func aus(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "off", "0", "false", "aus", "nein":
		return true
	}
	return false
}
