package httpx

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

// dummy ist ein Handler, der immer 200 liefert.
func dummy() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func anfrage(t *testing.T, h http.Handler, methode, pfad, ip string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(methode, pfad, nil)
	r.RemoteAddr = ip + ":12345"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestRateLimitSchuetztSchreibendeZugriffe(t *testing.T) {
	jetzt := time.Now()
	rl := NewRateLimiter(RateLimitConfig{Burst: 3, PerMinute: 60, Now: func() time.Time { return jetzt }})
	h := rl.Middleware(dummy())

	// Lesende Zugriffe zählen nicht mit — das Dorf darf beliebig oft schauen.
	for i := 0; i < 20; i++ {
		if w := anfrage(t, h, http.MethodGet, "/admin/dorfpflege/", "10.0.0.1"); w.Code != http.StatusOK {
			t.Fatalf("GET %d wurde begrenzt: %d", i, w.Code)
		}
	}

	// Schreibende Zugriffe: Burst erschöpft sich.
	for i := 0; i < 3; i++ {
		if w := anfrage(t, h, http.MethodPost, "/admin/dorfpflege/orte/neu", "10.0.0.1"); w.Code != http.StatusOK {
			t.Fatalf("POST %d wurde zu früh begrenzt: %d", i, w.Code)
		}
	}
	w := anfrage(t, h, http.MethodPost, "/admin/dorfpflege/orte/neu", "10.0.0.1")
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("vierter POST hätte 429 sein müssen, war %d", w.Code)
	}
	ra := w.Header().Get("Retry-After")
	if ra == "" {
		t.Fatal("Retry-After fehlt")
	}
	if n, err := strconv.Atoi(ra); err != nil || n < 1 {
		t.Fatalf("Retry-After unbrauchbar: %q", ra)
	}

	// Eine andere IP hat ihren eigenen Eimer.
	if w := anfrage(t, h, http.MethodPost, "/admin/dorfpflege/orte/neu", "10.0.0.2"); w.Code != http.StatusOK {
		t.Fatalf("fremde IP wurde mitbestraft: %d", w.Code)
	}

	// Nach genügend Zeit füllt sich der Eimer wieder.
	jetzt = jetzt.Add(2 * time.Second)
	if w := anfrage(t, h, http.MethodPost, "/admin/dorfpflege/orte/neu", "10.0.0.1"); w.Code != http.StatusOK {
		t.Fatalf("Eimer füllt sich nicht nach: %d", w.Code)
	}
}

func TestRateLimitBegrenztAuchLesendesMCP(t *testing.T) {
	jetzt := time.Now()
	rl := NewRateLimiter(RateLimitConfig{Burst: 2, PerMinute: 60, Now: func() time.Time { return jetzt }})
	h := rl.Middleware(dummy())

	for i := 0; i < 2; i++ {
		if w := anfrage(t, h, http.MethodPost, "/mcp", "10.0.0.3"); w.Code != http.StatusOK {
			t.Fatalf("MCP %d zu früh begrenzt: %d", i, w.Code)
		}
	}
	if w := anfrage(t, h, http.MethodPost, "/mcp", "10.0.0.3"); w.Code != http.StatusTooManyRequests {
		t.Fatalf("MCP wurde nicht begrenzt: %d", w.Code)
	}
}

// Verschiedene Bearer-Token sind verschiedene Nutzer — auch hinter demselben
// NAT-Anschluss (Dorf-Internet) darf einer den anderen nicht aussperren.
func TestRateLimitTrenntNachToken(t *testing.T) {
	jetzt := time.Now()
	rl := NewRateLimiter(RateLimitConfig{Burst: 1, PerMinute: 60, Now: func() time.Time { return jetzt }})
	h := rl.Middleware(dummy())

	mit := func(token string) int {
		r := httptest.NewRequest(http.MethodPost, "/api/v1/places", nil)
		r.RemoteAddr = "10.0.0.9:1234"
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}
	if code := mit("token-a"); code != http.StatusOK {
		t.Fatalf("erster Zugriff: %d", code)
	}
	if code := mit("token-a"); code != http.StatusTooManyRequests {
		t.Fatalf("zweiter Zugriff desselben Tokens: %d", code)
	}
	if code := mit("token-b"); code != http.StatusOK {
		t.Fatalf("anderes Token wurde mitbestraft: %d", code)
	}
}

func TestRateLimitAbschaltbar(t *testing.T) {
	rl := NewRateLimiter(RateLimitConfig{Enabled: falseZeiger()})
	if rl != nil {
		t.Fatal("abgeschaltet muss nil liefern (kein Overhead)")
	}
	// nil-Limiter reicht Anfragen unverändert durch.
	h := rl.Middleware(dummy())
	for i := 0; i < 50; i++ {
		if w := anfrage(t, h, http.MethodPost, "/mcp", "10.0.0.4"); w.Code != http.StatusOK {
			t.Fatalf("abgeschalteter Limiter begrenzt trotzdem: %d", w.Code)
		}
	}
}

func falseZeiger() *bool { b := false; return &b }

// Alte Eimer werden aufgeräumt, damit der Speicher nicht unbegrenzt wächst.
func TestRateLimitRaeumtAuf(t *testing.T) {
	jetzt := time.Now()
	rl := NewRateLimiter(RateLimitConfig{Burst: 2, PerMinute: 60, Now: func() time.Time { return jetzt }})
	h := rl.Middleware(dummy())
	for i := 0; i < 100; i++ {
		anfrage(t, h, http.MethodPost, "/mcp", "10.1."+strconv.Itoa(i/250)+"."+strconv.Itoa(i%250))
	}
	if n := rl.Size(); n != 100 {
		t.Fatalf("unerwartete Anzahl Eimer: %d", n)
	}
	jetzt = jetzt.Add(2 * time.Hour)
	anfrage(t, h, http.MethodPost, "/mcp", "10.9.9.9")
	if n := rl.Size(); n > 2 {
		t.Fatalf("alte Eimer wurden nicht aufgeräumt: %d", n)
	}
}

func TestRateLimitConfigAusUmgebung(t *testing.T) {
	t.Setenv("RATE_LIMIT", "off")
	if NewRateLimiter(RateLimitFromEnv()) != nil {
		t.Fatal("RATE_LIMIT=off muss abschalten")
	}
	t.Setenv("RATE_LIMIT", "")
	t.Setenv("RATE_LIMIT_BURST", "7")
	t.Setenv("RATE_LIMIT_PER_MINUTE", "42")
	cfg := RateLimitFromEnv()
	if cfg.Burst != 7 || cfg.PerMinute != 42 {
		t.Fatalf("Env nicht übernommen: %+v", cfg)
	}
}

// Der Schlüssel enthält das Bearer-Token — also etwas, das ein Angreifer bei
// jeder Anfrage neu erfinden kann. Ohne Deckel wüchse die Eimer-Tabelle
// unbegrenzt und der Speicher des Pods wäre das eigentliche Ziel.
func TestRateLimitDeckeltDieAnzahlEimer(t *testing.T) {
	jetzt := time.Now()
	rl := NewRateLimiter(RateLimitConfig{Burst: 2, PerMinute: 60, Now: func() time.Time { return jetzt }})
	h := rl.Middleware(dummy())

	for i := 0; i < MaxEimer*2; i++ {
		r := httptest.NewRequest(http.MethodPost, "/mcp", nil)
		r.RemoteAddr = "10.0.0.1:1234"
		r.Header.Set("Authorization", "Bearer erfunden-"+strconv.Itoa(i))
		h.ServeHTTP(httptest.NewRecorder(), r)
	}
	if n := rl.Size(); n > MaxEimer {
		t.Fatalf("Eimer-Tabelle wuchs auf %d (Deckel %d)", n, MaxEimer)
	}
}
