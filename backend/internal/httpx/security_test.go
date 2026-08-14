package httpx

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
)

func kopfzeilen(t *testing.T, h http.Handler, pfad string) http.Header {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, pfad, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w.Header()
}

func TestSicherheitsKopfzeilen(t *testing.T) {
	h := SecurityHeaders(SecurityConfig{})(dummy())
	kopf := kopfzeilen(t, h, "/admin/mithelfen/")

	pflicht := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "strict-origin-when-cross-origin",
	}
	for name, wert := range pflicht {
		if got := kopf.Get(name); got != wert {
			t.Errorf("%s = %q, erwartet %q", name, got, wert)
		}
	}
	if kopf.Get("Content-Security-Policy") == "" {
		t.Fatal("Content-Security-Policy fehlt")
	}
}

// Die CSP muss streng sein: kein 'unsafe-inline'/'unsafe-eval' für Skripte,
// keine fremden Skriptquellen, kein Einbetten in fremde Seiten.
func TestCSPIstStreng(t *testing.T) {
	csp := ContentSecurityPolicy(SecurityConfig{})
	teile := cspTeile(csp)

	for _, wollen := range []struct{ direktive, wert string }{
		{"default-src", "'self'"},
		{"script-src", "'self'"},
		{"base-uri", "'none'"},
		{"object-src", "'none'"},
		{"frame-ancestors", "'none'"},
		{"form-action", "'self'"},
	} {
		if teile[wollen.direktive] != wollen.wert {
			t.Errorf("%s = %q, erwartet %q (CSP: %s)", wollen.direktive, teile[wollen.direktive], wollen.wert, csp)
		}
	}
	for _, verboten := range []string{"'unsafe-inline'", "'unsafe-eval'", "*"} {
		if strings.Contains(teile["script-src"], verboten) {
			t.Errorf("script-src enthält %s: %s", verboten, csp)
		}
	}
}

// Die Karte muss funktionieren: MapLibre baut seinen Worker aus einem Blob und
// lädt Stil, Kacheln, Schriften und Sprites vom Kachel-Server.
func TestCSPErlaubtKarte(t *testing.T) {
	csp := ContentSecurityPolicy(SecurityConfig{})
	teile := cspTeile(csp)

	if !strings.Contains(teile["worker-src"], "blob:") {
		t.Errorf("worker-src ohne blob: — MapLibre kann keinen Worker starten: %s", csp)
	}
	if !strings.Contains(teile["child-src"], "blob:") {
		t.Errorf("child-src ohne blob: — ältere Browser blockieren den Worker: %s", csp)
	}
	for _, direktive := range []string{"img-src", "connect-src"} {
		if !strings.Contains(teile[direktive], KachelHost) {
			t.Errorf("%s ohne %s: %s", direktive, KachelHost, csp)
		}
	}
	if !strings.Contains(teile["img-src"], "data:") || !strings.Contains(teile["img-src"], "blob:") {
		t.Errorf("img-src braucht data: und blob: für MapLibre: %s", csp)
	}
}

// Regressionsschutz: Wechselt der Kachel-Anbieter in karte.js, muss die CSP
// mitwandern — sonst blockiert der Browser die Karte im Betrieb.
func TestCSPPasstZumKartenSkript(t *testing.T) {
	roh, err := os.ReadFile("../admin/static/karte.js")
	if err != nil {
		t.Fatalf("karte.js nicht lesbar: %v", err)
	}
	hosts := regexp.MustCompile(`https://[a-zA-Z0-9.-]+`).FindAllString(string(roh), -1)
	csp := ContentSecurityPolicy(SecurityConfig{})
	for _, host := range hosts {
		if !strings.Contains(csp, host) {
			t.Errorf("karte.js lädt von %s, die CSP erlaubt das nicht: %s", host, csp)
		}
	}
}

// HSTS nur auf verschlüsselten Verbindungen — der lokale E2E läuft über http.
func TestHSTSNurBeiHTTPS(t *testing.T) {
	h := SecurityHeaders(SecurityConfig{})(dummy())
	if kopfzeilen(t, h, "/").Get("Strict-Transport-Security") != "" {
		t.Error("HSTS über http gesetzt")
	}

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-Forwarded-Proto", "https")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Header().Get("Strict-Transport-Security") == "" {
		t.Error("HSTS fehlt hinter TLS-Ingress")
	}
}

func cspTeile(csp string) map[string]string {
	out := map[string]string{}
	for _, teil := range strings.Split(csp, ";") {
		teil = strings.TrimSpace(teil)
		if teil == "" {
			continue
		}
		name, wert, _ := strings.Cut(teil, " ")
		out[name] = strings.TrimSpace(wert)
	}
	return out
}

// --- Herkunftsprüfung (CSRF) -------------------------------------------------

func TestHerkunftsPruefung(t *testing.T) {
	h := SameOrigin(SameOriginConfig{Origin: "https://app.example", Prefixes: []string{"/admin/"}})(dummy())

	post := func(pfad, origin string) int {
		r := httptest.NewRequest(http.MethodPost, pfad, nil)
		r.Host = "app.example"
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}

	if code := post("/admin/mithelfen/orte/neu", "https://app.example"); code != http.StatusOK {
		t.Fatalf("eigene Herkunft abgelehnt: %d", code)
	}
	if code := post("/admin/mithelfen/orte/neu", ""); code != http.StatusOK {
		t.Fatalf("Anfrage ohne Origin abgelehnt: %d", code)
	}
	if code := post("/admin/mithelfen/orte/neu", "https://boese.example"); code != http.StatusForbidden {
		t.Fatalf("fremde Herkunft nicht abgewehrt: %d", code)
	}
	if code := post("/admin/mithelfen/orte/neu", "https://app.example.boese.example"); code != http.StatusForbidden {
		t.Fatalf("Präfix-Trick nicht abgewehrt: %d", code)
	}
	// Token-authentifizierte Endpunkte bleiben unangetastet: sie tragen keine
	// Cookies und claude.ai schickt dort eine fremde Herkunft mit.
	if code := post("/mcp", "https://claude.ai"); code != http.StatusOK {
		t.Fatalf("/mcp wurde durch die Herkunftsprüfung blockiert: %d", code)
	}
	if code := post("/oauth/register", "https://claude.ai"); code != http.StatusOK {
		t.Fatalf("/oauth/register wurde blockiert: %d", code)
	}
	// Lesende Zugriffe sind nie betroffen.
	r := httptest.NewRequest(http.MethodGet, "/admin/mithelfen/", nil)
	r.Header.Set("Origin", "https://boese.example")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("GET blockiert: %d", w.Code)
	}
}

// --- Panic-Recovery ----------------------------------------------------------

func TestRecoverFaengtPanic(t *testing.T) {
	h := Recover(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("kaputt: geheimes Detail")
	}))
	r := httptest.NewRequest(http.MethodGet, "/admin/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("Status %d statt 500", w.Code)
	}
	if strings.Contains(w.Body.String(), "geheimes Detail") {
		t.Fatalf("Panic-Text nach außen gegeben: %s", w.Body.String())
	}
}

// Nach bereits geschriebenem Header darf Recover nichts mehr anfassen, aber
// auch nicht selbst abstürzen.
func TestRecoverNachTeilantwort(t *testing.T) {
	h := Recover(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Anfang"))
		panic("mittendrin")
	}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("Status %d", w.Code)
	}
}

// --- Größenbegrenzung --------------------------------------------------------

func TestBodyBegrenzung(t *testing.T) {
	h := LimitBody(64)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.ReadAll(r.Body); err != nil {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))

	klein := httptest.NewRequest(http.MethodPost, "/api/v1/places", strings.NewReader(strings.Repeat("a", 10)))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, klein)
	if w.Code != http.StatusOK {
		t.Fatalf("kleiner Body abgelehnt: %d", w.Code)
	}

	gross := httptest.NewRequest(http.MethodPost, "/api/v1/places", strings.NewReader(strings.Repeat("a", 1000)))
	w = httptest.NewRecorder()
	h.ServeHTTP(w, gross)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("großer Body durchgelassen: %d", w.Code)
	}
}

// Der Logout der Verwaltung ist ein Formular-POST, dessen Antwort zur
// Rössing-ID weiterleitet (end_session). Chromium prüft form-action auch
// gegen das Ziel einer Weiterleitung — ohne den Anmeldedienst in der Liste
// bricht das Abmelden ab („Refused to send form data").
func TestCSPErlaubtAbmeldenBeimAnmeldedienst(t *testing.T) {
	csp := ContentSecurityPolicy(SecurityConfig{Anmeldedienst: "https://id.xn--rssing-wxa.de"})
	teile := cspTeile(csp)
	if !strings.Contains(teile["form-action"], "'self'") {
		t.Errorf("form-action ohne 'self': %s", csp)
	}
	if !strings.Contains(teile["form-action"], "https://id.xn--rssing-wxa.de") {
		t.Errorf("form-action ohne Anmeldedienst: %s", csp)
	}
	// Der Anmeldedienst darf keine Skripte liefern dürfen.
	if strings.Contains(teile["script-src"], "id.xn--rssing-wxa.de") {
		t.Errorf("Anmeldedienst in script-src: %s", csp)
	}
	// Ein Pfad im Issuer darf nicht in die Richtlinie durchschlagen.
	mitPfad := cspTeile(ContentSecurityPolicy(SecurityConfig{Anmeldedienst: "https://id.example/oauth/v2/"}))
	if !strings.Contains(mitPfad["form-action"], "https://id.example ") && !strings.HasSuffix(mitPfad["form-action"], "https://id.example") {
		t.Errorf("Issuer wurde nicht auf den Ursprung gekürzt: %s", mitPfad["form-action"])
	}
}
