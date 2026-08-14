package admin

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// Der Spielschutz muss auch in der Verwaltung sichtbar sein: eine frisch
// erledigte Aufgabe wird nicht noch einmal gemeldet. Admins dürfen die Sperre
// bewusst übergehen — über eine Angabe auf der Bestätigungsseite, nicht per
// Popup und nicht aus Versehen.
func TestSpielschutzInDerVerwaltung(t *testing.T) {
	_, h, d, sitzung := aufbau(t) // „jetzt" ist der 1. Juni 2026, 12:00 UTC
	task := beispielPflege(t, d, time.Date(2026, time.May, 1, 8, 0, 0, 0, time.UTC))
	ortPfad := fmt.Sprintf("/admin/mithelfen/orte/%d", task.PlaceID)
	erledigtPfad := fmt.Sprintf("/admin/mithelfen/aufgaben/%d/erledigt", task.ID)

	// Erna hat vor zwei Stunden gegossen; die Sperre läuft (7 Tage Intervall
	// → 3,5 Tage). Wieder möglich am 5. Juni um 00:00 Ortszeit.
	melde(t, d, task.ID, "erna", "Erna", time.Date(2026, time.June, 1, 10, 0, 0, 0, time.UTC), 10)
	const wiederAb = "05.06.2026, 00:00"

	// Die Ortsseite sagt, dass gerade nichts zu tun ist.
	seite := hole(t, h, ortPfad, sitzung).Body.String()
	for _, muss := range []string{"Bereits erledigt", wiederAb} {
		if !strings.Contains(seite, muss) {
			t.Errorf("Ortsseite ohne Hinweis %q", muss)
		}
	}

	// Die Bestätigungsseite warnt und bietet das Übergehen an.
	w := hole(t, h, erledigtPfad, sitzung)
	if w.Code != http.StatusOK {
		t.Fatalf("Bestätigungsseite: %d", w.Code)
	}
	seite = w.Body.String()
	for _, muss := range []string{"erledigt-gesperrt", "feld-uebergehen", wiederAb} {
		if !strings.Contains(seite, muss) {
			t.Errorf("Bestätigungsseite ohne %q: %s", muss, seite)
		}
	}

	// Ohne bewusstes Übergehen wird nichts eingetragen.
	w = sende(t, h, erledigtPfad, url.Values{"liter": {"10"}}, sitzung)
	if w.Code != http.StatusConflict {
		t.Errorf("Meldung trotz Sperre: %d, erwartet 409", w.Code)
	}
	if !strings.Contains(w.Body.String(), "erledigt-gesperrt") {
		t.Errorf("abgewiesene Meldung ohne Erklärung: %s", w.Body.String())
	}
	if cs, _ := d.ListCompletions(task.ID, 10); len(cs) != 1 {
		t.Fatalf("Sperre nicht durchgesetzt: %d Meldungen", len(cs))
	}

	// Mit gesetztem Haken darf der Admin nachtragen — der Eintrag wird als
	// erzwungen gekennzeichnet.
	w = sende(t, h, erledigtPfad, url.Values{"liter": {"10"}, "uebergehen": {"1"}}, sitzung)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("Übergehen: %d %s", w.Code, w.Body.String())
	}
	cs, _ := d.ListCompletions(task.ID, 10)
	if len(cs) != 2 {
		t.Fatalf("Nachtrag fehlt: %+v", cs)
	}
	if !cs[0].Forced {
		t.Errorf("Nachtrag nicht als erzwungen vermerkt: %+v", cs[0])
	}

	// Die Historie kennzeichnet ihn ebenfalls.
	if seite := hole(t, h, ortPfad, sitzung).Body.String(); !strings.Contains(seite, "nachgetragen") {
		t.Errorf("Historie kennzeichnet den Nachtrag nicht: %s", seite)
	}
}

// Ohne laufende Sperre bleibt die Bestätigungsseite schlicht: kein Hinweis,
// kein Haken zum Übergehen.
func TestBestaetigungOhneSperre(t *testing.T) {
	_, h, d, sitzung := aufbau(t)
	task := beispielPflege(t, d, time.Date(2026, time.May, 1, 8, 0, 0, 0, time.UTC))

	seite := hole(t, h, fmt.Sprintf("/admin/mithelfen/aufgaben/%d/erledigt", task.ID), sitzung).Body.String()
	for _, darfNicht := range []string{"erledigt-gesperrt", "feld-uebergehen"} {
		if strings.Contains(seite, darfNicht) {
			t.Errorf("unnötiger Hinweis %q auf der Bestätigungsseite", darfNicht)
		}
	}
	if seite := hole(t, h, fmt.Sprintf("/admin/mithelfen/orte/%d", task.PlaceID), sitzung).Body.String(); strings.Contains(seite, "Bereits erledigt") {
		t.Errorf("Ortsseite meldet eine Sperre, obwohl noch nie gemeldet wurde")
	}
}

// Zeiten werden in der Ortszeit des Dorfes angezeigt — der Server läuft in UTC.
func TestAnzeigeInOrtszeit(t *testing.T) {
	alteZone := time.Local
	time.Local = time.UTC
	t.Cleanup(func() { time.Local = alteZone })

	_, h, d, sitzung := aufbau(t)
	task := beispielPflege(t, d, time.Date(2026, time.May, 1, 8, 0, 0, 0, time.UTC))
	melde(t, d, task.ID, "erna", "Erna", time.Date(2026, time.June, 1, 10, 0, 0, 0, time.UTC), 10)

	seite := hole(t, h, fmt.Sprintf("/admin/mithelfen/orte/%d", task.PlaceID), sitzung).Body.String()
	// 10:00 UTC ist im Sommer 12:00 in Rössing.
	if !strings.Contains(seite, "01.06.2026, 12:00") {
		t.Errorf("Historie zeigt keine Ortszeit (erwartet 01.06.2026, 12:00): %s", seite)
	}
}
