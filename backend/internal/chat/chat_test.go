package chat

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/auth"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/db"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/mitglied"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// jetzt ist die feste Uhrzeit aller Tests.
var jetzt = time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)

// Tokens des Dev-Verifiers: "sub:name:rollen". Die Träger-Rollen kommen im
// Test über mitglied.DevQuelle aus denselben Rollen („<projektId>@<rolle>“).
const (
	tokenBetreiber = "betreiber-sub:Levin:admin"
	tokenNachbarin = "nachbarin-sub:Erna:"
	tokenMitglied  = "mitglied-sub:Heiko:4711@mitglied"
	tokenVorstand  = "vorstand-sub:Anke:4711@admin"
)

// dorf ist ein kleines Testdorf: ein Träger mit einem öffentlichen und einem
// internen Blumenkasten.
type dorf struct {
	DB       *db.DB
	Traeger  model.Traeger
	Offen    model.Place
	Intern   model.Place
	Giessen  model.CareTask
	Internes model.CareTask
}

func neuesDorf(t *testing.T) *dorf {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })

	traeger := model.Traeger{Name: "Dorfpflege", ProjektID: "4711",
		Status: model.TraegerZugelassen, Sichtbarkeit: model.TraegerOffen, CreatedAt: jetzt}
	if err := d.InsertTraeger(&traeger); err != nil {
		t.Fatal(err)
	}

	offen := model.Place{Name: "Kirchplatz — Kasten 1", Kind: model.PlaceFlowerbox,
		Lat: 52.211, Lon: 9.870, Active: true, CreatedAt: jetzt, TraegerID: traeger.ID}
	if err := d.InsertPlace(&offen); err != nil {
		t.Fatal(err)
	}
	intern := model.Place{Name: "Vereinsheim — Beet hinten", Kind: model.PlaceBed,
		Lat: 52.212, Lon: 9.871, Active: true, CreatedAt: jetzt, TraegerID: traeger.ID}
	if err := d.InsertPlace(&intern); err != nil {
		t.Fatal(err)
	}

	zehn := 10.0
	giessen := model.CareTask{PlaceID: offen.ID, Kind: model.TaskWatering, Liters: &zehn,
		IntervalDays: 7, RedAfterDays: 14, Active: true, CreatedAt: jetzt,
		Sichtbarkeit: model.AufgabeOeffentlich}
	if err := d.InsertTask(&giessen); err != nil {
		t.Fatal(err)
	}
	internes := model.CareTask{PlaceID: intern.ID, Kind: model.TaskWeeding,
		Title: "Geheimes Vereinsbeet jäten", IntervalDays: 21, RedAfterDays: 35,
		Active: true, CreatedAt: jetzt, Sichtbarkeit: model.AufgabeNurMitglieder}
	if err := d.InsertTask(&internes); err != nil {
		t.Fatal(err)
	}
	return &dorf{DB: d, Traeger: traeger, Offen: offen, Intern: intern,
		Giessen: giessen, Internes: internes}
}

// server baut den Chat mit diesem Modell. Ein nil-Modell heißt: kein
// Schlüssel eingerichtet.
func (dd *dorf) server(t *testing.T, modell *lokalesModell) *httptest.Server {
	t.Helper()
	cfg := Config{DB: dd.DB, Mitglieder: mitglied.DevQuelle{},
		Now: func() time.Time { return jetzt }}
	if modell != nil {
		cfg.Anbieter = modell.Anbieter()
	}
	mux := http.NewServeMux()
	Register(mux, auth.Middleware(auth.InsecureDevVerifier{}), cfg)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

func frage(t *testing.T, ts *httptest.Server, token, text string, verlauf ...Zug) *http.Response {
	t.Helper()
	var rumpf bytes.Buffer
	if err := json.NewEncoder(&rumpf).Encode(frageEingabe{Frage: text, Verlauf: verlauf}); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/chat", &rumpf)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func lies[T any](t *testing.T, resp *http.Response) T {
	t.Helper()
	defer resp.Body.Close()
	var v T
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

// --- Ohne Schlüssel ---------------------------------------------------------

// Ohne Schlüssel stürzt nichts ab: Der Bereich sagt, dass er noch nicht
// eingerichtet ist. Genau das ist der Zustand, solange der Betreiber den
// Schlüssel noch nicht nachgetragen hat.
func TestOhneSchluesselSchaltetSichVerstaendlichAb(t *testing.T) {
	dd := neuesDorf(t)
	ts := dd.server(t, nil)

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/chat", nil)
	req.Header.Set("Authorization", "Bearer "+tokenNachbarin)
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	stand := lies[standAusgabe](t, resp)
	if stand.Verfuegbar {
		t.Fatal("ohne Schlüssel darf der Chat nicht als verfügbar gelten")
	}
	if stand.Hinweis == "" {
		t.Fatal("ohne Schlüssel fehlt der Hinweis, warum gerade nichts geht")
	}

	antwort := frage(t, ts, tokenNachbarin, "Was steht an?")
	if antwort.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("Status = %d, erwartet 503", antwort.StatusCode)
	}
	antwort.Body.Close()
}

// Ohne Anmeldung gibt es den Chat nicht — er hängt an derselben
// Anmeldepflicht wie der Rest der API.
func TestOhneAnmeldungKeinChat(t *testing.T) {
	dd := neuesDorf(t)
	modell := starteModell(t, func(int, modellAnfrage) any { return antwortText("Moin!") })
	ts := dd.server(t, modell)

	resp, err := ts.Client().Post(ts.URL+"/api/v1/chat", "application/json",
		strings.NewReader(`{"frage":"Was steht an?"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("Status = %d, erwartet 401", resp.StatusCode)
	}
}

// --- Das Gespräch ------------------------------------------------------------

// Der Regelfall: Das Modell fragt ein Werkzeug, bekommt echte Daten und
// antwortet damit.
func TestFrageWirdAusEchtenDatenBeantwortet(t *testing.T) {
	dd := neuesDorf(t)
	modell := starteModell(t, func(nummer int, ein modellAnfrage) any {
		if nummer == 0 {
			return antwortWerkzeug("orte_liste", map[string]any{})
		}
		// Zweiter Zug: Das Modell hat die Ortsliste gesehen und antwortet.
		if !enthaelt(ein.Werkzeugergebnisse(), "Kirchplatz") {
			return antwortText("keine Daten bekommen")
		}
		return antwortText("Am Kirchplatz steht der Kasten 1.")
	})
	ts := dd.server(t, modell)

	aus := lies[frageAusgabe](t, frage(t, ts, tokenNachbarin, "Was steht gerade an?"))
	if !strings.Contains(aus.Antwort, "Kirchplatz") {
		t.Fatalf("Antwort = %q, erwartet den Ort aus der Datenbank", aus.Antwort)
	}
	if len(aus.Werkzeuge) != 1 || aus.Werkzeuge[0] != "orte_liste" {
		t.Fatalf("Werkzeuge = %v, erwartet [orte_liste]", aus.Werkzeuge)
	}
}

// Der Schlüssel geht in der Kopfzeile hinaus, die die API dafür vorsieht —
// und niemals in der Antwort an die App.
func TestSchluesselStehtNurInDerKopfzeile(t *testing.T) {
	dd := neuesDorf(t)
	modell := starteModell(t, func(int, modellAnfrage) any { return antwortText("Moin!") })
	ts := dd.server(t, modell)

	resp := frage(t, ts, tokenNachbarin, "Moin")
	defer resp.Body.Close()
	rumpf := new(bytes.Buffer)
	if _, err := rumpf.ReadFrom(resp.Body); err != nil {
		t.Fatal(err)
	}
	if modell.Schluessel != "test-schluessel" {
		t.Fatalf("x-api-key = %q, erwartet den eingerichteten Schlüssel", modell.Schluessel)
	}
	if modell.Version != apiVersion {
		t.Fatalf("anthropic-version = %q, erwartet %q", modell.Version, apiVersion)
	}
	if strings.Contains(rumpf.String(), "test-schluessel") {
		t.Fatal("der Schlüssel darf nicht in der Antwort an die App stehen")
	}
}

// Modell, Werkzeuge und Aufwand gehen so hinaus, wie sie eingestellt sind.
func TestAnfrageTraegtModellUndWerkzeuge(t *testing.T) {
	dd := neuesDorf(t)
	modell := starteModell(t, func(int, modellAnfrage) any { return antwortText("Moin!") })
	ts := dd.server(t, modell)
	frage(t, ts, tokenNachbarin, "Moin").Body.Close()

	ein := modell.letzteAnfrage(t)
	if ein.Model != "lokales-testmodell" {
		t.Fatalf("model = %q", ein.Model)
	}
	if ein.MaxTokens <= 0 {
		t.Fatal("max_tokens fehlt — die API weist die Anfrage sonst ab")
	}
	if ein.OutputConfig == nil || ein.OutputConfig.Effort != StandardAufwand {
		t.Fatalf("output_config = %+v, erwartet effort=%q", ein.OutputConfig, StandardAufwand)
	}
	namen := map[string]bool{}
	for _, w := range ein.Tools {
		namen[w.Name] = true
		if w.Description == "" || w.InputSchema == nil {
			t.Fatalf("Werkzeug %q ohne Beschreibung oder Schema", w.Name)
		}
	}
	for _, pflicht := range []string{"orte_liste", "historie", "rangliste", "traeger_liste"} {
		if !namen[pflicht] {
			t.Fatalf("Werkzeug %q fehlt in der Anfrage", pflicht)
		}
	}
	// Der Verleih gehört einem eigenen Dienst; hier hat er nichts zu suchen.
	for name := range namen {
		if strings.Contains(name, "miet") || strings.Contains(name, "verleih") ||
			strings.Contains(name, "geraet") {
			t.Fatalf("Werkzeug %q gehört nicht in den Chat des Dorfservers", name)
		}
	}
}

// Der serverseitige Rückfall geht in der Form hinaus, die zusammengehört:
// die Kennung „…-07-01" zur Kurzform „default". Die andere Kennung gehört zur
// Listenform, und beides zu mischen weist die API mit 400 ab — der Chat wäre
// dann nicht schlechter, sondern kaputt.
func TestRueckfallGehtInEinemStueckHinaus(t *testing.T) {
	dd := neuesDorf(t)
	modell := starteModell(t, func(int, modellAnfrage) any { return antwortText("Moin!") })
	ts := dd.server(t, modell)
	frage(t, ts, tokenNachbarin, "Moin").Body.Close()

	if modell.Beta != rueckfallBeta {
		t.Fatalf("anthropic-beta = %q, erwartet %q", modell.Beta, rueckfallBeta)
	}
	if ein := modell.letzteAnfrage(t); ein.Fallbacks != "default" {
		t.Fatalf("fallbacks = %q, erwartet „default“", ein.Fallbacks)
	}
}

// Abschaltbar muss es sein: Eine Beta-Kennung, die es nicht mehr gibt, lässt
// die API die ganze Anfrage abweisen. Dann soll der Betreiber den Bereich mit
// einer Umgebungsvariablen zurückbekommen und nicht auf eine Auslieferung
// warten müssen.
func TestOhneRueckfallGehtNichtsDavonHinaus(t *testing.T) {
	dd := neuesDorf(t)
	modell := starteModell(t, func(int, modellAnfrage) any { return antwortText("Moin!") })
	anbieter := modell.Anbieter()
	anbieter.Rueckfall = false
	cfg := Config{DB: dd.DB, Mitglieder: mitglied.DevQuelle{}, Anbieter: anbieter,
		Now: func() time.Time { return jetzt }}
	mux := http.NewServeMux()
	Register(mux, auth.Middleware(auth.InsecureDevVerifier{}), cfg)
	ts := httptest.NewServer(mux)
	defer ts.Close()
	frage(t, ts, tokenNachbarin, "Moin").Body.Close()

	if modell.Beta != "" {
		t.Fatalf("anthropic-beta = %q, erwartet keine Kopfzeile", modell.Beta)
	}
	if ein := modell.letzteAnfrage(t); ein.Fallbacks != "" {
		t.Fatalf("fallbacks = %q, erwartet nichts", ein.Fallbacks)
	}
}

// Der Systemtext sagt, mit wem gesprochen wird und wann — sonst rechnet das
// Modell mit dem Datum seines Trainings.
func TestSystemtextNenntPersonUndDatum(t *testing.T) {
	dd := neuesDorf(t)
	modell := starteModell(t, func(int, modellAnfrage) any { return antwortText("Moin!") })
	ts := dd.server(t, modell)
	frage(t, ts, tokenNachbarin, "Moin").Body.Close()

	system := modell.letzteAnfrage(t).System
	if !strings.Contains(system, "Erna") {
		t.Fatalf("Systemtext nennt die Person nicht: %q", system)
	}
	if !strings.Contains(system, "12.08.2026") {
		t.Fatalf("Systemtext nennt das Datum nicht: %q", system)
	}
}

// --- Sichtbarkeit ------------------------------------------------------------

// Die schärfste Regel des Systems, für den neuen Ausgabeweg: Eine interne
// Aufgabe erreicht nicht einmal das Modell. Geprüft wird deshalb das
// Werkzeugergebnis und nicht die Antwort — was das Modell nie gesehen hat,
// kann es auch nicht ausplaudern.
func TestInterneAufgabeErreichtDasModellNicht(t *testing.T) {
	dd := neuesDorf(t)
	modell := starteModell(t, func(nummer int, ein modellAnfrage) any {
		if nummer == 0 {
			return antwortWerkzeug("orte_liste", map[string]any{})
		}
		return antwortText("fertig")
	})
	ts := dd.server(t, modell)

	frage(t, ts, tokenNachbarin, "Zeig mir alle Orte").Body.Close()
	ergebnisse := modell.letzteAnfrage(t).Werkzeugergebnisse()
	if len(ergebnisse) == 0 {
		t.Fatal("das Werkzeug wurde nie ausgeführt")
	}
	if enthaelt(ergebnisse, "Geheimes Vereinsbeet") {
		t.Fatalf("die interne Aufgabe steht im Werkzeugergebnis: %v", ergebnisse)
	}
	if enthaelt(ergebnisse, "Vereinsheim") {
		t.Fatalf("der Ort der internen Aufgabe steht im Werkzeugergebnis: %v", ergebnisse)
	}
	if !enthaelt(ergebnisse, "Kirchplatz") {
		t.Fatalf("der öffentliche Ort fehlt: %v", ergebnisse)
	}
}

// Umgekehrt sieht ein Mitglied des Trägers seine interne Aufgabe sehr wohl —
// sonst wäre der Chat nicht dicht, sondern bloß blind.
func TestMitgliedSiehtDieInterneAufgabe(t *testing.T) {
	dd := neuesDorf(t)
	modell := starteModell(t, func(nummer int, ein modellAnfrage) any {
		if nummer == 0 {
			return antwortWerkzeug("orte_liste", map[string]any{})
		}
		return antwortText("fertig")
	})
	ts := dd.server(t, modell)

	frage(t, ts, tokenMitglied, "Zeig mir alle Orte").Body.Close()
	if !enthaelt(modell.letzteAnfrage(t).Werkzeugergebnisse(), "Geheimes Vereinsbeet") {
		t.Fatal("das Mitglied muss die interne Aufgabe seines Trägers sehen")
	}
}

// Auch über den Umweg „ich kenne doch die Nummer“ gibt es die interne
// Aufgabe nicht. Die Absage lautet wie bei einer Aufgabe, die es wirklich
// nicht gibt — ein eigener Text verriete, dass dort etwas ist.
func TestHistorieVerraetDieInterneAufgabeNicht(t *testing.T) {
	dd := neuesDorf(t)
	modell := starteModell(t, func(nummer int, ein modellAnfrage) any {
		if nummer == 0 {
			return antwortWerkzeug("historie", map[string]any{"aufgabeId": dd.Internes.ID})
		}
		return antwortText("fertig")
	})
	ts := dd.server(t, modell)

	frage(t, ts, tokenNachbarin, "Wer war da zuletzt?").Body.Close()
	ergebnisse := modell.letzteAnfrage(t).Werkzeugergebnisse()
	if !enthaelt(ergebnisse, "gibt es nicht") {
		t.Fatalf("erwartet wurde die Absage „gibt es nicht“, bekommen: %v", ergebnisse)
	}
	if enthaelt(ergebnisse, "Vereinsbeet") {
		t.Fatalf("die interne Aufgabe steht im Ergebnis: %v", ergebnisse)
	}
}

// --- Verwalten ---------------------------------------------------------------

// Wer nicht verwalten darf, kann über den Chat nichts anlegen.
func TestOhneVerwaltungsrechtWirdNichtsAngelegt(t *testing.T) {
	dd := neuesDorf(t)
	modell := starteModell(t, func(nummer int, ein modellAnfrage) any {
		if nummer == 0 {
			return antwortWerkzeug("ort_anlegen", map[string]any{
				"name": "Heimlicher Kasten", "lat": 52.21, "lon": 9.87,
				"traegerId": dd.Traeger.ID,
			})
		}
		return antwortText("fertig")
	})
	ts := dd.server(t, modell)

	frage(t, ts, tokenMitglied, "Leg hier einen Blumenkasten an").Body.Close()
	if !enthaelt(modell.letzteAnfrage(t).Werkzeugergebnisse(), "Fehler:") {
		t.Fatal("ein Mitglied ohne admin-Rolle darf nichts anlegen")
	}
	orte, err := dd.DB.ListPlaces()
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range orte {
		if o.Name == "Heimlicher Kasten" {
			t.Fatal("der Ort wurde trotz fehlender Berechtigung angelegt")
		}
	}
}

// Der Vorstand seines Trägers darf es — und dann steht der Ort wirklich in
// der Datenbank, mit dem richtigen Träger.
func TestVorstandLegtOrtAn(t *testing.T) {
	dd := neuesDorf(t)
	modell := starteModell(t, func(nummer int, ein modellAnfrage) any {
		if nummer == 0 {
			return antwortWerkzeug("ort_anlegen", map[string]any{
				"name": "Am Bahnhof — Kasten", "lat": 52.213, "lon": 9.872,
			})
		}
		return antwortText("Angelegt.")
	})
	ts := dd.server(t, modell)

	frage(t, ts, tokenVorstand, "Leg am Bahnhof einen Blumenkasten an").Body.Close()
	orte, err := dd.DB.ListPlaces()
	if err != nil {
		t.Fatal(err)
	}
	var gefunden *model.Place
	for i := range orte {
		if orte[i].Name == "Am Bahnhof — Kasten" {
			gefunden = &orte[i]
		}
	}
	if gefunden == nil {
		t.Fatalf("der Ort wurde nicht angelegt; Werkzeugergebnis: %v",
			modell.letzteAnfrage(t).Werkzeugergebnisse())
	}
	// Ohne traegerId wird der einzige Träger genommen, den diese Person
	// verwaltet — nicht irgendeiner.
	if gefunden.TraegerID != dd.Traeger.ID {
		t.Fatalf("Träger = %d, erwartet %d", gefunden.TraegerID, dd.Traeger.ID)
	}
}

// Eine Meldung über den Chat läuft durch dieselbe Prüfung wie über die App —
// samt Spielschutz. Der zweite Versuch scheitert an der Sperrfrist.
func TestErledigungMeldenUndSperrfrist(t *testing.T) {
	dd := neuesDorf(t)
	modell := starteModell(t, func(nummer int, ein modellAnfrage) any {
		if nummer%2 == 0 {
			return antwortWerkzeug("erledigung_melden",
				map[string]any{"aufgabeId": dd.Giessen.ID, "liter": 10})
		}
		return antwortText("Eingetragen.")
	})
	ts := dd.server(t, modell)

	frage(t, ts, tokenNachbarin, "Ich habe gerade gegossen").Body.Close()
	meldungen, err := dd.DB.ListCompletions(dd.Giessen.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(meldungen) != 1 {
		t.Fatalf("%d Meldungen, erwartet 1", len(meldungen))
	}
	if meldungen[0].UserSub != "nachbarin-sub" {
		t.Fatalf("gemeldet von %q — eine Meldung läuft auf die fragende Person",
			meldungen[0].UserSub)
	}

	frage(t, ts, tokenNachbarin, "Ich habe nochmal gegossen").Body.Close()
	if !enthaelt(modell.letzteAnfrage(t).Werkzeugergebnisse(), "gerade erst") {
		t.Fatal("der Spielschutz muss auch im Chat greifen")
	}
	meldungen, _ = dd.DB.ListCompletions(dd.Giessen.ID, 10)
	if len(meldungen) != 1 {
		t.Fatalf("%d Meldungen nach dem zweiten Versuch, erwartet 1", len(meldungen))
	}
}

// --- Störungen ---------------------------------------------------------------

// Eine Überlast der API ist eine Störung, kein Fehler der App: Sie bekommt
// „gleich noch einmal“ und keinen kaputten Zustand.
func TestUeberlastWirdUebersetzt(t *testing.T) {
	dd := neuesDorf(t)
	modell := starteModell(t, func(int, modellAnfrage) any {
		return modellFehler{Status: 529, Art: "overloaded_error", Text: "Overloaded"}
	})
	ts := dd.server(t, modell)

	resp := frage(t, ts, tokenNachbarin, "Was steht an?")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("Status = %d, erwartet 503", resp.StatusCode)
	}
}

// Eine Absage wegen eines falschen Schlüssels darf nicht als „später nochmal“
// durchgehen — und ihre Meldung gehört nicht in die App.
func TestFalscherSchluesselMeldungBleibtDrinnen(t *testing.T) {
	dd := neuesDorf(t)
	modell := starteModell(t, func(int, modellAnfrage) any {
		return modellFehler{Status: 401, Art: "authentication_error",
			Text: "invalid x-api-key sk-ant-geheim"}
	})
	ts := dd.server(t, modell)

	resp := frage(t, ts, tokenNachbarin, "Was steht an?")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("Status = %d, erwartet 502", resp.StatusCode)
	}
	rumpf := new(bytes.Buffer)
	if _, err := rumpf.ReadFrom(resp.Body); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rumpf.String(), "sk-ant-geheim") {
		t.Fatal("die Meldung der Claude-API darf nicht an die App durchgereicht werden")
	}
}

// Ein Modell, das nur noch Werkzeuge will, wird nach der Rundengrenze
// gestoppt — sonst dreht sich das Gespräch auf Kosten des Betreibers.
func TestRundengrenzeBrichtAb(t *testing.T) {
	dd := neuesDorf(t)
	modell := starteModell(t, func(int, modellAnfrage) any {
		return antwortWerkzeug("orte_liste", map[string]any{})
	})
	cfg := Config{DB: dd.DB, Mitglieder: mitglied.DevQuelle{}, Anbieter: modell.Anbieter(),
		Now: func() time.Time { return jetzt }, MaxRunden: 3}
	mux := http.NewServeMux()
	Register(mux, auth.Middleware(auth.InsecureDevVerifier{}), cfg)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	aus := lies[frageAusgabe](t, frage(t, ts, tokenNachbarin, "Was steht an?"))
	if !aus.Abgebrochen {
		t.Fatal("die Rundengrenze muss als Abbruch gemeldet werden")
	}
	if len(modell.Anfragen) != 3 {
		t.Fatalf("%d Anfragen ans Modell, erwartet 3", len(modell.Anfragen))
	}
}

// Eine Absage des Modells („refusal“) ist kein Serverfehler.
func TestAbsageDesModells(t *testing.T) {
	dd := neuesDorf(t)
	modell := starteModell(t, func(int, modellAnfrage) any {
		return map[string]any{
			"id": "msg_test", "type": "message", "role": "assistant",
			"content": []any{}, "stop_reason": "refusal",
			"usage": map[string]any{"input_tokens": 1, "output_tokens": 0},
		}
	})
	ts := dd.server(t, modell)

	resp := frage(t, ts, tokenNachbarin, "Sag was Verbotenes")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Status = %d, erwartet 200", resp.StatusCode)
	}
	aus := lies[frageAusgabe](t, resp)
	if aus.Antwort == "" {
		t.Fatal("auch eine Absage braucht einen Satz für die Person")
	}
}

// --- Eingaben ----------------------------------------------------------------

func TestLeereUndZuLangeFragen(t *testing.T) {
	dd := neuesDorf(t)
	modell := starteModell(t, func(int, modellAnfrage) any { return antwortText("Moin!") })
	ts := dd.server(t, modell)

	for _, fall := range []struct {
		name string
		text string
	}{
		{"leer", "   "},
		{"zu lang", strings.Repeat("ä", MaxFrage+1)},
	} {
		t.Run(fall.name, func(t *testing.T) {
			resp := frage(t, ts, tokenNachbarin, fall.text)
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("Status = %d, erwartet 400", resp.StatusCode)
			}
		})
	}
	if len(modell.Anfragen) != 0 {
		t.Fatal("eine unbrauchbare Frage darf gar nicht erst Geld kosten")
	}
}

// Der Verlauf kommt aus der App und ist deshalb nichts, worauf man sich
// verlassen kann: Er wird gekürzt und auf abwechselnde Rollen gebracht.
func TestVerlaufWirdGeordnet(t *testing.T) {
	fälle := []struct {
		name    string
		verlauf []Zug
		rollen  []string
	}{
		{"leer", nil, []string{"user"}},
		{"gewoehnlich", []Zug{{RolleIch, "Moin"}, {RolleApp, "Moin!"}},
			[]string{"user", "assistant", "user"}},
		{"leere Zuege fallen raus", []Zug{{RolleIch, "  "}, {RolleIch, "Moin"}, {RolleApp, "Moin!"}},
			[]string{"user", "assistant", "user"}},
		{"doppelte Rolle wird uebergangen",
			[]Zug{{RolleIch, "Moin"}, {RolleIch, "Hallo?"}, {RolleApp, "Moin!"}},
			[]string{"user", "assistant", "user"}},
		{"offener Zug am Ende faellt weg", []Zug{{RolleIch, "Moin"}},
			[]string{"user"}},
	}
	for _, fall := range fälle {
		t.Run(fall.name, func(t *testing.T) {
			nachrichten := nachrichtenAus(fall.verlauf, "Was steht an?")
			if len(nachrichten) != len(fall.rollen) {
				t.Fatalf("%d Nachrichten, erwartet %d", len(nachrichten), len(fall.rollen))
			}
			for i, rolle := range fall.rollen {
				if nachrichten[i].Role != rolle {
					t.Fatalf("Nachricht %d hat Rolle %q, erwartet %q", i, nachrichten[i].Role, rolle)
				}
			}
			// Die letzte Nachricht ist immer die neue Frage.
			var text string
			if err := json.Unmarshal(nachrichten[len(nachrichten)-1].Content, &text); err != nil {
				t.Fatal(err)
			}
			if text != "Was steht an?" {
				t.Fatalf("letzte Nachricht = %q", text)
			}
		})
	}
}

func TestVerlaufWirdGekuerzt(t *testing.T) {
	var lang []Zug
	for i := 0; i < 2*MaxVerlauf; i++ {
		rolle := RolleIch
		if i%2 == 1 {
			rolle = RolleApp
		}
		lang = append(lang, Zug{Rolle: rolle, Text: "Zug"})
	}
	nachrichten := nachrichtenAus(lang, "Und jetzt?")
	if len(nachrichten) > MaxVerlauf+1 {
		t.Fatalf("%d Nachrichten, höchstens %d erwartet", len(nachrichten), MaxVerlauf+1)
	}
	if nachrichten[0].Role != "user" {
		t.Fatalf("der Verlauf muss mit „user“ anfangen, fängt aber mit %q an", nachrichten[0].Role)
	}
}

// --- Stundenlimit ------------------------------------------------------------

func TestStundenlimitZaehltJePerson(t *testing.T) {
	limit := neuesStundenlimit(2)
	if !limit.erlaubt("a", jetzt) || !limit.erlaubt("a", jetzt) {
		t.Fatal("die ersten beiden Fragen müssen durchgehen")
	}
	if limit.erlaubt("a", jetzt) {
		t.Fatal("die dritte Frage derselben Person muss abgewiesen werden")
	}
	if !limit.erlaubt("b", jetzt) {
		t.Fatal("eine andere Person hat ihr eigenes Fenster")
	}
	if !limit.erlaubt("a", jetzt.Add(61*time.Minute)) {
		t.Fatal("nach einer Stunde geht es weiter")
	}
}

func TestStundenlimitGreiftInDerAPI(t *testing.T) {
	dd := neuesDorf(t)
	modell := starteModell(t, func(int, modellAnfrage) any { return antwortText("Moin!") })
	cfg := Config{DB: dd.DB, Mitglieder: mitglied.DevQuelle{}, Anbieter: modell.Anbieter(),
		Now: func() time.Time { return jetzt }, LimitProStunde: 1}
	mux := http.NewServeMux()
	Register(mux, auth.Middleware(auth.InsecureDevVerifier{}), cfg)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	frage(t, ts, tokenNachbarin, "Moin").Body.Close()
	resp := frage(t, ts, tokenNachbarin, "Und jetzt?")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("Status = %d, erwartet 429", resp.StatusCode)
	}
}
