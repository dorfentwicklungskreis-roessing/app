package admin

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/db"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// stillgelegt setzt Aufgabe bzw. Ort auf inaktiv.
func stillgelegt(t *testing.T, d *db.DB, task model.CareTask, aufgabe, ort bool) {
	t.Helper()
	if aufgabe {
		task.Active = false
		if err := d.UpdateTask(&task); err != nil {
			t.Fatal(err)
		}
	}
	if ort {
		p, err := d.GetPlace(task.PlaceID)
		if err != nil {
			t.Fatal(err)
		}
		p.Active = false
		if err := d.UpdatePlace(p); err != nil {
			t.Fatal(err)
		}
	}
}

// Eine stillgelegte Aufgabe bietet in der Verwaltung kein „Erledigt melden"
// mehr an — und sagt auch, warum.
func TestOrtsseiteOhneMeldenBeiInaktiverAufgabe(t *testing.T) {
	_, h, d, sitzung := aufbau(t)
	task := beispielPflege(t, d, time.Date(2026, time.May, 1, 8, 0, 0, 0, time.UTC))
	ortPfad := fmt.Sprintf("/admin/mithelfen/orte/%d", task.PlaceID)
	melden := fmt.Sprintf("/admin/mithelfen/aufgaben/%d/erledigt", task.ID)

	// Solange alles aktiv ist, gibt es den Knopf.
	if seite := hole(t, h, ortPfad, sitzung).Body.String(); !strings.Contains(seite, melden) {
		t.Fatal("aktive Aufgabe bietet kein Melden an")
	}

	stillgelegt(t, d, task, true, false)
	seite := hole(t, h, ortPfad, sitzung).Body.String()
	if strings.Contains(seite, melden) {
		t.Error("stillgelegte Aufgabe bietet weiterhin „Erledigt melden“ an")
	}
	if !strings.Contains(seite, "deaktiviert") {
		t.Error("die Seite erklärt die Stilllegung nicht")
	}
	// Bearbeiten und Löschen bleiben erreichbar — sonst käme man nicht mehr ran.
	if !strings.Contains(seite, fmt.Sprintf("/admin/mithelfen/aufgaben/%d\"", task.ID)) {
		t.Error("stillgelegte Aufgabe lässt sich nicht mehr bearbeiten")
	}
}

// Auch ein stillgelegter Ort nimmt nichts mehr an.
func TestOrtsseiteOhneMeldenBeiInaktivemOrt(t *testing.T) {
	_, h, d, sitzung := aufbau(t)
	task := beispielPflege(t, d, time.Date(2026, time.May, 1, 8, 0, 0, 0, time.UTC))
	stillgelegt(t, d, task, false, true)

	seite := hole(t, h, fmt.Sprintf("/admin/mithelfen/orte/%d", task.PlaceID), sitzung).Body.String()
	if strings.Contains(seite, fmt.Sprintf("/admin/mithelfen/aufgaben/%d/erledigt", task.ID)) {
		t.Error("stillgelegter Ort bietet weiterhin „Erledigt melden“ an")
	}
	if !strings.Contains(seite, "deaktiviert") {
		t.Error("die Seite erklärt die Stilllegung nicht")
	}
}

// Wer die Adresse direkt aufruft, bekommt keine Meldemöglichkeit untergeschoben.
func TestMeldeseiteBeiStilllegung(t *testing.T) {
	_, h, d, sitzung := aufbau(t)
	task := beispielPflege(t, d, time.Date(2026, time.May, 1, 8, 0, 0, 0, time.UTC))
	stillgelegt(t, d, task, true, false)
	pfad := fmt.Sprintf("/admin/mithelfen/aufgaben/%d/erledigt", task.ID)

	w := hole(t, h, pfad, sitzung)
	seite := w.Body.String()
	if w.Code != http.StatusOK {
		t.Fatalf("Status %d", w.Code)
	}
	if !strings.Contains(seite, "deaktiviert") {
		t.Errorf("Meldeseite weist nicht auf die Stilllegung hin: %s", seite)
	}

	// Abschicken ohne bewussten Nachtrag: abgelehnt, nichts gespeichert.
	w = sende(t, h, pfad, url.Values{"liter": {"10"}}, sitzung)
	if w.Code != http.StatusConflict {
		t.Fatalf("Status %d, erwartet 409", w.Code)
	}
	if cs, _ := d.ListCompletions(task.ID, 10); len(cs) != 0 {
		t.Fatalf("trotz Stilllegung gespeichert: %+v", cs)
	}

	// Mit dem Haken „übergehen" trägt der Admin bewusst nach.
	w = sende(t, h, pfad, url.Values{"liter": {"10"}, "uebergehen": {"1"}}, sitzung)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("Nachtrag: Status %d — %s", w.Code, w.Body.String())
	}
	if cs, _ := d.ListCompletions(task.ID, 10); len(cs) != 1 {
		t.Fatalf("Nachtrag nicht gespeichert: %+v", cs)
	}
}
