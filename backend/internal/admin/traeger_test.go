package admin

import (
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/db"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/mitglied"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Der Träger-Bereich der Web-Verwaltung: zulassen und sperren (Betreiber),
// Stammdaten und Befähigungen pflegen, Anträge bearbeiten (Träger-Admin).
//
// Alles läuft gegen die echte Datenbank und die echten Templates — geprüft
// wird, was wirklich im HTML steht.

// traegerAufbau liefert die Verwaltung mit einer Mitgliedschafts-Quelle, die
// im Test aus den Rollen des Nutzers liest („<projektId>@<rolle>“).
func traegerAufbau(t *testing.T) (*App, http.Handler, *db.DB) {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })

	a := newApp(Config{
		DB: d, Issuer: "https://id.invalid", ClientID: "test-client",
		PublicURL: "http://localhost:8080", SessionKey: []byte("test-schluessel"),
		Mitglieder: mitglied.DevQuelle{},
		Now:        func() time.Time { return time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC) },
	})
	mux := http.NewServeMux()
	a.register(mux)
	return a, mux, d
}

// sitzung baut ein Session-Cookie. istBetreiber setzt die globale
// admin-Rolle; rollen sind Träger-Rollen in der Form „222@admin“.
//
// Die Träger-Rollen wandern hier über den Namen ins Cookie, weil die
// DevQuelle sie aus den Rollen des Nutzers liest — im Betrieb kommen sie
// stattdessen aus der Rössing-ID.
func sitzung(t *testing.T, a *App, sub string, istBetreiber bool, rollen ...string) *http.Cookie {
	t.Helper()
	s := session{Sub: sub, Name: sub, Admin: istBetreiber,
		Exp: time.Now().Add(time.Hour).Unix()}
	wert, err := a.signer.encode(cookieSession, s)
	if err != nil {
		t.Fatal(err)
	}
	// Die Träger-Rollen hängen an der Session; a.zugriff baut daraus den
	// auth.User, den die DevQuelle auswertet.
	if len(rollen) > 0 {
		s.Rollen = rollen
		if wert, err = a.signer.encode(cookieSession, s); err != nil {
			t.Fatal(err)
		}
	}
	return &http.Cookie{Name: cookieSession, Value: wert}
}

func traegerAnlegen(t *testing.T, d *db.DB, name, projektID string, status model.TraegerStatus) model.Traeger {
	t.Helper()
	tr := model.Traeger{Name: name, ProjektID: projektID, Status: status,
		Sichtbarkeit: model.TraegerOffen, CreatedAt: time.Now().UTC()}
	if err := d.InsertTraeger(&tr); err != nil {
		t.Fatal(err)
	}
	return tr
}

// Der Betreiber sieht auch die noch nicht zugelassenen Träger und kann sie
// zulassen — das ist der ganze Sinn der Zulassung.
func TestBetreiberLaesstTraegerZu(t *testing.T) {
	a, h, d := traegerAufbau(t)
	betreiber := sitzung(t, a, "levin", true)
	tr := traegerAnlegen(t, d, "Dorfpflege", "222", model.TraegerBeantragt)

	w := hole(t, h, "/admin/traeger/", betreiber)
	if w.Code != http.StatusOK {
		t.Fatalf("Träger-Liste: HTTP %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Dorfpflege") {
		t.Fatal("der beantragte Träger fehlt in der Liste des Betreibers")
	}

	w = sende(t, h, fmt.Sprintf("/admin/traeger/%d/zulassung", tr.ID),
		url.Values{"status": {"zugelassen"}}, betreiber)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("Zulassen: HTTP %d", w.Code)
	}
	nachher, err := d.GetTraeger(tr.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !nachher.Zugelassen() {
		t.Fatalf("nicht zugelassen: %+v", nachher)
	}

	// Und sperren geht genauso.
	sende(t, h, fmt.Sprintf("/admin/traeger/%d/zulassung", tr.ID),
		url.Values{"status": {"gesperrt"}}, betreiber)
	nachher, _ = d.GetTraeger(tr.ID)
	if nachher.Status != model.TraegerGesperrt {
		t.Fatalf("nicht gesperrt: %+v", nachher)
	}
}

// Ein Träger-Admin pflegt seinen eigenen Träger — und kommt an keinen
// fremden heran.
func TestTraegerAdminPflegtNurSeinen(t *testing.T) {
	a, h, d := traegerAufbau(t)
	eigener := traegerAnlegen(t, d, "Dorfpflege", "222", model.TraegerZugelassen)
	fremder := traegerAnlegen(t, d, "Schützenverein", "333", model.TraegerZugelassen)
	vorstand := sitzung(t, a, "vorstand", false, "222@admin")

	w := hole(t, h, fmt.Sprintf("/admin/traeger/%d", eigener.ID), vorstand)
	if w.Code != http.StatusOK {
		t.Fatalf("eigener Träger: HTTP %d", w.Code)
	}
	// Der Zulassungsstand ist für ihn kein Formularfeld.
	if strings.Contains(w.Body.String(), `id="zulassen"`) {
		t.Error("der Träger-Admin bekommt den Zulassen-Knopf zu sehen")
	}

	if w := hole(t, h, fmt.Sprintf("/admin/traeger/%d", fremder.ID), vorstand); w.Code != http.StatusNotFound {
		t.Errorf("fremder Träger: HTTP %d, erwartet 404", w.Code)
	}

	// In der Liste steht nur der eigene.
	w = hole(t, h, "/admin/traeger/", vorstand)
	if strings.Contains(w.Body.String(), "Schützenverein") {
		t.Error("der fremde Träger steht in der Liste")
	}
	if !strings.Contains(w.Body.String(), "Dorfpflege") {
		t.Error("der eigene Träger fehlt in der Liste")
	}
}

// Befähigungen anlegen und einen Antrag entscheiden — der Durchstich, auf
// den es ankommt: Nach der Freigabe gilt die Befähigung sofort.
func TestBefaehigungPflegenUndAntragEntscheiden(t *testing.T) {
	a, h, d := traegerAufbau(t)
	tr := traegerAnlegen(t, d, "Dorfpflege", "222", model.TraegerZugelassen)
	vorstand := sitzung(t, a, "vorstand", false, "222@admin")

	w := sende(t, h, fmt.Sprintf("/admin/traeger/%d/befaehigungen", tr.ID),
		url.Values{"name": {"Motorsense"}, "beschreibung": {"Einweisung am Gerät"}}, vorstand)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("Befähigung anlegen: HTTP %d — %s", w.Code, w.Body.String())
	}
	liste, err := d.ListBefaehigungen(tr.ID)
	if err != nil || len(liste) != 1 {
		t.Fatalf("Befähigung nicht angelegt: %+v %v", liste, err)
	}
	befaehigung := liste[0]

	// Jemand beantragt sie.
	antrag := model.BefaehigungsAntrag{BefaehigungID: befaehigung.ID, UserSub: "erna",
		Status: model.AntragBeantragt, Begruendung: "War bei der Einweisung",
		CreatedAt: time.Now().UTC()}
	if err := d.InsertAntrag(&antrag); err != nil {
		t.Fatal(err)
	}

	// Er steht auf der Seite und im Zähler der Bereichsübersicht.
	w = hole(t, h, fmt.Sprintf("/admin/traeger/%d", tr.ID), vorstand)
	if !strings.Contains(w.Body.String(), "War bei der Einweisung") {
		t.Error("der offene Antrag steht nicht auf der Träger-Seite")
	}

	// Der Vorstand erteilt.
	w = sende(t, h, fmt.Sprintf("/admin/traeger/%d/antraege/%d", tr.ID, antrag.ID),
		url.Values{"status": {"erteilt"}, "notiz": {"am 12.8. eingewiesen"}}, vorstand)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("Antrag erteilen: HTTP %d", w.Code)
	}
	if !d.HatBefaehigung("erna", befaehigung.ID) {
		t.Fatal("die Befähigung gilt nach der Freigabe nicht")
	}
}

// Ein fremder Träger-Admin darf über Anträge und Befähigungen eines anderen
// Vereins nicht entscheiden — auch nicht, wenn er die Kennungen kennt.
func TestFremderTraegerAdminKommtNichtRan(t *testing.T) {
	a, h, d := traegerAufbau(t)
	eigener := traegerAnlegen(t, d, "Dorfpflege", "222", model.TraegerZugelassen)
	traegerAnlegen(t, d, "Schützenverein", "333", model.TraegerZugelassen)
	fremd := sitzung(t, a, "fremd", false, "333@admin")

	b := model.Befaehigung{TraegerID: eigener.ID, Name: "Motorsense", CreatedAt: time.Now().UTC()}
	if err := d.InsertBefaehigung(&b); err != nil {
		t.Fatal(err)
	}
	w := sende(t, h, fmt.Sprintf("/admin/traeger/%d/befaehigungen/%d", eigener.ID, b.ID),
		url.Values{"name": {"Umbenannt"}}, fremd)
	if w.Code != http.StatusNotFound {
		t.Fatalf("fremde Befähigung geändert: HTTP %d, erwartet 404", w.Code)
	}
	wieder, _ := d.GetBefaehigung(b.ID)
	if wieder.Name != "Motorsense" {
		t.Fatalf("die Befähigung wurde umbenannt: %+v", wieder)
	}
}

// Wer weder Betreiber ist noch einen Träger verwaltet, hat im Bereich nichts
// verloren — die App ist der Ort zum Mitmachen.
func TestEinfachesMitgliedKommtNichtInDenBereich(t *testing.T) {
	a, h, d := traegerAufbau(t)
	traegerAnlegen(t, d, "Dorfpflege", "222", model.TraegerZugelassen)
	mitgliedSitzung := sitzung(t, a, "erna", false, "222@mitglied")

	w := hole(t, h, "/admin/traeger/", mitgliedSitzung)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("HTTP %d, erwartet eine Weiterleitung zurück", w.Code)
	}
}

// Das Aufgaben-Formular bietet Sichtbarkeit und Einweisung an — sonst ließe
// sich in der Verwaltung gar keine interne Aufgabe einstellen.
func TestAufgabenformularZeigtSichtbarkeitUndBefaehigung(t *testing.T) {
	a, h, d := traegerAufbau(t)
	betreiber := sitzung(t, a, "levin", true)
	tr := traegerAnlegen(t, d, "Dorfpflege", "222", model.TraegerZugelassen)
	b := model.Befaehigung{TraegerID: tr.ID, Name: "Motorsense", CreatedAt: time.Now().UTC()}
	if err := d.InsertBefaehigung(&b); err != nil {
		t.Fatal(err)
	}
	p := model.Place{Name: "Streuobstwiese", TraegerID: tr.ID, Kind: model.PlaceOther,
		Lat: 52.2, Lon: 9.87, Active: true, CreatedAt: time.Now().UTC()}
	if err := d.InsertPlace(&p); err != nil {
		t.Fatal(err)
	}

	w := hole(t, h, fmt.Sprintf("/admin/mithelfen/orte/%d/aufgaben/neu", p.ID), betreiber)
	if w.Code != http.StatusOK {
		t.Fatalf("HTTP %d", w.Code)
	}
	body := w.Body.String()
	for _, erwartet := range []string{"nur_mitglieder", "Motorsense", `id="feld-sichtbarkeit"`, `id="feld-befaehigung"`} {
		if !strings.Contains(body, erwartet) {
			t.Errorf("im Formular fehlt %q", erwartet)
		}
	}

	// Und eine interne Aufgabe mit Einweisung lässt sich damit anlegen.
	w = sende(t, h, fmt.Sprintf("/admin/mithelfen/orte/%d/aufgaben/neu", p.ID), url.Values{
		"art": {"sonstiges"}, "titel": {"Rasenmähen"}, "wiederholung": {"regelmaessig"},
		"intervall": {"14"}, "rot": {"28"}, "aktiv": {"1"},
		"sichtbarkeit": {"nur_mitglieder"}, "befaehigungId": {strconv.FormatInt(b.ID, 10)},
	}, betreiber)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("Anlegen: HTTP %d — %s", w.Code, w.Body.String())
	}
	tasks, err := d.ListTasks()
	if err != nil {
		t.Fatal(err)
	}
	var neu *model.CareTask
	for i := range tasks {
		if tasks[i].PlaceID == p.ID {
			neu = &tasks[i]
		}
	}
	if neu == nil {
		t.Fatal("die Aufgabe wurde nicht angelegt")
	}
	if !neu.Intern() || neu.BefaehigungID != b.ID {
		t.Fatalf("Sichtbarkeit oder Einweisung nicht gespeichert: %+v", *neu)
	}
}
