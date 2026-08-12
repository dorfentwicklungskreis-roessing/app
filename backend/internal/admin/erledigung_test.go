package admin

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// Erledigungen werden über eine eigene Bestätigungsseite gemeldet (kein
// Popup, kein confirm()) — und lassen sich genauso wieder zurücknehmen.
func TestErledigungBestaetigenUndZuruecknehmen(t *testing.T) {
	_, h, d, sitzung := aufbau(t)
	task := beispielPflege(t, d, time.Date(2026, time.May, 1, 8, 0, 0, 0, time.UTC))
	aufgabePfad := fmt.Sprintf("/admin/dorfpflege/aufgaben/%d", task.ID)

	// Auf der Ortsseite führt „Erledigt melden" auf die Bestätigungsseite.
	ortPfad := fmt.Sprintf("/admin/dorfpflege/orte/%d", task.PlaceID)
	if seite := hole(t, h, ortPfad, sitzung).Body.String(); !strings.Contains(seite, aufgabePfad+"/erledigt\"") {
		t.Errorf("Ortsseite verlinkt die Bestätigung nicht: %s", seite)
	}

	// Die Bestätigungsseite nennt Ort, Aufgabe und die vorgesehene Menge.
	w := hole(t, h, aufgabePfad+"/erledigt", sitzung)
	if w.Code != http.StatusOK {
		t.Fatalf("Bestätigungsseite: %d %s", w.Code, w.Body.String())
	}
	seite := w.Body.String()
	for _, muss := range []string{"erledigt-bestaetigen", "erledigt-abbrechen", "Teststelle", "Gießen", "10"} {
		if !strings.Contains(seite, muss) {
			t.Errorf("Bestätigungsseite ohne %q", muss)
		}
	}

	// Erst das Absenden meldet die Erledigung.
	if cs, _ := d.ListCompletions(task.ID, 10); len(cs) != 0 {
		t.Fatalf("Aufruf der Seite hat schon gemeldet: %v", cs)
	}
	w = sende(t, h, aufgabePfad+"/erledigt", url.Values{"liter": {"12"}, "notiz": {"gegossen"}}, sitzung)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("Melden: %d %s", w.Code, w.Body.String())
	}
	cs, _ := d.ListCompletions(task.ID, 10)
	if len(cs) != 1 || cs[0].Liters == nil || *cs[0].Liters != 12 {
		t.Fatalf("Erledigung nicht gespeichert: %+v", cs)
	}
	erledigung := cs[0]

	// Die Historie bietet die Rücknahme an.
	ruecknahme := fmt.Sprintf("/admin/dorfpflege/erledigungen/%d/zuruecknehmen", erledigung.ID)
	if seite := hole(t, h, ortPfad, sitzung).Body.String(); !strings.Contains(seite, ruecknahme) {
		t.Errorf("Ortsseite bietet keine Rücknahme an: %s", seite)
	}

	// Auch die Rücknahme fragt auf einer eigenen Seite nach.
	w = hole(t, h, ruecknahme, sitzung)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "loeschen-bestaetigen") {
		t.Fatalf("Rücknahme-Bestätigung: %d %s", w.Code, w.Body.String())
	}
	if cs, _ := d.ListCompletions(task.ID, 10); len(cs) != 1 {
		t.Fatal("Aufruf der Bestätigungsseite hat bereits zurückgenommen")
	}

	w = sende(t, h, ruecknahme, nil, sitzung)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("Rücknahme: %d %s", w.Code, w.Body.String())
	}
	if cs, _ := d.ListCompletions(task.ID, 10); len(cs) != 0 {
		t.Fatalf("Erledigung nicht zurückgenommen: %+v", cs)
	}

	// Unbekannte Erledigung → 404.
	for _, pfad := range []string{
		"/admin/dorfpflege/erledigungen/999999/zuruecknehmen",
		ruecknahme, // dieselbe zweimal zurücknehmen
	} {
		if w := hole(t, h, pfad, sitzung); w.Code != http.StatusNotFound {
			t.Errorf("GET %s: %d, erwartet 404", pfad, w.Code)
		}
		if w := sende(t, h, pfad, nil, sitzung); w.Code != http.StatusNotFound {
			t.Errorf("POST %s: %d, erwartet 404", pfad, w.Code)
		}
	}

	// Unbekannte Aufgabe → 404.
	if w := hole(t, h, "/admin/dorfpflege/aufgaben/999999/erledigt", sitzung); w.Code != http.StatusNotFound {
		t.Errorf("unbekannte Aufgabe: %d, erwartet 404", w.Code)
	}
}
