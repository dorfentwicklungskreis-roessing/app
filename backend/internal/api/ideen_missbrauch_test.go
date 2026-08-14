package api

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/httpx"
)

// Der Ideen-Eingang ist der einzige Endpunkt ohne Anmeldung. Diese Reihe
// prüft ihn mit den Mustern, die dort im Betrieb tatsächlich ankommen.

func TestIdeenEingangHaeltMissbrauchsmusterAus(t *testing.T) {
	ts, _, d := ideenServer(t)

	// Alles, was hier durchkommt, darf nichts speichern.
	faelle := map[string]url.Values{
		"leeres Formular":       {},
		"nur Leerzeichen":       {"wunsch": {"      "}},
		"riesiger Wunsch":       {"wunsch": {strings.Repeat("a", 50_000)}},
		"riesiger Name":         {"wunsch": {guterWunsch}, "name": {strings.Repeat("N", 5_000)}},
		"riesige E-Mail":        {"wunsch": {guterWunsch}, "email": {strings.Repeat("e", 5_000) + "@example.org"}},
		"Nullbyte im Namen":     {"wunsch": {guterWunsch}, "name": {"Erna\x00"}},
		"Rücklauf im Wunsch":    {"wunsch": {"Ein Wunsch\x08mit Rücktaste"}},
		"Kopfzeile in E-Mail":   {"wunsch": {guterWunsch}, "email": {"a@b.de\nBcc: opfer@example.org"}},
		"E-Mail mit Leerzeiche": {"wunsch": {guterWunsch}, "email": {"erna musterfrau@example.org"}},
		"E-Mail in spitzen":     {"wunsch": {guterWunsch}, "email": {"<erna@example.org>"}},
		"E-Mail ohne Endung":    {"wunsch": {guterWunsch}, "email": {"erna@example."}},
		"E-Mail Zahlenendung":   {"wunsch": {guterWunsch}, "email": {"erna@example.12"}},
	}
	for name, werte := range faelle {
		resp := sendeIdee(t, ts, "", werte, nil)
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: Status %d, erwartet 400", name, resp.StatusCode)
		}
	}
	if ideen, _ := d.ListIdeen(""); len(ideen) != 0 {
		t.Fatalf("trotz Abweisung gespeichert: %v", ideen)
	}

	// Und das, was ankommen soll, kommt an — samt Markup, das erst beim
	// Anzeigen entschärft wird (siehe internal/admin).
	gut := map[string]url.Values{
		"mit Markup":       {"wunsch": {`Bitte <b>fett</b> & "in Anführung" einbauen.`}},
		"mit Umlauten":     {"wunsch": {"Ein Fußgängerüberweg an der Straße wäre schön."}},
		"mit Emoji":        {"wunsch": {"Ein Dorfkalender 📅 mit allen Terminen."}},
		"Name ohne E-Mail": {"wunsch": {guterWunsch}, "name": {"Erna"}},
		"E-Mail ohne Name": {"wunsch": {guterWunsch}, "email": {"erna@example.org"}},
		"lange E-Mail ok":  {"wunsch": {guterWunsch}, "email": {strings.Repeat("e", 60) + "@example.org"}},
		"genau 5 Zeichen":  {"wunsch": {"Radweg"}},
		"genau 2000":       {"wunsch": {strings.Repeat("ü", 2000)}},
	}
	for name, werte := range gut {
		resp := sendeIdee(t, ts, "", werte, nil)
		resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			t.Errorf("%s: Status %d, erwartet 201", name, resp.StatusCode)
		}
	}
	if ideen, _ := d.ListIdeen(""); len(ideen) != len(gut) {
		t.Fatalf("%d Ideen gespeichert, erwartet %d", len(ideen), len(gut))
	}
}

// Ein Skript, das stur weiterschickt, wird ausgebremst — und die Grenze
// greift auch dann, wenn die Eingaben ungültig sind.
func TestIdeenEingangBremstDauerfeuer(t *testing.T) {
	ts, srv, d := ideenServer(t)
	srv.IdeenLimiter = httpx.NewRateLimiter(httpx.RateLimitConfig{Burst: 5, PerHour: 5})

	abgewiesen := 0
	for i := 0; i < 40; i++ {
		// Absichtlich ungültig: Auch das darf keinen Freifahrtschein geben.
		resp := sendeIdee(t, ts, "", url.Values{"wunsch": {"x"}}, nil)
		resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests {
			abgewiesen++
		}
	}
	if abgewiesen < 30 {
		t.Fatalf("nur %d von 40 Versuchen abgewiesen — die Grenze greift zu spät", abgewiesen)
	}
	if ideen, _ := d.ListIdeen(""); len(ideen) != 0 {
		t.Fatalf("Dauerfeuer hat etwas gespeichert: %v", ideen)
	}
}

func TestIdeenWeiterleitungKenntNurGanzeUrspruenge(t *testing.T) {
	ts, srv, _ := ideenServer(t)
	srv.IdeenRedirects = []string{"https://xn--rssing-wxa.de"}

	// Groß-/Kleinschreibung von Schema und Host ist unerheblich.
	for _, ziel := range []string{
		"https://xn--rssing-wxa.de/app/danke",
		"HTTPS://XN--RSSING-WXA.DE/app/danke",
		"https://xn--rssing-wxa.de/app/danke?ok=1",
	} {
		resp := sendeIdee(t, ts, "", url.Values{"wunsch": {guterWunsch}, "redirect": {ziel}}, nil)
		resp.Body.Close()
		if resp.StatusCode != http.StatusSeeOther {
			t.Errorf("Ziel %q: Status %d, erwartet 303", ziel, resp.StatusCode)
		}
	}

	// Alles, was nur so aussieht, wird abgewiesen.
	for _, ziel := range []string{
		"https://xn--rssing-wxa.de:8080/app/danke",
		"https://boese.example/#https://xn--rssing-wxa.de",
		"https:/\\boese.example/app/danke",
		"https://xn--rssing-wxa.de.boese.example",
		"//xn--rssing-wxa.de/app/danke",
		"data:text/html,<script>alert(1)</script>",
		"\thttps://boese.example",
		"https://xn--rssing-wxa.de:@boese.example/",
	} {
		resp := sendeIdee(t, ts, "", url.Values{"wunsch": {guterWunsch}, "redirect": {ziel}}, nil)
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Ziel %q: Status %d, erwartet 400", ziel, resp.StatusCode)
		}
	}
}

// Die Fehlerseite gibt den getippten Text zurück — als Text, nicht als Markup.
func TestIdeeFehlerseiteEscaptDenText(t *testing.T) {
	ts, _, _ := ideenServer(t)
	resp := sendeIdee(t, ts, "", url.Values{
		"wunsch": {"x"}, "name": {`<script>alert(1)</script>`},
	}, map[string]string{"Accept": "text/html"})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("Status %d, erwartet 400", resp.StatusCode)
	}
	seite := lies(t, resp)
	if strings.Contains(seite, "<script>alert(1)</script>") {
		t.Fatal("die Fehlerseite gibt Markup ungeschützt aus")
	}
	if !strings.Contains(seite, "&lt;script&gt;") && !strings.Contains(seite, "&#34;") {
		t.Fatalf("der Text kommt nicht escaped zurück:\n%s", seite)
	}
}

// Auch wer in die Zugriffsgrenze läuft, darf seinen Text nicht verlieren:
// Im Browser kommt er auf einer verständlichen Seite zurück.
func TestIdeeRateLimitVerliertDenTextNicht(t *testing.T) {
	ts, srv, _ := ideenServer(t)
	srv.IdeenLimiter = httpx.NewRateLimiter(httpx.RateLimitConfig{Burst: 1, PerHour: 1})
	html := map[string]string{"Accept": "text/html,application/xhtml+xml"}

	// Der erste Versuch geht durch.
	resp := sendeIdee(t, ts, "", url.Values{"wunsch": {guterWunsch}}, html)
	resp.Body.Close()

	// Der zweite läuft in die Grenze — und bekommt seine Eingabe zurück.
	resp = sendeIdee(t, ts, "", url.Values{
		"wunsch": {"Ein Radweg nach Nordstemmen wäre großartig."},
		"name":   {"Erna Musterfrau"}, "email": {"erna@example.org"},
	}, html)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("Status %d, erwartet 429", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("Content-Type = %q, erwartet HTML", ct)
	}
	if resp.Header.Get("Retry-After") == "" {
		t.Error("Retry-After fehlt")
	}
	seite := lies(t, resp)
	for _, muss := range []string{
		"Ein Radweg nach Nordstemmen wäre großartig.",
		"Erna Musterfrau", "erna@example.org",
	} {
		if !strings.Contains(seite, muss) {
			t.Errorf("die Seite enthält %q nicht", muss)
		}
	}
}

// Ohne Browser bleibt es bei der knappen JSON-Antwort.
func TestIdeeRateLimitAntwortetJSONOhneBrowser(t *testing.T) {
	ts, srv, _ := ideenServer(t)
	srv.IdeenLimiter = httpx.NewRateLimiter(httpx.RateLimitConfig{Burst: 1, PerHour: 1})

	sendeIdee(t, ts, "", url.Values{"wunsch": {guterWunsch}}, nil).Body.Close()
	resp := sendeIdee(t, ts, "", url.Values{"wunsch": {guterWunsch}}, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("Status %d, erwartet 429", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "json") {
		t.Fatalf("Content-Type = %q, erwartet JSON", ct)
	}
}
