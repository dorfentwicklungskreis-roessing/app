package admin

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Die Jahreszeit einer Aufgabe in der Web-Verwaltung (#78) — als zwei
// Auswahlfelder, ohne JavaScript, wie der Rest des Formulars.

func TestAufgabeFormularBietetJahreszeit(t *testing.T) {
	a, h, _, sitzung := aufbau(t)
	p := ortAnlegenDB(t, a)

	w := hole(t, h, "/admin/mithelfen/orte/"+strconv.FormatInt(p.ID, 10)+"/aufgaben/neu", sitzung)
	if w.Code != http.StatusOK {
		t.Fatalf("Formular: HTTP %d", w.Code)
	}
	koerper := w.Body.String()
	for _, muss := range []string{"feld-season-start", "feld-season-end", "ganzjährig", "September"} {
		if !strings.Contains(koerper, muss) {
			t.Errorf("im Formular fehlt %q", muss)
		}
	}
}

func TestJahreszeitUeberFormularSetzenUndAbraeumen(t *testing.T) {
	a, h, d, sitzung := aufbau(t)
	p := ortAnlegenDB(t, a)
	pfad := "/admin/mithelfen/orte/" + strconv.FormatInt(p.ID, 10) + "/aufgaben/neu"

	w := sende(t, h, pfad, url.Values{
		"art":              {"jaeten"},
		"titel":            {"Beet jäten"},
		"wiederholung":     {"regelmaessig"},
		"intervall":        {"56"},
		"rot":              {"70"},
		"seasonStartMonth": {"4"},
		"seasonEndMonth":   {"9"},
		"aktiv":            {"1"},
	}, sitzung)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("Anlegen: HTTP %d — %s", w.Code, w.Body.String())
	}
	aufgaben, err := d.ListTasks()
	if err != nil {
		t.Fatal(err)
	}
	if len(aufgaben) != 1 {
		t.Fatalf("%d Aufgaben, erwartet 1", len(aufgaben))
	}
	if aufgaben[0].SeasonStartMonth != 4 || aufgaben[0].SeasonEndMonth != 9 {
		t.Fatalf("Jahreszeit = %d/%d, erwartet 4/9",
			aufgaben[0].SeasonStartMonth, aufgaben[0].SeasonEndMonth)
	}

	// Die Ortsseite sagt im Klartext, wann die Aufgabe anfällt.
	w = hole(t, h, "/admin/mithelfen/orte/"+strconv.FormatInt(p.ID, 10), sitzung)
	if !strings.Contains(w.Body.String(), "April bis September") {
		t.Error("die Ortsseite nennt die Jahreszeit nicht")
	}

	// Und zurück auf ganzjährig.
	w = sende(t, h, "/admin/mithelfen/aufgaben/"+strconv.FormatInt(aufgaben[0].ID, 10), url.Values{
		"art":              {"jaeten"},
		"wiederholung":     {"regelmaessig"},
		"intervall":        {"56"},
		"rot":              {"70"},
		"seasonStartMonth": {"0"},
		"seasonEndMonth":   {"0"},
		"aktiv":            {"1"},
	}, sitzung)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("Speichern: HTTP %d — %s", w.Code, w.Body.String())
	}
	geaendert, err := d.GetTask(aufgaben[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, saisonal := geaendert.SeasonOf(); saisonal {
		t.Fatalf("Jahreszeit nicht abgeräumt: %d/%d",
			geaendert.SeasonStartMonth, geaendert.SeasonEndMonth)
	}
}

// Eine halbe Angabe kommt als Fehlermeldung zurück, nicht als stille
// Auslegung — und das Formular steht danach noch da.
func TestHalbeJahreszeitWirdAbgewiesen(t *testing.T) {
	a, h, _, sitzung := aufbau(t)
	p := ortAnlegenDB(t, a)

	w := sende(t, h, "/admin/mithelfen/orte/"+strconv.FormatInt(p.ID, 10)+"/aufgaben/neu", url.Values{
		"art":              {"jaeten"},
		"wiederholung":     {"regelmaessig"},
		"intervall":        {"56"},
		"rot":              {"70"},
		"seasonStartMonth": {"4"},
		"seasonEndMonth":   {"0"},
		"aktiv":            {"1"},
	}, sitzung)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("HTTP %d, erwartet 400", w.Code)
	}
	if !strings.Contains(w.Body.String(), "formularfehler") {
		t.Error("keine Fehlermeldung im Formular")
	}
}

// Die Anzeige einer ruhenden Aufgabe: „außer Dienst", grau statt grün.
func TestStatusAnzeigeRuhend(t *testing.T) {
	if got := statusText(model.StatusDormant); got != "außer Dienst" {
		t.Errorf("statusText = %q", got)
	}
	if got := statusBadge(model.StatusDormant); got != "badge-ghost" {
		t.Errorf("statusBadge = %q", got)
	}
}
