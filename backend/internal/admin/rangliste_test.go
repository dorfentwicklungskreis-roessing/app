package admin

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/db"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// beispielPflege legt einen Ort mit Gießaufgabe an und liefert die Aufgabe.
// Die Seite ist der Prüfgegenstand, deshalb kommen die Daten direkt in die
// (echte) Datenbank.
func beispielPflege(t *testing.T, d *db.DB, angelegt time.Time) model.CareTask {
	t.Helper()
	p := model.Place{Name: "Teststelle", Kind: model.PlaceFlowerbox,
		Lat: 52.2110, Lon: 9.8700, Active: true, CreatedAt: angelegt}
	if err := d.InsertPlace(&p); err != nil {
		t.Fatal(err)
	}
	zehn := 10.0
	task := model.CareTask{PlaceID: p.ID, Kind: model.TaskWatering, Liters: &zehn,
		IntervalDays: 7, RedAfterDays: 14, Active: true, CreatedAt: angelegt}
	if err := d.InsertTask(&task); err != nil {
		t.Fatal(err)
	}
	return task
}

func melde(t *testing.T, d *db.DB, taskID int64, sub, name string, wann time.Time, liter float64) model.Completion {
	t.Helper()
	c := model.Completion{TaskID: taskID, UserSub: sub, UserName: name, DoneAt: wann}
	if liter > 0 {
		c.Liters = &liter
	}
	if err := d.InsertCompletion(&c); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestRanglisteSeite(t *testing.T) {
	_, h, d, sitzung := aufbau(t) // „jetzt" ist der 1. Juni 2026, 12 Uhr
	mai := func(tag, stunde int) time.Time {
		return time.Date(2026, time.May, tag, stunde, 0, 0, 0, time.UTC)
	}
	task := beispielPflege(t, d, mai(1, 8))
	melde(t, d, task.ID, "erna", "Erna", mai(4, 9), 10)
	melde(t, d, task.ID, "erna", "Erna", mai(11, 9), 10)
	melde(t, d, task.ID, "erna", "Erna", mai(18, 9), 10)
	melde(t, d, task.ID, "karl", "Karl", mai(25, 9), 5)

	w := hole(t, h, "/admin/dorfpflege/rangliste", sitzung)
	if w.Code != http.StatusOK {
		t.Fatalf("Rangliste: %d %s", w.Code, w.Body.String())
	}
	seite := w.Body.String()
	for _, muss := range []string{"Rangliste", "Erna", "Karl", "rangliste-gesamt"} {
		if !strings.Contains(seite, muss) {
			t.Errorf("Rangliste ohne %q", muss)
		}
	}
	// Erna steht vor Karl.
	if strings.Index(seite, "Erna") > strings.Index(seite, "Karl") {
		t.Error("Reihenfolge stimmt nicht: Karl steht vor Erna")
	}
	// Der Standard-Zeitraum ist die Saison und ist als aktiv markiert.
	if !strings.Contains(seite, `data-zeitraum="saison"`) {
		t.Errorf("aktiver Zeitraum fehlt: %s", seite)
	}
	// Die Gesamtsummen des Dorfes stehen auf der Seite.
	if !strings.Contains(seite, `data-erledigungen="4"`) {
		t.Errorf("Gesamtsumme fehlt: %s", seite)
	}

	// Zeitraum per Query umschalten — echte Seitenwechsel, kein JavaScript.
	w = hole(t, h, "/admin/dorfpflege/rangliste?zeitraum=woche", sitzung)
	if w.Code != http.StatusOK {
		t.Fatalf("Woche: %d", w.Code)
	}
	seite = w.Body.String()
	if !strings.Contains(seite, `data-zeitraum="woche"`) {
		t.Error("Wochen-Zeitraum nicht aktiv")
	}
	if !strings.Contains(seite, "keine-erledigungen") {
		t.Errorf("leere Woche nicht als leer dargestellt: %s", seite)
	}
	if strings.Contains(seite, "Erna") {
		t.Error("Meldungen aus dem Mai tauchen in der laufenden Woche auf")
	}

	// Unbekannter Zeitraum wird abgewiesen.
	if w := hole(t, h, "/admin/dorfpflege/rangliste?zeitraum=jahrzehnt", sitzung); w.Code != http.StatusBadRequest {
		t.Errorf("unbekannter Zeitraum: %d, erwartet 400", w.Code)
	}

	// Ohne Anmeldung ist die Seite verschlossen.
	if w := hole(t, h, "/admin/dorfpflege/rangliste"); w.Code != http.StatusSeeOther {
		t.Errorf("ohne Session: %d, erwartet Weiterleitung", w.Code)
	}

	// Im Menü ist die Rangliste verlinkt.
	if seite := hole(t, h, "/admin/dorfpflege/", sitzung).Body.String(); !strings.Contains(seite, "/admin/dorfpflege/rangliste") {
		t.Error("Rangliste ist im Menü nicht verlinkt")
	}
}
