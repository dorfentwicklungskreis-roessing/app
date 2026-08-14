package admin

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Einmalige Aufgaben in der Web-Verwaltung: dieselbe Sache wie in der App,
// nur als Formular. Ohne JavaScript bedienbar — die Wahl zwischen
// regelmäßig und einmalig ist deshalb ein Radioknopf, und beide Feldgruppen
// stehen gleichzeitig auf der Seite.

// ortAnlegenDB legt einen Ort direkt in der Datenbank an.
func ortAnlegenDB(t *testing.T, a *App) model.Place {
	t.Helper()
	p := model.Place{Name: "Bahnhof", Kind: model.PlaceOther, Lat: 52.211, Lon: 9.87,
		Active: true, CreatedAt: a.now()}
	if err := a.db.InsertPlace(&p); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestAufgabeFormularBietetEinmalig(t *testing.T) {
	a, h, _, sitzung := aufbau(t)
	p := ortAnlegenDB(t, a)

	w := hole(t, h, "/admin/mithelfen/orte/"+strconv.FormatInt(p.ID, 10)+"/aufgaben/neu", sitzung)
	if w.Code != http.StatusOK {
		t.Fatalf("Formular: HTTP %d", w.Code)
	}
	koerper := w.Body.String()
	for _, muss := range []string{"feld-wiederholung", "feld-termin", "feld-entfernen"} {
		if !strings.Contains(koerper, muss) {
			t.Errorf("Formularfeld %q fehlt", muss)
		}
	}
}

func TestEinmaligeAufgabeUeberFormularAnlegen(t *testing.T) {
	a, h, d, sitzung := aufbau(t)
	p := ortAnlegenDB(t, a)

	w := sende(t, h, "/admin/mithelfen/orte/"+strconv.FormatInt(p.ID, 10)+"/aufgaben/neu", url.Values{
		"art":          {"sonstiges"},
		"titel":        {"Zum Bahnhof fahren"},
		"wiederholung": {"einmalig"},
		"termin":       {"2026-06-20"},
		"entfernen":    {"1"},
		"aktiv":        {"1"},
		"intervall":    {"7"},
		"rot":          {"14"},
	}, sitzung)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("Anlegen: HTTP %d — %s", w.Code, w.Body.String())
	}

	tasks, err := d.ListTasks()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 {
		t.Fatalf("%d Aufgaben, erwartet 1", len(tasks))
	}
	got := tasks[0]
	if !got.OneOff {
		t.Fatal("Aufgabe ist nicht einmalig")
	}
	if !got.RemoveWhenDone {
		t.Error("„nach dem Erledigen entfernen\" wurde nicht übernommen")
	}
	if got.DueDate == nil {
		t.Fatal("kein Termin gespeichert")
	}
	if ort := got.DueDate.In(model.Location()); ort.Day() != 20 || ort.Month() != time.June {
		t.Errorf("Termin = %v, erwartet den 20.06.2026", ort)
	}
	// Die Intervall-Felder aus dem Formular dürfen nicht durchschlagen.
	if got.IntervalDays != 0 || got.RedAfterDays != 0 {
		t.Errorf("Intervalle einer einmaligen Aufgabe: %+v", got)
	}
}

func TestEinmaligOhneTerminWirdAbgewiesen(t *testing.T) {
	a, h, d, sitzung := aufbau(t)
	p := ortAnlegenDB(t, a)

	w := sende(t, h, "/admin/mithelfen/orte/"+strconv.FormatInt(p.ID, 10)+"/aufgaben/neu", url.Values{
		"art": {"sonstiges"}, "wiederholung": {"einmalig"}, "termin": {""},
		"intervall": {"7"}, "rot": {"14"}, "aktiv": {"1"},
	}, sitzung)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("HTTP %d, erwartet 400", w.Code)
	}
	if !strings.Contains(w.Body.String(), "formularfehler") {
		t.Error("keine Fehlermeldung im Formular")
	}
	tasks, _ := d.ListTasks()
	if len(tasks) != 0 {
		t.Fatalf("Aufgabe trotzdem angelegt: %+v", tasks)
	}
}

// Eine erledigte einmalige Aufgabe mit Schalter verschwindet auch aus der
// Verwaltung — die Erledigung bleibt in der Rangliste.
func TestErledigteEinmaligeVerschwindetAusDerUebersicht(t *testing.T) {
	a, h, d, sitzung := aufbau(t)
	p := ortAnlegenDB(t, a)
	termin := a.now().Add(48 * time.Hour)
	task := model.CareTask{
		PlaceID: p.ID, Kind: model.TaskOther, Title: "Zum Bahnhof fahren",
		OneOff: true, DueDate: &termin, RemoveWhenDone: true, Active: true, CreatedAt: a.now(),
	}
	if err := d.InsertTask(&task); err != nil {
		t.Fatal(err)
	}

	w := sende(t, h, "/admin/mithelfen/aufgaben/"+strconv.FormatInt(task.ID, 10)+"/erledigt",
		url.Values{}, sitzung)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("Erledigt melden: HTTP %d — %s", w.Code, w.Body.String())
	}

	tasks, _ := d.ListTasks()
	if len(tasks) != 0 {
		t.Fatalf("Die erledigte einmalige Aufgabe steht noch in der Verwaltung: %+v", tasks)
	}
	cs, _ := d.ListCompletions(task.ID, 10)
	if len(cs) != 1 {
		t.Fatalf("Die Erledigung ist verschwunden: %+v", cs)
	}
}
