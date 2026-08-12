package httpx

import (
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Rate-Limiting: ein schlichter Token-Bucket je Aufrufer. Begrenzt werden
// schreibende Zugriffe (POST/PUT/PATCH/DELETE) und alles unter /mcp — also
// genau das, womit sich Schaden anrichten lässt. Lesende Seitenaufrufe bleiben
// frei, damit das Dorf ungestört schauen kann.
//
// Der Schlüssel ist so nutzerbezogen wie möglich: Bearer-Token vor
// Sitzungs-Cookie vor IP. Gespeichert wird davon nur ein Kurz-Hash, nie das
// Geheimnis selbst.

// RateLimitConfig konfiguriert den Limiter.
type RateLimitConfig struct {
	// Burst ist die Eimergröße: so viele Zugriffe am Stück sind möglich.
	Burst int
	// PerMinute ist die Nachfüllrate in Zugriffen pro Minute.
	PerMinute int
	// Enabled schaltet den Limiter ab, wenn es auf false zeigt (Tests).
	Enabled *bool
	// Now ist die Zeitquelle (Tests).
	Now func() time.Time
}

// Vorgaben: großzügig fürs Dorf, wirksam gegen Skripte. 60 Schreibzugriffe am
// Stück, danach zwei pro Sekunde. Ein Mensch kommt da nie hin, ein außer
// Rand und Band geratenes Skript sofort.
const (
	DefaultBurst     = 60
	DefaultPerMinute = 120
	// eimerTTL: wie lange ein unbenutzter Eimer aufbewahrt wird.
	eimerTTL = time.Hour
)

// RateLimitFromEnv liest die Konfiguration aus der Umgebung:
//
//	RATE_LIMIT=off          schaltet die Begrenzung ab (Tests, Notfall)
//	RATE_LIMIT_BURST        Eimergröße (Vorgabe 60)
//	RATE_LIMIT_PER_MINUTE   Nachfüllrate pro Minute (Vorgabe 120)
func RateLimitFromEnv() RateLimitConfig {
	an := !aus(envOr("RATE_LIMIT", "on"))
	return RateLimitConfig{
		Burst:     envInt("RATE_LIMIT_BURST", DefaultBurst),
		PerMinute: envInt("RATE_LIMIT_PER_MINUTE", DefaultPerMinute),
		Enabled:   &an,
	}
}

type eimer struct {
	tokens  float64
	letzter time.Time
}

// RateLimiter begrenzt Zugriffe je Aufrufer.
type RateLimiter struct {
	burst   float64
	proSek  float64
	now     func() time.Time
	mu      sync.Mutex
	eimer   map[string]*eimer
	aufraum time.Time
}

// NewRateLimiter baut den Limiter. Ist er abgeschaltet, kommt nil zurück —
// ein nil-Limiter reicht alles unverändert durch.
func NewRateLimiter(cfg RateLimitConfig) *RateLimiter {
	if cfg.Enabled != nil && !*cfg.Enabled {
		return nil
	}
	if cfg.Burst <= 0 {
		cfg.Burst = DefaultBurst
	}
	if cfg.PerMinute <= 0 {
		cfg.PerMinute = DefaultPerMinute
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &RateLimiter{
		burst:  float64(cfg.Burst),
		proSek: float64(cfg.PerMinute) / 60,
		now:    cfg.Now,
		eimer:  map[string]*eimer{},
	}
}

// Size liefert die Anzahl verwalteter Eimer (für Tests und Metriken).
func (l *RateLimiter) Size() int {
	if l == nil {
		return 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.eimer)
}

// Middleware begrenzt schreibende Zugriffe und /mcp.
func (l *RateLimiter) Middleware(next http.Handler) http.Handler {
	if l == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !begrenzt(r) {
			next.ServeHTTP(w, r)
			return
		}
		ok, warten := l.erlaube(schluessel(r))
		if !ok {
			w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(warten.Seconds()))))
			http.Error(w, "zu viele Anfragen — bitte kurz warten", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// begrenzt entscheidet, ob eine Anfrage überhaupt gegen den Eimer zählt.
func begrenzt(r *http.Request) bool {
	if r.URL.Path == "/mcp" || strings.HasPrefix(r.URL.Path, "/mcp/") {
		return true
	}
	switch r.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	return false
}

// schluessel bestimmt den Aufrufer: Token vor Sitzung vor IP.
func schluessel(r *http.Request) string {
	if ah := r.Header.Get("Authorization"); strings.HasPrefix(ah, "Bearer ") {
		return "t:" + kurzHash(strings.TrimPrefix(ah, "Bearer "))
	}
	if c, err := r.Cookie("dorf_admin_session"); err == nil && c.Value != "" {
		return "s:" + kurzHash(c.Value)
	}
	return "ip:" + ClientIP(r)
}

// erlaube nimmt einen Token aus dem Eimer. Ist er leer, liefert es die
// Wartezeit bis zum nächsten freien Token.
func (l *RateLimiter) erlaube(key string) (bool, time.Duration) {
	jetzt := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	l.aufraeumen(jetzt)

	b, ok := l.eimer[key]
	if !ok {
		b = &eimer{tokens: l.burst, letzter: jetzt}
		l.eimer[key] = b
	}
	b.tokens += jetzt.Sub(b.letzter).Seconds() * l.proSek
	if b.tokens > l.burst {
		b.tokens = l.burst
	}
	b.letzter = jetzt
	if b.tokens < 1 {
		warten := time.Duration((1 - b.tokens) / l.proSek * float64(time.Second))
		if warten < time.Second {
			warten = time.Second
		}
		return false, warten
	}
	b.tokens--
	return true, 0
}

// aufraeumen wirft alte Eimer weg, damit der Speicher nicht wächst.
// Läuft höchstens einmal pro Minute.
func (l *RateLimiter) aufraeumen(jetzt time.Time) {
	if jetzt.Sub(l.aufraum) < time.Minute {
		return
	}
	l.aufraum = jetzt
	for k, b := range l.eimer {
		if jetzt.Sub(b.letzter) > eimerTTL {
			delete(l.eimer, k)
		}
	}
}
