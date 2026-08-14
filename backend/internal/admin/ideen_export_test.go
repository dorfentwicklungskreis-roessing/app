package admin

import (
	"encoding/csv"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Die Wünsche sollen sich auch außerhalb der Verwaltung durchgehen lassen —
// als Tabelle, die jedes Tabellenprogramm öffnet.

func TestIdeenExportAlsCSV(t *testing.T) {
	_, h, d, sitzung := aufbau(t)
	neu := ideeAnlegen(t, d, "Ein Mitfahrbrett für Fahrten nach Hildesheim.", model.IdeeNeu)
	ideeAnlegen(t, d, "Ein Vertretungsplan für die Grundschule.", model.IdeeUmgesetzt)

	w := hole(t, h, "/admin/ideen/export.csv", sitzung)
	if w.Code != http.StatusOK {
		t.Fatalf("Export: %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Fatalf("Content-Type = %q, erwartet text/csv", ct)
	}
	if cd := w.Header().Get("Content-Disposition"); !strings.Contains(cd, "attachment") ||
		!strings.Contains(cd, "ideen") {
		t.Fatalf("Content-Disposition = %q", cd)
	}

	zeilen := leseCSV(t, w.Body.String())
	if len(zeilen) != 3 {
		t.Fatalf("%d Zeilen, erwartet Kopf + 2 Ideen: %v", len(zeilen), zeilen)
	}
	kopf := strings.Join(zeilen[0], ";")
	for _, spalte := range []string{"ID", "Eingegangen", "Name", "E-Mail", "Wunsch", "Weg", "Stand", "Notiz"} {
		if !strings.Contains(kopf, spalte) {
			t.Errorf("Spalte %q fehlt im Kopf: %q", spalte, kopf)
		}
	}
	alles := w.Body.String()
	for _, muss := range []string{"Mitfahrbrett", "Vertretungsplan", "Erna Musterfrau", "erna@example.org"} {
		if !strings.Contains(alles, muss) {
			t.Errorf("Export enthält %q nicht", muss)
		}
	}
	if !strings.Contains(alles, strconv.FormatInt(neu.ID, 10)) {
		t.Error("Export enthält die IDs nicht")
	}

	// Der Statusfilter der Liste gilt auch für den Export.
	w = hole(t, h, "/admin/ideen/export.csv?status=neu", sitzung)
	if w.Code != http.StatusOK {
		t.Fatalf("gefilterter Export: %d", w.Code)
	}
	if strings.Contains(w.Body.String(), "Vertretungsplan") {
		t.Error("gefilterter Export enthält fremde Stände")
	}
}

// Tabellenprogramme werten Zellen, die mit =, +, - oder @ beginnen, als
// Formel aus. Ein eingereichter Wunsch darf im Export nichts auslösen.
func TestIdeenExportEntschaerftFormeln(t *testing.T) {
	_, h, d, sitzung := aufbau(t)
	ideeAnlegen(t, d, `=HYPERLINK("http://boese.example";"Klick mich")`, model.IdeeNeu)

	w := hole(t, h, "/admin/ideen/export.csv", sitzung)
	if w.Code != http.StatusOK {
		t.Fatalf("Export: %d", w.Code)
	}
	zeilen := leseCSV(t, w.Body.String())
	if len(zeilen) != 2 {
		t.Fatalf("%d Zeilen: %v", len(zeilen), zeilen)
	}
	wunsch := zeilen[1][4]
	if strings.HasPrefix(wunsch, "=") {
		t.Fatalf("Formel steht ungeschützt in der Zelle: %q", wunsch)
	}
	if !strings.Contains(wunsch, "HYPERLINK") {
		t.Fatalf("Inhalt ist verlorengegangen: %q", wunsch)
	}
}

func TestIdeenExportNurMitAnmeldung(t *testing.T) {
	_, h, d, _ := aufbau(t)
	ideeAnlegen(t, d, "Ein Mitfahrbrett für Fahrten nach Hildesheim.", model.IdeeNeu)

	w := hole(t, h, "/admin/ideen/export.csv")
	if w.Code != http.StatusSeeOther {
		t.Fatalf("Export ohne Anmeldung: %d, erwartet 303", w.Code)
	}
	if strings.Contains(w.Body.String(), "Mitfahrbrett") {
		t.Fatal("Export gibt ohne Anmeldung Daten heraus")
	}
}

// Eingereichter Text darf beim Anzeigen niemals als Markup wirken.
func TestIdeeMitMarkupWirdEscaped(t *testing.T) {
	_, h, d, sitzung := aufbau(t)
	boese := `<script>alert("hallo")</script><img src=x onerror=alert(1)>`
	idee := ideeAnlegen(t, d, "Bitte "+boese+" einbauen.", model.IdeeNeu)

	for _, pfad := range []string{"/admin/ideen/", "/admin/ideen/" + strconv.FormatInt(idee.ID, 10)} {
		w := hole(t, h, pfad, sitzung)
		if w.Code != http.StatusOK {
			t.Fatalf("%s: %d", pfad, w.Code)
		}
		koerper := w.Body.String()
		if strings.Contains(koerper, "<script>") || strings.Contains(koerper, "<img ") {
			t.Fatalf("%s liefert das Markup ungeschützt aus", pfad)
		}
		if !strings.Contains(koerper, "&lt;script&gt;") {
			t.Fatalf("%s zeigt den Text nicht escaped an", pfad)
		}
	}

	// Auch der Name landet nur escaped auf der Seite.
	i := model.Idee{Name: `<b>Erna</b>`, Wunsch: "Ein ganz gewöhnlicher Wunsch.",
		Quelle: model.IdeeQuelleWebsite, Status: model.IdeeNeu, CreatedAt: ideenJetzt}
	if err := d.InsertIdee(&i); err != nil {
		t.Fatal(err)
	}
	w := hole(t, h, "/admin/ideen/", sitzung)
	if strings.Contains(w.Body.String(), "<b>Erna</b>") {
		t.Fatal("Name wird ungeschützt ausgegeben")
	}
}

func TestIdeenListeZeigtSinnvolleLeereZustaende(t *testing.T) {
	_, h, d, sitzung := aufbau(t)

	// Ganz ohne Ideen.
	w := hole(t, h, "/admin/ideen/", sitzung)
	if !strings.Contains(w.Body.String(), "keine-ideen") {
		t.Fatalf("leerer Zustand fehlt: %s", w.Body.String())
	}

	// Mit Ideen, aber leerem Filter: der Hinweis muss zurück zu „Alle“ führen.
	ideeAnlegen(t, d, "Ein Mitfahrbrett für Fahrten nach Hildesheim.", model.IdeeNeu)
	w = hole(t, h, "/admin/ideen/?status=abgelehnt", sitzung)
	koerper := w.Body.String()
	if !strings.Contains(koerper, "keine-ideen") {
		t.Fatal("leerer Zustand beim Filter fehlt")
	}
	if !strings.Contains(koerper, "keine-ideen-zurueck") {
		t.Fatal("aus dem leeren Filter führt kein Weg zurück")
	}
}

// leseCSV zerlegt den Export (Semikolon, wie es deutsche Tabellenprogramme
// erwarten).
func leseCSV(t *testing.T, s string) [][]string {
	t.Helper()
	s = strings.TrimPrefix(s, "\ufeff")
	r := csv.NewReader(strings.NewReader(s))
	r.Comma = ';'
	r.FieldsPerRecord = -1
	zeilen, err := r.ReadAll()
	if err != nil {
		t.Fatalf("CSV nicht lesbar: %v\n%s", err, s)
	}
	return zeilen
}
