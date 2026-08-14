package admin

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/db"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/vergabe"
)

// Die Verwaltung zeigt auf der Ortsseite, wie die Vergabe steht — wer
// angemeldet ist, wer gefragt wurde, wer zugesagt hat und bis wann — und
// lässt Verwaltende eine Zusage aufheben. Alles auf eigenen Seiten, ohne
// Modals.

var vergabeJetzt = time.Date(2026, 6, 1, 12, 0, 0, 0, model.Location())

// vergabeAufbau legt einen fälligen Gießplan mit zwei Angemeldeten an und
// lässt die Vergabe einmal takten.
func vergabeAufbau(t *testing.T, d *db.DB) (model.Place, model.CareTask, *vergabe.Engine) {
	t.Helper()
	p := model.Place{Name: "Unter den Eichen — Kasten 1", Kind: model.PlaceFlowerbox,
		Lat: 52.211, Lon: 9.87, Active: true, CreatedAt: vergabeJetzt.AddDate(0, 0, -30)}
	if err := d.InsertPlace(&p); err != nil {
		t.Fatal(err)
	}
	task := model.CareTask{PlaceID: p.ID, Kind: model.TaskWatering, IntervalDays: 7,
		RedAfterDays: 14, Active: true, CreatedAt: vergabeJetzt.AddDate(0, 0, -30)}
	if err := d.InsertTask(&task); err != nil {
		t.Fatal(err)
	}
	for sub, name := range map[string]string{"anna": "Anna Apfel", "bernd": "Bernd Birne"} {
		profil := model.Profile{UserSub: sub, DisplayName: name, TokenName: name,
			Visibility: model.DefaultVisibility(), UpdatedAt: vergabeJetzt}
		if err := d.UpsertProfile(&profil); err != nil {
			t.Fatal(err)
		}
		s := model.Signup{UserSub: sub, PlaceID: p.ID, CreatedAt: vergabeJetzt.AddDate(0, 0, -10)}
		if _, err := d.InsertSignup(&s); err != nil {
			t.Fatal(err)
		}
	}
	e := vergabe.New(d, vergabe.Config{Now: func() time.Time { return vergabeJetzt }})
	if err := e.Durchlauf(); err != nil {
		t.Fatal(err)
	}
	return p, task, e
}

func TestOrtsseiteZeigtVergabestand(t *testing.T) {
	a, h, d, sitzung := aufbau(t)
	a.now = func() time.Time { return vergabeJetzt }
	p, task, e := vergabeAufbau(t, d)

	w := hole(t, h, ortsPfad(p.ID), sitzung)
	if w.Code != http.StatusOK {
		t.Fatalf("Ortsseite: %d", w.Code)
	}
	seite := w.Body.String()
	for _, erwartet := range []string{"Vergabe", "Anna Apfel", "Bernd Birne", "angemeldet"} {
		if !strings.Contains(seite, erwartet) {
			t.Errorf("Ortsseite nennt %q nicht", erwartet)
		}
	}
	if !strings.Contains(seite, "gefragt") {
		t.Error("Ortsseite zeigt nicht, wer gefragt wurde")
	}

	// Jetzt sagt jemand zu — die Seite zeigt „übernommen von … bis …".
	vorgang, err := d.ActiveAssignment(task.ID)
	if err != nil || vorgang == nil {
		t.Fatalf("kein Vorgang: %v", err)
	}
	if _, err := e.Zusagen(vorgang.ID, "anna", "Anna Apfel"); err != nil {
		t.Fatal(err)
	}
	seite = hole(t, h, ortsPfad(p.ID), sitzung).Body.String()
	if !strings.Contains(seite, "übernommen von Anna Apfel") {
		t.Error("Ortsseite zeigt die Zusage nicht")
	}
	if !strings.Contains(seite, "02.06.2026") {
		t.Error("Ortsseite nennt die Frist der Zusage nicht")
	}
	if !strings.Contains(seite, zusagePfad(vorgang.ID)) {
		t.Error("Ortsseite bietet kein Aufheben der Zusage an")
	}
}

func TestZusageAufhebenBrauchtBestaetigung(t *testing.T) {
	a, h, d, sitzung := aufbau(t)
	a.now = func() time.Time { return vergabeJetzt }
	p, task, e := vergabeAufbau(t, d)
	vorgang, _ := d.ActiveAssignment(task.ID)
	if _, err := e.Zusagen(vorgang.ID, "anna", "Anna Apfel"); err != nil {
		t.Fatal(err)
	}

	// Erst die Frage auf eigener Seite …
	w := hole(t, h, zusagePfad(vorgang.ID), sitzung)
	if w.Code != http.StatusOK {
		t.Fatalf("Bestätigungsseite: %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Anna Apfel") {
		t.Error("Bestätigungsseite nennt nicht, wessen Zusage aufgehoben wird")
	}
	if danach, _ := d.ActiveAssignment(task.ID); danach.ClaimedBy == "" {
		t.Fatal("das bloße Aufrufen hat die Zusage schon aufgehoben")
	}

	// … dann die Tat.
	w = sende(t, h, zusagePfad(vorgang.ID), url.Values{}, sitzung)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("Aufheben: %d", w.Code)
	}
	if ziel := w.Header().Get("Location"); ziel != ortsPfad(p.ID) {
		t.Errorf("Weiterleitung nach %q, erwartet %q", ziel, ortsPfad(p.ID))
	}
	danach, _ := d.ActiveAssignment(task.ID)
	if danach == nil || danach.ClaimedBy != "" {
		t.Fatalf("Zusage nicht aufgehoben: %+v", danach)
	}
	// Die betroffene Person erfährt davon.
	offen, err := d.OpenNotifications("anna")
	if err != nil {
		t.Fatal(err)
	}
	hinweis := false
	for _, n := range offen {
		if n.Kind == model.NotifyClaimRevoked {
			hinweis = true
		}
	}
	if !hinweis {
		t.Error("kein Hinweis an die betroffene Person")
	}
}

func TestEinstellungenDerVergabe(t *testing.T) {
	a, h, d, sitzung := aufbau(t)
	a.now = func() time.Time { return vergabeJetzt }

	w := hole(t, h, "/admin/mithelfen/einstellungen", sitzung)
	seite := w.Body.String()
	for _, feld := range []string{"abstand", "zusagefrist", "ruhe-von", "ruhe-bis"} {
		if !strings.Contains(seite, `name="`+feld+`"`) {
			t.Errorf("Einstellungsseite hat kein Feld %q", feld)
		}
	}

	w = sende(t, h, "/admin/mithelfen/einstellungen", url.Values{
		"hitzefaktor": {"1"}, "abstand": {"30"}, "zusagefrist": {"12"},
		"ruhe-von": {"22"}, "ruhe-bis": {"6"},
	}, sitzung)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("Speichern: %d — %s", w.Code, w.Body.String())
	}
	regeln, err := d.AssignmentRules()
	if err != nil {
		t.Fatal(err)
	}
	if regeln.OfferInterval != 30*time.Minute || regeln.ClaimDuration != 12*time.Hour ||
		regeln.QuietFrom != 22 || regeln.QuietTo != 6 {
		t.Fatalf("gespeicherte Regeln = %+v", regeln)
	}

	// Unsinn wird mit verständlichem Text abgewiesen, nicht gespeichert.
	w = sende(t, h, "/admin/mithelfen/einstellungen", url.Values{
		"hitzefaktor": {"1"}, "abstand": {"0"}, "zusagefrist": {"12"},
		"ruhe-von": {"22"}, "ruhe-bis": {"6"},
	}, sitzung)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("kaputter Abstand: %d, erwartet 400", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Abstand") {
		t.Errorf("Fehlermeldung nennt das Problem nicht: %s", w.Body.String())
	}
	if regeln, _ := d.AssignmentRules(); regeln.OfferInterval != 30*time.Minute {
		t.Errorf("kaputte Eingabe wurde gespeichert: %+v", regeln)
	}
}

func ortsPfad(id int64) string { return "/admin/mithelfen/orte/" + itoa64(id) }
func zusagePfad(id int64) string {
	return "/admin/mithelfen/vorgaenge/" + itoa64(id) + "/zusage-aufheben"
}
