package admin

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/db"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// aufbau liefert eine echte SQLite-DB, die registrierte Verwaltung und ein
// gültiges Session-Cookie. Nichts wird gemockt.
func aufbau(t *testing.T) (*App, http.Handler, *db.DB, *http.Cookie) {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })

	a := newApp(Config{
		DB: d, Issuer: "https://id.invalid", ClientID: "test-client",
		PublicURL: "http://localhost:8080", SessionKey: []byte("test-schluessel"),
		Now: func() time.Time { return time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC) },
	})
	mux := http.NewServeMux()
	a.register(mux)

	wert, err := a.signer.encode(session{Sub: "u1", Name: "Testadmin", Admin: true,
		Exp: time.Now().Add(time.Hour).Unix()})
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	return a, mux, d, &http.Cookie{Name: cookieSession, Value: wert}
}

func hole(t *testing.T, h http.Handler, pfad string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, pfad, nil)
	for _, c := range cookies {
		r.AddCookie(c)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func sende(t *testing.T, h http.Handler, pfad string, werte url.Values, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, pfad, strings.NewReader(werte.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, c := range cookies {
		r.AddCookie(c)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestStartseiteUndAnmeldung(t *testing.T) {
	_, h, _, sitzung := aufbau(t)

	w := hole(t, h, "/")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Dorf-App Rössing") {
		t.Fatalf("Startseite: %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "/admin/static/app.css") {
		t.Fatal("Startseite lädt das gebaute CSS nicht")
	}

	// Ohne Session: Anmeldeseite statt Verwaltung.
	w = hole(t, h, "/admin/")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "/admin/login") {
		t.Fatalf("Anmeldeseite fehlt: %d", w.Code)
	}
	if strings.Contains(w.Body.String(), "/admin/dorfpflege/") {
		t.Fatal("Verwaltung ist ohne Anmeldung sichtbar")
	}

	// Mit Session: Bereichsübersicht.
	w = hole(t, h, "/admin/", sitzung)
	if !strings.Contains(w.Body.String(), "/admin/dorfpflege/") {
		t.Fatalf("Bereich Dorfpflege fehlt: %s", w.Body.String())
	}

	// Geschützte Seite ohne Session leitet auf die Anmeldung um.
	w = hole(t, h, "/admin/dorfpflege/")
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/admin/" {
		t.Fatalf("Schutz greift nicht: %d %s", w.Code, w.Header().Get("Location"))
	}
}

func TestOrtAufgabeErledigungUndLoeschen(t *testing.T) {
	_, h, d, sitzung := aufbau(t)

	// Ort anlegen.
	w := sende(t, h, "/admin/dorfpflege/orte/neu", url.Values{
		"name": {"Teststelle"}, "art": {"beet"}, "beschreibung": {"aus dem Test"},
		"lat": {"52,2115"}, "lon": {"9.8710"}, "aktiv": {"1"},
	}, sitzung)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("Ort anlegen: %d %s", w.Code, w.Body.String())
	}
	orte, err := d.ListPlaces()
	if err != nil || len(orte) != 1 {
		t.Fatalf("Ort nicht gespeichert: %v %v", orte, err)
	}
	ort := orte[0]
	if ort.Lat != 52.2115 || ort.Kind != model.PlaceBed {
		t.Fatalf("Ort falsch übernommen: %+v", ort)
	}

	// Fehlerhafte Eingabe: Formular kommt mit Meldung zurück, nichts wird gespeichert.
	w = sende(t, h, "/admin/dorfpflege/orte/neu", url.Values{
		"name": {""}, "art": {"beet"}, "lat": {"52.2"}, "lon": {"9.8"},
	}, sitzung)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "formularfehler") {
		t.Fatalf("Validierung fehlt: %d", w.Code)
	}

	// Aufgabe anlegen.
	pfad := "/admin/dorfpflege/orte/" + strconv.FormatInt(ort.ID, 10)
	w = sende(t, h, pfad+"/aufgaben/neu", url.Values{
		"art": {"giessen"}, "liter": {"10"}, "intervall": {"7"}, "rot": {"14"}, "aktiv": {"1"},
	}, sitzung)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("Aufgabe anlegen: %d %s", w.Code, w.Body.String())
	}
	aufgaben, _ := d.ListTasks()
	if len(aufgaben) != 1 || aufgaben[0].Liters == nil || *aufgaben[0].Liters != 10 {
		t.Fatalf("Aufgabe falsch: %+v", aufgaben)
	}

	// Detailseite zeigt Aufgabe und leere Historie.
	w = hole(t, h, pfad, sitzung)
	if !strings.Contains(w.Body.String(), "noch nie erledigt") || !strings.Contains(w.Body.String(), "keine-historie") {
		t.Fatalf("Detailseite unvollständig: %s", w.Body.String())
	}

	// Erledigung melden → Status grün, Historie gefüllt.
	w = sende(t, h, "/admin/dorfpflege/aufgaben/"+strconv.FormatInt(aufgaben[0].ID, 10)+"/erledigt",
		url.Values{"notiz": {"gegossen"}}, sitzung)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("Erledigung: %d %s", w.Code, w.Body.String())
	}
	w = hole(t, h, pfad, sitzung)
	if !strings.Contains(w.Body.String(), `data-status="green"`) {
		t.Fatal("Status nach Erledigung nicht grün")
	}
	if !strings.Contains(w.Body.String(), "gegossen") || !strings.Contains(w.Body.String(), "Testadmin") {
		t.Fatal("Historie fehlt")
	}

	// Löschen läuft über eine eigene Bestätigungsseite (kein confirm()).
	w = hole(t, h, pfad+"/loeschen", sitzung)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "loeschen-bestaetigen") {
		t.Fatalf("Bestätigungsseite fehlt: %d", w.Code)
	}
	w = sende(t, h, pfad+"/loeschen", nil, sitzung)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("Löschen: %d", w.Code)
	}
	if orte, _ := d.ListPlaces(); len(orte) != 0 {
		t.Fatalf("Ort nicht gelöscht: %v", orte)
	}
}

func TestHitzefaktor(t *testing.T) {
	_, h, d, sitzung := aufbau(t)

	w := sende(t, h, "/admin/dorfpflege/einstellungen", url.Values{"hitzefaktor": {"0,5"}}, sitzung)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("Speichern: %d %s", w.Code, w.Body.String())
	}
	if f, _ := d.WateringFactor(); f != 0.5 {
		t.Fatalf("Faktor nicht gespeichert: %v", f)
	}

	w = sende(t, h, "/admin/dorfpflege/einstellungen", url.Values{"hitzefaktor": {"99"}}, sitzung)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("unsinniger Faktor wurde akzeptiert: %d", w.Code)
	}
	if f, _ := d.WateringFactor(); f != 0.5 {
		t.Fatalf("Faktor wurde trotz Fehler geändert: %v", f)
	}
}

func TestSessionCookieIstManipulationssicher(t *testing.T) {
	a, h, _, sitzung := aufbau(t)

	gefaelscht := &http.Cookie{Name: cookieSession, Value: sitzung.Value + "x"}
	w := hole(t, h, "/admin/dorfpflege/", gefaelscht)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("gefälschtes Cookie wurde akzeptiert: %d", w.Code)
	}

	abgelaufen, _ := a.signer.encode(session{Sub: "u1", Admin: true, Exp: time.Now().Add(-time.Minute).Unix()})
	w = hole(t, h, "/admin/dorfpflege/", &http.Cookie{Name: cookieSession, Value: abgelaufen})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("abgelaufene Session wurde akzeptiert: %d", w.Code)
	}
}
