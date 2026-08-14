package admin

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/db"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Bereich „Ideen“ der Verwaltung: die Wünsche aus dem Dorf lesen, einordnen
// und aufräumen — server-gerendert, ohne Modals, mit echter Navigation.

var ideenJetzt = time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)

func ideeAnlegen(t *testing.T, d *db.DB, wunsch string, status model.IdeeStatus) model.Idee {
	t.Helper()
	i := model.Idee{
		Name: "Erna Musterfrau", Email: "erna@example.org", Wunsch: wunsch,
		Quelle: model.IdeeQuelleWebsite, Status: status, CreatedAt: ideenJetzt,
	}
	if err := d.InsertIdee(&i); err != nil {
		t.Fatalf("Idee anlegen: %v", err)
	}
	return i
}

func TestIdeenListeUndDetail(t *testing.T) {
	_, h, d, sitzung := aufbau(t)
	neu := ideeAnlegen(t, d, "Ein Mitfahrbrett für Fahrten nach Hildesheim.", model.IdeeNeu)
	alt := ideeAnlegen(t, d, "Ein Vertretungsplan für die Grundschule.", model.IdeeUmgesetzt)

	// Liste: Datum, Name, E-Mail, Wunsch und Status stehen drin.
	w := hole(t, h, "/admin/ideen/", sitzung)
	if w.Code != http.StatusOK {
		t.Fatalf("Ideenliste: %d", w.Code)
	}
	koerper := w.Body.String()
	for _, muss := range []string{"Mitfahrbrett", "Vertretungsplan", "Erna Musterfrau",
		"erna@example.org", "14.08.2026", "umgesetzt"} {
		if !strings.Contains(koerper, muss) {
			t.Errorf("Liste enthält %q nicht", muss)
		}
	}

	// Filter nach Status: echte Seitennavigation, kein clientseitiges Gefummel.
	w = hole(t, h, "/admin/ideen/?status=neu", sitzung)
	if strings.Contains(w.Body.String(), "Vertretungsplan") {
		t.Error("Filter status=neu zeigt auch umgesetzte Ideen")
	}
	if !strings.Contains(w.Body.String(), "Mitfahrbrett") {
		t.Error("Filter status=neu zeigt die neue Idee nicht")
	}

	// Detailseite mit Formular für Status und interne Notiz.
	pfad := "/admin/ideen/" + strconv.FormatInt(neu.ID, 10)
	w = hole(t, h, pfad, sitzung)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Mitfahrbrett") {
		t.Fatalf("Detailseite: %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `name="notiz"`) || !strings.Contains(w.Body.String(), `name="status"`) {
		t.Fatal("Detailseite hat kein Formular für Status und Notiz")
	}
	_ = alt
}

func TestIdeeStatusUndNotizSpeichern(t *testing.T) {
	_, h, d, sitzung := aufbau(t)
	idee := ideeAnlegen(t, d, "Ein Mitfahrbrett für Fahrten nach Hildesheim.", model.IdeeNeu)
	pfad := "/admin/ideen/" + strconv.FormatInt(idee.ID, 10)

	w := sende(t, h, pfad, url.Values{
		"status": {"gelesen"}, "notiz": {"Mit dem Vorstand besprechen."},
	}, sitzung)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("Speichern: %d %s", w.Code, w.Body.String())
	}
	nach, err := d.GetIdee(idee.ID)
	if err != nil {
		t.Fatal(err)
	}
	if nach.Status != model.IdeeGelesen || nach.Notiz != "Mit dem Vorstand besprechen." {
		t.Fatalf("nicht gespeichert: %+v", nach)
	}

	// Unsinniger Status wird abgewiesen, nichts wird verändert.
	w = sende(t, h, pfad, url.Values{"status": {"vielleicht"}, "notiz": {"egal"}}, sitzung)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("unsinniger Status: %d", w.Code)
	}
	nach, _ = d.GetIdee(idee.ID)
	if nach.Status != model.IdeeGelesen || nach.Notiz != "Mit dem Vorstand besprechen." {
		t.Fatalf("trotz Fehler geändert: %+v", nach)
	}
}

func TestIdeeLoeschenUeberBestaetigungsseite(t *testing.T) {
	_, h, d, sitzung := aufbau(t)
	idee := ideeAnlegen(t, d, "Ein Mitfahrbrett für Fahrten nach Hildesheim.", model.IdeeNeu)
	pfad := "/admin/ideen/" + strconv.FormatInt(idee.ID, 10)

	w := hole(t, h, pfad+"/loeschen", sitzung)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "loeschen-bestaetigen") {
		t.Fatalf("Bestätigungsseite fehlt: %d", w.Code)
	}
	w = sende(t, h, pfad+"/loeschen", nil, sitzung)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("Löschen: %d", w.Code)
	}
	if ideen, _ := d.ListIdeen(""); len(ideen) != 0 {
		t.Fatalf("Idee nicht gelöscht: %v", ideen)
	}
}

func TestIdeenZaehlerAufDerBereichsuebersicht(t *testing.T) {
	_, h, d, sitzung := aufbau(t)
	ideeAnlegen(t, d, "Ein Mitfahrbrett für Fahrten nach Hildesheim.", model.IdeeNeu)
	ideeAnlegen(t, d, "Ein Vertretungsplan für die Grundschule.", model.IdeeNeu)
	ideeAnlegen(t, d, "Ein Dorfkalender mit allen Terminen.", model.IdeeUmgesetzt)

	w := hole(t, h, "/admin/", sitzung)
	if w.Code != http.StatusOK {
		t.Fatalf("Übersicht: %d", w.Code)
	}
	koerper := w.Body.String()
	if !strings.Contains(koerper, "/admin/ideen/") {
		t.Fatal("Bereich „Ideen“ fehlt auf der Übersicht")
	}
	if !strings.Contains(koerper, `id="ideen-neu"`) {
		t.Fatal("Zähler für neue Ideen fehlt")
	}
	// Genau zwei neue Ideen — der Zähler zählt nur die noch ungelesenen.
	if !strings.Contains(koerper, ">2<") {
		t.Errorf("Zähler zeigt nicht 2:\n%s", koerper)
	}
}

func TestIdeenNurMitAnmeldung(t *testing.T) {
	_, h, d, _ := aufbau(t)
	idee := ideeAnlegen(t, d, "Ein Mitfahrbrett für Fahrten nach Hildesheim.", model.IdeeNeu)
	pfad := "/admin/ideen/" + strconv.FormatInt(idee.ID, 10)

	for _, p := range []string{"/admin/ideen/", pfad, pfad + "/loeschen"} {
		w := hole(t, h, p)
		if w.Code != http.StatusSeeOther {
			t.Errorf("%s ohne Anmeldung: %d, erwartet 303", p, w.Code)
		}
	}
	w := sende(t, h, pfad, url.Values{"status": {"abgelehnt"}})
	if w.Code != http.StatusSeeOther {
		t.Errorf("Speichern ohne Anmeldung: %d", w.Code)
	}
	nach, _ := d.GetIdee(idee.ID)
	if nach.Status != model.IdeeNeu {
		t.Fatalf("ohne Anmeldung geändert: %+v", nach)
	}
}
