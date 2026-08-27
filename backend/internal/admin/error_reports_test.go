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

// Bereich „Fehlerberichte“ der Verwaltung: lesen, einordnen und aufräumen —
// server-gerendert, ohne Modals, mit echter Navigation. Die Berichte müssen
// gefunden werden, sonst sammeln wir Daten, die niemand sieht.

var berichtJetzt = time.Date(2026, 8, 27, 9, 30, 0, 0, time.UTC)

func berichtAnlegen(t *testing.T, d *db.DB, meldung string,
	art model.ErrorReportKind, status model.ErrorReportStatus,
) model.ErrorReport {
	t.Helper()
	e := model.ErrorReport{
		Kind: art, Message: meldung, Detail: "HTTP 500 · GET /api/v1/places",
		Area: "Mithelfen", Platform: "android", AppVersion: "0.1.10 (1000110)",
		OSVersion: "Android 15", DeviceModel: "Pixel 6",
		OccurredAt: berichtJetzt, CreatedAt: berichtJetzt,
		UserSub: "erna-sub", UserName: "Erna Musterfrau", Status: status,
	}
	if err := d.InsertErrorReport(&e); err != nil {
		t.Fatalf("Bericht anlegen: %v", err)
	}
	return e
}

func TestFehlerberichteListeUndDetail(t *testing.T) {
	_, h, d, sitzung := aufbau(t)
	absturz := berichtAnlegen(t, d, "Die App hat sich beim letzten Mal beendet.",
		model.ErrorReportCrash, model.ErrorReportNew)
	berichtAnlegen(t, d, "Der Server antwortet gerade nicht (500).",
		model.ErrorReportServer, model.ErrorReportFixed)

	w := hole(t, h, "/admin/fehlerberichte/", sitzung)
	if w.Code != http.StatusOK {
		t.Fatalf("Liste: %d", w.Code)
	}
	koerper := w.Body.String()
	for _, muss := range []string{"beendet", "antwortet gerade nicht", "Erna Musterfrau",
		"27.08.2026", "Absturz", "behoben", "Pixel 6"} {
		if !strings.Contains(koerper, muss) {
			t.Errorf("Liste enthält %q nicht", muss)
		}
	}

	// Filter nach Stand: echte Seitennavigation.
	w = hole(t, h, "/admin/fehlerberichte/?status=new", sitzung)
	if strings.Contains(w.Body.String(), "antwortet gerade nicht") {
		t.Error("Filter status=new zeigt auch behobene Berichte")
	}

	// Filter nach Art — „nur die Abstürze“ ist die häufigste Frage.
	w = hole(t, h, "/admin/fehlerberichte/?art=crash", sitzung)
	if strings.Contains(w.Body.String(), "antwortet gerade nicht") {
		t.Error("Filter art=crash zeigt auch Server-Fehler")
	}
	if !strings.Contains(w.Body.String(), "beendet") {
		t.Error("Filter art=crash zeigt den Absturz nicht")
	}

	// Unsinnige Filterwerte führen zurück auf die volle Liste, nicht auf 500.
	w = hole(t, h, "/admin/fehlerberichte/?status=quatsch", sitzung)
	if w.Code != http.StatusSeeOther {
		t.Errorf("unbekannter Stand: %d, erwartet 303", w.Code)
	}

	// Detailseite mit allem, was gemeldet wurde.
	pfad := "/admin/fehlerberichte/" + strconv.FormatInt(absturz.ID, 10)
	w = hole(t, h, pfad, sitzung)
	if w.Code != http.StatusOK {
		t.Fatalf("Detailseite: %d", w.Code)
	}
	for _, muss := range []string{"beendet", "GET /api/v1/places", "Mithelfen",
		"Pixel 6", `name="notiz"`, `name="status"`} {
		if !strings.Contains(w.Body.String(), muss) {
			t.Errorf("Detailseite enthält %q nicht", muss)
		}
	}
}

func TestFehlerberichtStandUndNotizSpeichern(t *testing.T) {
	_, h, d, sitzung := aufbau(t)
	bericht := berichtAnlegen(t, d, "Die App hat sich beendet.",
		model.ErrorReportCrash, model.ErrorReportNew)
	pfad := "/admin/fehlerberichte/" + strconv.FormatInt(bericht.ID, 10)

	w := sende(t, h, pfad, url.Values{
		"status": {"fixed"}, "notiz": {"Lag am fehlenden Kartenstil."},
	}, sitzung)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("Speichern: %d %s", w.Code, w.Body.String())
	}
	wieder, err := d.GetErrorReport(bericht.ID)
	if err != nil {
		t.Fatal(err)
	}
	if wieder.Status != model.ErrorReportFixed || wieder.Note != "Lag am fehlenden Kartenstil." {
		t.Fatalf("nicht gespeichert: %+v", wieder)
	}
	// Am gemeldeten Sachverhalt ändert das Einordnen nichts.
	if wieder.Message != bericht.Message {
		t.Fatalf("Meldung verändert: %q", wieder.Message)
	}

	// Ein unsinniger Stand wird abgewiesen, ohne die getippte Notiz zu fressen.
	w = sende(t, h, pfad, url.Values{"status": {"quatsch"}, "notiz": {"Bleibt stehen."}}, sitzung)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("unsinniger Stand: %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Bleibt stehen.") {
		t.Error("die getippte Notiz ist beim Abweisen verloren gegangen")
	}
}

func TestFehlerberichtLoeschenFragtNach(t *testing.T) {
	_, h, d, sitzung := aufbau(t)
	bericht := berichtAnlegen(t, d, "Die App hat sich beendet.",
		model.ErrorReportCrash, model.ErrorReportNew)
	pfad := "/admin/fehlerberichte/" + strconv.FormatInt(bericht.ID, 10)

	w := hole(t, h, pfad+"/loeschen", sitzung)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "dauerhaft gelöscht") {
		t.Fatalf("Bestätigungsseite: %d", w.Code)
	}
	if _, err := d.GetErrorReport(bericht.ID); err != nil {
		t.Fatal("das bloße Aufrufen der Frage hat schon gelöscht")
	}

	w = sende(t, h, pfad+"/loeschen", url.Values{}, sitzung)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("Löschen: %d", w.Code)
	}
	if _, err := d.GetErrorReport(bericht.ID); err == nil {
		t.Fatal("Bericht ist noch da")
	}
}

func TestFehlerberichteNurFuerAngemeldete(t *testing.T) {
	_, h, d, _ := aufbau(t)
	bericht := berichtAnlegen(t, d, "Die App hat sich beendet.",
		model.ErrorReportCrash, model.ErrorReportNew)

	for _, pfad := range []string{
		"/admin/fehlerberichte/",
		"/admin/fehlerberichte/" + strconv.FormatInt(bericht.ID, 10),
	} {
		w := hole(t, h, pfad)
		if w.Code != http.StatusSeeOther {
			t.Errorf("%s ohne Anmeldung: %d, erwartet 303", pfad, w.Code)
		}
	}
}

func TestFehlerberichtMitMarkupWirdEscaped(t *testing.T) {
	_, h, d, sitzung := aufbau(t)
	// Der Inhalt kommt vom Gerät und ist damit Fremdtext — er darf nie als
	// Markup wirksam werden.
	bericht := berichtAnlegen(t, d, `<script>alert("hallo")</script>`,
		model.ErrorReportUnexpected, model.ErrorReportNew)

	w := hole(t, h, "/admin/fehlerberichte/"+strconv.FormatInt(bericht.ID, 10), sitzung)
	if strings.Contains(w.Body.String(), "<script>alert") {
		t.Fatal("eingereichtes Markup steht ungeschützt in der Seite")
	}
	if !strings.Contains(w.Body.String(), "&lt;script&gt;") {
		t.Fatal("der gemeldete Text fehlt in der Seite")
	}
}

func TestNeueFehlerberichteAufDerUebersicht(t *testing.T) {
	_, h, d, sitzung := aufbau(t)
	berichtAnlegen(t, d, "Die App hat sich beendet.", model.ErrorReportCrash, model.ErrorReportNew)
	berichtAnlegen(t, d, "Noch ein Absturz.", model.ErrorReportCrash, model.ErrorReportNew)
	berichtAnlegen(t, d, "Alter Fall.", model.ErrorReportServer, model.ErrorReportFixed)

	w := hole(t, h, "/admin/", sitzung)
	if w.Code != http.StatusOK {
		t.Fatalf("Übersicht: %d", w.Code)
	}
	koerper := w.Body.String()
	if !strings.Contains(koerper, `id="fehlerberichte-neu">2<`) {
		t.Fatalf("Zähler der neuen Berichte fehlt oder zählt falsch")
	}
	if !strings.Contains(koerper, "/admin/fehlerberichte/") {
		t.Fatal("die Übersicht führt nicht zu den Fehlerberichten")
	}
}
